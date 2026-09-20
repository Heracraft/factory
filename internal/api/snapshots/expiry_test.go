package snapshots_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/snapshots"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
	"github.com/heracraft/repose/internal/fakes/blob"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

func project(t *testing.T, pool *db.Pool, destroyed bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	uid := store.NewID()
	pid := store.NewID()
	if _, err := pool.Exec(ctx, "insert into users (id, handle) values ($1, $2)", uid, "u"+uid.String()[24:]); err != nil {
		t.Fatal(err)
	}
	var d *time.Time
	st := "running"
	if destroyed {
		now := time.Now()
		d = &now
		st = "destroyed"
	}
	if _, err := pool.Exec(ctx, "insert into projects (id, user_id, name, slug, class, state, volume_bytes, destroyed_at) values ($1, $2, $3, $3, 'large', $4, 1, $5)", pid, uid, "p"+pid.String()[24:], st, d); err != nil {
		t.Fatal(err)
	}
	return pid
}

func snap(t *testing.T, pool *db.Pool, b *blob.Fake, pid uuid.UUID, age time.Duration, expires *time.Time) uuid.UUID {
	t.Helper()
	id := store.NewID()
	path := pid.String() + "/" + id.String() + ".img.zst"
	if _, err := pool.Exec(context.Background(), "insert into snapshots (id, project_id, blob_path, bytes, reason, taken_at, expires_at) values ($1, $2, $3, 1, 'scheduled', $4, $5)", id, pid, path, time.Now().Add(-age), expires); err != nil {
		t.Fatal(err)
	}
	b.Put(path, 1)
	return id
}

func TestExpiryRules(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	b := blob.New()
	e := snapshots.New(pool, b, metrics.NewNop(), slog.New(slog.NewTextHandler(os.Stderr, nil)))
	live := project(t, pool, false)
	old1 := snap(t, pool, b, live, 10*24*time.Hour, nil)
	old2 := snap(t, pool, b, live, 8*24*time.Hour, nil)
	fresh := snap(t, pool, b, live, 2*24*time.Hour, nil)
	// A long-stopped project keeps its only, old snapshot forever.
	lonely := project(t, pool, false)
	keep := snap(t, pool, b, lonely, 90*24*time.Hour, nil)
	// A destroyed project's snapshot goes at expires_at.
	gone := project(t, pool, true)
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(20 * 24 * time.Hour)
	expired := snap(t, pool, b, gone, 40*24*time.Hour, &past)
	notYet := snap(t, pool, b, gone, 40*24*time.Hour, &future)
	// A snapshot under a running restore is never touched.
	restoring := project(t, pool, false)
	held := snap(t, pool, b, restoring, 30*24*time.Hour, nil)
	_ = snap(t, pool, b, restoring, time.Hour, nil)
	opID := store.NewID()
	if _, err := pool.Exec(ctx, "insert into ops (id, project_id, kind, state, params) values ($1, $2, 'restore', 'running', '{}')", opID, restoring); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "update snapshots set restoring_op_id = $2 where id = $1", held, opID); err != nil {
		t.Fatal(err)
	}
	deleted, err := e.Once(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uuid.UUID]bool{old1: true, old2: true, expired: true}
	if len(deleted) != len(want) {
		t.Fatalf("deleted %v", deleted)
	}
	for _, d := range deleted {
		if !want[d] {
			t.Fatalf("deleted %s which should stay", d)
		}
	}
	for _, id := range []uuid.UUID{fresh, keep, notYet, held} {
		s, _ := store.GetSnapshot(ctx, pool, id)
		if s.DeletedAt != nil || !b.Exists(s.BlobPath) {
			t.Fatalf("snapshot %s should survive", id)
		}
	}
	for _, id := range []uuid.UUID{old1, old2, expired} {
		s, _ := store.GetSnapshot(ctx, pool, id)
		if s.DeletedAt == nil || b.Exists(s.BlobPath) {
			t.Fatalf("snapshot %s should be gone", id)
		}
	}
	// Blob failure leaves the row undeleted for the next run.
	late := snap(t, pool, b, live, 9*24*time.Hour, nil)
	b.Fail = blob.ErrDown
	if _, err := e.Once(ctx); err == nil {
		t.Fatal("expected a blob error")
	}
	s, _ := store.GetSnapshot(ctx, pool, late)
	if s.DeletedAt != nil {
		t.Fatal("row marked deleted although the blob delete failed")
	}
	b.Fail = nil
	if d, err := e.Once(ctx); err != nil || len(d) != 1 || d[0] != late {
		t.Fatalf("retry: %v %v", d, err)
	}
	e.UpdateAgeGauge(ctx)
}
