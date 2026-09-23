package billing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v83"

	"github.com/heracraft/repose/internal/api/events"
	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

// The M4 gate against Stripe itself (docs/MILESTONES.md M4, 09-billing.md §7
// "Stripe test mode, in CI with a test key", docs/ops/M4-GATE.md). It runs
// only with REPOSE_STRIPE_TEST_KEY=sk_test_... and is otherwise skipped:
//
//	REPOSE_STRIPE_TEST_KEY=sk_test_... TMPDIR=/tmp/rt go test -count=1 -v -timeout 60m \
//	    -run TestStripeTestModeM4Gate ./internal/billing/
//
// No 100 real hours are waited for. Each scenario puts one customer on a
// Stripe test clock frozen 33 days in the past, subscribes it through the
// api's own OnCardAttached, writes the fixed usage pattern into
// meter_samples (one large guest, 100 running hours, 40 GB, 10 GB egress),
// rolls up every hour of the billing period with the real rollup, which
// pushes real meter events, and then advances the clock across the period
// end so Stripe drafts, finalises and charges the invoice. 33 days keeps
// every event timestamp inside Stripe's 35-day window of real time, and the
// clock stands a minute before the period end while they are pushed (I-185).
//
// "paid" charges pm_card_visa and asserts the invoice equals the
// usage_hours rows to the cent, the trial credit depleted into `active`,
// and the real invoice.paid event raised the limits. "failed" charges the
// 4000000000000341 card (attaches, then declines), feeds the real
// invoice.payment_failed event to the webhook handler, runs the dunning
// job on day 2 (nothing) and day 3 plus an hour (stop with snapshot,
// billing_stopped, suspended), then pays with a good card and feeds
// invoice.paid (active, guests left stopped).
//
// REPOSE_STRIPE_KEEP=1 keeps the test clocks, and so the customers and
// invoices, for screenshots; Stripe deletes them itself after 30 days.

const m4Secret = "whsec_m4_gate_local"

func TestStripeTestModeM4Gate(t *testing.T) {
	key := os.Getenv("REPOSE_STRIPE_TEST_KEY")
	if key == "" {
		t.Skip("REPOSE_STRIPE_TEST_KEY is not set; docs/ops/M4-GATE.md")
	}
	if live, err := billing.KeyMode(key); err != nil || live {
		t.Fatalf("REPOSE_STRIPE_TEST_KEY must be a test-mode key: live=%v %v", live, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Minute)
	// Cleanup, not defer: the parallel subtests run after this body returns.
	t.Cleanup(cancel)
	boot, err := billing.Bootstrap(ctx, key, billing.BootstrapOptions{Out: testLog{t}})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	cfg := boot.Config(m4Secret)
	cfg.PortalReturnURL = "https://repose.herakraft.co/billing"
	c := stripe.NewClient(key)
	started := time.Now().UTC()
	t0 := started.Add(-33 * 24 * time.Hour).Truncate(time.Hour)

	t.Run("paid", func(t *testing.T) {
		t.Parallel()
		g := newGate(ctx, t, c, cfg, t0, "pm_card_visa", started)
		inv := g.invoiceAfterPeriodEnd()
		if inv.Status != stripe.InvoiceStatusPaid {
			t.Fatalf("invoice %s is %s, want paid", inv.ID, inv.Status)
		}
		g.checkInvoiceMatchesUsage(inv)
		// Trial credit depleted into active in the same transaction as the
		// debit that spent it, and the next hours went to Stripe (§5.3).
		if s := userString(t, g.pool, g.a, "billing_status"); s != "active" {
			t.Fatalf("after the credit was spent the account is %s, want active", s)
		}
		if bal, _ := billing.Balance(ctx, g.pool, g.a.UserID); bal != 0 {
			t.Fatalf("balance %d after depletion", bal)
		}
		g.logFirstBilledHour()
		// The real invoice.paid event raises the limits (§5.8).
		if err := g.deliverStripeEvent("invoice.paid", inv.ID); err != nil {
			t.Fatal(err)
		}
		var pl, xl int
		if err := g.pool.QueryRow(ctx, "select project_limit, xl_limit from users where id = $1", g.a.UserID).Scan(&pl, &xl); err != nil {
			t.Fatal(err)
		}
		if pl != billing.ProjectLimitPaid || xl != billing.XLLimitPaid {
			t.Fatalf("limits %d/%d after the first paid invoice", pl, xl)
		}
		t.Logf("M4 paid: limits after invoice.paid %d/%d", pl, xl)
	})

	t.Run("failed", func(t *testing.T) {
		t.Parallel()
		g := newGate(ctx, t, c, cfg, t0, "pm_card_chargeCustomerFail", started)
		inv := g.invoiceAfterPeriodEnd()
		if inv.Status == stripe.InvoiceStatusPaid {
			t.Fatalf("invoice %s was paid with the declining card", inv.ID)
		}
		g.checkInvoiceMatchesUsage(inv)
		if err := g.deliverStripeEvent("invoice.payment_failed", inv.ID); err != nil {
			t.Fatal(err)
		}
		if s := userString(t, g.pool, g.a, "billing_status"); s != "past_due" {
			t.Fatalf("after invoice.payment_failed the account is %s", s)
		}
		// The guest is running when the clock runs out.
		if _, err := g.pool.Exec(ctx, "update projects set state = 'running' where id = $1", g.a.ProjectID); err != nil {
			t.Fatal(err)
		}
		stop := &stopRecorder{}
		d := billing.NewDunning(g.pool, stop, events.New(g.pool, metrics.NewNop(), quiet()), quiet(), true)
		now := time.Now()
		d.Now = func() time.Time { return now.Add(2 * 24 * time.Hour) }
		if done, err := d.Run(ctx); err != nil || len(done) != 0 {
			t.Fatalf("day 2: %v %v", done, err)
		}
		d.Now = func() time.Time { return now.Add(3*24*time.Hour + time.Hour) }
		done, err := d.Run(ctx)
		if err != nil || len(done) != 1 || len(stop.calls) != 1 {
			t.Fatalf("day 3: %+v %v (%d stops)", done, err, len(stop.calls))
		}
		call := stop.calls[0]
		if call.Kind != ops.KindStop || call.Params["snapshot"] != true || call.Params["reason"] != "billing" {
			t.Fatalf("the stop was %+v", call)
		}
		var kind string
		if err := g.pool.QueryRow(ctx, "select kind from events where project_id = $1 order by ts desc limit 1", g.a.ProjectID).Scan(&kind); err != nil || kind != "billing_stopped" {
			t.Fatalf("notification: %q %v", kind, err)
		}
		if s := userString(t, g.pool, g.a, "billing_status"); s != "suspended" {
			t.Fatalf("after day 3 the account is %s", s)
		}
		t.Logf("M4 failed: invoice %s %s after %d attempt(s); day 3 + 1h: stop op with snapshot, reason billing, event %s, account suspended",
			inv.ID, inv.Status, inv.AttemptCount, kind)

		// The user fixes the card and pays; invoice.paid reactivates and
		// starts nothing.
		if _, err := g.pool.Exec(ctx, "update projects set state = 'stopped' where id = $1", g.a.ProjectID); err != nil {
			t.Fatal(err)
		}
		good := g.attach("pm_card_visa")
		if _, err := c.V1Invoices.Pay(ctx, inv.ID, &stripe.InvoicePayParams{PaymentMethod: stripe.String(good)}); err != nil {
			t.Fatalf("pay %s with a good card: %v", inv.ID, err)
		}
		if err := g.deliverStripeEvent("invoice.paid", inv.ID); err != nil {
			t.Fatal(err)
		}
		if s := userString(t, g.pool, g.a, "billing_status"); s != "active" {
			t.Fatalf("after invoice.paid the account is %s", s)
		}
		if s := projectState(t, g.pool, g.a); s != "stopped" || len(stop.calls) != 1 {
			t.Fatalf("invoice.paid touched the guest: %s, %d ops", s, len(stop.calls))
		}
		t.Logf("M4 failed: paid with a good card, invoice.paid -> active, guest left %s", projectState(t, g.pool, g.a))
	})
}

type testLog struct{ t *testing.T }

func (l testLog) Write(p []byte) (int, error) {
	l.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// gate is one customer on one test clock.
type gate struct {
	ctx     context.Context
	t       *testing.T
	c       *stripe.Client
	st      *billing.Stripe
	pool    *db.Pool
	a       account
	clock   string
	sub     string
	period  billing.Period
	since   time.Time
	pmToken string
}

func newGate(ctx context.Context, t *testing.T, c *stripe.Client, cfg billing.Config, t0 time.Time, pmToken string, since time.Time) *gate {
	t.Helper()
	g := &gate{ctx: ctx, t: t, c: c, pool: testdb.Open(t), since: since, pmToken: pmToken}
	st, err := billing.NewStripe(cfg, g.pool, quiet())
	if err != nil {
		t.Fatal(err)
	}
	st.Now = func() time.Time { return t0 }
	g.st = st
	clock, err := c.V1TestHelpersTestClocks.Create(ctx, &stripe.TestHelpersTestClockCreateParams{
		FrozenTime: stripe.Int64(t0.Unix()), Name: stripe.String(fmt.Sprintf("repose M4 gate %s %s", t.Name(), since.Format("0102T1504"))),
	})
	if err != nil {
		t.Fatalf("create the test clock: %v", err)
	}
	g.clock = clock.ID
	if os.Getenv("REPOSE_STRIPE_KEEP") == "" {
		t.Cleanup(func() { _, _ = c.V1TestHelpersTestClocks.Delete(context.Background(), clock.ID, nil) })
	}

	ensurePartitions(t, g.pool, t0.AddDate(0, -1, 0), t0.AddDate(0, 2, 0))
	g.a = seedAccount(t, g.pool, "large", t0, fixtureCreditCents)
	cus, err := c.V1Customers.Create(ctx, &stripe.CustomerCreateParams{
		TestClock: stripe.String(clock.ID), Email: stripe.String(g.a.Handle + "@example.test"),
		Metadata: map[string]string{"user_id": g.a.UserID.String(), "handle": g.a.Handle, "m4_gate": t.Name()},
		// An address Stripe Tax can locate, in case it is active.
		Address: &stripe.AddressParams{Line1: stripe.String("354 Oyster Point Blvd"), City: stripe.String("South San Francisco"),
			State: stripe.String("CA"), PostalCode: stripe.String("94080"), Country: stripe.String("US")},
	})
	if err != nil {
		t.Fatalf("create the customer: %v", err)
	}
	if _, err := g.pool.Exec(ctx, "update users set stripe_customer_id = $2 where id = $1", g.a.UserID, cus.ID); err != nil {
		t.Fatal(err)
	}
	// The api's own path from here: the card becomes the default and the
	// subscription is created at the clock's time, anchoring the period.
	if err := st.OnCardAttached(ctx, g.a.UserID, g.attach(pmToken)); err != nil {
		t.Fatalf("OnCardAttached: %v", err)
	}
	var anchor time.Time
	var sub string
	if err := g.pool.QueryRow(ctx, "select billing_anchor, stripe_subscription_id from users where id = $1", g.a.UserID).Scan(&anchor, &sub); err != nil {
		t.Fatal(err)
	}
	g.sub = sub
	g.period = billing.PeriodFor(anchor, anchor)
	t.Logf("M4 %s: clock %s at %s, customer %s, subscription %s, period %s to %s (%d h)", t.Name(), clock.ID, t0.Format(time.RFC3339),
		cus.ID, sub, g.period.Start.Format(time.RFC3339), g.period.End.Format(time.RFC3339), g.period.Hours())

	// The fixed pattern. The guest is stopped outside its 100 hours; the
	// 40 GB volume exists the whole period.
	if _, err := g.pool.Exec(ctx, "update projects set state = 'stopped' where id = $1", g.a.ProjectID); err != nil {
		t.Fatal(err)
	}
	fillRunning(t, g.pool, g.a, g.period.Start, 100, "large", 40<<30, 10<<30)

	// The clock goes to a minute before the period ends, so no event is
	// more than a minute in the clock's future (the last hour's is stamped
	// at End-1s, I-179; Stripe allows five) or at all in real time's; then
	// every hour is rolled up and pushed by the real rollup.
	g.advance(g.period.End.Add(-time.Minute))
	r := billing.NewRollup(g.pool, st, metrics.NewNop(), quiet())
	r.Now = func() time.Time { return g.period.End.Add(5 * time.Minute) }
	for h := g.period.Start; h.Before(g.period.End); h = h.Add(time.Hour) {
		if _, err := r.Hour(ctx, h); err != nil {
			t.Fatalf("rollup %s: %v", h, err)
		}
	}
	var unpushed int
	if err := g.pool.QueryRow(ctx, "select count(*) from usage_hours where stripe_usage_record_id is null and cost_cents > credit_cents").Scan(&unpushed); err != nil {
		t.Fatal(err)
	}
	if unpushed != 0 {
		t.Fatalf("%d rows were not pushed to Stripe", unpushed)
	}
	g.waitForMeters()
	return g
}

func (g *gate) attach(token string) string {
	g.t.Helper()
	var customer string
	if err := g.pool.QueryRow(g.ctx, "select stripe_customer_id from users where id = $1", g.a.UserID).Scan(&customer); err != nil {
		g.t.Fatal(err)
	}
	pm, err := g.c.V1PaymentMethods.Attach(g.ctx, token, &stripe.PaymentMethodAttachParams{Customer: stripe.String(customer)})
	if err != nil {
		g.t.Fatalf("attach %s: %v", token, err)
	}
	return pm.ID
}

func (g *gate) advance(to time.Time) {
	g.t.Helper()
	if _, err := g.c.V1TestHelpersTestClocks.Advance(g.ctx, g.clock, &stripe.TestHelpersTestClockAdvanceParams{FrozenTime: stripe.Int64(to.Unix())}); err != nil {
		g.t.Fatalf("advance the clock to %s: %v", to, err)
	}
	for i := 0; ; i++ {
		ck, err := g.c.V1TestHelpersTestClocks.Retrieve(g.ctx, g.clock, nil)
		if err == nil && ck.Status == stripe.TestHelpersTestClockStatusReady {
			return
		}
		if i > 150 || g.ctx.Err() != nil {
			g.t.Fatalf("the clock did not reach %s: %v", to, err)
		}
		time.Sleep(2 * time.Second)
	}
}

// expected is what usage_hours says Stripe should bill, per part, after
// the credit: the same split the push makes.
func (g *gate) expected() (compute, storage, egress int64) {
	g.t.Helper()
	rows, err := g.pool.Query(g.ctx, "select guest_cents, storage_cents, egress_cents, credit_cents from usage_hours where project_id = $1 and period_start = $2", g.a.ProjectID, g.period.Start)
	if err != nil {
		g.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var gu, s, e, c int64
		if err := rows.Scan(&gu, &s, &e, &c); err != nil {
			g.t.Fatal(err)
		}
		for _, p := range []*int64{&gu, &s, &e} {
			d := min(c, *p)
			*p -= d
			c -= d
		}
		compute, storage, egress = compute+gu, storage+s, egress+e
	}
	return
}

// waitForMeters waits until Stripe's meter summaries show every cent
// pushed, which is the reconciliation of §5.7 against the real account.
// Stripe aggregates asynchronously, so this polls; a summary that never
// catches up is logged, and the invoice check that follows is the
// authority.
func (g *gate) waitForMeters() {
	c, s, e := g.expected()
	for i := 0; i < 60; i++ {
		var customer string
		_ = g.pool.QueryRow(g.ctx, "select stripe_customer_id from users where id = $1", g.a.UserID).Scan(&customer)
		sum, err := g.st.PeriodSummary(g.ctx, customer, g.period.Start, g.period.End)
		if err == nil && sum.GuestCents == c && sum.StorageCents == s && sum.EgressCents == e {
			rec := billing.NewReconciler(g.pool, g.st, metrics.NewNop(), quiet())
			ms, err := rec.Reconcile(g.ctx, g.period.Start.Add(time.Hour))
			if err != nil {
				g.t.Fatalf("reconcile: %v", err)
			}
			if len(ms) != 0 {
				g.t.Fatalf("reconcile found %+v", ms)
			}
			g.t.Logf("M4 %s: meter summaries compute %d + storage %d + egress %d = %d; reconcile: no differences", g.t.Name(), c, s, e, c+s+e)
			return
		}
		time.Sleep(10 * time.Second)
	}
	g.t.Logf("M4 %s: meter summaries did not reach %d/%d/%d within 10 minutes; relying on the invoice", g.t.Name(), c, s, e)
}

// invoiceAfterPeriodEnd advances the clock past the period end, waits for
// the cycle invoice, then an hour and more for Stripe to finalise it and
// try the card.
func (g *gate) invoiceAfterPeriodEnd() *stripe.Invoice {
	g.t.Helper()
	g.advance(g.period.End.Add(time.Minute))
	var id string
	for i := 0; i < 30 && id == ""; i++ {
		for inv, err := range g.c.V1Invoices.List(g.ctx, &stripe.InvoiceListParams{Subscription: stripe.String(g.sub)}) {
			if err != nil {
				g.t.Fatalf("list invoices: %v", err)
			}
			if inv.BillingReason == stripe.InvoiceBillingReasonSubscriptionCycle {
				id = inv.ID
			}
		}
		if id == "" {
			time.Sleep(5 * time.Second)
		}
	}
	if id == "" {
		g.t.Fatal("no subscription_cycle invoice after the period end")
	}
	g.advance(g.period.End.Add(2 * time.Hour))
	var inv *stripe.Invoice
	for i := 0; i < 30; i++ {
		var err error
		inv, err = g.c.V1Invoices.Retrieve(g.ctx, id, nil)
		if err != nil {
			g.t.Fatalf("retrieve %s: %v", id, err)
		}
		if inv.Status != stripe.InvoiceStatusDraft && (inv.Status == stripe.InvoiceStatusPaid || inv.AttemptCount > 0) {
			break
		}
		time.Sleep(5 * time.Second)
	}
	return inv
}

// checkInvoiceMatchesUsage is the gate's sentence: the Stripe invoice
// matches the usage rows to the cent.
func (g *gate) checkInvoiceMatchesUsage(inv *stripe.Invoice) {
	g.t.Helper()
	cfg := g.st.Config()
	lines := map[string]int64{}
	for _, l := range inv.Lines.Data {
		if l.Pricing != nil && l.Pricing.PriceDetails != nil {
			lines[l.Pricing.PriceDetails.Price] += l.Amount
		}
	}
	c, s, e := g.expected()
	guest, storage, egress, cost, credit := sums(g.t, g.pool, g.a.ProjectID)
	if guest != 1400 || storage != 400 || egress != 0 || cost != 1800 || credit != fixtureCreditCents {
		g.t.Fatalf("usage_hours: compute %d storage %d egress %d total %d credit %d; want 1400/400/0/1800/1000", guest, storage, egress, cost, credit)
	}
	if lines[cfg.PriceCompute] != c || lines[cfg.PriceStorage] != s || lines[cfg.PriceEgress] != e || inv.Subtotal != c+s+e || inv.Subtotal != 800 {
		g.t.Fatalf("invoice %s lines compute %d storage %d egress %d subtotal %d; usage_hours says %d/%d/%d = %d",
			inv.ID, lines[cfg.PriceCompute], lines[cfg.PriceStorage], lines[cfg.PriceEgress], inv.Subtotal, c, s, e, c+s+e)
	}
	g.t.Logf("M4 %s: usage_hours compute %d + storage %d + egress %d = %d less trial credit %d = %d; invoice %s (%s) lines compute %d + storage %d + egress %d = subtotal %d, tax %d, total %d, status %s",
		g.t.Name(), guest, storage, egress, cost, credit, cost-credit, inv.ID, inv.Number, lines[cfg.PriceCompute], lines[cfg.PriceStorage],
		lines[cfg.PriceEgress], inv.Subtotal, inv.Total-inv.TotalExcludingTax, inv.Total, inv.Status)
	if inv.HostedInvoiceURL != "" {
		g.t.Logf("M4 %s: hosted invoice %s", g.t.Name(), inv.HostedInvoiceURL)
	}
}

// logFirstBilledHour prints the hour the credit ran out, for the trial
// depletion evidence.
func (g *gate) logFirstBilledHour() {
	var hour time.Time
	var cost, credit int64
	err := g.pool.QueryRow(g.ctx, `select hour, cost_cents, credit_cents from usage_hours where project_id = $1 and cost_cents > credit_cents order by hour limit 1`,
		g.a.ProjectID).Scan(&hour, &cost, &credit)
	if err == nil {
		g.t.Logf("M4 %s: trial credit ran out at %s (cost %d, credit %d, %d pushed); status active, balance 0", g.t.Name(), hour.Format(time.RFC3339), cost, credit, cost-credit)
	}
}

// deliverStripeEvent finds the event Stripe raised for the invoice and
// hands its object, signed, to the api's webhook handler: the handler is
// tested against Stripe's real payload rather than a hand-written one.
func (g *gate) deliverStripeEvent(kind, invoiceID string) error {
	g.t.Helper()
	for i := 0; i < 36; i++ {
		// No created filter: an event raised by a test clock may carry the
		// clock's time rather than today's. The newest 200 of the type are
		// plenty for one run.
		params := &stripe.EventListParams{Type: stripe.String(kind)}
		seen := 0
		for ev, err := range g.c.V1Events.List(g.ctx, params) {
			if err != nil {
				return err
			}
			if seen++; seen > 200 {
				break
			}
			var obj map[string]any
			if err := json.Unmarshal(ev.Data.Raw, &obj); err != nil {
				return err
			}
			if obj["id"] != invoiceID {
				continue
			}
			hooks := billing.NewWebhooks(g.pool, m4Secret, quiet())
			payload := stripeEvent(ev.ID, kind, obj)
			if _, err := hooks.Handle(g.ctx, payload, signPayload(g.t, payload, m4Secret)); err != nil {
				return fmt.Errorf("handle %s %s: %w", kind, ev.ID, err)
			}
			g.t.Logf("M4 %s: %s %s for %s applied", g.t.Name(), kind, ev.ID, invoiceID)
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("no %s event for %s within 3 minutes", kind, invoiceID)
}
