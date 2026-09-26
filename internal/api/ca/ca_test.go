package ca_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	"github.com/heracraft/repose/internal/api/ca"
	"github.com/heracraft/repose/internal/api/secrets"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/ca/sshca"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
	"github.com/heracraft/repose/internal/fakes/kv"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

func seedUserProjects(t *testing.T, pool *db.Pool, n int) (*store.User, []uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	uid := store.NewID()
	if _, err := pool.Exec(ctx, "insert into users (id, handle) values ($1, $2)", uid, "h"+uuid.NewString()[:8]); err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	for i := 0; i < n; i++ {
		pid := store.NewID()
		if _, err := pool.Exec(ctx, "insert into projects (id, user_id, name, slug, class, state, volume_bytes) values ($1, $2, $3, $3, 'large', 'running', 1)", pid, uid, "p"+uuid.NewString()[:8]); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, pid)
	}
	u, err := store.GetUser(ctx, pool, uid)
	if err != nil {
		t.Fatal(err)
	}
	return u, ids
}

func TestInitLoadAndUserCert(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	sec := secrets.New(pool, kv.New())
	if _, err := ca.Load(ctx, pool, sec); !errors.Is(err, ca.ErrNotInitialised) {
		t.Fatalf("expected not initialised, got %v", err)
	}
	if err := ca.Init(ctx, sec); err != nil {
		t.Fatal(err)
	}
	if err := ca.Init(ctx, sec); err == nil {
		t.Fatal("second init should refuse")
	}
	c, err := ca.Load(ctx, pool, sec)
	if err != nil {
		t.Fatal(err)
	}
	user, projects := seedUserProjects(t, pool, 2)
	_, other := seedUserProjects(t, pool, 1)
	_, pub, err := sshca.GenerateHostKey("laptop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.IssueUserCert(ctx, user, pub, []uuid.UUID{projects[0], other[0]}); !errors.Is(err, ca.ErrNotOwner) {
		t.Fatalf("another user's project accepted: %v", err)
	}
	issued, err := c.IssueUserCert(ctx, user, pub, projects)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(issued.Certificate))
	if err != nil {
		t.Fatal(err)
	}
	cert := parsed.(*ssh.Certificate)
	if len(cert.ValidPrincipals) != 2 || cert.ValidPrincipals[0] != projects[0].String() {
		t.Fatalf("principals %v", cert.ValidPrincipals)
	}
	if cert.KeyId != user.ID.String()+":"+user.Handle || cert.Serial != uint64(issued.Serial) {
		t.Fatalf("key id %s serial %d", cert.KeyId, cert.Serial)
	}
	for ext := range sshca.UserExtensions {
		if _, ok := cert.Extensions[ext]; !ok {
			t.Fatalf("missing extension %s", ext)
		}
	}
	if d := time.Until(time.Unix(int64(cert.ValidBefore), 0)); d < sshca.UserCertTTL-time.Hour || d > sshca.UserCertTTL+time.Minute {
		t.Fatalf("validity %v", d)
	}
	caPub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.UserCAPub()))
	if err != nil {
		t.Fatal(err)
	}
	checker := ssh.CertChecker{IsUserAuthority: func(k ssh.PublicKey) bool { return string(k.Marshal()) == string(caPub.Marshal()) }}
	// The gateway's route check: a cert for project A is refused for B.
	if err := checker.CheckCert(projects[0].String(), cert); err != nil {
		t.Fatal(err)
	}
	if err := checker.CheckCert(other[0].String(), cert); err == nil {
		t.Fatal("certificate accepted for a project it does not carry")
	}
	// Revocation shows in the next RevokedSince call.
	before, _ := c.RevokedSince(ctx, time.Time{})
	if len(before) != 0 {
		t.Fatalf("revoked before revoke: %v", before)
	}
	if err := c.Revoke(ctx, user.ID, issued.Serial); err != nil {
		t.Fatal(err)
	}
	after, err := c.RevokedSince(ctx, time.Time{})
	if err != nil || len(after) != 1 || after[0] != issued.Serial {
		t.Fatalf("revoked list %v %v", after, err)
	}
	if err := c.Revoke(ctx, other[0], issued.Serial); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("revoking someone else's serial: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "select count(*) from audit_log where action in ('cert_issue','cert_revoke')").Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit rows %d %v", n, err)
	}
}

func TestGatewayAndGuestHostCerts(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	c, err := ca.NewInMemory(pool)
	if err != nil {
		t.Fatal(err)
	}
	user, projects := seedUserProjects(t, pool, 1)
	p, _ := store.GetProject(ctx, pool, projects[0])
	_, gwPub, _ := sshca.GenerateHostKey("gateway")
	issued, err := c.IssueGatewayCert(ctx, gwPub, p)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(issued.Certificate))
	cert := parsed.(*ssh.Certificate)
	if cert.KeyId != user.ID.String()+":"+user.Handle+":via-gateway" || cert.ValidPrincipals[0] != p.ID.String() {
		t.Fatalf("gateway cert %+v", cert)
	}
	if d := time.Until(time.Unix(int64(cert.ValidBefore), 0)); d > 5*time.Minute+time.Second {
		t.Fatalf("gateway validity %v", d)
	}
	hk, err := c.NewGuestHostKey(ctx, []string{"10.64.4.2", p.Slug + "." + user.Handle})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(hk.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _, _, _ = ssh.ParseAuthorizedKey([]byte(hk.Certificate))
	hc := parsed.(*ssh.Certificate)
	if hc.CertType != ssh.HostCert || string(hc.Key.Marshal()) != string(signer.PublicKey().Marshal()) {
		t.Fatal("host cert does not match the generated key")
	}
	hostCAPub, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(c.HostCAPub()))
	checker := ssh.CertChecker{IsHostAuthority: func(k ssh.PublicKey, _ string) bool { return string(k.Marshal()) == string(hostCAPub.Marshal()) }}
	if err := checker.CheckCert("10.64.4.2", hc); err != nil {
		t.Fatal(err)
	}
	if err := checker.CheckCert(p.Slug+"."+user.Handle, hc); err != nil {
		t.Fatal(err)
	}
}
