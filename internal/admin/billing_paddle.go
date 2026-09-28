package admin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/billing"
)

// DefaultWebhookURL is the api's public webhook route (docs/ops/coolify.md).
const DefaultWebhookURL = "https://api.repose.herakraft.co/v1/billing/webhook"

// PaddleBaseURL overrides the Paddle API origin (tests point it at the fake).
var PaddleBaseURL string

// paddle builds the Paddle client from the environment, or returns
// ErrDisabled so a command can say billing is not configured.
func (e *Env) paddle() (*billing.Paddle, billing.Config, error) {
	cfg, on := billing.ConfigFromEnv()
	if !on {
		return nil, cfg, billing.ErrDisabled
	}
	cfg.BaseURL = PaddleBaseURL
	return billing.NewPaddle(cfg, e.logger()), cfg, nil
}

// billingPaddleBootstrap creates or finds every Paddle object the api
// needs and prints the PADDLE_* block for its environment (DECISIONS
// I-289). It needs no database: the key is read from PADDLE_API_KEY,
// never from the command line, where it would show in `ps` and shell
// history. Progress goes to stderr and the block alone to stdout, so
// `... > paddle.env` captures exactly what is pasted.
func (e *Env) billingPaddleBootstrap(ctx context.Context, args []string) error {
	fs, err := flagsFor("paddle-bootstrap", args, func(fs *flag.FlagSet) {
		fs.String("webhook-url", "", "the api's public webhook route (default "+DefaultWebhookURL+", or $API_PUBLIC_URL/v1/billing/webhook)")
		fs.Bool("no-webhook", false, "do not create the notification destination")
		fs.Bool("live", false, "allow a live key")
	})
	if err != nil {
		return err
	}
	key := strings.TrimSpace(os.Getenv("PADDLE_API_KEY"))
	if key == "" {
		return fmt.Errorf("%w: PADDLE_API_KEY=pdl_sdbx_... repose-admin billing paddle-bootstrap [--webhook-url URL] [--no-webhook] [--live]", ErrUsage)
	}
	get := func(name string) string { return fs.Lookup(name).Value.String() }
	opts := billing.BootstrapOptions{WebhookURL: get("webhook-url"), Live: get("live") == "true", Progress: e.Stderr}
	if opts.WebhookURL == "" {
		opts.WebhookURL = DefaultWebhookURL
		if u := strings.TrimRight(os.Getenv("API_PUBLIC_URL"), "/"); u != "" {
			opts.WebhookURL = u + "/v1/billing/webhook"
		}
	}
	if get("no-webhook") == "true" {
		opts.WebhookURL = ""
	}
	p := billing.NewPaddle(billing.Config{APIKey: key, BaseURL: PaddleBaseURL}, e.logger())
	res, err := billing.Bootstrap(ctx, p, opts)
	if errors.Is(err, billing.ErrLiveKey) {
		return err
	}
	if err != nil {
		return fmt.Errorf("paddle-bootstrap stopped; rerunning is safe, every object is found before it is made: %w", err)
	}
	_, _ = fmt.Fprintf(e.Stderr, "%s: created %d object(s), found %d\n\n", res.Environment, len(res.Created), len(res.Found))
	_, _ = fmt.Fprint(e.Stdout, res.EnvBlock())
	return nil
}

// billingShow prints one account's billing state: subscription, plan,
// period, usage and the overage arithmetic (docs/ops/M4-GATE.md).
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

// billingOverageNow sends this period's egress overage line for one
// account now rather than within three hours of the next bill: the gate
// proof of docs/ops/M4-GATE.md §4. A period already sent is not sent twice.
func (e *Env) billingOverageNow(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%w: billing overage-now HANDLE", ErrUsage)
	}
	u, err := e.findUser(ctx, args[0])
	if err != nil {
		return err
	}
	p, cfg, err := e.paddle()
	if err != nil {
		return err
	}
	sub, err := billing.LiveSubscription(ctx, e.pool, u.ID)
	if err != nil {
		return err
	}
	if sub == nil {
		return fmt.Errorf("%s has no live subscription", u.Handle)
	}
	o := billing.NewOverage(e.pool, p, cfg, nil, metrics.NewNop(), e.logger())
	c, err := o.ChargePeriod(ctx, sub, billing.EffectiveNextBillingPeriod)
	if err != nil {
		return err
	}
	switch {
	case c == nil:
		_, _ = fmt.Fprintf(e.Stdout, "%s: the subscription has no billing period yet\n", u.Handle)
	case c.Cents == 0:
		_, _ = fmt.Fprintf(e.Stdout, "%s: %s to now: egress within the allowance, nothing to charge; period marked\n", u.Handle, c.PeriodStart.Format("2006-01-02"))
	case c.Sent:
		_, _ = fmt.Fprintf(e.Stdout, "%s: sent %d GB over = %d cents to Paddle for the period from %s (transaction %s)\n", u.Handle, c.EgressGB, c.Cents, c.PeriodStart.Format("2006-01-02"), orNone(c.TransactionID))
	default:
		_, _ = fmt.Fprintf(e.Stdout, "%s: the period from %s already has its line (%d cents, transaction %s); nothing sent\n", u.Handle, c.PeriodStart.Format("2006-01-02"), c.Cents, orNone(c.TransactionID))
	}
	detail := map[string]any{"sent": false}
	if c != nil {
		detail = map[string]any{"cents": c.Cents, "sent": c.Sent}
	}
	_, err = e.audited(ctx, "billing_overage_now", u.Handle, detail)
	return err
}

func orNone(s string) string {
	if s == "" {
		return "none yet"
	}
	return s
}
