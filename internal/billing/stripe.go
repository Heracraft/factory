package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
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
// DECISIONS I-77: Stripe retired the `usage_type=metered` /
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
	// Stripe is asked before anything is created (DECISIONS I-181): a live
	// subscription on our compute price is adopted rather than doubled, since
	// two subscriptions on the same meters would each invoice the same
	// usage. That covers a webhook retried after Stripe created the
	// subscription but before the id reached the database. The idempotency
	// key carries how many subscriptions the customer has ever had, so a
	// retry reuses it while a new subscription after a deleted one (within
	// Stripe's 24-hour key window) does not get the deleted one back.
	existing, generation, err := s.findSubscription(ctx, customer)
	if err != nil {
		return "", err
	}
	sub := existing
	if sub == nil {
		if sub, err = s.createSubscription(ctx, u.ID, generation, params); err != nil {
			return "", err
		}
	}
	// The period the rollup prices in is Stripe's, not the signup time the
	// customer was created with: the cap, the storage spread and the egress
	// allowance must reset exactly when the invoice does (DECISIONS I-179).
	// It is stored truncated to the hour, because usage_hours rows are
	// hours; PushUsage stamps each row at its last second, so an hour lands
	// in the same period on both sides (see meterTimestamp).
	anchor := s.Now().UTC()
	if sub.BillingCycleAnchor > 0 {
		anchor = time.Unix(sub.BillingCycleAnchor, 0).UTC()
	}
	anchor = anchor.Truncate(time.Hour)
	if _, err := s.pool.Exec(ctx, "update users set stripe_subscription_id = $2, billing_anchor = $3 where id = $1", u.ID, sub.ID, anchor); err != nil {
		return "", err
	}
	return sub.ID, nil
}

// createSubscription creates the subscription, falling back to one without
// Stripe Tax when Stripe refuses automatic tax (no customer address it can
// locate, or Stripe Tax not activated on the account). A card must never be
// refused because of tax configuration: the charge is still correct, the
// invoice simply carries no tax line, and the log line says so
// (DECISIONS I-181).
func (s *Stripe) createSubscription(ctx context.Context, userID uuid.UUID, generation int, params *stripe.SubscriptionCreateParams) (*stripe.Subscription, error) {
	key := "subscription:" + userID.String() + ":" + strconv.Itoa(generation)
	params.SetIdempotencyKey(key)
	sub, err := s.c.V1Subscriptions.Create(ctx, params)
	if err == nil {
		return sub, nil
	}
	if params.AutomaticTax == nil || !isTaxRefusal(err) {
		return nil, fmt.Errorf("create the Stripe subscription: %w", err)
	}
	s.log.Warn("Stripe refused automatic tax; subscription created without it", "event", obs.EventStripeWebhook,
		"user_id", userID.String(), "action", "subscription_create_no_tax", "stripe_code", stripeCode(err))
	params.AutomaticTax = nil
	params.SetIdempotencyKey(key + ":no-tax")
	sub, err = s.c.V1Subscriptions.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create the Stripe subscription: %w", err)
	}
	return sub, nil
}

// findSubscription returns the customer's live subscription on the compute
// price, if there is one, and how many subscriptions the customer has had
// in any state.
func (s *Stripe) findSubscription(ctx context.Context, customer string) (*stripe.Subscription, int, error) {
	var live *stripe.Subscription
	n := 0
	for sub, err := range s.c.V1Subscriptions.List(ctx, &stripe.SubscriptionListParams{Customer: str(customer), Status: str("all")}) {
		if err != nil {
			return nil, 0, fmt.Errorf("list the customer's subscriptions: %w", err)
		}
		n++
		if live != nil || sub.Status == stripe.SubscriptionStatusCanceled || sub.Status == stripe.SubscriptionStatusIncompleteExpired || sub.Items == nil {
			continue
		}
		for _, it := range sub.Items.Data {
			if it.Price != nil && it.Price.ID == s.cfg.PriceCompute {
				live = sub
				break
			}
		}
	}
	return live, n, nil
}

// isTaxRefusal reports whether a Stripe error is about automatic tax: the
// customer's location cannot be determined, or Stripe Tax is not set up.
func isTaxRefusal(err error) bool {
	var se *stripe.Error
	if !errors.As(err, &se) {
		return false
	}
	if se.Code == "customer_tax_location_invalid" {
		return true
	}
	msg := strings.ToLower(se.Msg)
	return strings.Contains(msg, "automatic_tax") || strings.Contains(msg, "stripe tax") || strings.Contains(se.Param, "automatic_tax")
}

func stripeCode(err error) string {
	var se *stripe.Error
	if errors.As(err, &se) {
		return string(se.Code)
	}
	return ""
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
		upd := &stripe.CustomerUpdateParams{
			InvoiceSettings: &stripe.CustomerUpdateInvoiceSettingsParams{DefaultPaymentMethod: str(paymentMethod)},
		}
		// Stripe Tax locates the customer by customer.address. A card form
		// collects the billing address on the payment method, so it is
		// copied onto the customer before the subscription asks for
		// automatic tax (§5.9). Checkout already does this itself
		// (customer_update.address=auto); a card without an address leaves
		// the customer as it is.
		if pm, err := s.c.V1PaymentMethods.Retrieve(ctx, paymentMethod, nil); err == nil && pm.BillingDetails != nil &&
			pm.BillingDetails.Address != nil && pm.BillingDetails.Address.Country != "" {
			a := pm.BillingDetails.Address
			upd.Address = &stripe.AddressParams{City: str(a.City), Country: str(a.Country), Line1: str(a.Line1),
				Line2: str(a.Line2), PostalCode: str(a.PostalCode), State: str(a.State)}
		}
		if _, err := s.c.V1Customers.Update(ctx, customer, upd); err != nil {
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

// SetupCheckout creates a Stripe Checkout session in setup mode and returns
// its URL (DECISIONS I-182). The dashboard sends the user there to add a
// card: Stripe hosts the form, collects the full billing address Stripe Tax
// needs and saves it on the customer, and the SetupIntent it confirms fires
// the same setup_intent.succeeded webhook as an embedded form would, so
// OnCardAttached runs either way. No publishable key is needed anywhere.
func (s *Stripe) SetupCheckout(ctx context.Context, userID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return "", fmt.Errorf("setup checkout: %w", err)
	}
	customer, err := s.EnsureCustomer(ctx, id)
	if err != nil {
		return "", err
	}
	back := s.cfg.PortalReturnURL
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	sess, err := s.c.V1CheckoutSessions.Create(ctx, &stripe.CheckoutSessionCreateParams{
		Mode:                     str("setup"),
		Customer:                 str(customer),
		PaymentMethodTypes:       []*string{str("card")},
		BillingAddressCollection: str("required"),
		CustomerUpdate:           &stripe.CheckoutSessionCreateCustomerUpdateParams{Address: str("auto"), Name: str("auto")},
		SetupIntentData:          &stripe.CheckoutSessionCreateSetupIntentDataParams{Metadata: map[string]string{"user_id": userID}},
		Metadata:                 map[string]string{"user_id": userID},
		SuccessURL:               str(back + sep + "card=saved"),
		CancelURL:                str(back + sep + "card=cancelled"),
	})
	if err != nil {
		return "", fmt.Errorf("create the card setup checkout session: %w", err)
	}
	return sess.URL, nil
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
	params := &stripe.BillingPortalSessionCreateParams{Customer: str(customer), ReturnURL: str(s.cfg.PortalReturnURL)}
	if s.cfg.PortalConfiguration != "" {
		// The configuration `stripe-bootstrap` created; without it Stripe
		// uses the account's default, which a fresh account does not have.
		params.Configuration = str(s.cfg.PortalConfiguration)
	}
	sess, err := s.c.V1BillingPortalSessions.Create(ctx, params)
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
		out = append(out, invoiceView(inv))
	}
	return out, nil
}

// invoiceView is one element of GET /billing/invoices (docs/interfaces/
// api.md). The first names are the documented ones the dashboard reads;
// `total_cents`, `created`, `hosted_invoice_url` and `pdf` are the names
// this route answered with before, kept for one release (DECISIONS I-183).
func invoiceView(inv *stripe.Invoice) map[string]any {
	created := time.Unix(inv.Created, 0).UTC()
	return map[string]any{
		"id": inv.ID, "number": inv.Number, "status": string(inv.Status), "currency": string(inv.Currency),
		"amount_cents": inv.Total, "subtotal_cents": inv.Subtotal, "tax_cents": inv.Total - inv.TotalExcludingTax,
		"created_at": created, "period_start": time.Unix(inv.PeriodStart, 0).UTC(), "period_end": time.Unix(inv.PeriodEnd, 0).UTC(),
		"hosted_url": inv.HostedInvoiceURL, "pdf_url": inv.InvoicePDF,
		"total_cents": inv.Total, "created": created, "hosted_invoice_url": inv.HostedInvoiceURL, "pdf": inv.InvoicePDF,
	}
}

// --- UsagePusher -----------------------------------------------------

// meterTimestamp is the time a usage_hours row is reported at: the last
// second of its hour. Stripe's period starts at the subscription's exact
// anchor (say 14:32:10) while the rollup's starts at that hour (14:00), so
// an event stamped at the start of the hour would put the first hour
// before the subscription existed and the anchor-day hour of every later
// month in the previous invoice. Stamped at hh:59:59, an hour falls in
// Stripe's period exactly when it falls in the rollup's (DECISIONS I-179).
func meterTimestamp(hour time.Time) int64 {
	return hour.UTC().Truncate(time.Hour).Add(time.Hour - time.Second).Unix()
}

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
	ts := meterTimestamp(row.Hour)
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
