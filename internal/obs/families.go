package obs

import "github.com/prometheus/client_golang/prometheus"

// The api and gateway metric families of
// docs/workstreams/10-observability.md §5 live here rather than in
// internal/api and internal/gateway because this workstream owns metric
// names and those two binaries are built later (workstreams 05 and 06). A
// family defined here is a family they cannot accidentally rename: the test
// in metrics_test.go asserts every name and label of §5, and the dashboards
// and alert rules under ops/ query exactly these series.
//
// hostd's family is in internal/hostd/metrics, registered through the same
// checked registry.
//
// Two series here are not in §5's list because §5's alert table and failure
// modes ask for them and nothing else could carry them:
// repose_api_egress_alert_projects (the EgressHigh input, which cannot be a
// per-project series) and repose_api_partition_drop_fail_total (the §6
// failure mode that says the api "logs partition_drop_fail and alerts").

// Buckets shared by the duration histograms. An api request that takes more
// than 10 s is a bug; a gateway route lookup more than 1 s is one too, so
// the high buckets exist only to keep the +Inf bucket from swallowing them.
var (
	apiRequestBuckets   = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}
	gatewayRouteBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5}
)

// AuthFailReasons is the `reason` enum of the gateway's auth_fail event and
// of repose_gateway_auth_fail_total (docs/workstreams/10-observability.md §5,
// docs/interfaces/ssh-gateway.md).
var AuthFailReasons = []string{"bad_cert", "expired", "revoked", "wrong_principal", "stopped", "not_found"}

// APIMetrics is the api's family. Every field is registered on the Metrics
// passed to NewAPIMetrics.
type APIMetrics struct {
	RequestsTotal     *prometheus.CounterVec   // route, method, status
	RequestDuration   *prometheus.HistogramVec // route
	Hosts             *prometheus.GaugeVec     // state
	Projects          *prometheus.GaugeVec     // state, class
	ScheduleTotal     *prometheus.CounterVec   // result
	CertsIssuedTotal  prometheus.Counter
	CertsRevokedTotal prometheus.Counter
	RollupLagSeconds  prometheus.Gauge
	NotifyTotal       *prometheus.CounterVec // channel, result
	StripePushTotal   *prometheus.CounterVec // result
	SnapshotAge       prometheus.Gauge
	// EgressAlertProjects is the EgressHigh alert input: the number of
	// projects over 1 TB of egress in the last 24 hours, recomputed hourly
	// by the api from meter_samples. It is a count, not a label per project,
	// for the reason in allowedLabels.
	EgressAlertProjects prometheus.Gauge
	// PartitionDropFailTotal is the §6 failure mode "proc_samples partition
	// drop fails": the api logs partition_drop_fail and alerts, and an alert
	// needs a series. Disk grows and nothing else breaks, so it is a warning.
	PartitionDropFailTotal prometheus.Counter
}

// NewAPIMetrics registers the api family.
func NewAPIMetrics(m *Metrics) *APIMetrics {
	f := factory{m, "api"}
	a := &APIMetrics{
		RequestsTotal:       f.counterVec("requests_total", "HTTP requests by route, method and status.", "route", "method", "status"),
		RequestDuration:     f.histogramVec("request_duration_seconds", "HTTP request duration by route.", apiRequestBuckets, "route"),
		Hosts:               f.gaugeVec("hosts", "Hosts by state.", "state"),
		Projects:            f.gaugeVec("projects", "Projects by state and size class.", "state", "class"),
		ScheduleTotal:       f.counterVec("schedule_total", "Placement attempts by result.", "result"),
		CertsIssuedTotal:    f.counter("certs_issued_total", "SSH certificates issued."),
		CertsRevokedTotal:   f.counter("certs_revoked_total", "SSH certificates revoked."),
		RollupLagSeconds:    f.gauge("rollup_lag_seconds", "Seconds between now and the last hour rolled up into usage_hours."),
		NotifyTotal:         f.counterVec("notify_total", "Notification deliveries by channel and result.", "channel", "result"),
		StripePushTotal:     f.counterVec("stripe_usage_push_total", "Stripe usage record pushes by result.", "result"),
		SnapshotAge:         f.gauge("snapshot_age_seconds", "Age of the oldest last-snapshot among running projects."),
		EgressAlertProjects: f.gauge("egress_alert_projects", "Projects over 1 TB of egress in the last 24 hours."),
		PartitionDropFailTotal: f.counter("partition_drop_fail_total",
			"Hourly partition maintenance runs that failed (docs/workstreams/10-observability.md §6)."),
	}
	// The StripePushFail alert is an increase() over the error series and the
	// Billing dashboard has a panel for it, so both series exist at zero
	// rather than appearing on the first failure. ScheduleTotal is left alone:
	// its `result` values are workstream 05's to name.
	a.StripePushTotal.WithLabelValues("ok")
	a.StripePushTotal.WithLabelValues("error")
	return a
}

// GatewayMetrics is the gateway's family.
type GatewayMetrics struct {
	Sessions      prometheus.Gauge
	SessionsTotal prometheus.Counter
	AuthFailTotal *prometheus.CounterVec // reason
	DialFailTotal prometheus.Counter
	RouteDuration prometheus.Histogram
}

// NewGatewayMetrics registers the gateway family. The auth-failure counter
// is initialised at zero for every reason, because
// rate(repose_gateway_auth_fail_total[5m]) in the GatewayAuthSpike alert
// cannot fire on a series that has never appeared.
func NewGatewayMetrics(m *Metrics) *GatewayMetrics {
	f := factory{m, "gateway"}
	g := &GatewayMetrics{
		Sessions:      f.gauge("sessions", "SSH sessions relayed right now."),
		SessionsTotal: f.counter("sessions_total", "SSH sessions relayed."),
		AuthFailTotal: f.counterVec("auth_fail_total", "Authentication failures by reason.", "reason"),
		DialFailTotal: f.counter("dial_fail_total", "Failures dialling a guest's sshd."),
		RouteDuration: f.histogram("route_duration_seconds", "Time to resolve a login name to a guest address.", gatewayRouteBuckets),
	}
	for _, r := range AuthFailReasons {
		g.AuthFailTotal.WithLabelValues(r)
	}
	return g
}

// factory registers each instrument on a Metrics under repose_<subsystem>_.
type factory struct {
	m         *Metrics
	subsystem string
}

func (f factory) gauge(name, help string) prometheus.Gauge {
	c := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: Namespace, Subsystem: f.subsystem, Name: name, Help: help})
	f.m.MustRegister(c)
	return c
}

func (f factory) gaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	c := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: Namespace, Subsystem: f.subsystem, Name: name, Help: help}, labels)
	f.m.MustRegister(c)
	return c
}

func (f factory) counter(name, help string) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{Namespace: Namespace, Subsystem: f.subsystem, Name: name, Help: help})
	f.m.MustRegister(c)
	return c
}

func (f factory) counterVec(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: Namespace, Subsystem: f.subsystem, Name: name, Help: help}, labels)
	f.m.MustRegister(c)
	return c
}

func (f factory) histogram(name, help string, buckets []float64) prometheus.Histogram {
	c := prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: Namespace, Subsystem: f.subsystem, Name: name, Help: help, Buckets: buckets})
	f.m.MustRegister(c)
	return c
}

func (f factory) histogramVec(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	c := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: Namespace, Subsystem: f.subsystem, Name: name, Help: help, Buckets: buckets}, labels)
	f.m.MustRegister(c)
	return c
}
