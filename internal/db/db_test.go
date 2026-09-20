package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

func TestMigrateUpDownUp(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	st, err := db.MigrateStatus(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Pending) != 0 || len(st.Applied) < 2 {
		t.Fatalf("expected all applied, got %+v", st)
	}
	down, err := db.MigrateDown(ctx, pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(down) != 1 || down[0] != st.Applied[len(st.Applied)-1] {
		t.Fatalf("down 1 reverted %v", down)
	}
	var n int
	if err := pool.QueryRow(ctx, "select count(*) from information_schema.tables where table_name = 'credit_ledger'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("credit_ledger still exists after down")
	}
	up, err := db.MigrateUp(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(up) != 1 {
		t.Fatalf("up reapplied %v", up)
	}
	st, err = db.MigrateStatus(ctx, pool)
	if err != nil || len(st.Pending) != 0 {
		t.Fatalf("after up: %+v %v", st, err)
	}
	// Every migration's down script reverts cleanly all the way to empty.
	all, err := db.MigrateDown(ctx, pool, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(st.Applied) {
		t.Fatalf("full down reverted %d of %d", len(all), len(st.Applied))
	}
	if _, err := db.MigrateUp(ctx, pool); err != nil {
		t.Fatal(err)
	}
}

func TestPartitions(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	now := time.Now()
	if err := db.EnsurePartitions(ctx, pool, now); err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, -5, 0)
	if err := db.EnsurePartitions(ctx, pool, old); err != nil {
		t.Fatal(err)
	}
	dropped, err := db.DropExpiredPartitions(ctx, pool, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(dropped) < 2 {
		t.Fatalf("expected the five-month-old partitions dropped, got %v", dropped)
	}
	if _, err := pool.Exec(ctx, "insert into meter_samples (ts, project_id, state, class) values ($1, '00000000-0000-7000-8000-000000000000', 'running', 'large')", now); err != nil {
		t.Fatalf("insert into current partition: %v", err)
	}
}

func TestAdvisoryLock(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	rel, ok, err := db.TryLock(ctx, pool, db.LockRollup)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	_, ok2, err := db.TryLock(ctx, pool, db.LockRollup)
	if err != nil || ok2 {
		t.Fatalf("second lock should fail: %v %v", ok2, err)
	}
	rel()
	rel3, ok3, err := db.TryLock(ctx, pool, db.LockRollup)
	if err != nil || !ok3 {
		t.Fatalf("relock after release: %v %v", ok3, err)
	}
	rel3()
}
