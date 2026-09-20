package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
)

// ErrProvisionFailed is `internal: identity provider unavailable` at first
// sign-in: no user row was created.
var ErrProvisionFailed = errors.New("identity provider unavailable")

// Identity is what the Management API tells us about a subject.
type Identity struct {
	Email       string
	GithubLogin string
	Username    string
}

// IdentitySource looks a subject up; the real one is Logto's Management
// API with client credentials.
type IdentitySource interface {
	Lookup(ctx context.Context, sub string) (Identity, error)
}

// LogtoManagement is the Management API client.
type LogtoManagement struct {
	issuer       string
	clientID     string
	clientSecret string
	http         *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewLogtoManagement builds a client with M2M credentials.
func NewLogtoManagement(issuer, clientID, clientSecret string, client *http.Client) *LogtoManagement {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &LogtoManagement{issuer: strings.TrimRight(issuer, "/"), clientID: clientID, clientSecret: clientSecret, http: client}
}

func (m *LogtoManagement) accessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	if m.token != "" && time.Now().Before(m.expires.Add(-time.Minute)) {
		t := m.token
		m.mu.Unlock()
		return t, nil
	}
	m.mu.Unlock()
	form := url.Values{"grant_type": {"client_credentials"}, "resource": {"https://default.logto.app/api"}, "scope": {"all"}, "client_id": {m.clientID}, "client_secret": {m.clientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.issuer+"/oidc/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }() // drained below
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("logto token: status %d", resp.StatusCode)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return "", errors.New("logto token: malformed response")
	}
	m.mu.Lock()
	m.token, m.expires = tr.AccessToken, time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second)
	m.mu.Unlock()
	return tr.AccessToken, nil
}

// Lookup implements IdentitySource.
func (m *LogtoManagement) Lookup(ctx context.Context, sub string) (Identity, error) {
	tok, err := m.accessToken(ctx)
	if err != nil {
		return Identity{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.issuer+"/api/users/"+url.PathEscape(sub), nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := m.http.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer func() { _ = resp.Body.Close() }() // drained below
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Identity{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Identity{}, fmt.Errorf("logto users: status %d", resp.StatusCode)
	}
	var u struct {
		PrimaryEmail string `json:"primaryEmail"`
		Username     string `json:"username"`
		Identities   map[string]struct {
			Details map[string]any `json:"details"`
		} `json:"identities"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return Identity{}, fmt.Errorf("logto users: %w", err)
	}
	id := Identity{Email: u.PrimaryEmail, Username: u.Username}
	if gh, ok := u.Identities["github"]; ok {
		// Logto's GitHub connector stores {id, name, avatar, email, rawData}
		// with rawData = {userInfo: <GitHub's user object>, userEmails};
		// the login lives at rawData.userInfo.login (seen on the owner's
		// account at the M2 gate, DECISIONS I-105). The two flatter shapes
		// are kept for older connector versions.
		if login, ok := gh.Details["login"].(string); ok {
			id.GithubLogin = login
		} else if raw, ok := gh.Details["rawData"].(map[string]any); ok {
			if login, ok := raw["login"].(string); ok {
				id.GithubLogin = login
			} else if info, ok := raw["userInfo"].(map[string]any); ok {
				id.GithubLogin, _ = info["login"].(string)
			}
		}
		if id.Email == "" {
			id.Email, _ = gh.Details["email"].(string)
		}
	}
	return id, nil
}

var handleClean = regexp.MustCompile(`[^a-z0-9-]+`)
var handleDashes = regexp.MustCompile(`-{2,}`)

// DeriveHandle lowercases the login, replaces anything outside [a-z0-9-]
// with '-', collapses runs, trims, and caps at 32.
func DeriveHandle(login string) string {
	h := strings.ToLower(login)
	h = handleClean.ReplaceAllString(h, "-")
	h = handleDashes.ReplaceAllString(h, "-")
	h = strings.Trim(h, "-")
	if len(h) > 32 {
		h = strings.TrimRight(h[:32], "-")
	}
	if h == "" {
		h = "user"
	}
	return h
}

// Provisioner creates users on first sign-in.
type Provisioner struct {
	pool *db.Pool
	src  IdentitySource
}

// NewProvisioner builds one.
func NewProvisioner(pool *db.Pool, src IdentitySource) *Provisioner {
	return &Provisioner{pool: pool, src: src}
}

// EnsureUser returns the user for a subject, creating it from the
// identity provider when unknown. A provider failure creates nothing.
func (p *Provisioner) EnsureUser(ctx context.Context, sub string) (*store.User, error) {
	u, err := store.GetUserBySub(ctx, p.pool, sub)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	id, err := p.src.Lookup(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProvisionFailed, err)
	}
	login := id.GithubLogin
	if login == "" {
		login = id.Username
	}
	if login == "" {
		login = "user-" + sub
	}
	base := DeriveHandle(login)
	for i := 1; i <= 1000; i++ {
		handle := base
		if i > 1 {
			suffix := fmt.Sprintf("-%d", i)
			if len(base)+len(suffix) > 32 {
				handle = strings.TrimRight(base[:32-len(suffix)], "-") + suffix
			} else {
				handle = base + suffix
			}
		}
		uid := store.NewID()
		var email, gh *string
		if id.Email != "" {
			email = &id.Email
		}
		if id.GithubLogin != "" {
			gh = &id.GithubLogin
		}
		// The trial credit is a credit_ledger row, not a column the row is
		// born with: sum(credit_ledger.cents) is the balance of record and
		// users.trial_credit_cents is the trigger-maintained projection of
		// it (09-billing.md §5.3, DECISIONS I-78). The two go in one
		// transaction so a signup cannot leave a balance that disagrees
		// with its ledger. billing_anchor is the signup instant the billing
		// period is counted from (§5.1).
		err := db.InTx(ctx, p.pool, func(tx db.Tx) error {
			if _, err := tx.Exec(ctx, `insert into users (id, logto_sub, handle, email, github_login, billing_status, trial_credit_cents, project_limit, xl_limit, billing_anchor)
				values ($1, $2, $3, $4, $5, 'trial', 0, $6, $7, now())`,
				uid, sub, handle, email, gh, billing.ProjectLimitTrial, billing.XLLimitTrial); err != nil {
				return err
			}
			_, err := billing.Credit(ctx, tx, uid, billing.TrialCreditCents, billing.ReasonTrial, "signup:"+uid.String())
			return err
		})
		if err == nil {
			return store.GetUser(ctx, p.pool, uid)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "logto_sub") {
				return store.GetUserBySub(ctx, p.pool, sub) // raced with ourselves
			}
			continue // handle collision: -2, -3, ...
		}
		return nil, err
	}
	return nil, errors.New("could not derive a unique handle")
}
