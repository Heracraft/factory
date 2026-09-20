package billing_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

// §9: "Reconciliation job and explain exist; a deliberate mismatch raises
// the alert and fixes nothing."
func TestReconcileReportsAndFixesNothing(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	a := seedAccount(t, pool, "large", start, 0)
	// The seeded customer id is what the fake keys its meter events on.
	r := billing.NewRollup(pool, st, metrics.NewNop(), quiet())
	fillRunning(t, pool, a, start, 6, "large", 40<<30, 0)
	for h := 0; h < 6; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	rec := billing.NewReconciler(pool, st, metrics.NewNop(), quiet())
	ms, err := rec.Reconcile(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 0 {
		t.Fatalf("a clean period reported %d mismatch(es): %+v", len(ms), ms)
	}

	// A deliberate mismatch: a row is charged more than was ever pushed.
	before := usageTotal(t, pool, a)
	if _, err := pool.Exec(ctx, "update usage_hours set cost_cents = cost_cents + 500, guest_cents = guest_cents + 500 where project_id = $1 and hour = $2", a.ProjectID, start); err != nil {
		t.Fatal(err)
	}
	ms, err = rec.Reconcile(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Fatalf("the mismatch was not reported: %+v", ms)
	}
	m := ms[0]
	if m.Diff() != 500 {
		t.Fatalf("difference %d cents, want 500", m.Diff())
	}
	if m.Handle != a.Handle || m.FirstProject != a.ProjectID {
		t.Fatalf("the report does not name the project: %+v", m)
	}
	t.Logf("mismatch: handle=%s period=%s ours=%d stripe=%d diff=%+d first=%s@%s",
		m.Handle, m.Period.Start.Format("2006-01-02"), m.OursCents, m.TheirCents, m.Diff(), m.FirstProject, m.FirstHour.Format(time.RFC3339))

	// Nothing was fixed: the rows are exactly as the operator left them,
	// and no ledger row was written to paper over the difference.
	if after := usageTotal(t, pool, a); after != before+500 {
		t.Fatalf("reconciliation changed usage_hours: %d then %d", before+500, after)
	}
	if n := ledgerRows(t, pool, a); n != 0 {
		t.Fatalf("reconciliation wrote %d ledger rows", n)
	}
	// Running it again reports the same thing rather than converging.
	again, err := rec.Reconcile(ctx, start)
	if err != nil || len(again) != 1 || again[0].Diff() != 500 {
		t.Fatalf("second run: %+v %v", again, err)
	}
}

// Without the meter ids there is nothing to compare against, and the job
// says so rather than reporting every account as a mismatch against zero.
func TestReconcileWithoutAReaderSaysSo(t *testing.T) {
	pool := testdb.Open(t)
	rec := billing.NewReconciler(pool, nil, metrics.NewNop(), quiet())
	if _, err := rec.Reconcile(context.Background(), time.Now()); !errors.Is(err, billing.ErrNoReader) {
		t.Fatalf("%v, want ErrNoReader", err)
	}
}

// §5.7: "explain prints every input and each step of 5.4 for one row, which
// is the tool for answering a support ticket."
func TestExplainShowsTheArithmetic(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	a := seedAccount(t, pool, "large", start, 20)
	r := newRollup(pool, billing.Disabled{})
	fillRunning(t, pool, a, start, 2, "large", 40<<30, 0)
	for h := 0; h < 2; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	ex, err := billing.Explain(ctx, pool, a.ProjectID, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if _, err := ex.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	t.Logf("explain output:\n%s", out)
	for _, want := range []string{
		a.Handle, a.ProjectID.String(), "price version " + billing.PriceVersion,
		"period", "class        large at 14 cents/hour", "running    3600 s from 60 running samples",
		"guest      14 * 3600 / 3600", "storage    40 GB * 10 cents", "egress     500 GB included",
		"cost       ", "credit     ", "billable   ", "stripe     not pushed yet",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("explain does not show %q", want)
		}
	}
	if ex.SampleMinutes != 60 || ex.RunningSeconds != 3600 {
		t.Errorf("explain read %d samples and %d seconds", ex.SampleMinutes, ex.RunningSeconds)
	}
	if ex.CreditCents == 0 {
		t.Error("explain does not show the credit that was applied")
	}

	// A gap hour says so, because that is the question a support ticket
	// about a low bill actually asks.
	if _, err := r.Hour(ctx, start.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	gap, err := billing.Explain(ctx, pool, a.ProjectID, start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var gb bytes.Buffer
	if _, err := gap.WriteTo(&gb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gb.String(), "GAP") {
		t.Errorf("a gap hour does not say so:\n%s", gb.String())
	}
}

func usageTotal(t *testing.T, pool *db.Pool, a account) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(context.Background(), "select coalesce(sum(cost_cents), 0) from usage_hours where project_id = $1", a.ProjectID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
