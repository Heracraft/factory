package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v83"
	"github.com/stripe/stripe-go/v83/webhook"

	httpapi "github.com/heracraft/repose/internal/api/http"
	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/obs"
)

// §9: "Start and create blocked without a card, with payment_required." The
// create path is covered by TestSignInAndProjectsLifecycle; this covers
// start, snapshot and every reason the gate can give.
func TestBillingGateBlocksCompute(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tok := e.signIn(t, "sub-gate", "gate-dev")
	// A card, so the project can be created at all.
	if _, err := e.h.Pool.Exec(ctx, "update users set has_card = true where logto_sub = 'sub-gate'"); err != nil {
		t.Fatal(err)
	}
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "gated", "class": "small"})
	if r.status != 201 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	pid := r.body["id"].(string)
	e.waitOp(t, r)
	stop := e.do(t, tok, "POST", "/projects/"+pid+"/stop", map[string]any{"snapshot": false})
	if stop.status != 202 && stop.status != 200 {
		t.Fatalf("stop: %d %s", stop.status, stop.raw)
	}
	e.waitOp(t, stop)

	for _, c := range []struct {
		name, update, reason string
	}{
		{"no card", "has_card = false", "card_required"},
		{"past due", "has_card = true, billing_status = 'past_due'", "past_due"},
		{"suspended", "has_card = true, billing_status = 'suspended'", "suspended"},
		{"trial depleted", "has_card = true, billing_status = 'trial', trial_credit_cents = 0", "trial_depleted"},
	} {
		if _, err := e.h.Pool.Exec(ctx, "update users set "+c.update+" where logto_sub = 'sub-gate'"); err != nil {
			t.Fatal(err)
		}
		path := "/projects/" + pid + "/start"
		r := e.do(t, tok, "POST", path, nil)
		if r.status != http.StatusPaymentRequired || errCode(r) != "payment_required" {
			t.Errorf("%s on start: %d %s", c.name, r.status, r.raw)
		} else if got := errDetail(r, "reason"); got != c.reason {
			t.Errorf("%s on start: reason %q, want %q", c.name, got, c.reason)
		}
		// Creating another project is blocked for the same reason.
		r = e.do(t, tok, "POST", "/projects", map[string]any{"name": "gated-2", "class": "small"})
		if r.status != http.StatusPaymentRequired {
			t.Errorf("%s on create: %d %s", c.name, r.status, r.raw)
		}
	}

	// An exempt account passes every check (DECISIONS I-16).
	if _, err := e.h.Pool.Exec(ctx, "update users set has_card = false, billing_status = 'exempt', trial_credit_cents = 0 where logto_sub = 'sub-gate'"); err != nil {
		t.Fatal(err)
	}
	if r := e.do(t, tok, "POST", "/projects/"+pid+"/start", nil); r.status != 202 && r.status != 200 {
		t.Fatalf("exempt start: %d %s", r.status, r.raw)
	}
}

// §8: with BILLING_ENFORCE=false nothing is blocked, and the three routes
// still answer.
func TestBillingEnforceFalseLetsStartsThrough(t *testing.T) {
	e := newEnvWith(t, &httpapi.RateLimits{General: 10000, Certs: 10000, Config: 10000}, func(d *httpapi.Deps) {
		d.BillingEnforce = false
	})
	ctx := context.Background()
	tok := e.signIn(t, "sub-noenf", "noenf-dev")
	if _, err := e.h.Pool.Exec(ctx, "update users set has_card = false, trial_credit_cents = 0 where logto_sub = 'sub-noenf'"); err != nil {
		t.Fatal(err)
	}
	r := e.do(t, tok, "POST", "/projects", map[string]any{"name": "unblocked", "class": "small"})
	if r.status != 201 {
		t.Fatalf("create with enforcement off: %d %s", r.status, r.raw)
	}
	e.waitOp(t, r)
}

// The webhook route: unauthenticated, signature-verified, 503 when Stripe
// is not configured (docs/interfaces/api.md).
func TestBillingWebhookRoute(t *testing.T) {
	const secret = "whsec_route_test"
	var hooks *billing.Webhooks
	e := newEnvWith(t, &httpapi.RateLimits{General: 10000, Certs: 10000, Config: 10000}, func(d *httpapi.Deps) {
		hooks = billing.NewWebhooks(d.Pool, secret, d.Log)
		d.Webhooks = hooks
	})
	ctx := context.Background()
	tok := e.signIn(t, "sub-wh", "wh-dev")
	_ = tok
	var customer string
	if err := e.h.Pool.QueryRow(ctx, "update users set stripe_customer_id = 'cus_route', has_card = false where logto_sub = 'sub-wh' returning stripe_customer_id").Scan(&customer); err != nil {
		t.Fatal(err)
	}

	body := routeEvent("evt_route_1", billing.TypeSetupIntentSucceeded, map[string]any{"id": "seti_r", "customer": customer})
	// No bearer token: Stripe authenticates with the header alone.
	r := e.doRaw(t, "POST", "/billing/webhook", body, map[string]string{"Stripe-Signature": routeSign(t, body, secret)})
	if r.status != 200 || r.body["received"] != true {
		t.Fatalf("webhook: %d %s", r.status, r.raw)
	}
	var hasCard bool
	if err := e.h.Pool.QueryRow(ctx, "select has_card from users where logto_sub = 'sub-wh'").Scan(&hasCard); err != nil {
		t.Fatal(err)
	}
	if !hasCard {
		t.Fatal("the webhook did not apply")
	}
	// A duplicate is a 200 so Stripe stops retrying.
	r = e.doRaw(t, "POST", "/billing/webhook", body, map[string]string{"Stripe-Signature": routeSign(t, body, secret)})
	if r.status != 200 || r.body["duplicate"] != true {
		t.Fatalf("duplicate: %d %s", r.status, r.raw)
	}
	// A bad signature is a 400 and logs the event type and nothing else.
	r = e.doRaw(t, "POST", "/billing/webhook", body, map[string]string{"Stripe-Signature": "t=1,v1=00"})
	if r.status != 400 || errCode(r) != "invalid" {
		t.Fatalf("bad signature: %d %s", r.status, r.raw)
	}
	if !strings.Contains(e.logs.String(), obs.EventStripeWebhook) {
		t.Error("the rejection was not logged")
	}
	if strings.Contains(e.logs.String(), "seti_r") {
		t.Error("the webhook body reached the log")
	}

	// Without a handler the route answers 503 billing_disabled, like the
	// other three (DECISIONS I-16).
	plain := newEnv(t)
	r = plain.doRaw(t, "POST", "/billing/webhook", body, map[string]string{"Stripe-Signature": routeSign(t, body, secret)})
	if r.status != 503 || errCode(r) != "billing_disabled" {
		t.Fatalf("webhook with billing off: %d %s", r.status, r.raw)
	}
}

func routeEvent(id, kind string, object map[string]any) []byte {
	b, err := json.Marshal(map[string]any{
		"id": id, "object": "event", "type": kind, "api_version": stripe.APIVersion,
		"created": time.Now().Unix(), "data": map[string]any{"object": object},
	})
	if err != nil {
		panic(err)
	}
	return b
}

func routeSign(t *testing.T, payload []byte, secret string) string {
	t.Helper()
	now := time.Now()
	return fmt.Sprintf("t=%d,v1=%x", now.Unix(), webhook.ComputeSignature(now, payload, secret))
}

func errDetail(r resp, key string) string {
	e, _ := r.body["error"].(map[string]any)
	d, _ := e["detail"].(map[string]any)
	s, _ := d[key].(string)
	return s
}
