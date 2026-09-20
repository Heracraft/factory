package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// loginOptions configures runLogin so tests can stub the browser and the
// clock; the real command builds this from cobra flags and the OS.
type loginOptions struct {
	NoBrowser bool
	Display   string // GOOS "" means "no DISPLAY env"; darwin/windows ignore it
	GOOS      string
	GuestEnv  bool // REPOSE=1
	Stdout    *os.File
	// Open opens a URL in a browser; nil means openBrowser. Tests replace
	// it with a stub that hits the loopback callback directly instead of
	// launching a real browser.
	Open func(string) error
}

func runLogin(ctx context.Context, dir string, cfg Config, httpClient *http.Client, opts loginOptions) error {
	doc, err := discover(ctx, httpClient, dir, cfg.LogtoIssuer)
	if err != nil {
		return exitf(ExitGeneric, "Cannot reach %s: %v", cfg.LogtoIssuer, err)
	}

	open := opts.Open
	if open == nil {
		open = openBrowser
	}
	var tr *tokenResponse
	if browserAvailable(opts.NoBrowser, opts.GuestEnv, opts.Display, opts.GOOS) {
		tr, err = loginPKCE(ctx, httpClient, doc, cfg.LogtoClientID, open)
	} else {
		tr, err = loginDeviceCode(ctx, httpClient, doc, cfg.LogtoClientID, func(s string) { fmt.Println(s) })
	}
	if err != nil {
		return exitf(ExitGeneric, "Login failed: %v", err)
	}

	creds := Credentials{
		RefreshToken:  tr.RefreshToken,
		AccessToken:   tr.AccessToken,
		ExpiresAt:     time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		LogtoIssuer:   cfg.LogtoIssuer,
		LogtoClientID: cfg.LogtoClientID,
	}
	if err := saveCredentials(dir, creds); err != nil {
		return err
	}

	client := newClient(cfg.APIURL, staticToken(tr.AccessToken))
	me, err := client.GetMe(ctx)
	if err != nil {
		return exitf(ExitGeneric, "Logged in, but could not fetch your account: %v", err)
	}
	fmt.Printf("Logged in as %s (%s)\n", me.Handle, me.Email)
	if !me.Billing.HasCard {
		fmt.Printf("No card on file. Add one at https://repose.herakraft.co/billing before the first `repose run`.\n")
	}
	return nil
}

func runLogout(ctx context.Context, dir string, cfg Config, httpClient *http.Client, purge bool) error {
	creds, ok, err := loadCredentials(dir)
	if err != nil {
		return err
	}
	// Best effort: revoking the SSH certificates and deleting the local
	// credentials matters more than Logto's own refresh-token revocation,
	// which this CLI does not call (not part of the discovery document it
	// reads); an un-revoked refresh token simply expires on its own.
	if ok && creds.AccessToken != "" {
		client := newClient(cfg.APIURL, newOIDCTokenSource(dir, httpClient, creds))
		_ = client.RevokeCertsAll(ctx)
	}
	if err := deleteCredentials(dir); err != nil {
		return err
	}
	sd, err := sshDir()
	if err != nil {
		return err
	}
	_ = os.Remove(sd + "/id_ed25519-cert.pub")
	if purge {
		return purgeCLIFiles(dir, sd)
	}
	return nil
}
