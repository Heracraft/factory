package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	httpapi "github.com/heracraft/repose/internal/api/http"
	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
)

// docs/MILESTONES.md M4, "trial credit depletes and blocks a start at
// zero", as 09-billing.md §5.3 and DECISIONS I-184/I-185 read it: the hour
// that spends the last cent moves an account with a card to `active` and
// its next start goes through (and is billed); at zero without a card
// every start is refused; a `trial` account at zero is refused
// `trial_depleted`; and a card arriving at zero ends the trial instead of
// locking the account out.
func TestTrialCreditDepletesThroughTheRollup(t *testing.T) {
	const secret = "whsec_depletion"
	e := newEnvWith(t, &httpapi.RateLimits{General: 10000, Certs: 10000, Config: 10000}, func(d *httpapi.Deps) {
		d.Webhooks = billing.NewWebhooks(d.Pool, secret, d.Log)
	})
	ctx := context.Background()
	tok := e.signIn(t, "sub-deplete", "deplete-dev")
	var uid string
	if err := e.h.Pool.QueryRow(ctx, "update users set has_card = true, stripe_customer_id = 'cus_deplete' where logto_sub = 'sub-deplete' returning id::text").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	status := func() (string, int64) {
		t.Helper()
		var s string
		var bal int64
		if err := e.h.Pool.QueryRow(ctx, "select billing_status, (select coalesce(sum(cents),0) from credit_ledger where user_id = users.id) from users where id = $1", uid).Scan(&s, &bal); err != nil {
			t.Fatal(err)
		}
		return s, bal
	}
	if s, bal := status(); s != "trial" || bal != billing.TrialCreditCents {
		t.Fatalf("at sign-in: %s with %d cents, want trial with %d", s, bal, billing.TrialCreditCents)
	}
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "spend", "class": "small"})
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	pid := r.body["id"].(string)
	e.waitOp(t, r)
	stopIt := func() {
		t.Helper()
		s := e.do(t, tok, "POST", "/projects/"+pid+"/stop", map[string]any{"snapshot": false})
		if s.status != 202 && s.status != 200 {
			t.Fatalf("stop: %d %s", s.status, s.raw)
		}
		e.waitOp(t, s)
	}
	startIt := func(what string) {
		t.Helper()
		s := e.do(t, tok, "POST", "/projects/"+pid+"/start", nil)
		if s.status != 202 && s.status != 200 {
			t.Fatalf("%s: %d %s", what, s.status, s.raw)
		}
		e.waitOp(t, s)
	}
	blocked := func(what, reason string) {
		t.Helper()
		s := e.do(t, tok, "POST", "/projects/"+pid+"/start", nil)
		if s.status != http.StatusPaymentRequired || errDetail(s, "reason") != reason {
			t.Fatalf("%s: %d %s, want 402 %s", what, s.status, s.raw, reason)
		}
	}
	stopIt()

	// Five cents left, then one running hour of a small guest (7 cents)
	// through the real rollup.
	if _, err := billing.Credit(ctx, e.h.Pool, uuid.MustParse(uid), 5-billing.TrialCreditCents, billing.ReasonAdjustment, "test"); err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	// The project existed during that hour.
	if _, err := e.h.Pool.Exec(ctx, "update projects set created_at = $2 where id = $1", pid, hour.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsurePartitions(ctx, e.h.Pool, hour); err != nil {
		t.Fatal(err)
	}
	if _, err := e.h.Pool.Exec(ctx, `insert into meter_samples (ts, project_id, host_id, state, class, disk_alloc, net_tx, guestd_ok)
		select $1::timestamptz + (g || ' minutes')::interval, $2, '00000000-0000-0000-0000-000000000000', 'running', 'small', 0, 0, true
		from generate_series(0, 59) g`, hour, pid); err != nil {
		t.Fatal(err)
	}
	rows, err := billing.NewRollup(e.h.Pool, billing.Disabled{}, metrics.NewNop(), e.h.Log).Hour(ctx, hour)
	if err != nil {
		t.Fatal(err)
	}
	s, bal := status()
	if s != "active" || bal != 0 {
		t.Fatalf("after the hour that spent the credit: %s with %d cents, want active with 0", s, bal)
	}
	for _, row := range rows {
		if row.ProjectID.String() == pid {
			t.Logf("depleting hour %s: cost %d cents, credit %d, %d billed to Stripe; account %s, balance %d",
				row.Hour.Format(time.RFC3339), row.CostCents, row.CreditCents, row.Billable(), s, bal)
		}
	}
	// At zero with a card: the start goes through, and is billed.
	startIt("start at zero with a card")
	stopIt()

	// At zero without a card: blocked.
	if _, err := e.h.Pool.Exec(ctx, "update users set has_card = false where id = $1", uid); err != nil {
		t.Fatal(err)
	}
	blocked("start at zero without a card", "card_required")

	// A card removed before the credit ran out leaves the account `trial`
	// at zero (the end of the trial needs a card): blocked as
	// trial_depleted...
	if _, err := e.h.Pool.Exec(ctx, "update users set billing_status = 'trial', has_card = true where id = $1", uid); err != nil {
		t.Fatal(err)
	}
	blocked("trial at zero", "trial_depleted")
	// ...until a card arrives (setup_intent.succeeded), which ends the trial
	// rather than locking the account out.
	if _, err := e.h.Pool.Exec(ctx, "update users set has_card = false where id = $1", uid); err != nil {
		t.Fatal(err)
	}
	body := routeEvent("evt_deplete_card", billing.TypeSetupIntentSucceeded, map[string]any{"id": "seti_d", "customer": "cus_deplete"})
	if w := e.doRaw(t, "POST", "/billing/webhook", body, map[string]string{"Stripe-Signature": routeSign(t, body, secret)}); w.status != 200 {
		t.Fatalf("webhook: %d %s", w.status, w.raw)
	}
	if s, _ := status(); s != "active" {
		t.Fatalf("after a card at zero the account is %s, want active", s)
	}
	startIt("start after the card arrived")
}

// stubPortal answers the Stripe customer-facing calls with fixed values.
type stubPortal struct{}

func (stubPortal) PortalURL(context.Context, string) (string, error) {
	return "https://billing.stripe.com/p/session/stub", nil
}
func (stubPortal) SetupIntent(context.Context, string) (string, error) {
	return "seti_stub_secret", nil
}
func (stubPortal) SetupCheckout(context.Context, string) (string, error) {
	return "https://checkout.stripe.com/c/pay/cs_stub", nil
}
func (stubPortal) Invoices(context.Context, string) ([]map[string]any, error) { return nil, nil }

// DECISIONS I-182: POST /billing/setup answers the hosted Checkout URL for
// {"flow": "checkout"} and keeps the client secret for no body.
func TestBillingSetupCheckoutFlow(t *testing.T) {
	e := newEnvWith(t, &httpapi.RateLimits{General: 10000, Certs: 10000, Config: 10000}, func(d *httpapi.Deps) {
		d.Billing = stubPortal{}
	})
	tok := e.signIn(t, "sub-checkout", "checkout-dev")
	if r := e.do(t, tok, "POST", "/billing/setup", map[string]any{"flow": "checkout"}); r.status != 200 || r.body["url"] != "https://checkout.stripe.com/c/pay/cs_stub" {
		t.Fatalf("checkout flow: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "POST", "/billing/setup", nil); r.status != 200 || r.body["client_secret"] != "seti_stub_secret" {
		t.Fatalf("no body: %d %s", r.status, r.raw)
	}
	if r := e.do(t, tok, "POST", "/billing/setup", map[string]any{"flow": "paypal"}); r.status != 400 || errCode(r) != "invalid" {
		t.Fatalf("unknown flow: %d %s", r.status, r.raw)
	}
}
