package billing

import (
	"errors"
	"os"
	"strings"
)

// Config is the STRIPE_* environment the api reads (09-billing.md §5.5,
// DECISIONS I-16: with none of it set the api starts normally and the
// billing routes answer 503 billing_disabled).
type Config struct {
	SecretKey     string
	WebhookSecret string
	// PortalReturnURL is where Stripe's customer portal sends the user back.
	PortalReturnURL string
	// PortalConfiguration is the bpc_... customer portal configuration
	// `repose-admin billing stripe-bootstrap` creates; empty uses the
	// account's default configuration.
	PortalConfiguration string

	// The four Stripe objects of §5.5: one product, three metered prices,
	// each attached to a billing meter whose event name is pushed per
	// usage_hours row. Cents are the unit, so the invoice carries the
	// amounts this code computed and Stripe adds nothing of its own.
	PriceCompute string
	PriceStorage string
	PriceEgress  string
	MeterCompute string
	MeterStorage string
	MeterEgress  string
	// MeterIDCompute and friends are the `mtr_...` ids reconciliation reads
	// summaries from; the event names above are what a push writes.
	MeterIDCompute string
	MeterIDStorage string
	MeterIDEgress  string

	// Enforce is BILLING_ENFORCE (§8). False keeps rolling up and pushing
	// but stops blocking starts and stopping guests.
	Enforce bool
	// AutomaticTax switches Stripe Tax on for the subscription (§5.9).
	AutomaticTax bool
}

// Default meter event names. They are configurable because a Stripe account
// that already has meters under other names should not need a code change,
// but these are what `ops/coolify/api.env.example` documents.
const (
	DefaultMeterCompute = "repose_compute_cents"
	DefaultMeterStorage = "repose_storage_cents"
	DefaultMeterEgress  = "repose_egress_cents"
)

func env(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv reads the billing configuration. Enabled reports whether
// Stripe is configured at all.
func ConfigFromEnv() (cfg Config, enabled bool) {
	cfg = Config{
		SecretKey:           strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")),
		WebhookSecret:       strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
		PortalReturnURL:     env("STRIPE_PORTAL_RETURN_URL", env("DASHBOARD_URL", "https://repose.herakraft.co")+"/billing"),
		PortalConfiguration: strings.TrimSpace(os.Getenv("STRIPE_PORTAL_CONFIGURATION")),
		PriceCompute:        strings.TrimSpace(os.Getenv("STRIPE_PRICE_COMPUTE")),
		PriceStorage:        strings.TrimSpace(os.Getenv("STRIPE_PRICE_STORAGE")),
		PriceEgress:         strings.TrimSpace(os.Getenv("STRIPE_PRICE_EGRESS")),
		MeterCompute:        env("STRIPE_METER_COMPUTE", DefaultMeterCompute),
		MeterStorage:        env("STRIPE_METER_STORAGE", DefaultMeterStorage),
		MeterEgress:         env("STRIPE_METER_EGRESS", DefaultMeterEgress),
		MeterIDCompute:      strings.TrimSpace(os.Getenv("STRIPE_METER_ID_COMPUTE")),
		MeterIDStorage:      strings.TrimSpace(os.Getenv("STRIPE_METER_ID_STORAGE")),
		MeterIDEgress:       strings.TrimSpace(os.Getenv("STRIPE_METER_ID_EGRESS")),
		Enforce:             os.Getenv("BILLING_ENFORCE") != "false",
		AutomaticTax:        os.Getenv("STRIPE_AUTOMATIC_TAX") != "false",
	}
	return cfg, cfg.SecretKey != ""
}

// Validate refuses a half-configured Stripe: a secret key with no prices
// would create subscriptions that bill nothing, and no webhook secret
// would leave every invoice event unverified and dropped.
func (c Config) Validate() error {
	var missing []string
	for name, v := range map[string]string{
		"STRIPE_WEBHOOK_SECRET": c.WebhookSecret,
		"STRIPE_PRICE_COMPUTE":  c.PriceCompute,
		"STRIPE_PRICE_STORAGE":  c.PriceStorage,
		"STRIPE_PRICE_EGRESS":   c.PriceEgress,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sortStrings(missing)
		return errors.New("STRIPE_SECRET_KEY is set but " + strings.Join(missing, ", ") + " is not")
	}
	return nil
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
