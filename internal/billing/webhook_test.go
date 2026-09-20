package billing_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/webhook"

	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/db/testdb"
)

const testWebhookSecret = "whsec_fake"

// stripeEvent builds the JSON body Stripe posts.
func stripeEvent(id, kind string, object map[string]any) []byte {
	b, err := json.Marshal(map[string]any{
		// The endpoint must be created with the SDK's API version, or every
		// delivery is refused as a version mismatch; ops/AZURE-SETUP.md
		// step 17 says so and this is the same version.
		"id": id, "object": "event", "type": kind, "api_version": stripe.APIVersion,
		"created": time.Now().Unix(),
		"data":    map[string]any{"object": object},
	})
	if err != nil {
		panic(err) // test fixture on a literal map; a marshal failure is a bug in the test
	}
	return b
}

// signPayload produces the Stripe-Signature header the endpoint verifies.
func signPayload(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	now := time.Now()
	sig := webhook.ComputeSignature(now, payload, secret)
	return fmt.Sprintf("t=%d,v1=%x", now.Unix(), sig)
}

func newHooks(t *testing.T, pool *db.Pool) *billing.Webhooks {
	t.Helper()
	return billing.NewWebhooks(pool, testWebhookSecret, quiet())
}

// deliver posts one event and returns the handler's error.
func deliver(t *testing.T, w *billing.Webhooks, id, kind string, object map[string]any) error {
	t.Helper()
	payload := stripeEvent(id, kind, object)
	_, err := w.Handle(context.Background(), payload, signPayload(t, payload, testWebhookSecret))
	return err
}

// §9: "All six webhooks handled idempotently; replay test passes; signature
// failures rejected."
func TestAllSixWebhooks(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	w := newHooks(t, pool)
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	customer := "cus_" + a.Handle
	if _, err := pool.Exec(ctx, "update users set stripe_subscription_id = 'sub_1', has_card = false where id = $1", a.UserID); err != nil {
		t.Fatal(err)
	}

	// setup_intent.succeeded: has_card flips (§5.2).
	if err := deliver(t, w, "evt_1", billing.TypeSetupIntentSucceeded, map[string]any{"id": "seti_1", "customer": customer, "payment_method": "pm_1"}); err != nil {
		t.Fatal(err)
	}
	if !userBool(t, pool, a, "has_card") {
		t.Fatal("setup_intent.succeeded did not set has_card")
	}

	// invoice.payment_failed: past_due with the clock started (§5.6).
	if err := deliver(t, w, "evt_2", billing.TypeInvoicePaymentFailed, map[string]any{
		"id": "in_1", "customer": customer, "total": 1234, "status": "open",
		"period_start": base().Unix(), "period_end": base().AddDate(0, 1, 0).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if s := userString(t, pool, a, "billing_status"); s != "past_due" {
		t.Fatalf("billing_status after a failed payment: %s", s)
	}
	if userTime(t, pool, a, "past_due_since") == nil {
		t.Fatal("past_due_since was not set")
	}
	if n := invoiceCount(t, pool, "in_1"); n != 1 {
		t.Fatalf("%d invoices rows for in_1", n)
	}

	// invoice.paid: active again, limits raised, past_due_since cleared,
	// and guests are NOT started (§5.6).
	if err := deliver(t, w, "evt_3", billing.TypeInvoicePaid, map[string]any{
		"id": "in_1", "customer": customer, "total": 1234, "status": "paid",
		"period_start": base().Unix(), "period_end": base().AddDate(0, 1, 0).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if s := userString(t, pool, a, "billing_status"); s != "active" {
		t.Fatalf("billing_status after payment: %s", s)
	}
	if userTime(t, pool, a, "past_due_since") != nil {
		t.Fatal("past_due_since survived the payment")
	}
	var projects, xl int
	if err := pool.QueryRow(ctx, "select project_limit, xl_limit from users where id = $1", a.UserID).Scan(&projects, &xl); err != nil {
		t.Fatal(err)
	}
	if projects != billing.ProjectLimitPaid || xl != billing.XLLimitPaid {
		t.Fatalf("limits after the first paid invoice: %d/%d, want %d/%d", projects, xl, billing.ProjectLimitPaid, billing.XLLimitPaid)
	}
	if s := projectState(t, pool, a); s != "running" {
		t.Fatalf("invoice.paid changed the project state to %s", s)
	}
	if n := invoiceCount(t, pool, "in_1"); n != 1 {
		t.Fatalf("the paid event inserted a second invoices row: %d", n)
	}

	// payment_method.detached: has_card false, guests keep running (§6).
	if err := deliver(t, w, "evt_4", billing.TypePaymentMethodDetached, map[string]any{"id": "pm_1", "customer": customer}); err != nil {
		t.Fatal(err)
	}
	if userBool(t, pool, a, "has_card") {
		t.Fatal("has_card survived the detach")
	}
	if s := projectState(t, pool, a); s != "running" {
		t.Fatalf("a detached card stopped a guest: %s", s)
	}

	// charge.refunded: a credit row for the record (§5.9).
	if err := deliver(t, w, "evt_5", billing.TypeChargeRefunded, map[string]any{"id": "ch_1", "customer": customer, "amount_refunded": 500}); err != nil {
		t.Fatal(err)
	}
	balance, err := billing.Balance(ctx, pool, a.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != billing.TrialCreditCents+500 {
		t.Fatalf("balance after a 500 cent refund: %d", balance)
	}

	// customer.subscription.deleted: the stored id is dropped.
	if err := deliver(t, w, "evt_6", billing.TypeSubscriptionDeleted, map[string]any{"id": "sub_1", "customer": customer}); err != nil {
		t.Fatal(err)
	}
	var sub *string
	if err := pool.QueryRow(ctx, "select stripe_subscription_id from users where id = $1", a.UserID).Scan(&sub); err != nil {
		t.Fatal(err)
	}
	if sub != nil {
		t.Fatalf("subscription id survived the delete: %s", *sub)
	}

	// All six are recorded and processed.
	var processed int
	if err := pool.QueryRow(ctx, "select count(*) from stripe_events where processed_at is not null and error is null").Scan(&processed); err != nil {
		t.Fatal(err)
	}
	if processed != 6 {
		t.Fatalf("%d of 6 events processed", processed)
	}
}

// §6: "duplicate webhook delivery: ignored by the stripe_events primary
// key." The replay test: every event delivered twice changes nothing.
func TestWebhookReplayIsANoOp(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	w := newHooks(t, pool)
	a := seedAccount(t, pool, "large", base(), billing.TrialCreditCents)
	customer := "cus_" + a.Handle
	events := []struct {
		id, kind string
		object   map[string]any
	}{
		{"evt_r1", billing.TypeSetupIntentSucceeded, map[string]any{"id": "seti_1", "customer": customer}},
		{"evt_r2", billing.TypeChargeRefunded, map[string]any{"id": "ch_1", "customer": customer, "amount_refunded": 250}},
		{"evt_r3", billing.TypeInvoicePaid, map[string]any{"id": "in_9", "customer": customer, "total": 900, "status": "paid"}},
	}
	for _, e := range events {
		if err := deliver(t, w, e.id, e.kind, e.object); err != nil {
			t.Fatalf("%s: %v", e.kind, err)
		}
	}
	before := accountSnapshot(t, pool, a)
	for _, e := range events {
		err := deliver(t, w, e.id, e.kind, e.object)
		if !errors.Is(err, billing.ErrDuplicate) {
			t.Fatalf("replay of %s returned %v, want ErrDuplicate", e.kind, err)
		}
	}
	if after := accountSnapshot(t, pool, a); after != before {
		t.Fatalf("a replay changed the account:\n%s\nwant\n%s", after, before)
	}
	// Specifically: the refund was credited once.
	balance, err := billing.Balance(ctx, pool, a.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != billing.TrialCreditCents+250 {
		t.Fatalf("the replayed refund credited twice: balance %d", balance)
	}
}

// §6: "webhook signature invalid: 400, logged with the event type only".
func TestWebhookSignatureFailuresAreRejected(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	w := newHooks(t, pool)
	payload := stripeEvent("evt_bad", billing.TypeInvoicePaid, map[string]any{"id": "in_x", "customer": "cus_x"})
	for _, c := range []struct{ name, header string }{
		{"empty", ""},
		{"garbage", "t=1,v1=deadbeef"},
		{"signed with another secret", signPayload(t, payload, "whsec_someone_else")},
	} {
		if _, err := w.Handle(ctx, payload, c.header); !errors.Is(err, billing.ErrBadSignature) {
			t.Errorf("%s: %v, want ErrBadSignature", c.name, err)
		}
	}
	// A rejected event is not recorded, so a correctly signed delivery of
	// the same id still applies.
	var n int
	if err := pool.QueryRow(ctx, "select count(*) from stripe_events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d unverified events were recorded", n)
	}
	// A tampered body fails too: the signature covers the payload.
	sig := signPayload(t, payload, testWebhookSecret)
	tampered := append(append([]byte{}, payload[:len(payload)-1]...), []byte(`,"x":1}`)...)
	if _, err := w.Handle(ctx, tampered, sig); !errors.Is(err, billing.ErrBadSignature) {
		t.Errorf("tampered body: %v, want ErrBadSignature", err)
	}
}

// With no secret configured the handler refuses rather than accepting
// unverified events (DECISIONS I-16).
func TestWebhookWithoutASecretIsDisabled(t *testing.T) {
	pool := testdb.Open(t)
	w := billing.NewWebhooks(pool, "", quiet())
	payload := stripeEvent("evt_0", billing.TypeInvoicePaid, map[string]any{"id": "in_0"})
	if _, err := w.Handle(context.Background(), payload, "t=1,v1=00"); !errors.Is(err, billing.ErrDisabled) {
		t.Fatalf("%v, want ErrDisabled", err)
	}
}

// --- small readers ----------------------------------------------------

func userBool(t *testing.T, pool *db.Pool, a account, col string) bool {
	t.Helper()
	var v bool
	if err := pool.QueryRow(context.Background(), "select "+col+" from users where id = $1", a.UserID).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func userString(t *testing.T, pool *db.Pool, a account, col string) string {
	t.Helper()
	var v string
	if err := pool.QueryRow(context.Background(), "select "+col+" from users where id = $1", a.UserID).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func userTime(t *testing.T, pool *db.Pool, a account, col string) *time.Time {
	t.Helper()
	var v *time.Time
	if err := pool.QueryRow(context.Background(), "select "+col+" from users where id = $1", a.UserID).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func projectState(t *testing.T, pool *db.Pool, a account) string {
	t.Helper()
	var v string
	if err := pool.QueryRow(context.Background(), "select state from projects where id = $1", a.ProjectID).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func invoiceCount(t *testing.T, pool *db.Pool, stripeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "select count(*) from invoices where stripe_invoice_id = $1", stripeID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func accountSnapshot(t *testing.T, pool *db.Pool, a account) string {
	t.Helper()
	var s string
	err := pool.QueryRow(context.Background(), `select concat_ws(' ', billing_status, has_card::text, trial_credit_cents::text,
		project_limit::text, xl_limit::text, coalesce(stripe_subscription_id, '-'), coalesce(past_due_since::text, '-'),
		(select count(*)::text from credit_ledger where user_id = users.id),
		(select count(*)::text from invoices where user_id = users.id))
		from users where id = $1`, a.UserID).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
