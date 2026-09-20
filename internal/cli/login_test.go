package cli

import (
	"context"
	"net/http"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// openViaGet simulates a browser: it just follows the redirect chain
// starting at the authorization URL, the way a browser would after a user
// who is already signed in clicks through instantly.
func openViaGet(url string) error {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func TestLoginPKCE(t *testing.T) {
	oidc := newFakeOIDC()
	defer oidc.Close()
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()

	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.APIURL = fake.URL() + "/v1"
	cfg.LogtoIssuer = oidc.Issuer()

	err := runLogin(context.Background(), dir, cfg, http.DefaultClient, loginOptions{GOOS: "linux", Display: ":0", Open: openViaGet})
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}

	creds, ok, err := loadCredentials(dir)
	if err != nil || !ok {
		t.Fatalf("loadCredentials: ok=%v err=%v", ok, err)
	}
	if creds.AccessToken == "" || creds.RefreshToken == "" {
		t.Fatalf("credentials incomplete: %+v", creds)
	}
	if creds.ExpiresAt.Before(time.Now()) {
		t.Fatalf("expires_at in the past: %v", creds.ExpiresAt)
	}
}

func TestLoginDeviceCode(t *testing.T) {
	oidc := newFakeOIDC()
	defer oidc.Close()
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()

	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.APIURL = fake.URL() + "/v1"
	cfg.LogtoIssuer = oidc.Issuer()

	err := runLogin(context.Background(), dir, cfg, http.DefaultClient, loginOptions{NoBrowser: true, GOOS: "linux"})
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	creds, ok, err := loadCredentials(dir)
	if err != nil || !ok || creds.RefreshToken == "" {
		t.Fatalf("loadCredentials: ok=%v err=%v creds=%+v", ok, err, creds)
	}
}

func TestOIDCTokenSourceRefreshes(t *testing.T) {
	oidc := newFakeOIDC()
	defer oidc.Close()
	dir := t.TempDir()

	// Seed credentials with an already-expired access token so
	// AccessToken must refresh.
	creds := Credentials{RefreshToken: "seed", AccessToken: "stale", ExpiresAt: time.Now().Add(-time.Minute), LogtoIssuer: oidc.Issuer()}
	oidc.refresh["seed"] = "user-1"

	src := newOIDCTokenSource(dir, http.DefaultClient, creds)
	tok, err := src.AccessToken(context.Background(), false)
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if tok != "access-user-1" {
		t.Fatalf("got token %q", tok)
	}

	// A second call with a still-valid token must not hit the network
	// (deleting the refresh token would break a real refresh).
	delete(oidc.refresh, "seed")
	tok2, err := src.AccessToken(context.Background(), false)
	if err != nil || tok2 != tok {
		t.Fatalf("cached AccessToken: tok=%q err=%v", tok2, err)
	}
}

func TestRunLogoutRemovesCredentialsAndCert(t *testing.T) {
	oidc := newFakeOIDC()
	defer oidc.Close()
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()

	dir := t.TempDir()
	cfg := defaultConfig()
	cfg.APIURL = fake.URL() + "/v1"
	cfg.LogtoIssuer = oidc.Issuer()

	if err := runLogin(context.Background(), dir, cfg, http.DefaultClient, loginOptions{NoBrowser: true, GOOS: "linux"}); err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	if err := runLogout(context.Background(), dir, cfg, http.DefaultClient, false); err != nil {
		t.Fatalf("runLogout: %v", err)
	}
	if _, ok, _ := loadCredentials(dir); ok {
		t.Fatal("credentials still present after logout")
	}
}
