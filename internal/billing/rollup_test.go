package billing_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

// base is a fixed hour inside the current month, so the meter_samples
// partitions a test needs are the ones the migration already created plus
// whatever ensurePartitions adds.
func base() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// §9: "Hourly rollup produces the golden usage_hours rows for every case in
// §7, including exact storage sums and the egress threshold hour."
func TestRollupGoldenHours(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 2, 0))
	a := seedAccount(t, pool, "large", start, 0)
	r := newRollup(pool, billing.Disabled{})
	period := billing.PeriodFor(a.Anchor, start)

	// Hour 0: a full running hour, 40 GB, no egress.
	fillRunning(t, pool, a, start, 1, "large", 40<<30, 0)
	rows, err := r.Hour(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("hour 0 produced %d rows", len(rows))
	}
	got := rows[0]
	if got.GuestCents != 14 || got.RunningSeconds != 3600 || got.GBAlloc != 40 {
		t.Fatalf("full hour: %+v", got)
	}
	if !got.Period.Start.Equal(period.Start) || !got.Period.End.Equal(period.End) {
		t.Fatalf("hour 0 period [%s, %s), want [%s, %s)", got.Period.Start, got.Period.End, period.Start, period.End)
	}
	// Storage: 40 GB * 10 cents spread over the period's hours is under a
	// cent an hour, so the first hour is 0 and the remainder carries.
	if got.StorageCents != 0 || got.StorageRemainder == 0 {
		t.Fatalf("storage in hour 0: %+v", got)
	}

	// Hour 1: a partial hour, 20 running minutes. 14 * 1200 / 3600 = 4.67,
	// rounded half up to 5.
	h1 := start.Add(time.Hour)
	for m := 0; m < 20; m++ {
		sample(t, pool, a, h1.Add(time.Duration(m)*time.Minute), "running", "large", 40<<30, 0)
	}
	for m := 20; m < 60; m++ {
		sample(t, pool, a, h1.Add(time.Duration(m)*time.Minute), "stopped", "large", 40<<30, 0)
	}
	rows, err = r.Hour(ctx, h1)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].RunningSeconds != 1200 || rows[0].GuestCents != 5 {
		t.Fatalf("partial hour: %+v", rows[0])
	}

	// Hour 2: a gap. The project is running and no sample arrived, so the
	// minutes are under-billed and the row says so; nothing is estimated.
	h2 := start.Add(2 * time.Hour)
	rows, err = r.Hour(ctx, h2)
	if err != nil {
		t.Fatal(err)
	}
	if !rows[0].Gap || rows[0].RunningSeconds != 0 || rows[0].GuestCents != 0 {
		t.Fatalf("gap hour: %+v", rows[0])
	}

	// Hour 3: a class change. The guest comes back as xl, and from here the
	// period's cap is the xl cap.
	h3 := start.Add(3 * time.Hour)
	fillRunning(t, pool, a, h3, 1, "xl", 80<<30, 0)
	rows, err = r.Hour(ctx, h3)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Class != "xl" || rows[0].GuestCents != 28 || rows[0].CapClass != "xl" || rows[0].CapCents != billing.CapXL {
		t.Fatalf("class change hour: %+v", rows[0])
	}
	if rows[0].GBAlloc != 80 {
		t.Fatalf("disk after resize: %+v", rows[0])
	}

	// Hour 4: the egress threshold. 501 GB have already left this period,
	// so only the excess is charged.
	h4 := start.Add(4 * time.Hour)
	if _, err := pool.Exec(ctx, "update usage_hours set egress_bytes = $2 where project_id = $1 and hour = $3", a.ProjectID, int64(499)<<30, start); err != nil {
		t.Fatal(err)
	}
	fillRunning(t, pool, a, h4, 1, "xl", 80<<30, 3<<30)
	rows, err = r.Hour(ctx, h4)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].EgressCents != 2*billing.EgressPerGB {
		t.Fatalf("threshold hour charged %d cents: %+v", rows[0].EgressCents, rows[0])
	}
}

// §9: "Rollup is idempotent: run twice, zero diff."
func TestRollupIsIdempotent(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	// Enough credit that the period does not exhaust it, so the idempotency
	// check is not confused by the trial-to-active transition.
	a := seedAccount(t, pool, "large", start, 5000)
	p := &recorder{}
	r := newRollup(pool, p)
	fillRunning(t, pool, a, start, 6, "large", 40<<30, 1<<30)
	for h := 0; h < 6; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotUsage(t, pool)
	creditsBefore := ledgerRows(t, pool, a)
	pushesBefore := len(p.rows)
	for h := 0; h < 6; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if after := snapshotUsage(t, pool); after != before {
		t.Fatalf("usage_hours changed on the second run:\n%s\nwant\n%s", after, before)
	}
	if n := ledgerRows(t, pool, a); n != creditsBefore {
		t.Fatalf("the second run wrote %d extra ledger rows", n-creditsBefore)
	}
	if len(p.rows) != pushesBefore {
		t.Fatalf("a row with a Stripe record was pushed again: %d then %d", pushesBefore, len(p.rows))
	}
}

// §9: "Cap: a guest running 720 hours in a period is charged exactly the
// cap; 360 hours exactly half." Through the database, not just the
// arithmetic, so the period running-total query is covered too.
func TestRollupCapOverAPeriod(t *testing.T) {
	if testing.Short() {
		t.Skip("720 rollup hours")
	}
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 2, 0))
	a := seedAccount(t, pool, "large", start, 0)
	period := billing.PeriodFor(a.Anchor, start)
	hours := period.Hours()
	fillRunning(t, pool, a, start, hours, "large", 40<<30, 0)
	r := newRollup(pool, billing.Disabled{})
	// Due stops at the previous full hour, so Now is the hour after the
	// last one with samples.
	r.Now = func() time.Time { return start.Add(time.Duration(hours) * time.Hour) }
	if n, err := r.Due(ctx); err != nil || n != hours {
		t.Fatalf("due rolled %d of %d hours: %v", n, hours, err)
	}
	guest, storage, _, _, _ := sums(t, pool, a.ProjectID)
	if guest != billing.CapLarge {
		t.Fatalf("a guest running the whole period paid %d cents, want the cap %d", guest, billing.CapLarge)
	}
	// And the storage line sums to exactly 40 GB * 10 cents.
	if storage != 40*billing.StoragePerGBMonth {
		t.Fatalf("storage over the period: %d cents, want %d", storage, 40*billing.StoragePerGBMonth)
	}
	// Half the period is half the hours at the hourly rate, under the cap.
	pool2 := testdb.Open(t)
	ensurePartitions(t, pool2, start, start.AddDate(0, 2, 0))
	b := seedAccount(t, pool2, "large", start, 0)
	half := hours / 2
	fillRunning(t, pool2, b, start, half, "large", 40<<30, 0)
	r2 := newRollup(pool2, billing.Disabled{})
	r2.Now = func() time.Time { return start.Add(time.Duration(half) * time.Hour) }
	if _, err := r2.Due(ctx); err != nil {
		t.Fatal(err)
	}
	guest2, _, _, _, _ := sums(t, pool2, b.ProjectID)
	if guest2 != int64(half)*billing.HourLarge {
		t.Fatalf("half a period paid %d cents, want %d", guest2, int64(half)*billing.HourLarge)
	}
	if guest2 >= guest {
		t.Fatalf("half a period (%d) is not less than a whole one (%d)", guest2, guest)
	}
}

// §5.3: the hour's cost debits the trial credit first, and only the
// remainder reaches Stripe. §9: "Trial credit inserted at signup, debited
// before Stripe, balance computed from the ledger."
func TestTrialCreditIsDebitedBeforeStripe(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	// 30 cents of credit against a large guest: two full hours are free,
	// the third is half free.
	a := seedAccount(t, pool, "large", start, 30)
	p := &recorder{}
	r := newRollup(pool, p)
	fillRunning(t, pool, a, start, 4, "large", 40<<30, 0)
	for h := 0; h < 4; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	guest, _, _, cost, credit := sums(t, pool, a.ProjectID)
	if credit != 30 {
		t.Fatalf("credit applied %d cents, want the whole 30", credit)
	}
	if guest != 4*billing.HourLarge {
		t.Fatalf("guest part %d cents, want %d", guest, 4*billing.HourLarge)
	}
	balance, err := billing.Balance(ctx, pool, a.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("balance after the credit ran out: %d", balance)
	}
	// The projection on users agrees with the ledger.
	var projected int64
	if err := pool.QueryRow(ctx, "select trial_credit_cents from users where id = $1", a.UserID).Scan(&projected); err != nil {
		t.Fatal(err)
	}
	if projected != balance {
		t.Fatalf("users.trial_credit_cents is %d, the ledger says %d", projected, balance)
	}
	// Stripe only ever saw the remainder.
	if want := cost - credit; p.total() != want {
		t.Fatalf("pushed %d cents to Stripe, want %d", p.total(), want)
	}
	// §5.3: with the credit gone and a card on file the account is `active`,
	// so the next hour is billed instead of the card gate refusing the start
	// with trial_depleted.
	var status string
	if err := pool.QueryRow(ctx, "select billing_status from users where id = $1", a.UserID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("billing_status after the trial ran out: %s, want active", status)
	}
	// The first two hours were entirely covered, so they were never pushed.
	var unpushedFree int
	if err := pool.QueryRow(ctx, "select count(*) from usage_hours where project_id = $1 and cost_cents = credit_cents and stripe_usage_record_id is null", a.ProjectID).Scan(&unpushedFree); err != nil {
		t.Fatal(err)
	}
	if unpushedFree == 0 {
		t.Fatal("a fully credited hour should not be pushed at all")
	}
}

// §6: "trial balance negative (race): impossible by construction: debit
// inside the same transaction as the usage_hours insert with select ... for
// update on the user row." Two rollup runners over the same hours.
func TestCreditLedgerUnderConcurrency(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	a := seedAccount(t, pool, "xl", start, 100)
	fillRunning(t, pool, a, start, 8, "xl", 40<<30, 0)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := newRollup(pool, billing.Disabled{})
			for h := 0; h < 8; h++ {
				if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		// A serialisation failure is a legitimate outcome of two runners;
		// a negative balance is not.
		t.Logf("runner: %v", err)
	}
	balance, err := billing.Balance(ctx, pool, a.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if balance < 0 {
		t.Fatalf("the ledger went negative: %d cents", balance)
	}
	_, _, _, cost, credit := sums(t, pool, a.ProjectID)
	if credit > 100 {
		t.Fatalf("debited %d cents of a 100 cent credit", credit)
	}
	if credit+balance != 100 {
		t.Fatalf("%d debited plus %d left is not the 100 cents granted", credit, balance)
	}
	if cost < credit {
		t.Fatalf("debited %d cents against a cost of %d", credit, cost)
	}
}

// DECISIONS I-16: an exempt account accrues usage_hours so the meters are
// exercised, and is never pushed to Stripe.
func TestExemptAccountIsNeverPushed(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	a := seedAccount(t, pool, "large", start, 0)
	if _, err := pool.Exec(ctx, "update users set billing_status = 'exempt' where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	p := &recorder{}
	r := newRollup(pool, p)
	fillRunning(t, pool, a, start, 3, "large", 40<<30, 0)
	for h := 0; h < 3; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	guest, _, _, cost, _ := sums(t, pool, a.ProjectID)
	if guest != 3*billing.HourLarge || cost < guest {
		t.Fatalf("an exempt account should still accrue: guest %d, cost %d", guest, cost)
	}
	if len(p.rows) != 0 {
		t.Fatalf("an exempt account was pushed to Stripe: %+v", p.rows)
	}
	var marked int
	if err := pool.QueryRow(ctx, "select count(*) from usage_hours where project_id = $1 and stripe_usage_record_id = 'exempt'", a.ProjectID).Scan(&marked); err != nil {
		t.Fatal(err)
	}
	if marked != 3 {
		t.Fatalf("%d of 3 rows marked exempt", marked)
	}
}

// §6: "Stripe unreachable during the hourly push: usage_hours row exists
// with null record id; the next hourly run retries every null row."
func TestPushRetriesAfterAStripeFailure(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	a := seedAccount(t, pool, "large", start, 0)
	p := &recorder{fail: errors.New("stripe: connection refused")}
	r := newRollup(pool, p)
	fillRunning(t, pool, a, start, 3, "large", 40<<30, 0)
	for h := 0; h < 3; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	var pending int
	if err := pool.QueryRow(ctx, "select count(*) from usage_hours where project_id = $1 and stripe_usage_record_id is null", a.ProjectID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 3 {
		t.Fatalf("%d of 3 rows left pending after Stripe refused", pending)
	}
	// Stripe comes back; the next run pushes the backlog.
	p.fail = nil
	if err := r.Push(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "select count(*) from usage_hours where project_id = $1 and stripe_usage_record_id is null", a.ProjectID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("%d rows still pending after Stripe came back", pending)
	}
	if len(p.rows) != 3 {
		t.Fatalf("pushed %d rows, want 3", len(p.rows))
	}
}

func snapshotUsage(t *testing.T, pool *db.Pool) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `select project_id::text, hour, class, running_seconds, gb_alloc, egress_bytes,
		cost_cents, guest_cents, storage_cents, egress_cents, storage_remainder, credit_cents, gap
		from usage_hours order by project_id, hour`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := ""
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatal(err)
		}
		out += formatRow(vals) + "\n"
	}
	return out
}

func formatRow(vals []any) string {
	s := ""
	for i, v := range vals {
		if i > 0 {
			s += " "
		}
		if ts, ok := v.(time.Time); ok {
			s += ts.UTC().Format(time.RFC3339)
			continue
		}
		s += fmt.Sprint(v)
	}
	return s
}

func ledgerRows(t *testing.T, pool *db.Pool, a account) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "select count(*) from credit_ledger where user_id = $1", a.UserID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
