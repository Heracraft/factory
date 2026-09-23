package billing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v83"
)

// Bootstrap creates, or finds, every Stripe object the api needs
// (09-billing.md §5.5, docs/ops/AZURE-SETUP.md step 17) from nothing but a
// secret key, and returns the STRIPE_* block the api reads. It is what
// `repose-admin billing stripe-bootstrap` runs (DECISIONS I-180).
//
// Every object is found before it is created, by a key Stripe itself holds
// (the product's fixed id, the meters' event names, the prices' lookup
// keys, the portal configuration's metadata, the endpoint's URL), so a
// second run changes nothing and prints the same block. The one thing a
// second run cannot print is the webhook signing secret, which Stripe
// returns only when an endpoint is created; the block says so, and
// RotateWebhook replaces the endpoint to get a new one.

// Stripe object keys the bootstrap finds its objects by.
const (
	BootstrapProductID    = "repose"
	bootstrapMetadataKey  = "repose"
	bootstrapMetadataMark = "stripe-bootstrap"
)

// WebhookEvents are the six events §5.6 handles, which is exactly what the
// endpoint is subscribed to.
var WebhookEvents = []string{
	TypeInvoicePaid, TypeInvoicePaymentFailed, TypeSubscriptionDeleted,
	TypeSetupIntentSucceeded, TypePaymentMethodDetached, TypeChargeRefunded,
}

// ErrLiveKey refuses a live-mode key without the explicit flag.
var ErrLiveKey = errors.New("this is a live-mode Stripe key; pass --live to bootstrap live mode on purpose")

// BootstrapOptions steer one run.
type BootstrapOptions struct {
	// WebhookURL is the api's public webhook route, e.g.
	// https://api.repose.herakraft.co/v1/billing/webhook. Empty skips the
	// endpoint (the Stripe test-mode gate test has no public URL).
	WebhookURL string
	// DashboardURL is where the customer portal returns to and whose
	// /terms and /privacy it links.
	DashboardURL string
	// AllowLive permits a live-mode key.
	AllowLive bool
	// RotateWebhook deletes an existing endpoint at WebhookURL and creates
	// a new one, which is the only way to learn a signing secret again.
	RotateWebhook bool
	// Out receives one line per object found or created; nil discards.
	Out io.Writer
}

// BootstrapMeter is one of the three meters with its price.
type BootstrapMeter struct {
	Part      string // compute, storage, egress
	EventName string
	MeterID   string
	PriceID   string
	LookupKey string
}

// BootstrapResult is what the run found or made.
type BootstrapResult struct {
	Live                bool
	SecretKey           string
	ProductID           string
	Meters              []BootstrapMeter
	PortalConfiguration string
	WebhookEndpointID   string
	// WebhookSecret is empty when the endpoint already existed.
	WebhookSecret string
	TaxStatus     string
	Created       []string
	At            time.Time
}

// KeyMode reports whether a Stripe secret or restricted key is live, and
// refuses anything that is neither kind.
func KeyMode(key string) (live bool, err error) {
	switch {
	case strings.HasPrefix(key, "sk_test_"), strings.HasPrefix(key, "rk_test_"):
		return false, nil
	case strings.HasPrefix(key, "sk_live_"), strings.HasPrefix(key, "rk_live_"):
		return true, nil
	}
	return false, errors.New("STRIPE_SECRET_KEY must be a Stripe secret key (sk_test_... or sk_live_...)")
}

type bootstrapper struct {
	c    *stripe.Client
	opts BootstrapOptions
	res  *BootstrapResult
}

func (b *bootstrapper) say(format string, a ...any) {
	if b.opts.Out != nil {
		_, _ = fmt.Fprintf(b.opts.Out, format+"\n", a...)
	}
}

func (b *bootstrapper) created(what string) {
	b.res.Created = append(b.res.Created, what)
	b.say("created  %s", what)
}

// Bootstrap runs against Stripe with key; opts are passed to
// stripe.NewClient, which is how tests point it at a fake.
func Bootstrap(ctx context.Context, key string, o BootstrapOptions, opts ...stripe.ClientOption) (*BootstrapResult, error) {
	live, err := KeyMode(key)
	if err != nil {
		return nil, err
	}
	if live && !o.AllowLive {
		return nil, ErrLiveKey
	}
	if o.DashboardURL == "" {
		o.DashboardURL = "https://repose.herakraft.co"
	}
	o.DashboardURL = strings.TrimRight(o.DashboardURL, "/")
	if o.WebhookURL != "" {
		u, err := url.Parse(o.WebhookURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("the webhook URL must be an https URL, got %q", o.WebhookURL)
		}
	}
	b := &bootstrapper{c: stripe.NewClient(key, opts...), opts: o,
		res: &BootstrapResult{Live: live, SecretKey: key, At: time.Now().UTC()}}
	for _, step := range []func(context.Context) error{b.product, b.meters, b.prices, b.portal, b.webhook, b.tax} {
		if err := step(ctx); err != nil {
			return b.res, err
		}
	}
	return b.res, nil
}

func isMissing(err error) bool {
	var se *stripe.Error
	return errors.As(err, &se) && (se.Code == stripe.ErrorCodeResourceMissing || se.HTTPStatusCode == 404)
}

func (b *bootstrapper) product(ctx context.Context) error {
	// Listed by id rather than retrieved, so a first run does not make the
	// Stripe library log a 404 that looks like a failure.
	var p *stripe.Product
	var err error
	for got, lerr := range b.c.V1Products.List(ctx, &stripe.ProductListParams{IDs: []*string{str(BootstrapProductID)}}) {
		if lerr != nil {
			err = lerr
			break
		}
		p = got
	}
	if err == nil && p == nil {
		err = &stripe.Error{Code: stripe.ErrorCodeResourceMissing, HTTPStatusCode: 404}
	}
	switch {
	case err == nil:
		if !p.Active {
			if _, err := b.c.V1Products.Update(ctx, p.ID, &stripe.ProductUpdateParams{Active: stripe.Bool(true)}); err != nil {
				return fmt.Errorf("reactivate product %s: %w", p.ID, err)
			}
			b.say("reactivated product %s", p.ID)
		} else {
			b.say("found    product %s", p.ID)
		}
	case isMissing(err):
		params := &stripe.ProductCreateParams{ID: str(BootstrapProductID), Name: str("Repose"),
			Description: str("Persistent remote environments for coding agents, billed per hour of use"),
			Metadata:    map[string]string{bootstrapMetadataKey: bootstrapMetadataMark}}
		params.SetIdempotencyKey("bootstrap:product:" + BootstrapProductID)
		p, err = b.c.V1Products.Create(ctx, params)
		if err != nil {
			return fmt.Errorf("create product %s: %w", BootstrapProductID, err)
		}
		b.created("product " + p.ID)
	default:
		return fmt.Errorf("read product %s: %w", BootstrapProductID, err)
	}
	b.res.ProductID = p.ID
	return nil
}

func (b *bootstrapper) meters(ctx context.Context) error {
	existing := map[string]*stripe.BillingMeter{}
	for m, err := range b.c.V1BillingMeters.List(ctx, &stripe.BillingMeterListParams{}) {
		if err != nil {
			return fmt.Errorf("list meters: %w", err)
		}
		// An active meter wins over an inactive one with the same name.
		if prev, ok := existing[m.EventName]; !ok || prev.Status != stripe.BillingMeterStatusActive {
			existing[m.EventName] = m
		}
	}
	for _, part := range []struct{ part, event string }{
		{"compute", DefaultMeterCompute}, {"storage", DefaultMeterStorage}, {"egress", DefaultMeterEgress},
	} {
		m := existing[part.event]
		if m != nil {
			if err := checkMeter(m); err != nil {
				return err
			}
			if m.Status != stripe.BillingMeterStatusActive {
				if _, err := b.c.V1BillingMeters.Reactivate(ctx, m.ID, nil); err != nil {
					return fmt.Errorf("reactivate meter %s: %w", m.ID, err)
				}
				b.say("reactivated meter %s (%s)", m.ID, part.event)
			} else {
				b.say("found    meter %s (%s)", m.ID, part.event)
			}
		} else {
			params := &stripe.BillingMeterCreateParams{
				DisplayName:        str("Repose " + part.part + " (cents)"),
				EventName:          str(part.event),
				DefaultAggregation: &stripe.BillingMeterCreateDefaultAggregationParams{Formula: str("sum")},
				CustomerMapping:    &stripe.BillingMeterCreateCustomerMappingParams{EventPayloadKey: str("stripe_customer_id"), Type: str("by_id")},
				ValueSettings:      &stripe.BillingMeterCreateValueSettingsParams{EventPayloadKey: str("value")},
			}
			params.SetIdempotencyKey("bootstrap:meter:" + part.event)
			var err error
			m, err = b.c.V1BillingMeters.Create(ctx, params)
			if err != nil {
				return fmt.Errorf("create meter %s: %w", part.event, err)
			}
			b.created("meter " + m.ID + " (" + part.event + ")")
		}
		b.res.Meters = append(b.res.Meters, BootstrapMeter{Part: part.part, EventName: part.event, MeterID: m.ID,
			LookupKey: "repose_" + part.part + "_cents_" + PriceVersion})
	}
	return nil
}

// checkMeter refuses a meter that exists under our event name but would
// aggregate differently from what PushUsage sends: summing `value` per
// `stripe_customer_id` is the contract (§5.5).
func checkMeter(m *stripe.BillingMeter) error {
	var bad []string
	if m.DefaultAggregation == nil || m.DefaultAggregation.Formula != "sum" {
		bad = append(bad, "aggregation is not sum")
	}
	if m.CustomerMapping == nil || m.CustomerMapping.EventPayloadKey != "stripe_customer_id" {
		bad = append(bad, "customer key is not stripe_customer_id")
	}
	if m.ValueSettings == nil || m.ValueSettings.EventPayloadKey != "value" {
		bad = append(bad, "value key is not value")
	}
	if len(bad) > 0 {
		return fmt.Errorf("meter %s (%s) exists but %s; deactivate it in the Stripe dashboard and rerun", m.ID, m.EventName, strings.Join(bad, ", "))
	}
	return nil
}

func (b *bootstrapper) prices(ctx context.Context) error {
	params := &stripe.PriceListParams{Active: stripe.Bool(true)}
	for _, m := range b.res.Meters {
		params.LookupKeys = append(params.LookupKeys, str(m.LookupKey))
	}
	found := map[string]*stripe.Price{}
	for p, err := range b.c.V1Prices.List(ctx, params) {
		if err != nil {
			return fmt.Errorf("list prices: %w", err)
		}
		found[p.LookupKey] = p
	}
	for i := range b.res.Meters {
		m := &b.res.Meters[i]
		if p := found[m.LookupKey]; p != nil {
			if p.UnitAmount != 1 || p.Currency != stripe.CurrencyUSD || p.Recurring == nil || p.Recurring.Meter != m.MeterID ||
				p.Recurring.UsageType != stripe.PriceRecurringUsageTypeMetered || p.Recurring.Interval != stripe.PriceRecurringIntervalMonth {
				return fmt.Errorf("price %s (lookup key %s) is not 1 USD cent per unit, monthly, metered on %s; archive it and rerun (a price is never edited, 09-billing.md §8)",
					p.ID, m.LookupKey, m.MeterID)
			}
			m.PriceID = p.ID
			b.say("found    price %s (%s)", p.ID, m.LookupKey)
			continue
		}
		cp := &stripe.PriceCreateParams{
			Product: str(b.res.ProductID), Currency: str("usd"), UnitAmount: stripe.Int64(1), BillingScheme: str("per_unit"),
			LookupKey: str(m.LookupKey), Nickname: str("Repose " + m.Part + ", 1 cent per unit"),
			Recurring: &stripe.PriceCreateRecurringParams{Interval: str("month"), UsageType: str("metered"), Meter: str(m.MeterID)},
			Metadata:  map[string]string{bootstrapMetadataKey: bootstrapMetadataMark, "part": m.Part},
		}
		cp.SetIdempotencyKey("bootstrap:price:" + m.LookupKey + ":" + m.MeterID)
		p, err := b.c.V1Prices.Create(ctx, cp)
		if err != nil {
			return fmt.Errorf("create price %s: %w", m.LookupKey, err)
		}
		m.PriceID = p.ID
		b.created("price " + p.ID + " (" + m.LookupKey + ")")
	}
	return nil
}

// portal finds or creates the customer portal configuration: update the
// card, the billing address and the email, and see invoices. Cancelling or
// changing the subscription is not offered, because the subscription is
// the platform's, not a plan the user picks (§5.5).
func (b *bootstrapper) portal(ctx context.Context) error {
	for pc, err := range b.c.V1BillingPortalConfigurations.List(ctx, &stripe.BillingPortalConfigurationListParams{Active: stripe.Bool(true)}) {
		if err != nil {
			return fmt.Errorf("list portal configurations: %w", err)
		}
		if pc.Metadata[bootstrapMetadataKey] == bootstrapMetadataMark {
			b.res.PortalConfiguration = pc.ID
			b.say("found    portal configuration %s", pc.ID)
			return nil
		}
	}
	params := &stripe.BillingPortalConfigurationCreateParams{
		BusinessProfile: &stripe.BillingPortalConfigurationCreateBusinessProfileParams{
			Headline:          str("Repose billing"),
			PrivacyPolicyURL:  str(b.opts.DashboardURL + "/privacy"),
			TermsOfServiceURL: str(b.opts.DashboardURL + "/terms"),
		},
		DefaultReturnURL: str(b.opts.DashboardURL + "/billing"),
		Features: &stripe.BillingPortalConfigurationCreateFeaturesParams{
			CustomerUpdate: &stripe.BillingPortalConfigurationCreateFeaturesCustomerUpdateParams{
				Enabled: stripe.Bool(true), AllowedUpdates: []*string{str("address"), str("email"), str("name"), str("tax_id")},
			},
			InvoiceHistory:      &stripe.BillingPortalConfigurationCreateFeaturesInvoiceHistoryParams{Enabled: stripe.Bool(true)},
			PaymentMethodUpdate: &stripe.BillingPortalConfigurationCreateFeaturesPaymentMethodUpdateParams{Enabled: stripe.Bool(true)},
			SubscriptionCancel:  &stripe.BillingPortalConfigurationCreateFeaturesSubscriptionCancelParams{Enabled: stripe.Bool(false)},
		},
		Metadata: map[string]string{bootstrapMetadataKey: bootstrapMetadataMark},
	}
	params.SetIdempotencyKey("bootstrap:portal:" + b.opts.DashboardURL)
	pc, err := b.c.V1BillingPortalConfigurations.Create(ctx, params)
	if err != nil {
		return fmt.Errorf("create the customer portal configuration: %w", err)
	}
	b.res.PortalConfiguration = pc.ID
	b.created("portal configuration " + pc.ID)
	return nil
}

// webhook finds or creates the endpoint at WebhookURL, subscribed to the
// six events and pinned to the API version this stripe-go speaks: an
// endpoint on another version has every delivery rejected by
// webhook.ConstructEvent, which looks like a silent billing outage.
func (b *bootstrapper) webhook(ctx context.Context) error {
	if b.opts.WebhookURL == "" {
		b.say("skipped  webhook endpoint (no --webhook-url)")
		return nil
	}
	events := make([]*string, 0, len(WebhookEvents))
	for _, e := range WebhookEvents {
		events = append(events, str(e))
	}
	var existing *stripe.WebhookEndpoint
	for we, err := range b.c.V1WebhookEndpoints.List(ctx, &stripe.WebhookEndpointListParams{}) {
		if err != nil {
			return fmt.Errorf("list webhook endpoints: %w", err)
		}
		if we.URL == b.opts.WebhookURL {
			existing = we
			break
		}
	}
	if existing != nil && existing.APIVersion != stripe.APIVersion && !b.opts.RotateWebhook {
		return fmt.Errorf("webhook endpoint %s is on API version %s, not %s, and would have every event rejected; rerun with --rotate-webhook to replace it",
			existing.ID, existing.APIVersion, stripe.APIVersion)
	}
	if existing != nil && b.opts.RotateWebhook {
		if _, err := b.c.V1WebhookEndpoints.Delete(ctx, existing.ID, nil); err != nil {
			return fmt.Errorf("delete webhook endpoint %s: %w", existing.ID, err)
		}
		b.say("deleted  webhook endpoint %s (--rotate-webhook)", existing.ID)
		existing = nil
	}
	if existing != nil {
		if _, err := b.c.V1WebhookEndpoints.Update(ctx, existing.ID, &stripe.WebhookEndpointUpdateParams{
			EnabledEvents: events, Disabled: stripe.Bool(false),
		}); err != nil {
			return fmt.Errorf("update webhook endpoint %s: %w", existing.ID, err)
		}
		b.res.WebhookEndpointID = existing.ID
		b.say("found    webhook endpoint %s (%s); its signing secret is only shown at creation", existing.ID, existing.URL)
		return nil
	}
	params := &stripe.WebhookEndpointCreateParams{
		URL: str(b.opts.WebhookURL), EnabledEvents: events, APIVersion: str(stripe.APIVersion),
		Description: str("repose api: 09-billing.md §5.6"),
		Metadata:    map[string]string{bootstrapMetadataKey: bootstrapMetadataMark},
	}
	we, err := b.c.V1WebhookEndpoints.Create(ctx, params)
	if err != nil {
		return fmt.Errorf("create webhook endpoint at %s: %w", b.opts.WebhookURL, err)
	}
	b.res.WebhookEndpointID = we.ID
	b.res.WebhookSecret = we.Secret
	b.created("webhook endpoint " + we.ID + " (" + we.URL + ", API " + stripe.APIVersion + ")")
	return nil
}

// tax reads whether Stripe Tax is active, which decides
// STRIPE_AUTOMATIC_TAX. Activating it needs a business address in the
// Stripe dashboard, which is the owner's, so the bootstrap never does it.
func (b *bootstrapper) tax(ctx context.Context) error {
	ts, err := b.c.V1TaxSettings.Retrieve(ctx, nil)
	if err != nil {
		// A key without the tax permission is not a reason to stop.
		b.res.TaxStatus = "unknown"
		b.say("unknown  Stripe Tax status (%v); STRIPE_AUTOMATIC_TAX=false", err)
		return nil
	}
	b.res.TaxStatus = string(ts.Status)
	b.say("found    Stripe Tax %s", ts.Status)
	return nil
}

// meter returns the entry for one part.
func (r *BootstrapResult) meter(part string) BootstrapMeter {
	for _, m := range r.Meters {
		if m.Part == part {
			return m
		}
	}
	return BootstrapMeter{}
}

// Config is the api configuration the result describes, so a test can
// check the block against what ConfigFromEnv would read.
func (r *BootstrapResult) Config(webhookSecret string) Config {
	c, s, e := r.meter("compute"), r.meter("storage"), r.meter("egress")
	return Config{
		SecretKey: r.SecretKey, WebhookSecret: webhookSecret, PortalConfiguration: r.PortalConfiguration,
		PriceCompute: c.PriceID, PriceStorage: s.PriceID, PriceEgress: e.PriceID,
		MeterCompute: c.EventName, MeterStorage: s.EventName, MeterEgress: e.EventName,
		MeterIDCompute: c.MeterID, MeterIDStorage: s.MeterID, MeterIDEgress: e.MeterID,
		Enforce: true, AutomaticTax: r.TaxStatus == string(stripe.TaxSettingsStatusActive),
	}
}

// EnvBlock is the text pasted into the api's Coolify environment: every
// variable ConfigFromEnv reads, by the name it reads it under.
func (r *BootstrapResult) EnvBlock() string {
	mode := "test"
	if r.Live {
		mode = "LIVE"
	}
	c := r.Config(r.WebhookSecret)
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Billing, Stripe %s mode: repose-admin billing stripe-bootstrap, %s\n", mode, r.At.Format(time.RFC3339))
	fmt.Fprintf(&sb, "# Paste into the api's Coolify environment (both api and api-grpc read it); Coolify restarts the app.\n")
	lines := [][2]string{
		{"STRIPE_SECRET_KEY", c.SecretKey},
		{"STRIPE_WEBHOOK_SECRET", c.WebhookSecret},
		{"STRIPE_PRICE_COMPUTE", c.PriceCompute},
		{"STRIPE_PRICE_STORAGE", c.PriceStorage},
		{"STRIPE_PRICE_EGRESS", c.PriceEgress},
		{"STRIPE_METER_COMPUTE", c.MeterCompute},
		{"STRIPE_METER_STORAGE", c.MeterStorage},
		{"STRIPE_METER_EGRESS", c.MeterEgress},
		{"STRIPE_METER_ID_COMPUTE", c.MeterIDCompute},
		{"STRIPE_METER_ID_STORAGE", c.MeterIDStorage},
		{"STRIPE_METER_ID_EGRESS", c.MeterIDEgress},
		{"STRIPE_PORTAL_CONFIGURATION", c.PortalConfiguration},
		{"STRIPE_AUTOMATIC_TAX", fmt.Sprint(c.AutomaticTax)},
		{"BILLING_ENFORCE", "true"},
	}
	for _, l := range lines {
		if l[0] == "STRIPE_WEBHOOK_SECRET" && l[1] == "" {
			if r.WebhookEndpointID == "" {
				fmt.Fprintf(&sb, "# STRIPE_WEBHOOK_SECRET: no endpoint was made (no --webhook-url); the api refuses to start without one\n")
			} else {
				fmt.Fprintf(&sb, "# STRIPE_WEBHOOK_SECRET unchanged: endpoint %s already existed and Stripe shows a secret only at creation.\n", r.WebhookEndpointID)
				fmt.Fprintf(&sb, "# Keep the value the api has, or rerun with --rotate-webhook for a new endpoint and secret.\n")
			}
			continue
		}
		fmt.Fprintf(&sb, "%s=%s\n", l[0], l[1])
	}
	if !c.AutomaticTax {
		fmt.Fprintf(&sb, "# Stripe Tax is %s on this account, so invoices carry no tax line until it is activated in the dashboard\n", r.TaxStatus)
		fmt.Fprintf(&sb, "# (Settings > Tax) and STRIPE_AUTOMATIC_TAX is set to true.\n")
	}
	return sb.String()
}

// Summary lists what was created, for the end of the command's output.
func (r *BootstrapResult) Summary() string {
	if len(r.Created) == 0 {
		return "nothing created: every object already existed"
	}
	c := append([]string(nil), r.Created...)
	sort.Strings(c)
	return fmt.Sprintf("created %d object(s): %s", len(c), strings.Join(c, "; "))
}
