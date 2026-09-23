package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/heracraft/repose/internal/ca/testca"
)

// Options configures New. The zero value is a usable fake.
type Options struct {
	// Users maps bearer tokens to users. Nil accepts any non-empty token
	// as the canned user (CannedUser); non-nil rejects every other token.
	Users map[string]User
	// Billing makes the billing routes answer canned values instead of
	// 503 billing_disabled, and reports the user as active with a card.
	Billing bool
	// RateLimit enforces 60 requests per minute per token with 429.
	RateLimit bool
	// Now replaces the clock; nil means time.Now in UTC.
	Now func() time.Time
	// CA, when set, backs /certs, /internal/ca and /internal/gateway-certs
	// with real certificates from the test CA (docs/interfaces/
	// ssh-gateway.md "Test CA"), so the gateway relay can be tested end
	// to end. Nil keeps the canned placeholder strings.
	CA *testca.CA
	// CreateDelay makes POST /projects answer state "creating" (and read
	// back as "building" while the op runs, as the engine does) with an
	// op_id whose op finishes (and the guest runs) only after the delay,
	// the way the real engine does; a start meanwhile is a conflict. Zero
	// keeps the instant create.
	CreateDelay time.Duration
}

// CannedUser is the user every token maps to when Options.Users is nil.
var CannedUser = User{
	ID:          "00000000-0000-7000-8000-000000000001",
	Handle:      "heracraft",
	Email:       "dev@example.com",
	GitHubLogin: "heracraft",
}

const (
	gatewayHost = "ssh.repose.herakraft.co"
	hostCAPub   = "ssh-ed25519 AAAA...fakeca"
	userCAPub   = "ssh-ed25519 AAAA...fakeuserca"
	baseVersion = "2026.09.15"
	hostID      = "host-01"
	logPattern  = "GET /v1/projects/{id}/ops/{op_id}/log"
	rateWindow  = time.Minute
	rateBurst   = 60
)

// Fake is the running fake api.
type Fake struct {
	Server *httptest.Server

	opts   Options
	mux    *http.ServeMux
	routes []string

	mu           sync.Mutex
	seq          int64
	reqSeq       int64
	ipSeq        int
	serial       uint64
	tokens       map[string]string // token -> user id
	users        map[string]*userRec
	projects     map[string]*project
	ops          map[string]*op
	certs        map[uint64]*cert
	revoked      []revocation
	failOnce     []failRule
	failAlways   map[string]string
	hits         map[string][]time.Time
	hosts        []Host // nil means the one canned host
	gatewayCerts int    // POST /internal/gateway-certs calls, for cache tests
	sessions     []SessionReport
	// billing is BillingOff, BillingCard or BillingNoCard; Options.Billing
	// starts it at BillingCard and SetBilling changes it at run time.
	// It is atomic rather than under mu, because handlers run under mu.
	billing atomic.Int32
}

// Billing modes for SetBilling.
const (
	BillingOff    = 0 // the billing routes answer 503 billing_disabled
	BillingCard   = 1 // active, a card on file
	BillingNoCard = 2 // trial with credit and no card; a checkout adds one
)

// SetBilling switches the billing routes at run time, so one fake serves a
// browser test of every state of the billing page.
func (f *Fake) SetBilling(mode int) { f.billing.Store(int32(mode)) }

func (f *Fake) billingMode() int { return int(f.billing.Load()) }

type failRule struct {
	key  string
	code string
}

// New starts the fake. Close it when done.
func New(opts Options) *Fake {
	f := &Fake{
		opts:       opts,
		mux:        http.NewServeMux(),
		tokens:     map[string]string{},
		users:      map[string]*userRec{},
		projects:   map[string]*project{},
		ops:        map[string]*op{},
		certs:      map[uint64]*cert{},
		failAlways: map[string]string{},
		hits:       map[string][]time.Time{},
	}
	f.addUser(CannedUser)
	for tok, u := range opts.Users {
		f.addUser(u)
		f.tokens[tok] = u.ID
	}
	if opts.Billing {
		f.SetBilling(BillingCard)
	}
	f.register()
	f.Server = httptest.NewServer(f)
	return f
}

// URL is the server's base URL without the /v1 prefix.
func (f *Fake) URL() string { return f.Server.URL }

// Close stops the server.
func (f *Fake) Close() { f.Server.Close() }

// Routes lists the registered "METHOD /v1/path" patterns.
func (f *Fake) Routes() []string {
	out := make([]string, len(f.routes))
	copy(out, f.routes)
	return out
}

func (f *Fake) addUser(u User) {
	if _, ok := f.users[u.ID]; ok {
		return
	}
	f.users[u.ID] = &userRec{User: u, TZ: "UTC", CreatedAt: f.now(), NotifyEmail: true}
}

func (f *Fake) now() time.Time {
	if f.opts.Now != nil {
		return f.opts.Now().UTC()
	}
	return time.Now().UTC().Truncate(time.Second)
}

// nextID returns the next deterministic UUIDv7-looking id. Callers hold mu.
func (f *Fake) nextID() string {
	f.seq++
	return fmt.Sprintf("01900000-0000-7000-8000-%012d", f.seq)
}

// The error switch.

// FailNext makes the next request matching method and path (an exact
// request path such as /v1/projects/<id>, or a pattern such as
// /projects/:id or /v1/projects/{id}) answer the given error code once.
func (f *Fake) FailNext(method, path, code string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failOnce = append(f.failOnce, failRule{key: normalizeKey(method, path), code: code})
}

// Fail makes every request matching pattern ("METHOD path", the path in
// any of the forms FailNext accepts) answer the given error code until
// Unfail.
func (f *Fake) Fail(pattern, code string) {
	method, path, _ := strings.Cut(strings.TrimSpace(pattern), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAlways[normalizeKey(method, path)] = code
}

// Unfail removes a Fail rule.
func (f *Fake) Unfail(pattern string) {
	method, path, _ := strings.Cut(strings.TrimSpace(pattern), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.failAlways, normalizeKey(method, path))
}

// normalizeKey turns any accepted method/path spelling into the
// "METHOD /v1/path" form the mux reports, with :x params as {x}.
func normalizeKey(method, path string) string {
	path = strings.TrimSpace(path)
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + p[1:] + "}"
		}
	}
	path = strings.Join(parts, "/")
	if path != "/v1" && !strings.HasPrefix(path, "/v1/") {
		path = "/v1" + path
	}
	return strings.ToUpper(strings.TrimSpace(method)) + " " + path
}

// forced returns the error code a switch has queued for this request, or
// "". Callers hold mu.
func (f *Fake) forced(r *http.Request, pattern string) string {
	keys := []string{normalizeKey(r.Method, r.URL.Path)}
	if pattern != "" {
		keys = append(keys, pattern)
	}
	for i, rule := range f.failOnce {
		for _, k := range keys {
			if rule.key == k {
				f.failOnce = append(f.failOnce[:i], f.failOnce[i+1:]...)
				return rule.code
			}
		}
	}
	for _, k := range keys {
		if code, ok := f.failAlways[k]; ok {
			return code
		}
	}
	return ""
}

// Error envelope.

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Detail  map[string]any `json:"detail,omitempty"`
}

type errEnvelope struct {
	Error apiError `json:"error"`
}

var statusOf = map[string]int{
	"unauthenticated":  http.StatusUnauthorized,
	"forbidden":        http.StatusForbidden,
	"not_found":        http.StatusNotFound,
	"invalid":          http.StatusBadRequest,
	"conflict":         http.StatusConflict,
	"payment_required": http.StatusPaymentRequired,
	"capacity":         http.StatusServiceUnavailable,
	"rate_limited":     http.StatusTooManyRequests,
	"internal":         http.StatusInternalServerError,
	"billing_disabled": http.StatusServiceUnavailable,
}

func errf(code, format string, args ...any) *apiError {
	return &apiError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func invalid(format string, args ...any) *apiError { return errf("invalid", format, args...) }
func notFound(what string) *apiError               { return errf("not_found", "%s not found", what) }

func (e *apiError) withDetail(detail map[string]any) *apiError {
	e.Detail = detail
	return e
}

func writeError(w http.ResponseWriter, e *apiError) {
	status, ok := statusOf[e.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, errEnvelope{Error: *e})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte(`{"error":{"code":"internal","message":"encoding response"}}`)
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(b); err != nil {
		return // the client went away; there is nobody left to tell
	}
}

// decodeBody decodes a JSON body into v, rejecting unknown fields. An
// empty body is accepted only when optional is set.
func decodeBody(r *http.Request, v any, optional bool) *apiError {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) && optional {
			return nil
		}
		return invalid("body: %v", err)
	}
	return nil
}

// Request pipeline.

type handler func(w http.ResponseWriter, r *http.Request) *apiError

type ctxKey int

const ctxUser ctxKey = 1

func userFrom(r *http.Request) *userRec {
	u, _ := r.Context().Value(ctxUser).(*userRec) // internal routes carry no user; handlers that need one are never internal
	return u
}

func (f *Fake) handle(pattern string, h handler) {
	f.routes = append(f.routes, pattern)
	f.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if e := h(w, r); e != nil {
			writeError(w, e)
		}
	})
}

// ServeHTTP stamps X-Request-Id, applies the error switch, authenticates
// user routes, rate-limits when asked, and then dispatches. Handlers other
// than the SSE log run under mu so they never lock themselves.
//
// CORS headers on every response, and OPTIONS answered without touching the
// mux (which has no registered OPTIONS handlers): the dashboard fetches
// this fake cross-origin in the Playwright suite exactly like it fetches
// the real api cross-origin in production (I-79).
func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	f.mu.Lock()
	f.reqSeq++
	w.Header().Set("X-Request-Id", fmt.Sprintf("req-%06d", f.reqSeq))
	_, pattern := f.mux.Handler(r)
	if pattern == "" {
		f.mu.Unlock()
		writeError(w, notFound("route"))
		return
	}
	if code := f.forced(r, pattern); code != "" {
		f.mu.Unlock()
		writeError(w, errf(code, "forced by the test's error switch"))
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/v1/internal/") && r.URL.Path != "/v1/notify/unsubscribe" {
		u, tok, ok := f.authenticate(r, pattern)
		if !ok {
			f.mu.Unlock()
			writeError(w, errf("unauthenticated", "missing or unknown bearer token"))
			return
		}
		if retry := f.rateLimited(tok); retry > 0 {
			f.mu.Unlock()
			w.Header().Set("Retry-After", fmt.Sprint(retry))
			writeError(w, errf("rate_limited", "60 requests per minute exceeded"))
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), ctxUser, u))
	}
	if pattern == logPattern {
		f.mu.Unlock()
		f.mux.ServeHTTP(w, r)
		return
	}
	defer f.mu.Unlock()
	f.mux.ServeHTTP(w, r)
}

func (f *Fake) authenticate(r *http.Request, pattern string) (*userRec, string, bool) {
	tok := ""
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	if tok == "" && pattern == logPattern {
		tok = r.URL.Query().Get("access_token")
	}
	if tok == "" {
		return nil, "", false
	}
	id, ok := f.tokens[tok]
	if !ok {
		if f.opts.Users != nil {
			return nil, "", false
		}
		id = CannedUser.ID
		f.tokens[tok] = id
	}
	u, ok := f.users[id]
	if !ok {
		return nil, "", false
	}
	return u, tok, true
}

// rateLimited records a hit and returns the Retry-After seconds when the
// token is over budget, else 0.
func (f *Fake) rateLimited(tok string) int {
	if !f.opts.RateLimit {
		return 0
	}
	now := f.now()
	kept := f.hits[tok][:0]
	for _, t := range f.hits[tok] {
		if now.Sub(t) < rateWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rateBurst {
		f.hits[tok] = kept
		wait := rateWindow - now.Sub(kept[0])
		return int(math.Max(1, math.Ceil(wait.Seconds())))
	}
	f.hits[tok] = append(kept, now)
	return 0
}

// register wires every route of docs/interfaces/api.md.
func (f *Fake) register() {
	// Users.
	f.handle("GET /v1/me", f.getMe)
	f.handle("PATCH /v1/me", f.patchMe)
	f.handle("DELETE /v1/me", f.deleteMe)
	f.handle("POST /v1/me/notify-test", f.notifyTest)
	f.handle("GET /v1/notify/unsubscribe", f.notifyUnsubscribe)
	// Projects.
	f.handle("GET /v1/projects", f.listProjects)
	f.handle("POST /v1/projects", f.createProject)
	f.handle("GET /v1/projects/destroyed", f.listDestroyed)
	f.handle("POST /v1/projects/restore", f.restoreByName)
	f.handle("GET /v1/projects/{id}", f.getProject)
	f.handle("PATCH /v1/projects/{id}", f.patchProject)
	f.handle("DELETE /v1/projects/{id}", f.destroyProject)
	f.handle("POST /v1/projects/{id}/start", f.startProject)
	f.handle("POST /v1/projects/{id}/stop", f.stopProject)
	f.handle("GET /v1/projects/{id}/ops/{op_id}", f.getOp)
	f.handle(logPattern, f.opLog)
	f.handle("POST /v1/projects/{id}/resize", f.resizeProject)
	f.handle("GET /v1/projects/{id}/route", f.projectRoute)
	// Config.
	f.handle("GET /v1/projects/{id}/config", f.getConfig)
	f.handle("PUT /v1/projects/{id}/config", f.putConfig)
	f.handle("GET /v1/projects/{id}/config/revisions", f.listRevisions)
	f.handle("POST /v1/projects/{id}/config/revisions/{rev}/apply", f.applyRevision)
	f.handle("GET /v1/catalog", f.getCatalog)
	// Certificates.
	f.handle("POST /v1/certs", f.issueCert)
	f.handle("POST /v1/certs/revoke", f.revokeCerts)
	// Secrets.
	f.handle("GET /v1/projects/{id}/secrets", f.listSecrets)
	f.handle("PUT /v1/projects/{id}/secrets/{name}", f.putSecret)
	f.handle("DELETE /v1/projects/{id}/secrets/{name}", f.deleteSecret)
	// Snapshots.
	f.handle("GET /v1/projects/{id}/snapshots", f.listSnapshots)
	f.handle("POST /v1/projects/{id}/snapshots", f.createSnapshot)
	f.handle("POST /v1/projects/{id}/snapshots/{sid}/restore", f.restoreSnapshot)
	// Events and logs.
	f.handle("GET /v1/projects/{id}/events", f.listEvents)
	f.handle("GET /v1/projects/{id}/logs", f.projectLogs)
	// Usage and billing.
	f.handle("GET /v1/usage", f.getUsage)
	f.handle("POST /v1/billing/portal", f.billingPortal)
	f.handle("POST /v1/billing/setup", f.billingSetup)
	f.handle("GET /v1/billing/invoices", f.billingInvoices)
	f.handle("POST /v1/billing/webhook", f.billingWebhook)
	// Internal (gateway).
	f.handle("GET /v1/internal/route", f.internalRoute)
	f.handle("GET /v1/internal/revoked", f.internalRevoked)
	f.handle("GET /v1/internal/ca", f.internalCA)
	f.handle("POST /v1/internal/sessions", f.internalSessions)
	f.handle("GET /v1/internal/hosts", f.internalHosts)
	f.handle("POST /v1/internal/gateway-certs", f.internalGatewayCerts)
	f.handle("POST /v1/internal/events", f.internalEvents)
}
