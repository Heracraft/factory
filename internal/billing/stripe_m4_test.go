package billing_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v83"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db/testdb"
)

const testWebhookURL = "https://api.repose.test/v1/billing/webhook"

// parseEnvBlock reads KEY=VALUE lines the way Coolify's paste box does.
func parseEnvBlock(t *testing.T, block string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("env block line %q is not KEY=VALUE", line)
		}
		out[k] = v
	}
	return out
}

// DECISIONS I-180: from nothing but a test key, the first run creates the
// product, three meters, three prices, the portal configuration and the
// webhook endpoint, and prints a block the api starts with; the second run
// creates nothing.
func TestStripeBootstrapCreatesOnceAndPrintsTheAPIEnvironment(t *testing.T) {
	ctx := context.Background()
	f := newFakeStripe(t)
	var progress bytes.Buffer
	opts := billing.BootstrapOptions{WebhookURL: testWebhookURL, DashboardURL: "https://repose.test", Out: &progress}

	res, err := billing.Bootstrap(ctx, "sk_test_bootstrap", opts, f.options()...)
	if err != nil {
		t.Fatalf("first run: %v\n%s", err, progress.String())
	}
	if n := len(res.Created); n != 9 {
		t.Fatalf("first run created %d objects, want 9 (product, 3 meters, 3 prices, portal, endpoint): %v", n, res.Created)
	}
	block := res.EnvBlock()
	env := parseEnvBlock(t, block)
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, on := billing.ConfigFromEnv()
	if !on {
		t.Fatal("the block does not switch billing on")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the api would refuse to start with the block: %v\n%s", err, block)
	}
	want := res.Config("whsec_fake1")
	want.PortalReturnURL = cfg.PortalReturnURL
	if cfg != want {
		t.Fatalf("ConfigFromEnv read\n%+v\nfrom the block, want\n%+v", cfg, want)
	}
	if cfg.MeterIDCompute == "" || cfg.PriceEgress == "" || cfg.PortalConfiguration == "" {
		t.Fatalf("block is missing ids:\n%s", block)
	}
	if cfg.AutomaticTax {
		t.Fatal("Stripe Tax is pending on this account, so the block must say STRIPE_AUTOMATIC_TAX=false")
	}
	// The endpoint is pinned to the SDK's API version and subscribed to
	// exactly the six events.
	if got := f.endpoints[0]["api_version"]; got != stripe.APIVersion {
		t.Fatalf("endpoint API version %v, want %s", got, stripe.APIVersion)
	}
	if got := f.endpoints[0]["enabled_events"].([]string); strings.Join(got, ",") != strings.Join(billing.WebhookEvents, ",") {
		t.Fatalf("endpoint events %v, want %v", got, billing.WebhookEvents)
	}
	if f.portalConfigs[0]["cancel"] != "false" {
		t.Fatal("the portal lets a user cancel the platform's subscription")
	}
	t.Logf("first run:\n%s\n%s", progress.String(), block)

	// Second run: every object found, nothing created, the secret cannot be
	// shown again and the block says so instead of printing an empty value.
	progress.Reset()
	again, err := billing.Bootstrap(ctx, "sk_test_bootstrap", opts, f.options()...)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(again.Created) != 0 {
		t.Fatalf("second run created %v", again.Created)
	}
	b2 := again.EnvBlock()
	if strings.Contains(b2, "\nSTRIPE_WEBHOOK_SECRET=") || !strings.Contains(b2, "STRIPE_WEBHOOK_SECRET unchanged") {
		t.Fatalf("second block should keep the existing secret:\n%s", b2)
	}
	e2 := parseEnvBlock(t, b2)
	for _, k := range []string{"STRIPE_PRICE_COMPUTE", "STRIPE_PRICE_STORAGE", "STRIPE_PRICE_EGRESS", "STRIPE_METER_ID_COMPUTE", "STRIPE_PORTAL_CONFIGURATION"} {
		if e2[k] != env[k] {
			t.Fatalf("%s changed between runs: %s then %s", k, env[k], e2[k])
		}
	}
	for c, n := range map[string]int{"product_create": 1, "meter_create": 3, "price_create": 3, "portal_config_create": 1, "endpoint_create": 1} {
		if f.callCount(c) != n {
			t.Fatalf("%s called %d times over two runs, want %d", c, f.callCount(c), n)
		}
	}
	t.Logf("second run:\n%s%s", progress.String(), again.Summary())

	// --rotate-webhook replaces the endpoint and prints the new secret.
	opts.RotateWebhook = true
	rot, err := billing.Bootstrap(ctx, "sk_test_bootstrap", opts, f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	if rot.WebhookSecret != "whsec_fake2" || f.callCount("endpoint_delete") != 1 || len(f.endpoints) != 1 {
		t.Fatalf("rotate: secret %q, %d deletes, %d endpoints", rot.WebhookSecret, f.callCount("endpoint_delete"), len(f.endpoints))
	}
}

// A live key is refused before a single request unless --live is given.
func TestStripeBootstrapRefusesALiveKey(t *testing.T) {
	ctx := context.Background()
	f := newFakeStripe(t)
	_, err := billing.Bootstrap(ctx, "sk_live_nope", billing.BootstrapOptions{}, f.options()...)
	if !errors.Is(err, billing.ErrLiveKey) {
		t.Fatalf("live key: %v, want ErrLiveKey", err)
	}
	if n := f.callCount("product_get"); n != 0 {
		t.Fatalf("a refused live key still made %d requests", n)
	}
	for _, k := range []string{"pk_test_x", "whsec_x", ""} {
		if _, err := billing.Bootstrap(ctx, k, billing.BootstrapOptions{}, f.options()...); err == nil {
			t.Fatalf("key %q was accepted", k)
		}
	}
	res, err := billing.Bootstrap(ctx, "sk_live_ok", billing.BootstrapOptions{AllowLive: true}, f.options()...)
	if err != nil {
		t.Fatalf("--live: %v", err)
	}
	if !strings.Contains(res.EnvBlock(), "Stripe LIVE mode") {
		t.Fatal("a live block does not say it is live")
	}
}

// An endpoint on another API version would have every event rejected, so
// the bootstrap stops rather than reusing it.
func TestStripeBootstrapRefusesAnEndpointOnAnotherAPIVersion(t *testing.T) {
	ctx := context.Background()
	f := newFakeStripe(t)
	f.endpoints = append(f.endpoints, map[string]any{"id": "we_old", "object": "webhook_endpoint", "url": testWebhookURL,
		"api_version": "2024-06-20", "enabled_events": []string{"*"}})
	_, err := billing.Bootstrap(ctx, "sk_test_x", billing.BootstrapOptions{WebhookURL: testWebhookURL}, f.options()...)
	if err == nil || !strings.Contains(err.Error(), "--rotate-webhook") {
		t.Fatalf("old-version endpoint: %v", err)
	}
	f.taxStatus = "active"
	res, err := billing.Bootstrap(ctx, "sk_test_x", billing.BootstrapOptions{WebhookURL: testWebhookURL, RotateWebhook: true}, f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.EnvBlock(), "STRIPE_AUTOMATIC_TAX=true") {
		t.Fatal("active Stripe Tax should switch automatic tax on")
	}
}

// DECISIONS I-179: the billing period is the subscription's, truncated to
// the hour, and a row is reported at the last second of its hour.
func TestSubscriptionAnchorsThePeriodAndEventsLandInIt(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	anchor := time.Date(2026, 10, 3, 14, 32, 10, 0, time.UTC)
	f.anchor = anchor.Unix()
	f.pmCountry = "US"
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	if _, err := pool.Exec(ctx, "update users set stripe_customer_id = 'cus_anchor' where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	if err := st.OnCardAttached(ctx, a.UserID, "pm_card_visa"); err != nil {
		t.Fatal(err)
	}
	var got time.Time
	if err := pool.QueryRow(ctx, "select billing_anchor from users where id = $1", a.UserID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Equal(anchor.Truncate(time.Hour)) {
		t.Fatalf("billing_anchor %s, want the subscription's anchor hour %s", got.UTC(), anchor.Truncate(time.Hour))
	}
	if f.customerCountry != "US" {
		t.Fatal("the card's billing address was not copied to the customer for Stripe Tax")
	}
	// The anchor hour itself: in the rollup's new period and, stamped at
	// 14:59:59, inside Stripe's period that starts at 14:32:10.
	hour := anchor.Truncate(time.Hour)
	if _, err := st.PushUsage(ctx, billing.UsageRow{ProjectID: a.ProjectID.String(), CustomerID: "cus_anchor", Hour: hour, GuestCents: 14}); err != nil {
		t.Fatal(err)
	}
	id := "usage:" + a.ProjectID.String() + ":" + hour.Format(time.RFC3339) + ":compute"
	ts := time.Unix(f.eventTimestamps[id], 0).UTC()
	if want := hour.Add(time.Hour - time.Second); !ts.Equal(want) {
		t.Fatalf("meter event at %s, want %s", ts, want)
	}
	if ts.Before(anchor) {
		t.Fatal("the anchor hour's event lands before Stripe's period starts")
	}
}

// DECISIONS I-181: a card is never refused over tax configuration.
func TestSubscriptionFallsBackWhenStripeRefusesTax(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	f.refuseTax = true
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	sub, err := st.EnsureSubscription(ctx, a.UserID)
	if err != nil {
		t.Fatalf("tax refusal was not absorbed: %v", err)
	}
	if sub == "" || f.callCount("subscriptions") != 2 || f.automaticTax() {
		t.Fatalf("sub %q after %d attempts, automatic tax %v", sub, f.callCount("subscriptions"), f.automaticTax())
	}
}

// DECISIONS I-181: a subscription Stripe already has is adopted, never
// doubled (two subscriptions on the same meters would each invoice the same
// usage), and one after a deleted subscription is a new one.
func TestSubscriptionIsAdoptedNotDoubled(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	first, err := st.EnsureSubscription(ctx, a.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(f.lastSubKey, ":0") {
		t.Fatalf("first create used key %q", f.lastSubKey)
	}
	// The id never reached the database (a crash after Stripe answered).
	if _, err := pool.Exec(ctx, "update users set stripe_subscription_id = null where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	again, err := st.EnsureSubscription(ctx, a.UserID)
	if err != nil || again != first || f.callCount("subscriptions") != 1 {
		t.Fatalf("retry gave %q (%v) after %d creates; want %q adopted", again, err, f.callCount("subscriptions"), first)
	}
	// Deleted at Stripe: the next card makes a new one under a new key.
	f.subList[0]["status"] = "canceled"
	if _, err := pool.Exec(ctx, "update users set stripe_subscription_id = null where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	third, err := st.EnsureSubscription(ctx, a.UserID)
	if err != nil || third == first || !strings.HasSuffix(f.lastSubKey, ":1") {
		t.Fatalf("after a deletion: %q (%v), key %q", third, err, f.lastSubKey)
	}
}

// DECISIONS I-182: the hosted card form.
func TestSetupCheckoutCollectsTheAddress(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	url, err := st.SetupCheckout(ctx, a.UserID.String())
	if err != nil || !strings.HasPrefix(url, "https://checkout.stripe.com/") {
		t.Fatalf("checkout: %q %v", url, err)
	}
	for k, want := range map[string]string{
		"mode": "setup", "billing_address_collection": "required", "customer_update[address]": "auto",
		"customer": "cus_" + a.Handle, "success_url": "https://repose.test/billing?card=saved",
		"setup_intent_data[metadata][user_id]": a.UserID.String(),
	} {
		if got := f.lastCheckout[k]; got != want {
			t.Errorf("checkout %s = %q, want %q", k, got, want)
		}
	}
}

// DECISIONS I-185: `billing show` totals the period the way the invoice
// will, and `cycle-now` ends the period at Stripe and moves the rollup's
// anchor to the same hour.
func TestAccountTotalsAndCycleNow(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	start := base()
	ensurePartitions(t, pool, start, start.AddDate(0, 1, 0))
	a := seedAccount(t, pool, "large", start, billing.TrialCreditCents)
	if _, err := pool.Exec(ctx, "update projects set state = 'stopped' where id = $1", a.ProjectID); err != nil {
		t.Fatal(err)
	}
	fillRunning(t, pool, a, start, 100, "large", 40<<30, 10<<30)
	r := billing.NewRollup(pool, st, metrics.NewNop(), quiet())
	for h := start; h.Before(start.Add(120 * time.Hour)); h = h.Add(time.Hour) {
		if _, err := r.Hour(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	acct, err := billing.LoadAccount(ctx, pool, a.UserID, start.Add(121*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	customer := "cus_" + a.Handle
	if acct.Hours != 120 || acct.UnpushedRows != 0 || acct.Billed() != f.total(customer) || acct.CostCents-acct.CreditCents != acct.Billed() {
		t.Fatalf("account: %d hours, %d unpushed, billed %d, Stripe has %d, cost %d credit %d", acct.Hours, acct.UnpushedRows, acct.Billed(), f.total(customer), acct.CostCents, acct.CreditCents)
	}
	if acct.ComputeBilled != f.meter(customer, billing.DefaultMeterCompute) || acct.StorageBilled != f.meter(customer, billing.DefaultMeterStorage) {
		t.Fatalf("per-part totals differ from what was pushed: %+v", acct)
	}
	var out bytes.Buffer
	if _, err := acct.WriteTo(&out); err != nil || !strings.Contains(out.String(), "the invoice before tax") {
		t.Fatalf("show: %v\n%s", err, out.String())
	}
	t.Logf("billing show:\n%s", out.String())

	// cycle-now: Stripe resets the anchor to 14:32:10 on day 6; the rollup's
	// anchor becomes 14:00 that day.
	newAnchor := start.Add(125*time.Hour + 32*time.Minute + 10*time.Second)
	f.anchor = newAnchor.Unix()
	if _, err := pool.Exec(ctx, "update users set stripe_subscription_id = 'sub_cycle' where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}
	inv, anchor, err := st.CycleNow(ctx, a.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if inv != "in_cycle_now" || !anchor.Equal(newAnchor.Truncate(time.Hour)) || f.lastCheckout["billing_cycle_anchor"] != "now" || f.lastCheckout["proration_behavior"] != "none" {
		t.Fatalf("cycle-now: invoice %q anchor %s, sent %v", inv, anchor, f.lastCheckout)
	}
	after, err := billing.LoadAccount(ctx, pool, a.UserID, newAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Period.Start.Equal(newAnchor.Truncate(time.Hour)) || after.Hours != 0 {
		t.Fatalf("after cycle-now the period is %v with %d rows", after.Period, after.Hours)
	}
}

// DECISIONS I-183: the invoice list carries the names the dashboard reads.
func TestInvoicesCarryTheDocumentedNames(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	f := newFakeStripe(t)
	f.invoices = []map[string]any{{"id": "in_1", "object": "invoice", "status": "paid", "total": 816, "subtotal": 800,
		"total_excluding_tax": 800, "currency": "usd", "created": 1790000000, "number": "R-1",
		"hosted_invoice_url": "https://invoice.stripe.com/i/1", "invoice_pdf": "https://pay.stripe.com/1/pdf"}}
	st, err := billing.NewStripe(f.config(), pool, quiet(), f.options()...)
	if err != nil {
		t.Fatal(err)
	}
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	inv, err := st.Invoices(ctx, a.UserID.String())
	if err != nil || len(inv) != 1 {
		t.Fatalf("invoices: %v %v", inv, err)
	}
	i := inv[0]
	if i["amount_cents"] != int64(816) || i["subtotal_cents"] != int64(800) || i["tax_cents"] != int64(16) ||
		i["pdf_url"] != "https://pay.stripe.com/1/pdf" || i["hosted_url"] != "https://invoice.stripe.com/i/1" || i["created_at"] == nil {
		t.Fatalf("invoice view %v", i)
	}
	if i["total_cents"] != int64(816) || i["pdf"] == nil {
		t.Fatal("the previous names are gone before their release is up")
	}
}
