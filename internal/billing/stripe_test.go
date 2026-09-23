package billing_test

import (
	"context"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

// §9: "Customer created at first GET /me; SetupIntent flow attaches a card;
// has_card flips on webhook. Evidence: Stripe test-mode transcript." The
// transcript here is the fake's call log, which is what CI produces without
// a Stripe key; with STRIPE_SECRET_KEY set, the same sequence runs against
// Stripe test mode.
func TestCustomerSetupIntentAndSubscription(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	// The seed gives the account a customer id already; clear it so the
	// first-sight path runs.
	if _, err := pool.Exec(ctx, "update users set stripe_customer_id = null, has_card = false where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}

	cus, err := st.EnsureCustomer(ctx, a.UserID)
	if err != nil {
		t.Fatalf("ensure customer: %v", err)
	}
	if cus == "" {
		t.Fatal("no customer id")
	}
	// Idempotent: a second /me does not create a second customer.
	again, err := st.EnsureCustomer(ctx, a.UserID)
	if err != nil || again != cus {
		t.Fatalf("second EnsureCustomer gave %q (%v), want %q", again, err, cus)
	}
	if n := f.callCount("customers"); n != 1 {
		t.Fatalf("created %d customers", n)
	}

	secret, err := st.SetupIntent(ctx, a.UserID.String())
	if err != nil || secret == "" {
		t.Fatalf("setup intent: %q %v", secret, err)
	}

	// The webhook flips has_card and, through OnCardAttached, creates the
	// subscription with the three metered prices anchored now.
	hooks := billing.NewWebhooks(pool, f.config().WebhookSecret, quiet())
	hooks.OnCardAttached = st.OnCardAttached
	payload := stripeEvent("evt_setup_1", billing.TypeSetupIntentSucceeded, map[string]any{
		"id": "seti_fake", "customer": cus, "payment_method": "pm_card_visa",
	})
	if _, err := hooks.Handle(ctx, payload, signPayload(t, payload, f.config().WebhookSecret)); err != nil {
		t.Fatalf("setup_intent.succeeded: %v", err)
	}
	var hasCard bool
	var sub *string
	if err := pool.QueryRow(ctx, "select has_card, stripe_subscription_id from users where id = $1", a.UserID).Scan(&hasCard, &sub); err != nil {
		t.Fatal(err)
	}
	if !hasCard {
		t.Fatal("has_card did not flip on the webhook")
	}
	if sub == nil || *sub == "" {
		t.Fatal("no subscription was created when the card was attached")
	}
	if n := f.callCount("subscriptions"); n != 1 {
		t.Fatalf("created %d subscriptions", n)
	}
	// §5.9: Stripe Tax is switched on with `automatic_tax: {enabled: true}`
	// on the subscription and nothing else; the fake records what was sent.
	if !f.automaticTax() {
		t.Fatal("the subscription was created without automatic_tax")
	}
	if len(f.subscriptionPrices()) != 3 {
		t.Fatalf("the subscription carries %v, want the three metered prices", f.subscriptionPrices())
	}
	t.Logf("transcript: customer=%s setup_intent secret=%s... subscription=%s prices=%v automatic_tax=%v",
		cus, secret[:9], *sub, f.subscriptionPrices(), f.automaticTax())

	// The portal and the invoice list answer through the same client.
	if url, err := st.PortalURL(ctx, a.UserID.String()); err != nil || url == "" {
		t.Fatalf("portal: %q %v", url, err)
	}
	if _, err := st.Invoices(ctx, a.UserID.String()); err != nil {
		t.Fatalf("invoices: %v", err)
	}
}

// docs/CHECKLIST.md: "Stripe: test-mode invoice for the fixed usage pattern
// matches to the cent", and 09-billing.md §7's fixture: one large guest,
// 100 running hours across a period, 40 GB, 10 GB egress. Assert the lines:
// compute 1400, storage 400, egress 0, total 1800, minus the fixture's
// 1000 of credit = 800 pushed to Stripe. The fixture keeps its own credit
// (fixtureCreditCents) rather than the trial's, which I-205 made one day of
// compute: the 800-cent invoice is what docs/ops/M4-GATE.md checks against.
// fixtureCreditCents is the credit the 09-billing.md §7 usage fixture's
// account starts with (the pre-I-205 trial), so its invoice stays 800.
const fixtureCreditCents int64 = 1000

func TestChecklistUsageFixtureInvoicesTo800Cents(t *testing.T) {
	if testing.Short() {
		t.Skip("a whole billing period of rollups")
	}
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 2, 0))
	a := seedAccount(t, pool, "large", start, fixtureCreditCents)
	customer := "cus_" + a.Handle
	period := billing.PeriodFor(a.Anchor, start)
	hours := period.Hours()

	// The guest runs 100 of the period's hours and is stopped for the rest;
	// the volume is billed for the whole period either way.
	if _, err := pool.Exec(ctx, "update projects set state = 'stopped' where id = $1", a.ProjectID); err != nil {
		t.Fatal(err)
	}
	fillRunning(t, pool, a, start, 100, "large", 40<<30, 10<<30)

	r := billing.NewRollup(pool, st, metrics.NewNop(), quiet())
	r.Now = func() time.Time { return period.End }
	if n, err := r.Due(ctx); err != nil || n != hours {
		t.Fatalf("rolled %d of %d hours: %v", n, hours, err)
	}

	guest, storage, egress, cost, credit := sums(t, pool, a.ProjectID)
	if guest != 1400 {
		t.Errorf("compute line %d cents, want 1400 (100 hours x %d)", guest, billing.HourLarge)
	}
	if storage != 400 {
		t.Errorf("storage line %d cents, want 400 (40 GB x %d cents)", storage, billing.StoragePerGBMonth)
	}
	if egress != 0 {
		t.Errorf("egress line %d cents, want 0 (10 GB is inside the %d GB allowance)", egress, billing.EgressIncludedGB)
	}
	if cost != 1800 {
		t.Errorf("total %d cents, want 1800", cost)
	}
	if credit != fixtureCreditCents {
		t.Errorf("trial credit applied %d cents, want %d", credit, fixtureCreditCents)
	}
	if billed := cost - credit; billed != 800 {
		t.Errorf("billable %d cents, want 800", billed)
	}
	// And that is what Stripe was given, to the cent.
	if got := f.total(customer); got != 800 {
		t.Errorf("Stripe received %d cents, want 800", got)
	}
	t.Logf("invoice: compute %d + storage %d + egress %d = %d cents, less %d trial credit = %d pushed to Stripe (meters: compute %d, storage %d, egress %d)",
		guest, storage, egress, cost, credit, f.total(customer),
		f.meter(customer, billing.DefaultMeterCompute), f.meter(customer, billing.DefaultMeterStorage), f.meter(customer, billing.DefaultMeterEgress))

	// §9: "Usage records pushed with idempotency keys, ids stored, never
	// re-pushed." Pushing again sends nothing, and the identifiers Stripe
	// deduped on mean even a retry could not double-bill.
	pushesBefore := f.callCount("meter_events")
	if err := r.Push(ctx); err != nil {
		t.Fatal(err)
	}
	if n := f.callCount("meter_events"); n != pushesBefore {
		t.Fatalf("a stored row was pushed again: %d then %d meter events", pushesBefore, n)
	}
	var unrecorded int
	if err := pool.QueryRow(ctx, "select count(*) from usage_hours where project_id = $1 and cost_cents > credit_cents and stripe_usage_record_id is null", a.ProjectID).Scan(&unrecorded); err != nil {
		t.Fatal(err)
	}
	if unrecorded != 0 {
		t.Fatalf("%d billable rows have no Stripe record id", unrecorded)
	}
	// Stripe's own identifier uniqueness is the second guard: replaying the
	// same events changes no total.
	if err := replayMeterEvents(ctx, st, pool, a); err != nil {
		t.Fatal(err)
	}
	if got := f.total(customer); got != 800 {
		t.Fatalf("a replay changed the total to %d cents", got)
	}
}

// replayMeterEvents re-pushes every row's parts with the same identifiers,
// the way a retry after a timeout would.
func replayMeterEvents(ctx context.Context, st *billing.Stripe, pool *db.Pool, a account) error {
	rows, err := pool.Query(ctx, "select hour, guest_cents, storage_cents, egress_cents, credit_cents from usage_hours where project_id = $1 order by hour", a.ProjectID)
	if err != nil {
		return err
	}
	type row struct {
		hour                            time.Time
		guest, storage, egress, credite int64
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.hour, &r.guest, &r.storage, &r.egress, &r.credite); err != nil {
			rows.Close()
			return err
		}
		rs = append(rs, r)
	}
	rows.Close()
	for _, r := range rs {
		g, s, e := r.guest, r.storage, r.egress
		c := r.credite
		for _, p := range []*int64{&g, &s, &e} {
			if c <= 0 {
				break
			}
			d := c
			if d > *p {
				d = *p
			}
			*p -= d
			c -= d
		}
		if _, err := st.PushUsage(ctx, billing.UsageRow{
			ProjectID: a.ProjectID.String(), UserID: a.UserID.String(), CustomerID: "cus_" + a.Handle,
			Hour: r.hour, GuestCents: g, StorageCents: s, EgressCents: e,
		}); err != nil {
			return err
		}
	}
	return nil
}
