package gateway

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/heracraft/repose/internal/ca/testca"
)

func newTestCA() (*testca.CA, error) { return testca.New() }

func counterValue(t *testing.T, c prometheus.Metric) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatal(err)
	}
	if m.Counter != nil {
		return m.Counter.GetValue()
	}
	return m.Gauge.GetValue()
}

// Every metric of 06-gateway-edge.md §5.5 (and the three named in
// 10-observability.md) is registered and exposed from the first scrape.
func TestMetricsExposed(t *testing.T) {
	reg := prometheus.NewRegistry()
	NewMetrics(reg)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, f := range families {
		have[f.GetName()] = true
	}
	want := []string{
		"repose_gateway_connections_open",
		"repose_gateway_auth_total",
		"repose_gateway_relay_bytes_total",
		"repose_gateway_dial_errors_total",
		"repose_gateway_session_seconds",
		"repose_gateway_revocation_cache_age_seconds",
		"repose_gateway_wgsync_peers",
		"repose_gateway_wgsync_errors_total",
		"repose_gateway_sessions_total",
		"repose_gateway_auth_fail_total",
		"repose_gateway_route_duration_seconds",
		"repose_gateway_hook_events_total",
	}
	var missing []string
	for _, w := range want {
		if !have[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing: %s", strings.Join(missing, ", "))
	}
	for _, f := range families {
		if !strings.HasPrefix(f.GetName(), "repose_gateway_") {
			t.Fatalf("metric outside the namespace: %s", f.GetName())
		}
		if f.GetName() == "repose_gateway_auth_total" && len(f.Metric) != len(AuthResults) {
			t.Fatalf("auth_total has %d result series, want %d", len(f.Metric), len(AuthResults))
		}
	}
	t.Logf("exposed: %s", strings.Join(want, " "))
}
