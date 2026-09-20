package billing_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/obs"
	obsmetrics "github.com/heracraft/repose/internal/obs/metrics"
)

// §9: "Metrics for the rollup duration, the sample gap, the Stripe push
// backlog and the reconciliation difference exist. Evidence: /metrics
// scrape." They carry the repose_api_* prefix of DECISIONS I-49, I-60 and
// I-62, not §9's original repose_billing_* wording.
//
// The scrape is against the registry from internal/obs/metrics, which is
// the one the api serves, so a name or a label outside the rules would have
// failed registration before this test could read it.
func TestBillingMetricsAreExported(t *testing.T) {
	om := obsmetrics.NewVersion(obs.ComponentAPI, "test")
	m := metrics.New(om)
	// Touch each family so a counter or gauge that has never been set still
	// appears in the scrape.
	m.BillingGapMinutes.Add(60)
	m.RollupDuration.Observe(0.25)
	m.RollupLagSeconds.Set(120)
	m.StripePushBacklogSeconds.Set(3600)
	m.BillingMismatchCents.Set(0)
	m.StripeUsagePushTotal.WithLabelValues("ok").Inc()
	m.StripeWebhookTotal.WithLabelValues("ok").Inc()

	srv := httptest.NewServer(om.Handler())
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	scrape := string(body)
	for _, name := range []string{
		"repose_api_rollup_duration_seconds",
		"repose_api_rollup_lag_seconds",
		"repose_api_billing_gap_minutes_total",
		"repose_api_billing_stripe_push_backlog_seconds",
		"repose_api_billing_mismatch_cents",
		"repose_api_stripe_usage_push_total",
		"repose_api_stripe_webhook_total",
	} {
		if !strings.Contains(scrape, name+" ") && !strings.Contains(scrape, name+"{") {
			t.Errorf("%s is not in the scrape", name)
		}
	}
	// The alert expressions read these exact series; print them so the
	// checklist evidence is the scrape itself.
	for _, line := range strings.Split(scrape, "\n") {
		if strings.HasPrefix(line, "repose_api_billing") || strings.HasPrefix(line, "repose_api_rollup") || strings.HasPrefix(line, "repose_api_stripe") {
			if !strings.HasPrefix(line, "#") {
				t.Log(line)
			}
		}
	}
}
