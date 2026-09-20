package billing_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

func TestMain(m *testing.M) { os.Exit(testdb.Run(m)) }

// quiet is a logger that keeps the test output readable; set BILLING_TEST_LOG
// to see what the rollup and the webhooks say.
func quiet() *slog.Logger {
	if os.Getenv("BILLING_TEST_LOG") != "" {
		return slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// account is a seeded user with one project.
type account struct {
	UserID    uuid.UUID
	Handle    string
	ProjectID uuid.UUID
	GuestID   uuid.UUID
	Anchor    time.Time
}

// seedAccount inserts a user anchored at anchor, with the trial credit in
// the ledger the way auth.Provisioner does it, and one project.
func seedAccount(t *testing.T, pool *db.Pool, class string, anchor time.Time, credit int64) account {
	t.Helper()
	ctx := context.Background()
	a := account{UserID: store.NewID(), ProjectID: store.NewID(), GuestID: store.NewID(), Anchor: anchor.UTC()}
	a.Handle = "u" + a.UserID.String()[24:]
	if _, err := pool.Exec(ctx, `insert into users (id, handle, email, billing_status, has_card, trial_credit_cents, stripe_customer_id, created_at, billing_anchor)
		values ($1, $2, $3, 'trial', true, 0, $4, $5, $5)`, a.UserID, a.Handle, a.Handle+"@example.test", "cus_"+a.Handle, a.Anchor); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if credit != 0 {
		if _, err := billing.Credit(ctx, pool, a.UserID, credit, billing.ReasonTrial, ""); err != nil {
			t.Fatalf("seed credit: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `insert into projects (id, user_id, name, slug, class, state, volume_bytes, guest_id, created_at)
		values ($1, $2, 'todo', $3, $4, 'running', $5, $6, $7)`,
		a.ProjectID, a.UserID, "s"+a.ProjectID.String()[24:], class, int64(40)<<30, a.GuestID, a.Anchor); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return a
}

// sample writes one meter_samples minute.
func sample(t *testing.T, pool *db.Pool, a account, ts time.Time, state, class string, diskAlloc, netTx int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `insert into meter_samples (ts, project_id, host_id, state, class, disk_alloc, net_tx, guestd_ok)
		values ($1, $2, $3, $4, $5, $6, $7, true) on conflict do nothing`,
		ts.UTC(), a.ProjectID, uuid.Nil, state, class, diskAlloc, netTx); err != nil {
		t.Fatalf("sample at %s: %v", ts, err)
	}
}

// recorder is a UsagePusher that remembers what it was given.
type recorder struct {
	rows []billing.UsageRow
	fail error
}

func (p *recorder) PushUsage(_ context.Context, r billing.UsageRow) (string, error) {
	if p.fail != nil {
		return "", p.fail
	}
	p.rows = append(p.rows, r)
	return "usage:" + r.ProjectID + ":" + r.Hour.Format(time.RFC3339), nil
}

func (p *recorder) total() int64 {
	var n int64
	for _, r := range p.rows {
		n += r.Billable()
	}
	return n
}

func newRollup(pool *db.Pool, p billing.UsagePusher) *billing.Rollup {
	return billing.NewRollup(pool, p, metrics.NewNop(), quiet())
}

// sums reads the period totals of a project straight from usage_hours.
func sums(t *testing.T, pool *db.Pool, projectID uuid.UUID) (guest, storage, egress, cost, credit int64) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `select coalesce(sum(guest_cents),0), coalesce(sum(storage_cents),0),
		coalesce(sum(egress_cents),0), coalesce(sum(cost_cents),0), coalesce(sum(credit_cents),0)
		from usage_hours where project_id = $1`, projectID).Scan(&guest, &storage, &egress, &cost, &credit)
	if err != nil {
		t.Fatalf("sum usage_hours: %v", err)
	}
	return
}

// ensurePartitions creates the monthly meter_samples partitions the test's
// span needs; testdb's template only has the ones the api made at start.
func ensurePartitions(t *testing.T, pool *db.Pool, from, to time.Time) {
	t.Helper()
	ctx := context.Background()
	m := time.Date(from.UTC().Year(), from.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	for ; !m.After(to.UTC()); m = m.AddDate(0, 1, 0) {
		if err := db.EnsurePartitions(ctx, pool, m); err != nil {
			t.Fatalf("partitions for %s: %v", m.Format("2006-01"), err)
		}
	}
}

// fillRunning writes `hours * 60` running minutes in one statement, with
// the egress spread evenly across them. Inserting them one at a time makes
// a 720-hour period test take minutes.
func fillRunning(t *testing.T, pool *db.Pool, a account, start time.Time, hours int, class string, diskAlloc, egressTotal int64) {
	t.Helper()
	minutes := int64(hours) * 60
	per := int64(0)
	if minutes > 0 {
		per = egressTotal / minutes
	}
	_, err := pool.Exec(context.Background(), `insert into meter_samples (ts, project_id, host_id, state, class, disk_alloc, net_tx, guestd_ok)
		select $1::timestamptz + (g || ' minutes')::interval, $2, $3, 'running', $4, $5, $6, true
		from generate_series(0, $7::bigint - 1) g on conflict do nothing`,
		start.UTC(), a.ProjectID, uuid.Nil, class, diskAlloc, per, minutes)
	if err != nil {
		t.Fatalf("fill %d running hours: %v", hours, err)
	}
}
