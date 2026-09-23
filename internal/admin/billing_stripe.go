package admin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go/v83"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/billing"
)

// StripeClientOptions points `billing stripe-bootstrap` at a fake Stripe in
// tests; empty is the real API.
var StripeClientOptions []stripe.ClientOption

// DefaultWebhookURL is the api's public webhook route (docs/ops/coolify.md).
const DefaultWebhookURL = "https://api.repose.herakraft.co/v1/billing/webhook"

// billingStripeBootstrap creates or finds every Stripe object the api needs
// and prints the STRIPE_* block for its environment (DECISIONS I-180). It
// needs no database: the key is read from STRIPE_SECRET_KEY, never from
// the command line, where it would show in `ps` and shell history.
// Progress goes to stderr and the block alone to stdout, so
// `... > stripe.env` captures exactly what is pasted.
func (e *Env) billingStripeBootstrap(ctx context.Context, args []string) error {
	fs, err := flagsFor("stripe-bootstrap", args, func(fs *flag.FlagSet) {
		fs.String("webhook-url", "", "the api's public webhook route (default "+DefaultWebhookURL+", or $API_PUBLIC_URL/v1/billing/webhook)")
		fs.String("dashboard-url", "", "the dashboard the portal returns to (default $DASHBOARD_URL or https://repose.herakraft.co)")
		fs.Bool("no-webhook", false, "do not create the webhook endpoint")
		fs.Bool("rotate-webhook", false, "replace an existing endpoint to get a new signing secret")
		fs.Bool("live", false, "allow a live-mode key")
	})
	if err != nil {
		return err
	}
	key := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if key == "" {
		return fmt.Errorf("%w: STRIPE_SECRET_KEY=sk_test_... repose-admin billing stripe-bootstrap [--webhook-url URL] [--rotate-webhook] [--live]", ErrUsage)
	}
	get := func(name string) string { return fs.Lookup(name).Value.String() }
	opts := billing.BootstrapOptions{
		WebhookURL:    get("webhook-url"),
		DashboardURL:  get("dashboard-url"),
		AllowLive:     get("live") == "true",
		RotateWebhook: get("rotate-webhook") == "true",
		Out:           e.Stderr,
	}
	if opts.WebhookURL == "" {
		opts.WebhookURL = DefaultWebhookURL
		if u := strings.TrimRight(os.Getenv("API_PUBLIC_URL"), "/"); u != "" {
			opts.WebhookURL = u + "/v1/billing/webhook"
		}
	}
	if get("no-webhook") == "true" {
		opts.WebhookURL = ""
	}
	if opts.DashboardURL == "" {
		opts.DashboardURL = os.Getenv("DASHBOARD_URL")
	}
	res, err := billing.Bootstrap(ctx, key, opts, StripeClientOptions...)
	if errors.Is(err, billing.ErrLiveKey) || (err != nil && res == nil) {
		// Refused before anything was sent to Stripe.
		return err
	}
	if err != nil {
		return fmt.Errorf("stripe-bootstrap stopped; rerunning is safe, every object is found before it is made: %w", err)
	}
	_, _ = fmt.Fprintf(e.Stderr, "%s\n\n", res.Summary())
	_, _ = fmt.Fprint(e.Stdout, res.EnvBlock())
	return nil
}

// billingShow prints one account's billing state and its current period's
// totals: the evidence docs/ops/M4-GATE.md reads.
func (e *Env) billingShow(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%w: billing show HANDLE", ErrUsage)
	}
	u, err := e.findUser(ctx, args[0])
	if err != nil {
		return err
	}
	a, err := billing.LoadAccount(ctx, e.pool, u.ID, time.Now())
	if err != nil {
		return err
	}
	_, err = a.WriteTo(e.Stdout)
	return err
}

// billingCycleNow ends an account's billing period now so Stripe invoices
// the usage so far (DECISIONS I-185). It rolls up and pushes every
// finished hour, waits until Stripe's meter summaries hold every cent of
// the period, and only then resets the cycle, so the invoice cannot be cut
// before the last hours reached it. Without --yes it stops before the
// reset and says what it would do.
func (e *Env) billingCycleNow(ctx context.Context, args []string) error {
	fs, err := flagsFor("cycle-now", args, func(fs *flag.FlagSet) {
		fs.Bool("yes", false, "reset the cycle; without it nothing is changed at Stripe")
		fs.Duration("wait", 10*time.Minute, "how long to wait for Stripe's meter summaries to catch up")
	})
	if err != nil || fs.NArg() < 1 {
		return fmt.Errorf("%w: billing cycle-now HANDLE [--yes] [--wait 10m]", ErrUsage)
	}
	u, err := e.findUser(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	st, err := e.stripe()
	if err != nil {
		return err
	}
	r := billing.NewRollup(e.pool, st, metrics.NewNop(), e.logger())
	if n, err := r.Due(ctx); err != nil {
		return err
	} else if n > 0 {
		_, _ = fmt.Fprintf(e.Stderr, "rolled up and pushed %d hour(s)\n", n)
	}
	a, err := billing.LoadAccount(ctx, e.pool, u.ID, time.Now())
	if err != nil {
		return err
	}
	if a.UnpushedRows > 0 {
		return fmt.Errorf("%d row(s) of this period are not in Stripe yet; `repose-admin billing resync` first", a.UnpushedRows)
	}
	wait := 10 * time.Minute
	if d, err := time.ParseDuration(fs.Lookup("wait").Value.String()); err == nil {
		wait = d
	}
	deadline := time.Now().Add(wait)
	for {
		sum, err := st.PeriodSummary(ctx, a.Customer, a.Period.Start, time.Now())
		if err != nil {
			return fmt.Errorf("read Stripe's meter summaries (STRIPE_METER_ID_* must be set): %w", err)
		}
		_, _ = fmt.Fprintf(e.Stderr, "Stripe holds compute %d + storage %d + egress %d = %d of %d cents\n",
			sum.GuestCents, sum.StorageCents, sum.EgressCents, sum.Total(), a.Billed())
		if sum.GuestCents == a.ComputeBilled && sum.StorageCents == a.StorageBilled && sum.EgressCents == a.EgressBilled {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the meter summaries at Stripe did not reach usage_hours within %s; nothing was changed", wait)
		}
		time.Sleep(15 * time.Second)
	}
	if fs.Lookup("yes").Value.String() != "true" {
		_, _ = fmt.Fprintf(e.Stdout, "would end %s's period %s now and invoice %d cents (compute %d, storage %d, egress %d) plus any tax; rerun with --yes\n",
			a.Handle, a.Period.Start.Format(time.RFC3339), a.Billed(), a.ComputeBilled, a.StorageBilled, a.EgressBilled)
		return nil
	}
	if _, err := e.audited(ctx, "billing_cycle_now", a.Handle, map[string]any{"billed_cents": a.Billed()}); err != nil {
		return err
	}
	inv, anchor, err := st.CycleNow(ctx, u.ID)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Stdout, "period ended; invoice %s should be %d cents before tax (compute %d, storage %d, egress %d); the new period starts %s.\n"+
		"Stripe finalises the draft in about an hour and charges the card; invoice.paid then shows it in `repose-admin billing show %s`.\n",
		orNone(inv), a.Billed(), a.ComputeBilled, a.StorageBilled, a.EgressBilled, anchor.Format(time.RFC3339), a.Handle)
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(none returned)"
	}
	return s
}
