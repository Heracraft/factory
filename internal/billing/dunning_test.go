package billing_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/events"
	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

// stopRecorder stands in for the ops engine: it writes the same pending op
// row Enqueue would, so the test can read back the kind, the phases and
// the snapshot parameter.
type stopRecorder struct {
	calls []ops.NewOp
	kicks int
}

func (s *stopRecorder) Enqueue(ctx context.Context, q store.Querier, n ops.NewOp, allowQueue bool) (uuid.UUID, error) {
	s.calls = append(s.calls, n)
	id := store.NewID()
	params := map[string]any{"phases": n.Phases}
	for k, v := range n.Params {
		params[k] = v
	}
	_, err := q.Exec(ctx, "insert into ops (id, project_id, kind, state, params) values ($1, $2, $3, 'pending', $4)", id, n.ProjectID, n.Kind, params)
	return id, err
}

func (s *stopRecorder) Kick() { s.kicks++ }

// §9: "Past-due 3-day stop with snapshot and notification; invoice.paid
// reactivates without starting guests." The test clock is the Dunning job's
// Now, which is what the Stripe test clock stands in for in test mode.
func TestPastDueThreeDayStop(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := seedAccount(t, pool, "large", base(), 0)
	ev := events.New(pool, metrics.NewNop(), quiet())
	stop := &stopRecorder{}
	d := billing.NewDunning(pool, stop, ev, quiet(), true)

	// A failed payment two days ago: nothing happens yet.
	if _, err := pool.Exec(ctx, "update users set billing_status = 'past_due', past_due_since = now() - interval '2 days' where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	done, err := d.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 0 || len(stop.calls) != 0 {
		t.Fatalf("day 2 stopped %d account(s)", len(done))
	}
	if s := userString(t, pool, a, "billing_status"); s != "past_due" {
		t.Fatalf("status on day 2: %s", s)
	}

	// Day 4: the guests stop, with a snapshot, and the account is suspended.
	if _, err := pool.Exec(ctx, "update users set past_due_since = now() - interval '4 days' where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	done, err = d.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 || len(done[0].Projects) != 1 || done[0].Projects[0] != a.ProjectID {
		t.Fatalf("day 4: %+v", done)
	}
	if len(stop.calls) != 1 {
		t.Fatalf("%d stop commands", len(stop.calls))
	}
	call := stop.calls[0]
	if call.Kind != ops.KindStop {
		t.Fatalf("op kind %s", call.Kind)
	}
	if call.Params["snapshot"] != true {
		t.Fatalf("the stop did not ask for a snapshot: %+v", call.Params)
	}
	if call.Params["reason"] != "billing" {
		t.Fatalf("the stop reason is %v, want billing", call.Params["reason"])
	}
	if s := userString(t, pool, a, "billing_status"); s != "suspended" {
		t.Fatalf("status on day 4: %s", s)
	}
	if userTime(t, pool, a, "suspended_at") == nil {
		t.Fatal("suspended_at was not set; the 30-day retention starts there")
	}
	// Nothing is destroyed.
	if s := projectState(t, pool, a); s == "destroyed" {
		t.Fatal("the project was destroyed")
	}
	var destroyed *time.Time
	if err := pool.QueryRow(ctx, "select destroyed_at from projects where id = $1", a.ProjectID).Scan(&destroyed); err != nil {
		t.Fatal(err)
	}
	if destroyed != nil {
		t.Fatal("destroyed_at was set by the dunning job")
	}

	// The billing_stopped notification reached events and the outbox.
	var kind, summary string
	if err := pool.QueryRow(ctx, "select kind, summary from events where project_id = $1 order by ts desc limit 1", a.ProjectID).Scan(&kind, &summary); err != nil {
		t.Fatalf("no event was recorded: %v", err)
	}
	if kind != "billing_stopped" {
		t.Fatalf("event kind %s", kind)
	}
	t.Logf("event: kind=%s summary=%q", kind, summary)
	// The audit trail has the suspension.
	var actions int
	if err := pool.QueryRow(ctx, "select count(*) from audit_log where action = 'billing_suspend' and target = $1", a.Handle).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if actions != 1 {
		t.Fatalf("%d audit rows for the suspension", actions)
	}

	// Running again is a no-op: the account is suspended, not past due.
	before := len(stop.calls)
	if _, err := d.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(stop.calls) != before {
		t.Fatalf("a second run stopped the guests again")
	}

	// invoice.paid brings the account back to active and does NOT start the
	// guests (§5.6: "guests stay stopped until the user starts them").
	if _, err := pool.Exec(ctx, "update projects set state = 'stopped' where id = $1", a.ProjectID); err != nil {
		t.Fatal(err)
	}
	w := newHooks(t, pool)
	if err := deliver(t, w, "evt_paid", billing.TypeInvoicePaid, map[string]any{
		"id": "in_late", "customer": "cus_" + a.Handle, "total": 800, "status": "paid",
	}); err != nil {
		t.Fatal(err)
	}
	if s := userString(t, pool, a, "billing_status"); s != "active" {
		t.Fatalf("status after payment: %s", s)
	}
	if userTime(t, pool, a, "suspended_at") != nil {
		t.Fatal("suspended_at survived the payment")
	}
	if s := projectState(t, pool, a); s != "stopped" {
		t.Fatalf("invoice.paid started a guest: %s", s)
	}
	if len(stop.calls) != before {
		t.Fatal("invoice.paid enqueued an op")
	}
}

// §8: BILLING_ENFORCE=false keeps rolling up and pushing but stops nothing.
func TestBillingEnforceFalseStopsNothing(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	a := seedAccount(t, pool, "large", base(), 0)
	if _, err := pool.Exec(ctx, "update users set billing_status = 'past_due', past_due_since = now() - interval '9 days' where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	stop := &stopRecorder{}
	d := billing.NewDunning(pool, stop, events.New(pool, metrics.NewNop(), quiet()), quiet(), false)
	done, err := d.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stop.calls) != 0 {
		t.Fatalf("BILLING_ENFORCE=false stopped %d guest(s)", len(stop.calls))
	}
	if len(done) != 1 || len(done[0].Projects) != 0 {
		t.Fatalf("the run should report the account and stop nothing: %+v", done)
	}
	if s := userString(t, pool, a, "billing_status"); s != "past_due" {
		t.Fatalf("status with enforcement off: %s", s)
	}
	// And metering carries on regardless.
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	fillRunning(t, pool, a, start, 2, "large", 40<<30, 0)
	p := &recorder{}
	r := newRollup(pool, p)
	for h := 0; h < 2; h++ {
		if _, err := r.Hour(ctx, start.Add(time.Duration(h)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	guest, _, _, _, _ := sums(t, pool, a.ProjectID)
	if guest != 2*billing.HourLarge {
		t.Fatalf("metering stopped with enforcement off: %d cents", guest)
	}
	if len(p.rows) == 0 {
		t.Fatal("nothing was pushed to Stripe with enforcement off")
	}
}

// §8: flipping BILLING_ENFORCE is logged as an audit event, once per change
// rather than once per start.
func TestEnforcementFlipIsAudited(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	// The first observation records the value without an audit row: there
	// was nothing to change from.
	changed, err := billing.RecordEnforcement(ctx, pool, true, "api", quiet())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("the first observation should record the value")
	}
	if n := auditCount(t, pool, "billing_enforce"); n != 1 {
		t.Fatalf("%d audit rows after the first observation", n)
	}
	// Restarting with the same value writes nothing.
	if changed, err := billing.RecordEnforcement(ctx, pool, true, "api", quiet()); err != nil || changed {
		t.Fatalf("an unchanged value reported %v %v", changed, err)
	}
	if n := auditCount(t, pool, "billing_enforce"); n != 1 {
		t.Fatalf("%d audit rows after a restart with the same value", n)
	}
	// Flipping it writes one.
	if changed, err := billing.RecordEnforcement(ctx, pool, false, "api", quiet()); err != nil || !changed {
		t.Fatalf("the flip reported %v %v", changed, err)
	}
	if n := auditCount(t, pool, "billing_enforce"); n != 2 {
		t.Fatalf("%d audit rows after the flip", n)
	}
	var detail map[string]any
	if err := pool.QueryRow(ctx, "select detail from audit_log where action = 'billing_enforce' order by ts desc limit 1").Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if detail["from"] != "true" || detail["to"] != "false" {
		t.Fatalf("audit detail %v", detail)
	}
	t.Logf("audit_log: billing_enforce %v -> %v", detail["from"], detail["to"])
}

func auditCount(t *testing.T, pool *db.Pool, action string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "select count(*) from audit_log where action = $1", action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
