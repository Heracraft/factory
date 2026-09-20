package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v83"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// The Stripe side of 09-billing.md §5.2 and §5.5: one customer per user,
// one subscription with three metered prices created at first card attach,
// and one meter event per cost part per usage_hours row.
//
// DECISIONS I-61: Stripe retired the `usage_type=metered` /
// `aggregate_usage=sum` subscription-item usage records §5.5 was written
// against, so the three amounts travel as billing meter events with an
// `identifier` of `usage:<project_id>:<hour>:<part>`. That identifier is
// the idempotency key §5.5 asks for, and a row whose
// `stripe_usage_record_id` is set is never pushed again.

// Stripe is the configured client. It satisfies UsagePusher, Portal and
// Reader.
type Stripe struct {
	c    *stripe.Client
	cfg  Config
	pool *db.Pool
	log  *slog.Logger
	Now  func() time.Time
}

var (
	_ UsagePusher = (*Stripe)(nil)
	_ Portal      = (*Stripe)(nil)
	_ Reader      = (*Stripe)(nil)
)

// NewStripe builds the client. opts are passed to stripe.NewClient, which
// is how tests point it at an httptest server.
func NewStripe(cfg Config, pool *db.Pool, log *slog.Logger, opts ...stripe.ClientOption) (*Stripe, error) {
	if cfg.SecretKey == "" {
		return nil, ErrDisabled
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Stripe{c: stripe.NewClient(cfg.SecretKey, opts...), cfg: cfg, pool: pool, log: log.With("component", obs.ComponentAPI), Now: time.Now}, nil
}

// Config exposes the configuration the webhook handler and the api need.
func (s *Stripe) Config() Config { return s.cfg }

func str(v string) *string { return &v }

// --- customer and subscription ---------------------------------------

// EnsureCustomer creates the Stripe customer at first sight of a user and
// stores its id (§5.2). It is idempotent: a user with an id gets it back.
func (s *Stripe) EnsureCustomer(ctx context.Context, userID uuid.UUID) (string, error) {
	u, err := store.GetUser(ctx, s.pool, userID)
	if err != nil {
		return "", err
	}
	if u.BillingStatus == "exempt" {
		// An exempt account is never pushed to Stripe (DECISIONS I-16).
		return "", ErrDisabled
	}
	if u.StripeCustomerID != nil && *u.StripeCustomerID != "" {
		return *u.StripeCustomerID, nil
	}
	params := &stripe.CustomerCreateParams{
		Metadata: map[string]string{"user_id": u.ID.String(), "handle": u.Handle},
	}
	if u.Email != nil && *u.Email != "" {
		params.Email = u.Email
	}
	// The user id is the idempotency key, so a retried /me does not create
	// a second customer for the same person.
	params.SetIdempotencyKey("customer:" + u.ID.String())
	cus, err := s.c.V1Customers.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create the Stripe customer: %w", err)
	}
	if _, err := s.pool.Exec(ctx, "update users set stripe_customer_id = $2, billing_anchor = coalesce(billing_anchor, created_at) where id = $1", u.ID, cus.ID); err != nil {
		return "", err
	}
	s.log.Info("stripe customer created", "event", obs.EventStripeWebhook, "user_id", u.ID.String(), "action", "customer_create")
	return cus.ID, nil
}

// EnsureSubscription creates the user's subscription with the three
// metered prices, anchored now (§5.5). Called when a card is attached.
func (s *Stripe) EnsureSubscription(ctx context.Context, userID uuid.UUID) (string, error) {
	u, err := store.GetUser(ctx, s.pool, userID)
	if err != nil {
		return "", err
	}
	if u.StripeSubscriptionID != nil && *u.StripeSubscriptionID != "" {
		return *u.StripeSubscriptionID, nil
	}
	customer, err := s.EnsureCustomer(ctx, userID)
	if err != nil {
		return "", err
	}
	params := &stripe.SubscriptionCreateParams{
		Customer: str(customer),
		Items: []*stripe.SubscriptionCreateItemParams{
			{Price: str(s.cfg.PriceCompute)},
			{Price: str(s.cfg.PriceStorage)},
			{Price: str(s.cfg.PriceEgress)},
		},
		Metadata: map[string]string{"user_id": u.ID.String()},
	}
	if s.cfg.AutomaticTax {
		// §5.9: Stripe Tax on the subscription, no code beyond this.
		params.AutomaticTax = &stripe.SubscriptionCreateAutomaticTaxParams{Enabled: stripe.Bool(true)}
	}
	params.SetIdempotencyKey("subscription:" + u.ID.String())
	sub, err := s.c.V1Subscriptions.Create(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create the Stripe subscription: %w", err)
	}
	if _, err := s.pool.Exec(ctx, "update users set stripe_subscription_id = $2, billing_anchor = coalesce(billing_anchor, now()) where id = $1", u.ID, sub.ID); err != nil {
		return "", err
	}
	return sub.ID, nil
}

// OnCardAttached is what the setup_intent.succeeded webhook calls: it
// makes the new payment method the customer's default and creates the
// subscription whose period anchors the billing month (§5.2, §5.5).
func (s *Stripe) OnCardAttached(ctx context.Context, userID uuid.UUID, paymentMethod string) error {
	customer, err := s.EnsureCustomer(ctx, userID)
	if err != nil {
		return err
	}
	if paymentMethod != "" {
		_, err := s.c.V1Customers.Update(ctx, customer, &stripe.CustomerUpdateParams{
			InvoiceSettings: &stripe.CustomerUpdateInvoiceSettingsParams{DefaultPaymentMethod: str(paymentMethod)},
		})
		if err != nil {
			return fmt.Errorf("set the default payment method: %w", err)
		}
	}
	_, err = s.EnsureSubscription(ctx, userID)
	return err
}

// --- Portal ----------------------------------------------------------

// SetupIntent creates the SetupIntent whose client secret the dashboard's
// card form uses (§5.2).
func (s *Stripe) SetupIntent(ctx context.Context, userID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return "", fmt.Errorf("setup intent: %w", err)
	}
	customer, err := s.EnsureCustomer(ctx, id)
	if err != nil {
		return "", err
	}
	si, err := s.c.V1SetupIntents.Create(ctx, &stripe.SetupIntentCreateParams{
		Customer:           str(customer),
		Usage:              str("off_session"),
		PaymentMethodTypes: []*string{str("card")},
		Metadata:           map[string]string{"user_id": userID},
	})
	if err != nil {
		return "", fmt.Errorf("create the SetupIntent: %w", err)
	}
	return si.ClientSecret, nil
}

// PortalURL opens a Stripe customer portal session.
func (s *Stripe) PortalURL(ctx context.Context, userID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return "", fmt.Errorf("billing portal: %w", err)
	}
	customer, err := s.EnsureCustomer(ctx, id)
	if err != nil {
		return "", err
	}
	sess, err := s.c.V1BillingPortalSessions.Create(ctx, &stripe.BillingPortalSessionCreateParams{
		Customer: str(customer), ReturnURL: str(s.cfg.PortalReturnURL),
	})
	if err != nil {
		return "", fmt.Errorf("create the billing portal session: %w", err)
	}
	return sess.URL, nil
}

// Invoices lists the user's invoices as the dashboard shows them.
func (s *Stripe) Invoices(ctx context.Context, userID string) ([]map[string]any, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invoices: %w", err)
	}
	u, err := store.GetUser(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	if u.StripeCustomerID == nil || *u.StripeCustomerID == "" {
		return out, nil
	}
	params := &stripe.InvoiceListParams{Customer: u.StripeCustomerID}
	params.Limit = stripe.Int64(24)
	for inv, err := range s.c.V1Invoices.List(ctx, params) {
		if err != nil {
			return nil, fmt.Errorf("list invoices: %w", err)
		}
		out = append(out, map[string]any{
			"id": inv.ID, "status": string(inv.Status), "total_cents": inv.Total,
			"currency": string(inv.Currency), "created": time.Unix(inv.Created, 0).UTC(),
			"period_start": time.Unix(inv.PeriodStart, 0).UTC(), "period_end": time.Unix(inv.PeriodEnd, 0).UTC(),
			"hosted_invoice_url": inv.HostedInvoiceURL, "pdf": inv.InvoicePDF,
		})
	}
	return out, nil
}

// --- UsagePusher -----------------------------------------------------

// meterPart names one of the three lines of §5.5.
type meterPart struct {
	name  string
	meter string
	cents int64
}

// PushUsage sends the hour's three cost parts as billing meter events. The
// identifier makes the push idempotent at Stripe's end as well as ours, so
// a retry after a timeout cannot double-bill.
func (s *Stripe) PushUsage(ctx context.Context, row UsageRow) (string, error) {
	if row.CustomerID == "" {
		return "", errors.New("push usage: the user has no Stripe customer")
	}
	base := "usage:" + row.ProjectID + ":" + row.Hour.UTC().Format(time.RFC3339)
	ts := row.Hour.UTC().Unix()
	parts := []meterPart{
		{"compute", s.cfg.MeterCompute, row.GuestCents},
		{"storage", s.cfg.MeterStorage, row.StorageCents},
		{"egress", s.cfg.MeterEgress, row.EgressCents},
	}
	for _, p := range parts {
		if p.cents <= 0 {
			continue
		}
		params := &stripe.BillingMeterEventCreateParams{
			EventName:  str(p.meter),
			Identifier: str(base + ":" + p.name),
			Timestamp:  stripe.Int64(ts),
			Payload: map[string]string{
				"stripe_customer_id": row.CustomerID,
				"value":              strconv.FormatInt(p.cents, 10),
				"project_id":         row.ProjectID,
			},
		}
		params.SetIdempotencyKey(base + ":" + p.name)
		if _, err := s.c.V1BillingMeterEvents.Create(ctx, params); err != nil {
			return "", fmt.Errorf("push the %s meter event for %s: %w", p.name, base, err)
		}
	}
	return base, nil
}

// --- Reader ----------------------------------------------------------

// PeriodSummary reads back what Stripe has recorded for a customer over a
// period, which is the right-hand side of the reconciliation (§5.7).
// Meter ids are needed for this call; without them it returns ErrDisabled
// so `reconcile` says the comparison could not be made rather than
// reporting a mismatch against zero.
func (s *Stripe) PeriodSummary(ctx context.Context, customerID string, from, to time.Time) (Summary, error) {
	var sum Summary
	if s.cfg.MeterIDCompute == "" || s.cfg.MeterIDStorage == "" || s.cfg.MeterIDEgress == "" {
		return sum, ErrDisabled
	}
	for _, m := range []struct {
		id  string
		out *int64
	}{
		{s.cfg.MeterIDCompute, &sum.GuestCents},
		{s.cfg.MeterIDStorage, &sum.StorageCents},
		{s.cfg.MeterIDEgress, &sum.EgressCents},
	} {
		params := &stripe.BillingMeterEventSummaryListParams{
			ID:        str(m.id),
			Customer:  str(customerID),
			StartTime: stripe.Int64(from.UTC().Truncate(time.Minute).Unix()),
			EndTime:   stripe.Int64(to.UTC().Truncate(time.Minute).Unix()),
		}
		var total float64
		for es, err := range s.c.V1BillingMeterEventSummaries.List(ctx, params) {
			if err != nil {
				return sum, fmt.Errorf("read the meter %s summary: %w", m.id, err)
			}
			total += es.AggregatedValue
		}
		*m.out = int64(total + 0.5)
	}
	return sum, nil
}
