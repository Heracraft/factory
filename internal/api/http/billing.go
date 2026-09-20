package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/heracraft/repose/internal/billing"
	"github.com/heracraft/repose/internal/obs"
)

// POST /v1/billing/webhook (09-billing.md §5.6). Stripe authenticates
// itself with the Stripe-Signature header, so the route carries no bearer
// token and is registered outside the authenticated set. The body is read
// with a cap and never logged: it carries the customer's details, and a
// webhook that fails verification is logged with the event type only
// (§6, "webhook signature invalid: 400, logged with the event type only").
const webhookMaxBytes = 1 << 20

func (s *Server) billingWebhook(w http.ResponseWriter, r *http.Request) error {
	if s.d.Webhooks == nil {
		return errf("billing_disabled", "billing is not configured")
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookMaxBytes))
	if err != nil {
		return errf("invalid", "could not read the webhook body")
	}
	kind, err := s.d.Webhooks.Handle(r.Context(), body, r.Header.Get("Stripe-Signature"))
	switch {
	case errors.Is(err, billing.ErrDuplicate):
		// Already applied; answering 200 stops Stripe retrying.
		writeJSON(w, http.StatusOK, map[string]any{"received": true, "duplicate": true})
		return nil
	case errors.Is(err, billing.ErrBadSignature):
		s.d.Metrics.StripeWebhookTotal.WithLabelValues("bad_signature").Inc()
		obs.Logger(r.Context(), s.d.Log).Warn("stripe webhook signature rejected", "event", obs.EventStripeWebhook, "kind", kind, "result", "bad_signature")
		return errf("invalid", "stripe signature verification failed")
	case err != nil:
		s.d.Metrics.StripeWebhookTotal.WithLabelValues("error").Inc()
		return err
	}
	s.d.Metrics.StripeWebhookTotal.WithLabelValues("ok").Inc()
	writeJSON(w, http.StatusOK, map[string]any{"received": true})
	return nil
}
