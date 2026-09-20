package admin_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/admin"
	"github.com/heracraft/repose/internal/api/apitest"
	"github.com/heracraft/repose/internal/api/hostmgr"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

func run(t *testing.T, e *admin.Env, args ...string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	e.Stdout, e.Stderr = &out, &errb
	err := admin.Run(context.Background(), e, args)
	return out.String() + errb.String(), err
}

// Every subcommand named in DECISIONS I-9 and the runbook exists and
// does its work against the database; ops-backed commands complete
// through the harness's engine and fake host.
func TestAdminSurface(t *testing.T) {
	h := apitest.New(t, apitest.Options{})
	e := &admin.Env{KV: h.KV, Actor: "admin:test"}
	e.SetPool(h.Pool)
	ctx := h.Ctx
	// db
	out, err := run(t, e, "db", "status")
	if err != nil || !strings.Contains(out, "pending: []") {
		t.Fatalf("db status: %s %v", out, err)
	}
	if out, err := run(t, e, "db", "verify"); err != nil || !strings.Contains(out, "users") {
		t.Fatalf("db verify: %s %v", out, err)
	}
	if _, err := run(t, e, "db", "down", "1"); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, e, "db", "status"); err != nil || !strings.Contains(out, "pending: [3]") {
		t.Fatalf("after down: %s %v", out, err)
	}
	if _, err := run(t, e, "db", "migrate"); err != nil {
		t.Fatal(err)
	}
	// ca
	if _, err := run(t, e, "ca", "init"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, e, "ca", "init"); err == nil {
		t.Fatal("second ca init should refuse")
	}
	// hosts add mints a single-use token and refuses a second without --reissue once registered.
	out, err = run(t, e, "hosts", "add", "--name", "host-02", "--sku", "Standard_D16s_v7")
	if err != nil || len(strings.TrimSpace(strings.Split(out, "\n")[0])) < 40 {
		t.Fatalf("hosts add: %q %v", out, err)
	}
	if out, err := run(t, e, "hosts", "list"); err != nil || !strings.Contains(out, "host-02") || !strings.Contains(out, "host-01") {
		t.Fatalf("hosts list: %s %v", out, err)
	}
	if _, err := run(t, e, "hosts", "add", "--name", "host-01"); err == nil {
		t.Fatal("hosts add for a registered host without --reissue should refuse")
	}
	if _, err := run(t, e, "hosts", "add", "--name", "host-02", "--reissue"); err != nil {
		t.Fatalf("reissue: %v", err)
	}
	// users
	u := h.NewUser("zed")
	if out, err := run(t, e, "users", "list"); err != nil || !strings.Contains(out, "zed") {
		t.Fatalf("users list: %s %v", out, err)
	}
	if _, err := run(t, e, "users", "exempt", "zed"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, e, "users", "limits", "zed", "--projects", "10", "--xl", "10"); err != nil {
		t.Fatal(err)
	}
	uu, _ := store.GetUser(ctx, h.Pool, u.ID)
	if uu.BillingStatus != "exempt" || uu.ProjectLimit != 10 || uu.XLLimit != 10 {
		t.Fatalf("user after admin: %+v", uu)
	}
	// projects driven through the engine
	p := h.CreateRunning(u, "zp")
	if out, err := run(t, e, "projects", "show", p.ID.String()); err != nil || !strings.Contains(out, "running") {
		t.Fatalf("projects show: %s %v", out, err)
	}
	if out, err := run(t, e, "projects", "list", "--host", "host-01"); err != nil || !strings.Contains(out, "zp") {
		t.Fatalf("projects list: %s %v", out, err)
	}
	if _, err := run(t, e, "projects", "snapshot", "zp"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if out, err := run(t, e, "exec", "zp", "--", "uptime"); err != nil || !strings.Contains(out, "fake") {
		t.Fatalf("exec: %s %v", out, err)
	}
	if out, err := run(t, e, "audit", "--action", "exec"); err != nil || !strings.Contains(out, "uptime") {
		t.Fatalf("audit: %s %v", out, err)
	}
	if _, err := run(t, e, "projects", "restart", "zp"); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if h.Project(p.ID).State != "running" {
		t.Fatal("not running after restart")
	}
	if _, err := run(t, e, "projects", "stop", "zp", "--no-snapshot"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := run(t, e, "projects", "restore", "zp", "--latest"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if h.Project(p.ID).State != "running" {
		t.Fatal("not running after restore")
	}
	if out, err := run(t, e, "ops", "list", "--project", "zp"); err != nil || !strings.Contains(out, "restore") {
		t.Fatalf("ops list: %s %v", out, err)
	}
	if _, err := run(t, e, "users", "suspend", "zed", "--reason", "abuse"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	h.WaitFor("project stopped after suspend", func() bool { return h.Project(p.ID).State == "stopped" })
	if _, err := run(t, e, "users", "unsuspend", "zed"); err != nil {
		t.Fatal(err)
	}
	// base
	if _, err := run(t, e, "base", "publish", "--rev", "abc123", "--changelog", "first", "--version", "2026.09.20"); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, e, "base", "list"); err != nil || !strings.Contains(out, "2026.09.20") {
		t.Fatalf("base list: %s %v", out, err)
	}
	if out, err := run(t, e, "base", "status", "2026.09.20"); err != nil || !strings.Contains(out, "zp") {
		t.Fatalf("base status: %s %v", out, err)
	}
	// smoke: create, snapshot, stop, start, destroy on host-01
	if out, err := run(t, e, "hosts", "smoke", "host-01"); err != nil || !strings.Contains(out, "destroy   ok") {
		t.Fatalf("smoke: %s %v", out, err)
	}
	// drain, reconcile, retire refusing while projects remain, mark-lost
	if _, err := run(t, e, "hosts", "drain", "host-01"); err != nil {
		t.Fatalf("drain: %v", err)
	}
	h.WaitFor("host draining", func() bool {
		hr, _ := store.GetHost(ctx, h.Pool, h.HostID)
		return hr != nil && hr.Draining
	})
	if out, err := run(t, e, "hosts", "reconcile", "host-01"); err != nil || !strings.Contains(out, "zp") {
		t.Fatalf("reconcile: %s %v", out, err)
	}
	if _, err := run(t, e, "hosts", "retire", "host-01"); err == nil {
		t.Fatal("retire with projects should refuse")
	}
	if out, err := run(t, e, "hosts", "undrain", "host-01"); err != nil || !strings.Contains(out, "hostd") {
		t.Fatalf("undrain: %s %v", out, err)
	}
	// secrets rewrap and certs revoke
	if _, err := run(t, e, "secrets", "rewrap"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, e, "certs", "revoke", "--user", "zed"); err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, e, "billing", "rollup"); err != nil || !strings.Contains(out, "rolled up") {
		t.Fatalf("rollup: %s %v", out, err)
	}
	if _, err := run(t, e, "edge", "init", "--endpoint", "1.2.3.4:51820", "--pubkey", "abc="); err != nil {
		t.Fatal(err)
	}
	// ca show prints the public halves only; sign-server issues a leaf the
	// CA certificate verifies, with IP names as IP SANs.
	if out, err := run(t, e, "ca", "show"); err != nil || !strings.Contains(out, "-----BEGIN CERTIFICATE-----") || strings.Contains(out, "PRIVATE KEY") || !strings.Contains(out, "# user ca: ssh-ed25519") {
		t.Fatalf("ca show: %q %v", out, err)
	}
	if out, err := run(t, e, "ca", "sign-server", "--name", "10.255.0.1,edge.internal"); err != nil || !strings.Contains(out, "-----BEGIN CERTIFICATE-----") || !strings.Contains(out, "PRIVATE KEY") {
		t.Fatalf("ca sign-server: %q %v", out, err)
	} else {
		leaf, _ := pem.Decode([]byte(out))
		cert, err := x509.ParseCertificate(leaf.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		if len(cert.IPAddresses) != 1 || cert.IPAddresses[0].String() != "10.255.0.1" || len(cert.DNSNames) != 1 {
			t.Fatalf("sign-server SANs: ip %v dns %v", cert.IPAddresses, cert.DNSNames)
		}
		show, _ := run(t, e, "ca", "show")
		caBlock, _ := pem.Decode([]byte(show[strings.Index(show, "-----BEGIN"):]))
		caCert, err := x509.ParseCertificate(caBlock.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		if err := cert.CheckSignatureFrom(caCert); err != nil {
			t.Fatalf("leaf not signed by the shown CA: %v", err)
		}
	}
	if _, err := run(t, e, "ca", "sign-server"); err == nil {
		t.Fatal("sign-server without --name should refuse")
	}
	// --csr signs a request made elsewhere and prints the certificate alone:
	// the private key never travels (the edge's gateway key, I-92).
	csrKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, csrKey)
	csrFile := filepath.Join(t.TempDir(), "gateway.csr")
	if err := os.WriteFile(csrFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"ca", "sign-client", "--name", "gateway", "--csr", csrFile}, {"ca", "sign-server", "--name", "10.255.0.1", "--csr", csrFile}} {
		out, err := run(t, e, args...)
		if err != nil || !strings.Contains(out, "-----BEGIN CERTIFICATE-----") || strings.Contains(out, "PRIVATE KEY") {
			t.Fatalf("%v: %q %v", args, out, err)
		}
		blk, _ := pem.Decode([]byte(out[strings.Index(out, "-----BEGIN"):]))
		crt, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		if !csrKey.PublicKey.Equal(crt.PublicKey) {
			t.Fatalf("%v: certificate is not for the request's key", args)
		}
	}
	if v, _ := store.Setting(ctx, h.Pool, "edge_wg_endpoint"); v != "1.2.3.4:51820" {
		t.Fatalf("edge setting %q", v)
	}
	// edge loki: prints, records, refuses a URL with no scheme, clears
	// (DECISIONS I-95). Every host's Fluent Bit output address, so a typo
	// here is a fleet that ships nowhere.
	if out, err := run(t, e, "edge", "loki"); err != nil || !strings.Contains(out, "no Loki recorded") {
		t.Fatalf("edge loki (empty): %q %v", out, err)
	}
	if _, err := run(t, e, "edge", "loki", "10.255.0.3:3100"); err == nil {
		t.Fatal("edge loki should refuse a URL with no scheme")
	}
	if out, err := run(t, e, "edge", "loki", "http://10.255.0.3:3100"); err != nil || !strings.Contains(out, "Loki recorded") {
		t.Fatalf("edge loki: %q %v", out, err)
	}
	if v, _ := store.Setting(ctx, h.Pool, hostmgr.SettingLokiURL); v != "http://10.255.0.3:3100" {
		t.Fatalf("loki setting %q", v)
	}
	if out, err := run(t, e, "edge", "loki"); err != nil || !strings.Contains(out, "http://10.255.0.3:3100") {
		t.Fatalf("edge loki (show): %q %v", out, err)
	}
	if _, err := run(t, e, "hosts", "mark-lost", "host-02"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, e, "nope"); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("unknown command: %v", err)
	}
	var audits int
	_ = h.Pool.QueryRow(ctx, "select count(*) from audit_log where actor = 'admin:test'").Scan(&audits)
	if audits < 15 {
		t.Fatalf("admin actions audited: %d", audits)
	}
}
