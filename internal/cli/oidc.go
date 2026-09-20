package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// clientID is the CLI's public OAuth client id, registered with Logto as
// a native/CLI application (no secret: PKCE and device code are both
// secret-free flows).
const clientID = "repose-cli"

const loginScope = "openid offline_access profile email"

// discoveryDoc is the subset of RFC 8414 discovery this CLI uses.
type discoveryDoc struct {
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
}

type cachedDiscovery struct {
	Issuer    string       `json:"issuer"`
	Doc       discoveryDoc `json:"doc"`
	FetchedAt time.Time    `json:"fetched_at"`
}

func discoveryCachePath(dir string) string { return filepath.Join(dir, "oidc-discovery.json") }

// discover fetches (or reuses a cached, <24h old) OIDC discovery document.
func discover(ctx context.Context, httpClient *http.Client, configDirPath, issuer string) (*discoveryDoc, error) {
	if b, err := os.ReadFile(discoveryCachePath(configDirPath)); err == nil {
		var c cachedDiscovery
		if json.Unmarshal(b, &c) == nil && c.Issuer == issuer && time.Since(c.FetchedAt) < 24*time.Hour {
			doc := c.Doc
			return &doc, nil
		}
	}
	// Logto's OIDC endpoints live under /oidc relative to the configured
	// issuer (DECISIONS I-71; internal/api/auth hits "/oidc/jwks" and
	// "/oidc/token" off the same issuer value, and 07-cli.md's own device
	// code step names "POST /oidc/device/auth").
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(issuer, "/")+"/oidc/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovering %s: %w", issuer, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovering %s: %s: %s", issuer, resp.Status, string(b))
	}
	var doc discoveryDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	cache := cachedDiscovery{Issuer: issuer, Doc: doc, FetchedAt: time.Now()}
	if cb, err := json.Marshal(cache); err == nil {
		_ = writeFileAtomic(discoveryCachePath(configDirPath), cb, 0o600)
	}
	return &doc, nil
}

// pkcePair is a PKCE code_verifier/code_challenge pair (RFC 7636, S256).
type pkcePair struct{ verifier, challenge string }

func newPKCE() (pkcePair, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return pkcePair{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	return pkcePair{verifier: verifier, challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func postForm(ctx context.Context, httpClient *http.Client, endpoint string, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("%s: %s", tr.Error, tr.ErrorDesc)
	}
	return &tr, nil
}

// loginPKCE runs the authorization-code-with-PKCE loopback flow
// (07-cli.md §5.2 step 2). It blocks until the callback arrives or ctx is
// done.
func loginPKCE(ctx context.Context, httpClient *http.Client, doc *discoveryDoc, open func(string) error) (*tokenResponse, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer func() { _ = listener.Close() }()
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)

	pkce, err := newPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}

	authURL, err := url.Parse(doc.AuthorizationEndpoint)
	if err != nil {
		return nil, err
	}
	q := authURL.Query()
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("scope", loginScope)
	q.Set("resource", apiResource)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", pkce.challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	authURL.RawQuery = q.Encode()

	type result struct {
		code string
		err  error
	}
	resultCh := make(chan result, 1)
	var once sync.Once
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("error") != "" {
			once.Do(func() { resultCh <- result{err: fmt.Errorf("%s: %s", q.Get("error"), q.Get("error_description"))} })
			_, _ = fmt.Fprintln(w, "Login failed; you can close this tab.")
			return
		}
		if q.Get("state") != state {
			once.Do(func() { resultCh <- result{err: errors.New("state mismatch")} })
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		once.Do(func() { resultCh <- result{code: q.Get("code")} })
		_, _ = fmt.Fprintln(w, "Logged in. You can close this tab and return to the terminal.")
	})}
	go func() { _ = srv.Serve(listener) }()
	defer func() { _ = srv.Close() }()

	if err := open(authURL.String()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Open this URL to log in:\n%s\n", authURL.String())
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {clientID},
			"code":          {res.code},
			"redirect_uri":  {redirectURI},
			"code_verifier": {pkce.verifier},
		}
		return postForm(ctx, httpClient, doc.TokenEndpoint, form)
	case <-time.After(5 * time.Minute):
		return nil, errors.New("timed out waiting for the browser login (5 minutes)")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type deviceAuthResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

// loginDeviceCode runs RFC 8628 device authorization (07-cli.md §5.2 step
// 3). print is called once with the user-facing instructions.
func loginDeviceCode(ctx context.Context, httpClient *http.Client, doc *discoveryDoc, print func(string)) (*tokenResponse, error) {
	form := url.Values{"client_id": {clientID}, "scope": {loginScope}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, doc.DeviceAuthorizationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var da deviceAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&da); err != nil {
		return nil, err
	}
	print(fmt.Sprintf("Open %s and enter code %s\nWaiting...", da.VerificationURI, da.UserCode))

	interval := time.Duration(da.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(da.ExpiresIn) * time.Second)
	for {
		if time.Now().After(deadline) {
			return nil, errors.New("device code expired")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		tokenForm := url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"device_code": {da.DeviceCode},
			"client_id":   {clientID},
		}
		tr, err := postForm(ctx, httpClient, doc.TokenEndpoint, tokenForm)
		if err == nil {
			return tr, nil
		}
		if strings.Contains(err.Error(), "authorization_pending") {
			continue
		}
		if strings.Contains(err.Error(), "slow_down") {
			interval += 5 * time.Second
			continue
		}
		return nil, err
	}
}

func refreshToken(ctx context.Context, httpClient *http.Client, doc *discoveryDoc, refresh string) (*tokenResponse, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refresh},
	}
	return postForm(ctx, httpClient, doc.TokenEndpoint, form)
}

// oidcTokenSource is the real TokenSource backed by the on-disk
// credentials and Logto refresh.
type oidcTokenSource struct {
	dir        string
	httpClient *http.Client

	mu    sync.Mutex
	doc   *discoveryDoc
	creds Credentials
}

func newOIDCTokenSource(dir string, httpClient *http.Client, creds Credentials) *oidcTokenSource {
	return &oidcTokenSource{dir: dir, httpClient: httpClient, creds: creds}
}

func (s *oidcTokenSource) AccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !forceRefresh && s.creds.AccessToken != "" && time.Until(s.creds.ExpiresAt) > time.Minute {
		return s.creds.AccessToken, nil
	}
	if s.creds.RefreshToken == "" {
		return "", errors.New("not logged in")
	}
	if s.doc == nil {
		doc, err := discover(ctx, s.httpClient, s.dir, s.creds.LogtoIssuer)
		if err != nil {
			return "", err
		}
		s.doc = doc
	}
	tr, err := refreshToken(ctx, s.httpClient, s.doc, s.creds.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refreshing session: %w", err)
	}
	s.creds.AccessToken = tr.AccessToken
	s.creds.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if tr.RefreshToken != "" {
		s.creds.RefreshToken = tr.RefreshToken
	}
	if err := saveCredentials(s.dir, s.creds); err != nil {
		return "", err
	}
	return s.creds.AccessToken, nil
}
