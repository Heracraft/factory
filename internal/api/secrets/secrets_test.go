package secrets_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/secrets"
	"github.com/heracraft/repose/internal/db/testdb"
	"github.com/heracraft/repose/internal/fakes/kv"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

func newProject(t *testing.T, s *secrets.Store, pool *dbPool) (userID, projectID string) {
	t.Helper()
	ctx := context.Background()
	uid := uuid.Must(uuid.NewV7()).String()
	pid := uuid.Must(uuid.NewV7()).String()
	if _, err := pool.Exec(ctx, "insert into users (id, handle) values ($1, $2)", uid, "h-"+uuid.NewString()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "insert into projects (id, user_id, name, slug, class, state, volume_bytes) values ($1, $2, 'p', $3, 'large', 'stopped', 1)", pid, uid, "p-"+uuid.NewString()[:8]); err != nil {
		t.Fatal(err)
	}
	return uid, pid
}

type dbPool = testdbPool

func TestRoundTripAndAAD(t *testing.T) {
	pool := testdb.Open(t)
	fk := kv.New()
	s := secrets.New(pool, fk)
	ctx := context.Background()
	uid, pid := newProject(t, s, pool)
	if err := s.Put(ctx, uid, pid, "DATABASE_URL", []byte("postgres://secret")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, uid, pid, "OTHER", []byte("x")); err != nil {
		t.Fatal(err)
	}
	vals, err := s.DecryptForGuest(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 2 || vals[0].Name != "DATABASE_URL" || string(vals[0].Value) != "postgres://secret" {
		t.Fatalf("got %+v", vals)
	}
	// Ciphertext for name A must not decrypt under name B.
	var ct, wrapped []byte
	var version string
	if err := pool.QueryRow(ctx, "select ciphertext, dek_wrapped, kv_key_version from secrets where project_id = $1 and name = 'DATABASE_URL'", pid).Scan(&ct, &wrapped, &version); err != nil {
		t.Fatal(err)
	}
	dek, err := fk.Unwrap(ctx, wrapped, version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.Open(dek, "OTHER", ct); err == nil {
		t.Fatal("ciphertext decrypted under another name")
	}
	if v, err := secrets.Open(dek, "DATABASE_URL", ct); err != nil || string(v) != "postgres://secret" {
		t.Fatalf("open under own name: %v %q", err, v)
	}
	metas, err := s.List(ctx, pid)
	if err != nil || len(metas) != 2 || metas[0].Name != "DATABASE_URL" {
		t.Fatalf("list %+v %v", metas, err)
	}
	ok, err := s.Delete(ctx, pid, "OTHER")
	if err != nil || !ok {
		t.Fatal(err)
	}
	vals, _ = s.DecryptForGuest(ctx, pid)
	if len(vals) != 1 {
		t.Fatalf("after delete: %d", len(vals))
	}
}

func TestNames(t *testing.T) {
	for name, want := range map[string]bool{"A": true, "DATABASE_URL": true, "a": false, "1A": false, "ssh_host_ed25519_key": false, "user_ca.pub": false, "ssh_host_ed25519_key-cert.pub": false} {
		if got := secrets.ValidName(name); got != want {
			t.Errorf("%q: got %v want %v", name, got, want)
		}
	}
	pool := testdb.Open(t)
	s := secrets.New(pool, kv.New())
	uid, pid := newProject(t, s, pool)
	if err := s.Put(context.Background(), uid, pid, "bad", []byte("x")); !errors.Is(err, secrets.ErrInvalidName) {
		t.Fatalf("expected invalid name, got %v", err)
	}
	if err := s.Put(context.Background(), uid, pid, "BIG", bytes.Repeat([]byte("x"), secrets.MaxValueBytes+1)); !errors.Is(err, secrets.ErrTooLarge) {
		t.Fatalf("expected too large, got %v", err)
	}
}

func TestRewrapAfterKeyVersionBump(t *testing.T) {
	pool := testdb.Open(t)
	fk := kv.New()
	s := secrets.New(pool, fk)
	ctx := context.Background()
	uid, pid := newProject(t, s, pool)
	_, pid2 := newProject(t, s, pool)
	for _, n := range []string{"A", "B", "C"} {
		if err := s.Put(ctx, uid, pid, n, []byte("value-"+n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Put(ctx, uid, pid2, "D", []byte("value-D")); err != nil {
		t.Fatal(err)
	}
	var before [][]byte
	rows, _ := pool.Query(ctx, "select ciphertext from secrets order by name")
	for rows.Next() {
		var ct []byte
		_ = rows.Scan(&ct)
		before = append(before, ct)
	}
	rows.Close()
	v2 := fk.BumpVersion()
	n, err := s.Rewrap(ctx)
	if err != nil || n != 4 {
		t.Fatalf("rewrap n=%d err=%v", n, err)
	}
	var after [][]byte
	var versions []string
	rows, _ = pool.Query(ctx, "select ciphertext, kv_key_version from secrets order by name")
	for rows.Next() {
		var ct []byte
		var v string
		_ = rows.Scan(&ct, &v)
		after = append(after, ct)
		versions = append(versions, v)
	}
	rows.Close()
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Fatal("rewrap touched ciphertext")
		}
		if versions[i] != v2 {
			t.Fatalf("row %d still on %s", i, versions[i])
		}
	}
	// A fresh store (empty cache) decrypts with the new wrapping.
	s2 := secrets.New(pool, fk)
	vals, err := s2.DecryptForGuest(ctx, pid)
	if err != nil || len(vals) != 3 || string(vals[2].Value) != "value-C" {
		t.Fatalf("after rewrap: %+v %v", vals, err)
	}
	n, err = s.Rewrap(ctx)
	if err != nil || n != 0 {
		t.Fatalf("second rewrap should be a no-op: %d %v", n, err)
	}
}

func TestKeyVaultDownFailsClosed(t *testing.T) {
	pool := testdb.Open(t)
	fk := kv.New()
	s := secrets.New(pool, fk)
	ctx := context.Background()
	uid, pid := newProject(t, s, pool)
	if err := s.Put(ctx, uid, pid, "A", []byte("v")); err != nil {
		t.Fatal(err)
	}
	fk.Down = true
	s2 := secrets.New(pool, fk) // empty cache
	if _, err := s2.DecryptForGuest(ctx, pid); !errors.Is(err, secrets.ErrKeyServiceUnavailable) {
		t.Fatalf("expected key service unavailable, got %v", err)
	}
	if err := s2.Put(ctx, uid, pid, "B", []byte("v")); !errors.Is(err, secrets.ErrKeyServiceUnavailable) {
		t.Fatalf("expected key service unavailable on put, got %v", err)
	}
	// The warm cache still serves reads for the ttl.
	if _, err := s.DecryptForGuest(ctx, pid); err != nil {
		t.Fatalf("cached dek should still decrypt: %v", err)
	}
}

func TestDEKCacheLimitsKeyVaultCalls(t *testing.T) {
	pool := testdb.Open(t)
	fk := kv.New()
	s := secrets.New(pool, fk)
	ctx := context.Background()
	uid, pid := newProject(t, s, pool)
	for i := 0; i < 5; i++ {
		if err := s.Put(ctx, uid, pid, "K"+string(rune('A'+i)), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		if _, err := s.DecryptForGuest(ctx, pid); err != nil {
			t.Fatal(err)
		}
	}
	wraps, unwraps := fk.Counts()
	if wraps != 1 || unwraps != 0 {
		t.Fatalf("wraps=%d unwraps=%d; one DEK per user and no unwrap while cached", wraps, unwraps)
	}
}
