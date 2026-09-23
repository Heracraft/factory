package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/webhook"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// The webhook side of 09-billing.md §5.6. Every handler is idempotent on
// event.id, which is the primary key of stripe_events, so a duplicate
// delivery is a no-op and a replay of a recorded fixture reproduces the
// same rows.

// ErrBadSignature is returned when the Stripe-Signature header does not
// verify. The route answers 400 and logs the event type only; the body is
// never logged (it carries customer data).
var ErrBadSignature = errors.New("stripe webhook signature is invalid")

// ErrDuplicate means the event id was already processed.
var ErrDuplicate = errors.New("stripe event already processed")

// The six webhook types §5.6 handles.
const (
	TypeInvoicePaid            = "invoice.paid"
	TypeInvoicePaymentFailed   = "invoice.payment_failed"
	TypeSubscriptionDeleted    = "customer.subscription.deleted"
	TypeSetupIntentSucceeded   = "setup_intent.succeeded"
	TypePaymentMethodDetached  = "payment_method.detached"
	TypeChargeRefunded         = "charge.refunded"
	pastDueGraceDays           = 3
	subscriptionDeletedComment = "subscription deleted at Stripe"
)

// Webhooks applies Stripe events to the database.
type Webhooks struct {
	pool   *db.Pool
	secret string
	log    *slog.Logger
	Now    func() time.Time
	// OnCardAttached runs after a setup_intent.succeeded event so the api
	// can make the payment method the customer's default and create the
	// subscription. It is a hook rather than a direct call so the handler
	// holds no Stripe client of its own and stays testable offline.
	OnCardAttached func(ctx context.Context, userID uuid.UUID, paymentMethod string) error
}

// NewWebhooks builds the handler. secret is STRIPE_WEBHOOK_SECRET.
func NewWebhooks(pool *db.Pool, secret string, log *slog.Logger) *Webhooks {
	return &Webhooks{pool: pool, secret: secret, log: log.With("component", obs.ComponentAPI), Now: time.Now}
}

// Handle verifies the signature, records the event and applies it. A
// duplicate returns ErrDuplicate, which the route answers 200 to, because
// Stripe retries anything else.
func (w *Webhooks) Handle(ctx context.Context, payload []byte, sigHeader string) (kind string, err error) {
	if w.secret == "" {
		return "", ErrDisabled
	}
	ev, err := webhook.ConstructEvent(payload, sigHeader, w.secret)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBadSignature, err)
	}
	// The primary key is the dedupe (§6, "duplicate webhook delivery:
	// ignored by the stripe_events primary key").
	kind = string(ev.Type)
	tag, err := w.pool.Exec(ctx, "insert into stripe_events (id, type) values ($1, $2) on conflict (id) do nothing", ev.ID, kind)
	if err != nil {
		return kind, err
	}
	if tag.RowsAffected() == 0 {
		return kind, ErrDuplicate
	}
	applyErr := w.apply(ctx, &ev)
	if applyErr != nil {
		if _, err := w.pool.Exec(ctx, "update stripe_events set error = $2 where id = $1", ev.ID, applyErr.Error()); err != nil {
			return kind, err
		}
		return kind, applyErr
	}
	if _, err := w.pool.Exec(ctx, "update stripe_events set processed_at = now() where id = $1", ev.ID); err != nil {
		return kind, err
	}
	w.log.Info("stripe webhook applied", "event", obs.EventStripeWebhook, "kind", kind, "stripe_event_id", ev.ID)
	return kind, nil
}

func (w *Webhooks) apply(ctx context.Context, ev *stripe.Event) error {
	switch string(ev.Type) {
	case TypeInvoicePaid:
		return w.invoicePaid(ctx, ev)
	case TypeInvoicePaymentFailed:
		return w.invoiceFailed(ctx, ev)
	case TypeSubscriptionDeleted:
		return w.subscriptionDeleted(ctx, ev)
	case TypeSetupIntentSucceeded:
		return w.setupIntentSucceeded(ctx, ev)
	case TypePaymentMethodDetached:
		return w.paymentMethodDetached(ctx, ev)
	case TypeChargeRefunded:
		return w.chargeRefunded(ctx, ev)
	}
	// An event type we do not handle is recorded and ignored; Stripe sends
	// whatever the endpoint is subscribed to.
	return nil
}

// invoiceObject is the subset of an invoice the handlers read. The full
// object is not unmarshalled into stripe.Invoice because the shape of the
// parent and line items changes between API versions and nothing here
// needs them.
type invoiceObject struct {
	ID           string `json:"id"`
	Customer     string `json:"customer"`
	Total        int64  `json:"total"`
	Status       string `json:"status"`
	PeriodStart  int64  `json:"period_start"`
	PeriodEnd    int64  `json:"period_end"`
	Subscription string `json:"subscription"`
}

func decodeObject[T any](ev *stripe.Event) (T, error) {
	var out T
	if err := json.Unmarshal(ev.Data.Raw, &out); err != nil {
		return out, fmt.Errorf("decode the %s object: %w", string(ev.Type), err)
	}
	return out, nil
}

// userByCustomer resolves the Stripe customer to a user; an unknown
// customer is not an error the endpoint should retry forever, so it is
// reported and the event is marked processed.
func (w *Webhooks) userByCustomer(ctx context.Context, customer string) (*store.User, error) {
	if customer == "" {
		return nil, nil
	}
	rows, err := w.pool.Query(ctx, "select id from users where stripe_customer_id = $1", customer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		w.log.Warn("stripe event for an unknown customer", "event", obs.EventStripeWebhook, "stripe_customer_id", customer)
		return nil, nil
	}
	var id uuid.UUID
	if err := rows.Scan(&id); err != nil {
		return nil, err
	}
	rows.Close()
	return store.GetUser(ctx, w.pool, id)
}

// invoicePaid records the invoice and returns the account to active. Guests
// stay stopped until the user starts them (§5.6).
func (w *Webhooks) invoicePaid(ctx context.Context, ev *stripe.Event) error {
	inv, err := decodeObject[invoiceObject](ev)
	if err != nil {
		return err
	}
	u, err := w.userByCustomer(ctx, inv.Customer)
	if err != nil || u == nil {
		return err
	}
	return db.InTx(ctx, w.pool, func(tx db.Tx) error {
		if err := upsertInvoice(ctx, tx, u.ID, inv, "paid"); err != nil {
			return err
		}
		if inv.Total <= 0 {
			// A zero invoice is "paid" without money moving: the one Stripe
			// issues when the subscription is created at card attach, and
			// every month the trial credit covers. It settles nothing and
			// proves no card, so it neither raises the limits nor clears a
			// failed payment (DECISIONS I-184).
			return nil
		}
		// The first paid invoice raises the limits (§5.8).
		_, err := tx.Exec(ctx, `update users set billing_status = 'active', past_due_since = null,
			suspended_at = case when suspended_reason = 'billing' then null else suspended_at end,
			suspended_reason = case when suspended_reason = 'billing' then null else suspended_reason end,
			project_limit = greatest(project_limit, $2), xl_limit = greatest(xl_limit, $3)
			where id = $1 and billing_status <> 'exempt'`, u.ID, ProjectLimitPaid, XLLimitPaid)
		return err
	})
}

// invoiceFailed marks the account past due and starts the 3-day clock.
func (w *Webhooks) invoiceFailed(ctx context.Context, ev *stripe.Event) error {
	inv, err := decodeObject[invoiceObject](ev)
	if err != nil {
		return err
	}
	u, err := w.userByCustomer(ctx, inv.Customer)
	if err != nil || u == nil {
		return err
	}
	return db.InTx(ctx, w.pool, func(tx db.Tx) error {
		if err := upsertInvoice(ctx, tx, u.ID, inv, "payment_failed"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `update users set billing_status = 'past_due', past_due_since = coalesce(past_due_since, now())
			where id = $1 and billing_status not in ('exempt','suspended')`, u.ID)
		return err
	})
}

func upsertInvoice(ctx context.Context, q store.Querier, userID uuid.UUID, inv invoiceObject, status string) error {
	var start, end *time.Time
	if inv.PeriodStart > 0 {
		t := time.Unix(inv.PeriodStart, 0).UTC()
		start = &t
	}
	if inv.PeriodEnd > 0 {
		t := time.Unix(inv.PeriodEnd, 0).UTC()
		end = &t
	}
	if inv.Status != "" {
		status = inv.Status
	}
	_, err := q.Exec(ctx, `insert into invoices (id, user_id, stripe_invoice_id, period_start, period_end, total_cents, status)
		values ($1,$2,$3,$4,$5,$6,$7)
		on conflict (stripe_invoice_id) do update set period_start = excluded.period_start, period_end = excluded.period_end,
		total_cents = excluded.total_cents, status = excluded.status`,
		store.NewID(), userID, inv.ID, start, end, inv.Total, status)
	if err != nil {
		return fmt.Errorf("record invoice %s: %w", inv.ID, err)
	}
	return nil
}

// subscriptionDeleted drops the stored subscription id; a later card
// attach creates a new one.
func (w *Webhooks) subscriptionDeleted(ctx context.Context, ev *stripe.Event) error {
	sub, err := decodeObject[struct {
		ID       string `json:"id"`
		Customer string `json:"customer"`
	}](ev)
	if err != nil {
		return err
	}
	u, err := w.userByCustomer(ctx, sub.Customer)
	if err != nil || u == nil {
		return err
	}
	if _, err := w.pool.Exec(ctx, "update users set stripe_subscription_id = null where id = $1 and stripe_subscription_id = $2", u.ID, sub.ID); err != nil {
		return err
	}
	_, err = store.Audit(ctx, w.pool, "stripe", "subscription_deleted", u.Handle, map[string]any{"detail": subscriptionDeletedComment})
	return err
}

// setupIntentSucceeded attaches the payment method as the customer's
// default and flips has_card (§5.2). Attaching is the caller's job when a
// Stripe client is available; here the database side is applied so the
// webhook is meaningful in tests and with a portal-collected card.
func (w *Webhooks) setupIntentSucceeded(ctx context.Context, ev *stripe.Event) error {
	si, err := decodeObject[struct {
		ID            string `json:"id"`
		Customer      string `json:"customer"`
		PaymentMethod string `json:"payment_method"`
	}](ev)
	if err != nil {
		return err
	}
	u, err := w.userByCustomer(ctx, si.Customer)
	if err != nil || u == nil {
		return err
	}
	// A trial account whose credit ran out while it had no card (removed
	// while its guests kept running, §6) stayed `trial` at zero, because
	// the end of the trial needs a card. The card arriving ends it here, or
	// the gate would refuse its next start as trial_depleted with a card on
	// file (DECISIONS I-184).
	if _, err := w.pool.Exec(ctx, `update users set has_card = true, billing_anchor = coalesce(billing_anchor, now()),
		billing_status = case when billing_status = 'trial'
			and coalesce((select sum(cents) from credit_ledger where user_id = $1), 0) <= 0 then 'active' else billing_status end
		where id = $1`, u.ID); err != nil {
		return err
	}
	if w.OnCardAttached != nil {
		return w.OnCardAttached(ctx, u.ID, si.PaymentMethod)
	}
	return nil
}

func (w *Webhooks) paymentMethodDetached(ctx context.Context, ev *stripe.Event) error {
	pm, err := decodeObject[struct {
		ID       string `json:"id"`
		Customer string `json:"customer"`
	}](ev)
	if err != nil {
		return err
	}
	// A detached payment method carries a null customer, so the previous
	// attributes are where the owner is named.
	customer := pm.Customer
	if customer == "" && ev.Data != nil {
		if prev, ok := ev.Data.PreviousAttributes["customer"].(string); ok {
			customer = prev
		}
	}
	u, err := w.userByCustomer(ctx, customer)
	if err != nil || u == nil {
		return err
	}
	// Guests keep running until the invoice fails (§6, "card removed while
	// guests run"); only starts are blocked.
	_, err = w.pool.Exec(ctx, "update users set has_card = false where id = $1", u.ID)
	return err
}

// chargeRefunded records the refund as a credit row, which is the ledger
// the reconciliation reads (§5.9, "refunds are manual in the Stripe
// dashboard plus a credit row for the record").
func (w *Webhooks) chargeRefunded(ctx context.Context, ev *stripe.Event) error {
	ch, err := decodeObject[struct {
		ID             string `json:"id"`
		Customer       string `json:"customer"`
		AmountRefunded int64  `json:"amount_refunded"`
	}](ev)
	if err != nil {
		return err
	}
	u, err := w.userByCustomer(ctx, ch.Customer)
	if err != nil || u == nil {
		return err
	}
	if ch.AmountRefunded <= 0 {
		return nil
	}
	var exists bool
	if err := w.pool.QueryRow(ctx, "select exists (select 1 from credit_ledger where ref = $1 and reason = $2)", "refund:"+ch.ID, ReasonRefund).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = Credit(ctx, w.pool, u.ID, ch.AmountRefunded, ReasonRefund, "refund:"+ch.ID)
	return err
}
