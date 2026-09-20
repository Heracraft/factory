// Package logto is an in-process Logto for the api's tests: it serves the
// OIDC JWKS, the client-credentials token endpoint and the Management API
// user lookup, and mints RS256 access tokens for any subject.
package logto

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// User is what the Management API returns for a subject.
type User struct {
	Email       string
	GithubLogin string
	Username    string
}

// Fake is the server.
type Fake struct {
	Server   *httptest.Server
	key      *rsa.PrivateKey
	kid      string
	audience string
	mu       sync.Mutex
	users    map[string]User
	// JWKSDown makes the JWKS endpoint fail; MgmtDown the users endpoint.
	JWKSDown, MgmtDown bool
	jwksHits int
}

// New starts the fake for the given API resource audience.
func New(audience string) *Fake {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("fake logto: rsa keygen: " + err.Error()) // test fixture
	}
	f := &Fake{key: key, kid: "k1", audience: audience, users: map[string]User{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /oidc/jwks", f.jwks)
	mux.HandleFunc("POST /oidc/token", f.token)
	mux.HandleFunc("GET /api/users/{id}", f.user)
	f.Server = httptest.NewServer(mux)
	return f
}

// Close stops the server.
func (f *Fake) Close() { f.Server.Close() }

// Issuer is the OIDC issuer URL.
func (f *Fake) Issuer() string { return f.Server.URL }

// AddUser registers a subject's identity.
func (f *Fake) AddUser(sub string, u User) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[sub] = u
}

// JWKSHits reports how often the JWKS was fetched.
func (f *Fake) JWKSHits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jwksHits
}

// Token mints an access token for sub with the configured audience.
func (f *Fake) Token(sub string) string {
	return f.TokenWith(sub, f.audience, time.Hour)
}

// TokenWith mints a token with a chosen audience and lifetime.
func (f *Fake) TokenWith(sub, aud string, ttl time.Duration) string {
	claims := jwt.MapClaims{"sub": sub, "aud": aud, "iss": f.Issuer(), "exp": time.Now().Add(ttl).Unix(), "iat": time.Now().Unix()}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.kid
	s, err := tok.SignedString(f.key)
	if err != nil {
		panic("fake logto: sign: " + err.Error()) // test fixture
	}
	return s
}

// TokenWrongKey mints a token signed by an unknown key.
func (f *Fake) TokenWrongKey(sub string) string {
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	claims := jwt.MapClaims{"sub": sub, "aud": f.audience, "iss": f.Issuer(), "exp": time.Now().Add(time.Hour).Unix()}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.kid
	s, _ := tok.SignedString(other)
	return s
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (f *Fake) jwks(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.jwksHits++
	down := f.JWKSDown
	f.mu.Unlock()
	if down {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	pub := f.key.PublicKey
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{ // test server
		"kty": "RSA", "kid": f.kid, "use": "sig", "alg": "RS256",
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (f *Fake) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "client_credentials" {
		http.Error(w, "bad grant", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "m2m-" + r.Form.Get("client_id"), "token_type": "Bearer", "expires_in": 3600}) // test server
}

func (f *Fake) user(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	u, ok := f.users[r.PathValue("id")]
	down := f.MgmtDown
	f.mu.Unlock()
	if down {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer m2m-") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{ // test server
		"id": r.PathValue("id"), "primaryEmail": u.Email, "username": u.Username,
		"identities": map[string]any{"github": map[string]any{"userId": "42", "details": map[string]any{"login": u.GithubLogin, "email": u.Email}}},
	})
}
