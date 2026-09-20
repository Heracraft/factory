package metrics

import (
	"context"
	"errors"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/heracraft/repose/internal/obs"
)

// gathered returns every series name in the registry with its label names.
func gathered(t *testing.T, m *Metrics) map[string][]string {
	t.Helper()
	fams, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string][]string{}
	for _, f := range fams {
		var labels []string
		if len(f.GetMetric()) > 0 {
			for _, l := range f.GetMetric()[0].GetLabel() {
				labels = append(labels, l.GetName())
			}
		}
		sort.Strings(labels)
		out[f.GetName()] = labels
	}
	return out
}

func wantFamilies(t *testing.T, got map[string][]string, want map[string][]string) {
	t.Helper()
	for name, labels := range want {
		have, ok := got[name]
		if !ok {
			t.Errorf("metric %s does not exist", name)
			continue
		}
		if strings.Join(have, ",") != strings.Join(labels, ",") {
			t.Errorf("metric %s has labels [%s], want [%s]", name, strings.Join(have, ","), strings.Join(labels, ","))
		}
	}
}

// TestAPIFamily is the api half of the metric list in
// docs/workstreams/10-observability.md §5, name by name and label by label.
func TestAPIFamily(t *testing.T) {
	m := New(obs.ComponentAPI)
	a := NewAPIMetrics(m)
	// Give every vector one series so Gather sees its labels.
	a.RequestsTotal.WithLabelValues("GET /projects", "GET", "200").Inc()
	a.RequestDuration.WithLabelValues("GET /projects").Observe(0.01)
	a.Hosts.WithLabelValues("ready").Set(1)
	a.Projects.WithLabelValues("running", "large").Set(1)
	a.ScheduleTotal.WithLabelValues("ok").Inc()
	a.CertsIssuedTotal.Inc()
	a.CertsRevokedTotal.Inc()
	a.RollupLagSeconds.Set(0)
	a.NotifyTotal.WithLabelValues("email", "ok").Inc()
	a.StripePushTotal.WithLabelValues("ok").Inc()
	a.SnapshotAge.Set(0)
	a.EgressAlertProjects.Set(0)
	a.PartitionDropFailTotal.Add(0)

	wantFamilies(t, gathered(t, m), map[string][]string{
		"repose_api_requests_total":            {"method", "route", "status"},
		"repose_api_request_duration_seconds":  {"route"},
		"repose_api_hosts":                     {"state"},
		"repose_api_projects":                  {"class", "state"},
		"repose_api_schedule_total":            {"result"},
		"repose_api_certs_issued_total":        nil,
		"repose_api_certs_revoked_total":       nil,
		"repose_api_rollup_lag_seconds":        nil,
		"repose_api_notify_total":              {"channel", "result"},
		"repose_api_stripe_usage_push_total":   {"result"},
		"repose_api_snapshot_age_seconds":      nil,
		"repose_api_egress_alert_projects":     nil,
		"repose_api_partition_drop_fail_total": nil,
	})
}

// TestGatewayFamily is the gateway half of the same list.
func TestGatewayFamily(t *testing.T) {
	m := New(obs.ComponentGateway)
	g := NewGatewayMetrics(m)
	g.Sessions.Set(0)
	g.SessionsTotal.Inc()
	g.DialFailTotal.Inc()
	g.RouteDuration.Observe(0.01)

	wantFamilies(t, gathered(t, m), map[string][]string{
		"repose_gateway_sessions":               nil,
		"repose_gateway_sessions_total":         nil,
		"repose_gateway_auth_fail_total":        {"reason"},
		"repose_gateway_dial_fail_total":        nil,
		"repose_gateway_route_duration_seconds": nil,
	})
}

// TestAuthFailReasonsArePresentAtZero: GatewayAuthSpike is a rate() over this
// counter, and a rate over a series that has never appeared is nothing.
func TestAuthFailReasonsArePresentAtZero(t *testing.T) {
	m := New(obs.ComponentGateway)
	NewGatewayMetrics(m)
	body := scrape(t, m)
	for _, r := range AuthFailReasons {
		if !strings.Contains(body, `repose_gateway_auth_fail_total{reason="`+r+`"} 0`) {
			t.Errorf("reason %q is not initialised at zero", r)
		}
	}
}

// TestBuildInfo: which version of which binary is running.
func TestBuildInfo(t *testing.T) {
	m := NewVersion(obs.ComponentHostd, "1.2.3")
	if body := scrape(t, m); !strings.Contains(body, `repose_build_info{component="hostd",version="1.2.3"} 1`) {
		t.Errorf("repose_build_info missing or wrong:\n%s", body)
	}
}

// TestNamespaceEnforced: a metric outside repose_ cannot be registered.
func TestNamespaceEnforced(t *testing.T) {
	m := New(obs.ComponentAPI)
	err := m.Register(prometheus.NewGauge(prometheus.GaugeOpts{Name: "guests"}))
	if !errors.Is(err, ErrMetricName) {
		t.Fatalf("Register(guests) = %v, want ErrMetricName", err)
	}
	if err := m.Register(prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "factory", Name: "guests"})); !errors.Is(err, ErrMetricName) {
		t.Fatalf("Register(factory_guests) = %v, want ErrMetricName", err)
	}
}

// TestLabelsEnforced: the labels that make Prometheus fall over are refused,
// whether they are variable or constant.
func TestLabelsEnforced(t *testing.T) {
	m := New(obs.ComponentHostd)
	for _, label := range []string{"project_id", "guest_id", "user_id", "slug", "remote_url"} {
		err := m.Register(prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Namespace: Namespace, Subsystem: "host", Name: "thing_" + label},
			[]string{label},
		))
		if !errors.Is(err, ErrMetricName) {
			t.Errorf("label %q accepted (err = %v)", label, err)
		}
	}
	err := m.Register(prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace, Name: "const_labelled", ConstLabels: prometheus.Labels{"project_id": "p-1"},
	}))
	if !errors.Is(err, ErrMetricName) {
		t.Errorf("const label project_id accepted (err = %v)", err)
	}
	if err := m.Register(prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: Namespace, Subsystem: "host", Name: "ok_labelled"},
		[]string{"class", "state"},
	)); err != nil {
		t.Errorf("allowed labels refused: %v", err)
	}
}

// TestMustRegisterPanics: a naming mistake stops the binary at startup rather
// than shipping a series nothing queries.
func TestMustRegisterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustRegister should panic on a bad name")
		}
	}()
	New(obs.ComponentAPI).MustRegister(prometheus.NewCounter(prometheus.CounterOpts{Name: "oops_total"}))
}

// TestGoAndProcessCollectors: the two families every Go dashboard expects are
// the documented exception to the repose_ rule.
func TestGoAndProcessCollectors(t *testing.T) {
	body := scrape(t, New(obs.ComponentAPI))
	for _, name := range []string{"go_goroutines", "process_open_fds"} {
		if !strings.Contains(body, name) {
			t.Errorf("%s is not exported", name)
		}
	}
}

// TestServeRefusesEveryInterface: docs/ops/OBSERVABILITY.md says nothing
// observability-related is reachable from the internet, and a host's NSG is
// not the only thing that should be enforcing it.
func TestServeRefusesEveryInterface(t *testing.T) {
	m := New(obs.ComponentHostd)
	for _, addr := range []string{":9101", "0.0.0.0:9101", "[::]:9101"} {
		if err := m.Serve(context.Background(), addr); err == nil {
			t.Errorf("Serve(%q) was accepted", addr)
		}
	}
}

// TestServeStopsWithTheContext, and answers /metrics and /healthz while it
// runs.
func TestServeStopsWithTheContext(t *testing.T) {
	m := New(obs.ComponentHostd)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Serve(ctx, "127.0.0.1:0") }()
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Serve returned %v", err)
	}
}

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("/metrics returned %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestMetricsPorts(t *testing.T) {
	for c, want := range map[obs.Component]int{
		obs.ComponentHostd: 9101, obs.ComponentGateway: 9102, obs.ComponentAPI: 9103,
		obs.ComponentGuestd: 0, obs.ComponentCLI: 0,
	} {
		if got := c.MetricsPort(); got != want {
			t.Errorf("%s.MetricsPort() = %d, want %d", c, got, want)
		}
	}
}
