package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
)

// fakeOIDC is a minimal RFC 8414 / PKCE / device-code authorization
// server for this package's own login tests. It is not
// internal/fakes/logto (that fake serves the api's server-side JWT
// verification tests: JWKS and client-credentials, no user-facing
// authorization or device flow), so the CLI's login tests need their own.
type fakeOIDC struct {
	Server *httptest.Server

	mu        sync.Mutex
	nextSub   int
	challenge map[string]string // code -> code_challenge
	refresh   map[string]string // refresh_token -> subject
}

func newFakeOIDC() *fakeOIDC {
	f := &fakeOIDC{challenge: map[string]string{}, refresh: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/oidc/.well-known/openid-configuration", f.discovery)
	mux.HandleFunc("/oidc/auth", f.authorize)
	mux.HandleFunc("/oidc/token", f.token)
	mux.HandleFunc("/oidc/device/auth", f.deviceAuth)
	f.Server = httptest.NewServer(mux)
	return f
}

func (f *fakeOIDC) Close()         { f.Server.Close() }
func (f *fakeOIDC) Issuer() string { return f.Server.URL }

func (f *fakeOIDC) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSONTest(w, map[string]string{
		"authorization_endpoint":        f.Server.URL + "/oidc/auth",
		"token_endpoint":                f.Server.URL + "/oidc/token",
		"device_authorization_endpoint": f.Server.URL + "/oidc/device/auth",
	})
}

// authorize simulates a user who is already logged in and approves
// instantly: it 302s straight to redirect_uri with a code, the way a real
// browser session would look to the CLI's loopback listener after the
// user clicks through.
func (f *fakeOIDC) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := "code-" + q.Get("state")
	f.mu.Lock()
	f.challenge[code] = q.Get("code_challenge")
	f.mu.Unlock()
	redirect := q.Get("redirect_uri") + "?code=" + code + "&state=" + q.Get("state")
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (f *fakeOIDC) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		code := r.Form.Get("code")
		verifier := r.Form.Get("code_verifier")
		f.mu.Lock()
		challenge, ok := f.challenge[code]
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(verifier))
		want := base64.RawURLEncoding.EncodeToString(sum[:])
		if !ok || challenge != want {
			writeJSONTest(w, map[string]string{"error": "invalid_grant", "error_description": "pkce mismatch"})
			return
		}
		f.issue(w, "user-1")
	case "refresh_token":
		f.mu.Lock()
		sub, ok := f.refresh[r.Form.Get("refresh_token")]
		f.mu.Unlock()
		if !ok {
			writeJSONTest(w, map[string]string{"error": "invalid_grant", "error_description": "unknown refresh token"})
			return
		}
		f.issue(w, sub)
	case "urn:ietf:params:oauth:grant-type:device_code":
		f.issue(w, "user-1")
	default:
		writeJSONTest(w, map[string]string{"error": "unsupported_grant_type"})
	}
}

func (f *fakeOIDC) issue(w http.ResponseWriter, sub string) {
	f.mu.Lock()
	f.nextSub++
	refresh := "refresh-" + sub + "-" + strconv.Itoa(f.nextSub)
	f.refresh[refresh] = sub
	f.mu.Unlock()
	writeJSONTest(w, map[string]any{
		"access_token":  "access-" + sub,
		"refresh_token": refresh,
		"expires_in":    3600,
	})
}

func (f *fakeOIDC) deviceAuth(w http.ResponseWriter, r *http.Request) {
	writeJSONTest(w, map[string]any{
		"device_code":      "devcode-1",
		"user_code":        "ABCD-EFGH",
		"verification_uri": f.Server.URL + "/device",
		"interval":         0,
		"expires_in":       300,
	})
}

func writeJSONTest(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
