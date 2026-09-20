package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/heracraft/repose/internal/api/apidoc"
	"github.com/heracraft/repose/internal/api/apitest"
	"github.com/heracraft/repose/internal/api/auth"
	"github.com/heracraft/repose/internal/api/config"
	httpapi "github.com/heracraft/repose/internal/api/http"
	"github.com/heracraft/repose/internal/api/notify"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/ca/sshca"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
	"github.com/heracraft/repose/internal/fakes/logto"
	"github.com/heracraft/repose/internal/obs"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

const aud = "https://api.repose.herakraft.co"

type env struct {
	h        *apitest.Harness
	logto    *logto.Fake
	srv      *httpapi.Server
	api      *httptest.Server
	internal *httptest.Server
	logs     *bytes.Buffer
	sent     *sync.Map
	unsub    *notify.Unsubscriber
}

func newEnv(t *testing.T) *env {
	return newEnvLimits(t, &httpapi.RateLimits{General: 10000, Certs: 10000, Config: 10000})
}

func newEnvLimits(t *testing.T, limits *httpapi.RateLimits) *env {
	t.Helper()
	h := apitest.New(t, apitest.Options{})
	lf := logto.New(aud)
	t.Cleanup(lf.Close)
	logs := &bytes.Buffer{}
	syncw := &syncWriter{w: logs}
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentAPI, Writer: syncw, Level: slog.LevelDebug})
	parser, _ := config.NewParser()
	sent := &sync.Map{}
	sender := notify.SenderFunc(func(ctx context.Context, m notify.Message) error {
		sent.Store(m.EventID.String()+m.Kind, m)
		return nil
	})
	reg := prometheus.NewRegistry()
	unsub, err := notify.LoadOrCreateUnsubscriber(context.Background(), h.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{h: h, logto: lf, logs: logs, sent: sent, unsub: unsub}
	e.srv = httpapi.New(httpapi.Deps{
		Pool: h.Pool, Verifier: auth.NewVerifier(lf.Issuer(), aud, nil), Users: auth.NewProvisioner(h.Pool, auth.NewLogtoManagement(lf.Issuer(), "m2m", "s", nil)),
		CA: h.CA, Secrets: h.Secrets, Engine: h.Engine, Logs: h.Logs, Events: h.Events, Parser: parser, Metrics: h.Metrics, Registry: reg, Log: log,
		Outbox:  notify.New(h.Pool, map[string]notify.Sender{"email": sender, "ntfy": sender}, h.Metrics, log),
		Unsub:   unsub,
		Gateway: httpapi.Gateway{Host: "ssh.test", Port: 22}, Limits: limits, BillingEnforce: true,
		Migrations: func(ctx context.Context) (int, error) {
			st, err := db.MigrateStatus(ctx, h.Pool)
			return len(st.Pending), err
		},
	})
	e.srv.SetReady(true)
	e.api = httptest.NewServer(e.srv.Handler())
	e.internal = httptest.NewServer(e.srv.InternalHandler())
	t.Cleanup(e.api.Close)
	t.Cleanup(e.internal.Close)
	return e
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

type resp struct {
	status int
	body   map[string]any
	list   []any
	raw    []byte
	hdr    http.Header
}

func (e *env) do(t *testing.T, token, method, path string, body any) resp {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.api.URL+"/v1"+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw, hdr: res.Header}
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &out.body)
	} else if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &out.list)
	}
	return out
}

func (e *env) internalDo(t *testing.T, method, path string, body any) resp {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.internal.URL+"/v1"+path, rd)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw, hdr: res.Header}
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &out.body)
	} else if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &out.list)
	}
	return out
}

func errCode(r resp) string {
	if e, ok := r.body["error"].(map[string]any); ok {
		c, _ := e["code"].(string)
		return c
	}
	return ""
}

func (e *env) waitOp(t *testing.T, r resp) *store.Op {
	t.Helper()
	id, _ := r.body["op_id"].(string)
	if id == "" {
		t.Fatalf("no op_id in %s", r.raw)
	}
	return e.h.WaitOp(uuid.MustParse(id))
}

// signIn creates a Logto identity and returns a token; the user gets a
// card so compute is allowed.
func (e *env) signIn(t *testing.T, sub, login string) string {
	t.Helper()
	e.logto.AddUser(sub, logto.User{Email: login + "@example.com", GithubLogin: login})
	tok := e.logto.Token(sub)
	r := e.do(t, tok, "GET", "/me", nil)
	if r.status != 200 {
		t.Fatalf("first sign-in: %d %s", r.status, r.raw)
	}
	if _, err := e.h.Pool.Exec(e.h.Ctx, "update users set has_card = true where logto_sub = $1", sub); err != nil {
		t.Fatal(err)
	}
	return tok
}

// TestNotifyUnsubscribe covers docs/workstreams/13-notifications.md §5.6
// and §9's "unsubscribe link that works": a valid token flips
// notify_email off with no auth, and a forged or malformed one is refused.
func TestNotifyUnsubscribe(t *testing.T) {
	e := newEnv(t)
	tok := e.signIn(t, "sub-uns", "uns")
	r := e.do(t, tok, "GET", "/me", nil)
	if r.status != 200 {
		t.Fatalf("me: %d %s", r.status, r.raw)
	}
	userID, err := uuid.Parse(r.body["id"].(string))
	if err != nil {
		t.Fatal(err)
	}

	// Invalid tokens never touch the row.
	if resp := e.do(t, "", "GET", "/notify/unsubscribe?token=garbage", nil); resp.status != 400 {
		t.Fatalf("garbage token: %d %s", resp.status, resp.raw)
	}
	// A well-formed token signed for a user id nobody has is not an error
	// (it verifies; there is just no row to update) and must not touch the
	// real user's row.
	other := e.unsub.Sign(uuid.New())
	if resp := e.do(t, "", "GET", "/notify/unsubscribe?token="+other, nil); resp.status != 200 {
		t.Fatalf("unknown-user token: %d %s", resp.status, resp.raw)
	}
	var notifyEmail bool
	if err := e.h.Pool.QueryRow(e.h.Ctx, "select notify_email from users where id = $1", userID).Scan(&notifyEmail); err != nil {
		t.Fatal(err)
	}
	if !notifyEmail {
		t.Fatal("an unrelated token's success must not have touched this user's row")
	}

	// A valid token flips it off, with no Authorization header.
	token := e.unsub.Sign(userID)
	resp := e.do(t, "", "GET", "/notify/unsubscribe?token="+token, nil)
	if resp.status != 200 || !strings.Contains(string(resp.raw), "unsubscribed") {
		t.Fatalf("unsubscribe: %d %s", resp.status, resp.raw)
	}
	if err := e.h.Pool.QueryRow(e.h.Ctx, "select notify_email from users where id = $1", userID).Scan(&notifyEmail); err != nil {
		t.Fatal(err)
	}
	if notifyEmail {
		t.Fatal("notify_email was not cleared")
	}

	// A tampered signature is refused.
	idPart, _, _ := strings.Cut(token, ".")
	_, sigPart, _ := strings.Cut(other, ".")
	if resp := e.do(t, "", "GET", "/notify/unsubscribe?token="+idPart+"."+sigPart, nil); resp.status != 400 {
		t.Fatalf("tampered token: %d %s", resp.status, resp.raw)
	}
}

// TestNotifyTestRoute is 13-notifications.md §9's "POST /me/notify-test
// returns per-channel results": the settings page's test button.
func TestNotifyTestRoute(t *testing.T) {
	e := newEnv(t)
	tok := e.signIn(t, "sub-kim", "kim")
	// Email only, no ntfy: the result carries email but not ntfy.
	r := e.do(t, tok, "POST", "/me/notify-test", nil)
	if r.status != 200 {
		t.Fatalf("notify-test: %d %s", r.status, r.raw)
	}
	if r.body["email"] != "ok" {
		t.Fatalf("email result: %v", r.body)
	}
	if _, ok := r.body["ntfy"]; ok {
		t.Fatalf("ntfy attempted with no url configured: %v", r.body)
	}
	// Setting an ntfy url gets it included too; the fake sender in this
	// harness (newEnvLimits) answers every send with no error.
	if r := e.do(t, tok, "PATCH", "/me", map[string]any{"notify": map[string]any{"ntfy_url": "https://ntfy.example/topic"}}); r.status != 200 {
		t.Fatalf("set ntfy: %d %s", r.status, r.raw)
	}
	r = e.do(t, tok, "POST", "/me/notify-test", nil)
	if r.status != 200 || r.body["email"] != "ok" || r.body["ntfy"] != "ok" {
		t.Fatalf("notify-test with ntfy: %d %v", r.status, r.body)
	}
	// Turning email off drops it from the test, not just real events.
	if r := e.do(t, tok, "PATCH", "/me", map[string]any{"notify": map[string]any{"email": false}}); r.status != 200 {
		t.Fatalf("disable email: %d %s", r.status, r.raw)
	}
	r = e.do(t, tok, "POST", "/me/notify-test", nil)
	if _, ok := r.body["email"]; ok {
		t.Fatalf("email attempted after being disabled: %v", r.body)
	}
}

func TestRouteContract(t *testing.T) {
	e := newEnv(t)
	documented, err := apidoc.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, r := range documented {
		want[apidoc.Pattern(r)] = true
	}
	got := map[string]bool{}
	for _, r := range e.srv.Routes() {
		got[r] = true
	}
	var missing, extra []string
	for r := range want {
		if !got[r] {
			missing = append(missing, r)
		}
	}
	for r := range got {
		if !want[r] {
			extra = append(extra, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("missing from router: %v\nnot in api.md: %v", missing, extra)
	}
	t.Logf("%d routes documented, %d registered, all matched", len(want), len(got))
	for _, r := range e.srv.Routes() {
		t.Logf("  %s", r)
	}
}

func TestSignInAndProjectsLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := e.h.Ctx
	// Unauthenticated and bad tokens.
	if r := e.do(t, "", "GET", "/me", nil); r.status != 401 || errCode(r) != "unauthenticated" || r.hdr.Get("X-Request-Id") == "" {
		t.Fatalf("no token: %d %s", r.status, r.raw)
	}
	if r := e.do(t, "garbage", "GET", "/me", nil); r.status != 401 {
		t.Fatalf("bad token: %d", r.status)
	}
	// First sign-in creates the user with the derived handle and trial defaults.
	e.logto.AddUser("sub-alice", logto.User{Email: "alice@example.com", GithubLogin: "Alice_Dev"})
	tok := e.logto.Token("sub-alice")
	r := e.do(t, tok, "GET", "/me", nil)
	if r.status != 200 || r.body["handle"] != "alice-dev" {
		t.Fatalf("me: %d %s", r.status, r.raw)
	}
	b := r.body["billing"].(map[string]any)
	l := r.body["limits"].(map[string]any)
	if b["status"] != "trial" || b["trial_credit_cents"].(float64) != 1000 || l["projects"].(float64) != 3 || l["xl"].(float64) != 1 {
		t.Fatalf("defaults: %s", r.raw)
	}
	var row map[string]any
	rows, _ := e.h.Pool.Query(ctx, "select handle, trial_credit_cents, project_limit, xl_limit, billing_status from users where logto_sub = 'sub-alice'")
	for rows.Next() {
		var handle, status string
		var credit int64
		var pl, xl int
		_ = rows.Scan(&handle, &credit, &pl, &xl, &status)
		row = map[string]any{"handle": handle, "trial_credit_cents": credit, "project_limit": pl, "xl_limit": xl, "billing_status": status}
	}
	rows.Close()
	t.Logf("user row after first sign-in: %v", row)
	// No card: payment_required with the reason.
	r = e.do(t, tok, "POST", "/projects", map[string]any{"name": "todo-app", "class": "large", "remote_url": "github.com/alice/todo"})
	if r.status != 402 || errCode(r) != "payment_required" {
		t.Fatalf("no card: %d %s", r.status, r.raw)
	}
	if _, err := e.h.Pool.Exec(ctx, "update users set has_card = true where logto_sub = 'sub-alice'"); err != nil {
		t.Fatal(err)
	}
	// Validation.
	if r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "bad name!", "class": "large"}); r.status != 400 || errCode(r) != "invalid" {
		t.Fatalf("bad name: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "x", "class": "huge"}); r.status != 400 {
		t.Fatalf("bad class: %d", r.status)
	}
	if r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "x", "class": "small", "bogus": 1}); r.status != 400 {
		t.Fatalf("unknown field: %d", r.status)
	}
	// Create and watch it come up.
	r = e.do(t, tok, "POST", "/projects", map[string]any{"name": "Todo.App", "class": "large", "remote_url": "github.com/alice/todo"})
	if r.status != 201 || r.body["slug"] != "todo-app" || r.body["state"] != "creating" {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	pid := r.body["id"].(string)
	op := e.waitOp(t, r)
	if op.State != "done" {
		t.Fatalf("create op: %+v", op.Error)
	}
	r = e.do(t, tok, "GET", "/projects/"+pid, nil)
	if r.status != 200 || r.body["state"] != "running" || r.body["guest_ip"] == nil || r.body["host_id"] == nil {
		t.Fatalf("running project: %d %s", r.status, r.raw)
	}
	// Duplicates.
	if r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "todo-app", "class": "small"}); r.status != 409 || errCode(r) != "conflict" {
		t.Fatalf("dup name: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "other", "class": "small", "remote_url": "github.com/alice/todo"}); r.status != 409 {
		t.Fatalf("dup remote: %d %s", r.status, r.raw)
	}
	// Limits: 3 projects, 1 xl.
	r = e.do(t, tok, "POST", "/projects", map[string]any{"name": "big", "class": "xl"})
	if r.status != 201 {
		t.Fatalf("xl: %d %s", r.status, r.raw)
	}
	e.waitOp(t, r)
	if r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "big2", "class": "xl"}); r.status != 400 || r.body["error"].(map[string]any)["detail"].(map[string]any)["xl_limit"].(float64) != 1 {
		t.Fatalf("xl limit: %d %s", r.status, r.raw)
	}
	r = e.do(t, tok, "POST", "/projects", map[string]any{"name": "third", "class": "small"})
	if r.status != 201 {
		t.Fatalf("third: %d %s", r.status, r.raw)
	}
	e.waitOp(t, r)
	r = e.do(t, tok, "POST", "/projects", map[string]any{"name": "fourth", "class": "small"})
	if r.status != 400 || r.body["error"].(map[string]any)["detail"].(map[string]any)["limit"].(float64) != 3 {
		t.Fatalf("project limit: %d %s", r.status, r.raw)
	}
	// past_due is payment_required on start.
	if _, err := e.h.Pool.Exec(ctx, "update users set billing_status = 'past_due' where logto_sub = 'sub-alice'"); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/start", nil); r.status != 402 || errCode(r) != "payment_required" {
		t.Fatalf("past due start: %d %s", r.status, r.raw)
	}
	if _, err := e.h.Pool.Exec(ctx, "update users set billing_status = 'trial' where logto_sub = 'sub-alice'"); err != nil {
		t.Fatal(err)
	}
	// List shows three; cross-user is 404.
	if r := e.do(t, tok, "GET", "/projects", nil); r.status != 200 || len(r.list) != 3 {
		t.Fatalf("list: %d %d", r.status, len(r.list))
	}
	bobTok := e.signIn(t, "sub-bob", "bob")
	if r := e.do(t, bobTok, "GET", "/projects/"+pid, nil); r.status != 404 {
		t.Fatalf("cross-user: %d", r.status)
	}
	if r := e.do(t, bobTok, "POST", "/projects/"+pid+"/stop", nil); r.status != 404 {
		t.Fatalf("cross-user stop: %d", r.status)
	}
	// Secrets: put pushes to the running guest, list never shows values.
	val := base64.StdEncoding.EncodeToString([]byte("postgres://PLANTED-SECRET-VALUE"))
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/secrets/DATABASE_URL", map[string]any{"value": val}); r.status != 200 || r.body["pushed"] != true {
		t.Fatalf("put secret: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/secrets/user_ca.pub", map[string]any{"value": val}); r.status != 400 {
		t.Fatalf("reserved name: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/secrets/lower", map[string]any{"value": val}); r.status != 400 {
		t.Fatalf("bad name: %d", r.status)
	}
	r = e.do(t, tok, "GET", "/projects/"+pid+"/secrets", nil)
	if r.status != 200 || len(r.list) != 1 || strings.Contains(string(r.raw), "PLANTED") || strings.Contains(string(r.raw), val) {
		t.Fatalf("list secrets: %d %s", r.status, r.raw)
	}
	e.h.WaitFor("secret pushed to the guest", func() bool {
		for _, g := range e.h.Fake.Guests() {
			if string(g.Secrets["DATABASE_URL"]) == "postgres://PLANTED-SECRET-VALUE" {
				return true
			}
		}
		return false
	})
	if r := e.do(t, tok, "DELETE", "/projects/"+pid+"/secrets/DATABASE_URL", nil); r.status != 204 {
		t.Fatalf("delete secret: %d", r.status)
	}
	if r := e.do(t, tok, "DELETE", "/projects/"+pid+"/secrets/DATABASE_URL", nil); r.status != 404 {
		t.Fatalf("delete missing secret: %d", r.status)
	}
	// Certificates.
	_, pub, _ := sshca.GenerateHostKey("laptop")
	pubLine := strings.TrimSpace(string(sshMarshal(pub)))
	r = e.do(t, tok, "POST", "/certs", map[string]any{"public_key": pubLine, "project_ids": []string{pid}})
	if r.status != 200 || !strings.HasPrefix(r.body["certificate"].(string), "ssh-ed25519-cert-v01@openssh.com ") || r.body["gateway"].(map[string]any)["host"] != "ssh.test" {
		t.Fatalf("certs: %d %s", r.status, r.raw)
	}
	serial := int64(r.body["serial"].(float64))
	if r := e.do(t, tok, "POST", "/certs", map[string]any{"public_key": pubLine, "project_ids": []string{uuid.NewString()}}); r.status != 404 {
		t.Fatalf("cert for foreign project: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "POST", "/certs", map[string]any{"public_key": "nope", "project_ids": []string{pid}}); r.status != 400 {
		t.Fatalf("bad key: %d", r.status)
	}
	// The gateway's route check and revocation.
	ir := e.internalDo(t, "GET", "/internal/route?login=todo-app.alice-dev", nil)
	if ir.status != 200 || ir.body["project_id"] != pid || ir.body["state"] != "running" {
		t.Fatalf("route: %d %s", ir.status, ir.raw)
	}
	if ir := e.internalDo(t, "GET", "/internal/route?login=todo-app.bob", nil); ir.status != 404 {
		t.Fatalf("route wrong user: %d", ir.status)
	}
	if ir := e.internalDo(t, "GET", "/internal/revoked", nil); ir.status != 200 || len(ir.list) != 0 {
		t.Fatalf("revoked before: %d %s", ir.status, ir.raw)
	}
	if r := e.do(t, tok, "POST", "/certs/revoke", map[string]any{"serial": serial}); r.status != 200 {
		t.Fatalf("revoke: %d %s", r.status, r.raw)
	}
	if ir := e.internalDo(t, "GET", "/internal/revoked", nil); ir.status != 200 || len(ir.list) != 1 || int64(ir.list[0].(float64)) != serial {
		t.Fatalf("revoked after: %d %s", ir.status, ir.raw)
	}
	if ir := e.internalDo(t, "GET", "/internal/ca", nil); ir.status != 200 || !strings.HasPrefix(ir.body["user_ca_pub"].(string), "ssh-ed25519 ") {
		t.Fatalf("ca: %d %s", ir.status, ir.raw)
	}
	if ir := e.internalDo(t, "POST", "/internal/sessions", map[string]any{"project_id": pid, "event": "opened", "cert_serial": serial}); ir.status != 200 {
		t.Fatalf("sessions: %d %s", ir.status, ir.raw)
	}
	if ir := e.internalDo(t, "POST", "/internal/gateway-certs", map[string]any{"public_key": pubLine, "project_id": pid}); ir.status != 200 || !strings.Contains(ir.body["certificate"].(string), "ssh-ed25519-cert") {
		t.Fatalf("gateway cert: %d %s", ir.status, ir.raw)
	}
	if ir := e.internalDo(t, "GET", "/internal/hosts", nil); ir.status != 200 || len(ir.list) != 1 {
		t.Fatalf("hosts: %d %s", ir.status, ir.raw)
	}
	// Stop with snapshot, then start; ops are visible.
	r = e.do(t, tok, "POST", "/projects/"+pid+"/stop", map[string]any{"snapshot": true})
	if r.status != 202 {
		t.Fatalf("stop: %d %s", r.status, r.raw)
	}
	opID := r.body["op_id"].(string)
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("stop op: %+v", op.Error)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+opID, nil); r.status != 200 || r.body["state"] != "done" {
		t.Fatalf("get op: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/snapshots", nil); r.status != 200 || len(r.list) != 1 || r.list[0].(map[string]any)["reason"] != "stop" {
		t.Fatalf("snapshots: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/stop", nil); r.status != 409 {
		t.Fatalf("stop twice: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "PATCH", "/projects/"+pid, map[string]any{"class": "small"}); r.status != 200 || r.body["class"] != "small" {
		t.Fatalf("class change while stopped: %d %s", r.status, r.raw)
	}
	r = e.do(t, tok, "POST", "/projects/"+pid+"/start", nil)
	if r.status != 202 {
		t.Fatalf("start: %d %s", r.status, r.raw)
	}
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("start op: %+v", op.Error)
	}
	if r := e.do(t, tok, "PATCH", "/projects/"+pid, map[string]any{"class": "large"}); r.status != 409 {
		t.Fatalf("class change while running: %d", r.status)
	}
	// Resize grows only.
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/resize", map[string]any{"volume_bytes": 1}); r.status != 400 {
		t.Fatalf("shrink: %d", r.status)
	}
	r = e.do(t, tok, "POST", "/projects/"+pid+"/resize", map[string]any{"volume_bytes": 100 << 30})
	if r.status != 202 {
		t.Fatalf("resize: %d %s", r.status, r.raw)
	}
	e.waitOp(t, r)
	// Events and logs.
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/events", nil); r.status != 200 || len(r.list) == 0 {
		t.Fatalf("events: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/logs?kind=ops", nil); r.status != 200 || !strings.Contains(string(r.raw), `"kind":"create"`) {
		t.Fatalf("ops log: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/logs?kind=build", nil); r.status != 200 || !strings.Contains(string(r.raw), "evaluating") {
		t.Fatalf("build log: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "GET", "/usage", nil); r.status != 200 {
		t.Fatalf("usage: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "POST", "/billing/portal", nil); r.status != 503 || errCode(r) != "billing_disabled" {
		t.Fatalf("billing disabled: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "GET", "/catalog", nil); r.status != 200 || len(r.list) < 5 {
		t.Fatalf("catalog: %d", r.status)
	}
	// Restore as a new project.
	r = e.do(t, tok, "GET", "/projects/"+pid+"/snapshots", nil)
	sid := r.list[0].(map[string]any)["id"].(string)
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/snapshots/"+sid+"/restore", nil); r.status != 409 {
		t.Fatalf("restore into a running project: %d %s", r.status, r.raw)
	}
	// Destroy frees a slot; restore --as-new brings the snapshot back.
	r = e.do(t, tok, "DELETE", "/projects/"+pid, nil)
	if r.status != 202 {
		t.Fatalf("destroy: %d %s", r.status, r.raw)
	}
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("destroy op: %+v", op.Error)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid, nil); r.status != 404 {
		t.Fatalf("destroyed project visible: %d", r.status)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/snapshots", nil); r.status != 200 || len(r.list) != 2 {
		t.Fatalf("destroyed project's snapshots: %d %s", r.status, r.raw)
	}
	r = e.do(t, tok, "POST", "/projects/"+pid+"/snapshots/"+sid+"/restore", map[string]any{"as_new_project": "todo-yesterday"})
	if r.status != 202 {
		t.Fatalf("restore as new: %d %s", r.status, r.raw)
	}
	newID := r.body["project_id"].(string)
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("restore op: %+v", op.Error)
	}
	if r := e.do(t, tok, "GET", "/projects/"+newID, nil); r.status != 200 || r.body["state"] != "running" || r.body["slug"] != "todo-yesterday" {
		t.Fatalf("restored project: %d %s", r.status, r.raw)
	}
	// Suspended users get forbidden everywhere but GET /me.
	if _, err := e.h.Pool.Exec(ctx, "update users set suspended_at = now() where logto_sub = 'sub-alice'"); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, tok, "GET", "/projects", nil); r.status != 403 || errCode(r) != "forbidden" || !strings.Contains(string(r.raw), "account suspended") {
		t.Fatalf("suspended: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "GET", "/me", nil); r.status != 200 {
		t.Fatalf("suspended me: %d", r.status)
	}
	// Logs never carry the planted values.
	out := e.logs.String()
	for _, needle := range []string{"PLANTED-SECRET-VALUE", val, "alice@example.com", tok, "Alice_Dev", "alice-dev"} {
		if strings.Contains(out, needle) {
			t.Fatalf("log contains %q", needle)
		}
	}
	if !strings.Contains(out, `"event":"request"`) || !strings.Contains(out, `"event":"cert_issue"`) {
		t.Fatal("expected request and cert_issue log events")
	}
}

func TestConfigRoutes(t *testing.T) {
	e := newEnv(t)
	tok := e.signIn(t, "sub-carol", "carol")
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "cfg", "class": "small"})
	pid := r.body["id"].(string)
	e.waitOp(t, r)
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/config", nil); r.status != 200 || r.body["status"] != "applied" {
		t.Fatalf("get config: %d %s", r.status, r.raw)
	}
	if _, ok := config.NewParser(); ok {
		r = e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"fragment": "{ home.packages = [ pkgs.ripgrep ; }"})
		if r.status != 400 || errCode(r) != "invalid" || !strings.Contains(string(r.raw), "fragment.nix:1:") || r.body["error"].(map[string]any)["detail"].(map[string]any)["fragment_line"].(float64) != 1 {
			t.Fatalf("parse error: %d %s", r.status, r.raw)
		}
	}
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"fragment": "{ }", "menu": []any{}}); r.status != 400 {
		t.Fatalf("both fragment and menu: %d", r.status)
	}
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"fragment": strings.Repeat("x", 300<<10)}); r.status != 400 {
		t.Fatalf("oversize: %d", r.status)
	}
	// Menu renders and builds.
	r = e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"menu": []map[string]any{{"id": "bun"}, {"id": "nodejs", "options": map[string]string{"version": "22"}}}})
	if r.status != 202 || r.body["revision_id"] == nil {
		t.Fatalf("menu put: %d %s", r.status, r.raw)
	}
	rid := r.body["revision_id"].(string)
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("menu build: %+v", op.Error)
	}
	r = e.do(t, tok, "GET", "/projects/"+pid+"/config", nil)
	if r.body["revision_id"] != rid || !strings.Contains(r.body["fragment"].(string), "pkgs.nodejs_22") || r.body["menu"] == nil {
		t.Fatalf("config after menu: %s", r.raw)
	}
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"menu": []map[string]any{{"id": "nope"}}}); r.status != 400 {
		t.Fatalf("unknown menu item: %d %s", r.status, r.raw)
	}
	// Same fragment again is a no-op.
	frag := r.body["fragment"].(string)
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"fragment": frag}); r.status != 200 || r.body["unchanged"] != true {
		t.Fatalf("unchanged: %d %s", r.status, r.raw)
	}
	// Taking over with a custom fragment turns the menu off.
	r = e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"fragment": "{ pkgs, ... }: { home.packages = [ pkgs.deno ]; }"})
	if r.status != 202 {
		t.Fatalf("fragment put: %d %s", r.status, r.raw)
	}
	e.waitOp(t, r)
	if r := e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"menu": []map[string]any{{"id": "bun"}}}); r.status != 409 || !strings.Contains(string(r.raw), "custom fragment") {
		t.Fatalf("menu on fragment project: %d %s", r.status, r.raw)
	}
	// reboot_required blocks apply until confirmed.
	e.h.Fake.SetKernelChanged(true)
	r = e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"fragment": "{ pkgs, ... }: { home.packages = [ pkgs.zig ]; }"})
	if r.status != 202 {
		t.Fatalf("kernel put: %d %s", r.status, r.raw)
	}
	krid := r.body["revision_id"].(string)
	opID := r.body["op_id"].(string)
	if op := e.waitOp(t, r); op.State != "done" || !op.RebootRequired {
		t.Fatalf("kernel build: %s reboot=%v %+v", op.State, op.RebootRequired, op.Error)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/ops/"+opID, nil); r.body["reboot_required"] != true {
		t.Fatalf("op reboot flag: %s", r.raw)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/config", nil); r.body["revision_id"] == krid {
		t.Fatal("kernel revision applied without confirmation")
	}
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/config/revisions/"+krid+"/apply", nil); r.status != 409 {
		t.Fatalf("apply without reboot: %d %s", r.status, r.raw)
	}
	r = e.do(t, tok, "POST", "/projects/"+pid+"/config/revisions/"+krid+"/apply?reboot=true", nil)
	if r.status != 202 {
		t.Fatalf("apply with reboot: %d %s", r.status, r.raw)
	}
	if op := e.waitOp(t, r); op.State != "done" {
		t.Fatalf("apply op: %+v", op.Error)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/config", nil); r.body["revision_id"] != krid {
		t.Fatalf("after confirmed apply: %s", r.raw)
	}
	if r := e.do(t, tok, "GET", "/projects/"+pid+"/config/revisions", nil); r.status != 200 || len(r.list) < 4 {
		t.Fatalf("revisions: %d %d", r.status, len(r.list))
	}
	// Hold flag.
	if r := e.do(t, tok, "PATCH", "/projects/"+pid, map[string]any{"hold_base_updates": true}); r.status != 200 || r.body["hold_base_updates"] != true {
		t.Fatalf("hold: %d %s", r.status, r.raw)
	}
}

func TestRateLimits(t *testing.T) {
	e := newEnvLimits(t, nil)
	tok := e.signIn(t, "sub-dan", "dan")
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "rl", "class": "small"})
	pid := r.body["id"].(string)
	e.waitOp(t, r)
	_, pub, _ := sshca.GenerateHostKey("k")
	line := strings.TrimSpace(string(sshMarshal(pub)))
	limited := false
	for i := 0; i < 12; i++ {
		r := e.do(t, tok, "POST", "/certs", map[string]any{"public_key": line, "project_ids": []string{pid}})
		if r.status == 429 {
			if errCode(r) != "rate_limited" || r.hdr.Get("Retry-After") == "" {
				t.Fatalf("rate limited shape: %s %v", r.raw, r.hdr)
			}
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("12 cert requests were never rate limited (limit is 10/min)")
	}
	limited = false
	for i := 0; i < 60; i++ {
		r := e.do(t, tok, "GET", "/me", nil)
		if r.status == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("general limit of 60/min never hit")
	}
}

func TestSSEDeliversEveryLineInOrderWithSince(t *testing.T) {
	e := newEnv(t)
	tok := e.signIn(t, "sub-eve", "eve")
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "sse", "class": "small"})
	pid := r.body["id"].(string)
	opID := r.body["op_id"].(string)
	e.waitOp(t, r)
	// 10k lines appended to the finished op through the store (the fake host
	// only emits three).
	oid := uuid.MustParse(opID)
	for i := int64(4); i <= 10003; i++ {
		e.h.Logs.Append(oid, i, fmt.Sprintf("line %d", i))
	}
	e.h.Logs.Flush(e.h.Ctx)
	// The background flusher may still be inserting the batches it took.
	e.h.WaitFor("all lines stored", func() bool {
		var n int
		_ = e.h.Pool.QueryRow(e.h.Ctx, "select count(*) from build_logs where op_id = $1", oid).Scan(&n)
		return n == 10003
	})
	read := func(since int64, query bool) []int64 {
		url := e.api.URL + "/v1/projects/" + pid + "/ops/" + opID + "/log"
		if query {
			url += fmt.Sprintf("?access_token=%s&since=%d", tok, since)
		}
		req, _ := http.NewRequest("GET", url, nil)
		if !query {
			req.Header.Set("Authorization", "Bearer "+tok)
			req.Header.Set("Last-Event-ID", fmt.Sprint(since))
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("sse status %d %s", res.StatusCode, res.Header.Get("Content-Type"))
		}
		var seqs []int64
		done := false
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			l := sc.Text()
			if strings.HasPrefix(l, "id: ") {
				var n int64
				_, _ = fmt.Sscanf(l, "id: %d", &n)
				seqs = append(seqs, n)
			}
			if l == "event: done" {
				done = true
			}
		}
		if !done {
			t.Fatal("no done event")
		}
		return seqs
	}
	seqs := read(0, false)
	if len(seqs) != 10003 {
		t.Fatalf("got %d lines", len(seqs))
	}
	for i, s := range seqs {
		if s != int64(i+1) {
			t.Fatalf("out of order at %d: %d", i, s)
		}
	}
	tail := read(10000, true)
	if len(tail) != 3 || tail[0] != 10001 {
		t.Fatalf("since=10000 gave %v", tail)
	}
	// The token in the query string never reaches the log.
	if strings.Contains(e.logs.String(), tok) {
		t.Fatal("access_token logged")
	}
}

func TestSSELiveStreamAndConcurrentLoad(t *testing.T) {
	e := newEnv(t)
	tok := e.signIn(t, "sub-fay", "fay")
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "load", "class": "small"})
	pid := r.body["id"].(string)
	e.waitOp(t, r)
	// 20 concurrent live SSE streams on one build op.
	r = e.do(t, tok, "PUT", "/projects/"+pid+"/config", map[string]any{"fragment": "{ pkgs, ... }: { home.packages = [ pkgs.bun ]; }"})
	if r.status != 202 {
		t.Fatalf("put: %d %s", r.status, r.raw)
	}
	opID := r.body["op_id"].(string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[int]int{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, _ := http.NewRequest("GET", e.api.URL+"/v1/projects/"+pid+"/ops/"+opID+"/log", nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer func() { _ = res.Body.Close() }()
			n := 0
			sc := bufio.NewScanner(res.Body)
			for sc.Scan() {
				if strings.HasPrefix(sc.Text(), "id: ") {
					n++
				}
			}
			mu.Lock()
			counts[i] = n
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	for i, n := range counts {
		if n != 3 {
			t.Fatalf("stream %d saw %d lines", i, n)
		}
	}
	if len(counts) != 20 {
		t.Fatalf("%d streams finished", len(counts))
	}
	// 200 concurrent GET /projects, p99 under 200 ms. The general rate limit
	// is per user, so spread over users.
	var toks []string
	for i := 0; i < 5; i++ {
		toks = append(toks, e.signIn(t, fmt.Sprintf("sub-load-%d", i), fmt.Sprintf("load%d", i)))
	}
	durations := make([]time.Duration, 200)
	var lwg sync.WaitGroup
	for i := 0; i < 200; i++ {
		lwg.Add(1)
		go func(i int) {
			defer lwg.Done()
			start := time.Now()
			req, _ := http.NewRequest("GET", e.api.URL+"/v1/projects", nil)
			req.Header.Set("Authorization", "Bearer "+toks[i%len(toks)])
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
			}
			durations[i] = time.Since(start)
		}(i)
	}
	lwg.Wait()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p99 := durations[197]
	t.Logf("200 concurrent GET /projects: p50=%v p99=%v max=%v", durations[100], p99, durations[199])
	if p99 > 2*time.Second {
		t.Fatalf("p99 %v", p99)
	}
}

func TestHealthz(t *testing.T) {
	e := newEnv(t)
	res, err := http.Get(e.api.URL + "/healthz")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("healthz %v %v", res, err)
	}
	_ = res.Body.Close()
	if _, err := db.MigrateDown(e.h.Ctx, e.h.Pool, 1); err != nil {
		t.Fatal(err)
	}
	res, _ = http.Get(e.api.URL + "/healthz")
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 503 || !strings.Contains(string(body), "migrations pending") {
		t.Fatalf("healthz with pending migration: %d %s", res.StatusCode, body)
	}
	if _, err := db.MigrateUp(e.h.Ctx, e.h.Pool); err != nil {
		t.Fatal(err)
	}
	if ir := e.internalDo(t, "POST", "/internal/events", map[string]any{"source_ip": "10.0.0.1", "agent": "claude", "kind": "completed", "summary": "x"}); ir.status != 404 {
		t.Fatalf("event from unknown ip: %d %s", ir.status, ir.raw)
	}
}
