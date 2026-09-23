package admin_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/admin"
	"github.com/heracraft/repose/internal/billing"
)

// `billing stripe-bootstrap` needs no database, reads the key from the
// environment only, and refuses a live key without --live before any
// request (DECISIONS I-180). The object creation itself is tested against
// the fake Stripe in internal/billing.
func TestStripeBootstrapCommandGuards(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	run := func(args ...string) (string, error) {
		var out, errb bytes.Buffer
		err := admin.Run(context.Background(), &admin.Env{Stdout: &out, Stderr: &errb}, append([]string{"billing", "stripe-bootstrap"}, args...))
		return out.String() + errb.String(), err
	}
	t.Setenv("STRIPE_SECRET_KEY", "")
	if _, err := run(); !errors.Is(err, admin.ErrUsage) {
		t.Fatalf("no key: %v, want usage", err)
	}
	t.Setenv("STRIPE_SECRET_KEY", "sk_live_x")
	if out, err := run(); !errors.Is(err, billing.ErrLiveKey) || strings.Contains(out, "STRIPE_") {
		t.Fatalf("live key: %v %q", err, out)
	}
	t.Setenv("STRIPE_SECRET_KEY", "pk_test_x")
	if _, err := run(); err == nil || !strings.Contains(err.Error(), "secret key") {
		t.Fatalf("publishable key: %v", err)
	}
}
