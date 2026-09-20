// Package metrics defines the repose_api_* families from
// docs/workstreams/10-observability.md §5 and 05-control-plane-api.md
// §5.15. Labels are bounded enums only; project and guest ids never
// appear as labels.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// M holds every api metric.
type M struct {
	RequestsTotal        *prometheus.CounterVec
	RequestDuration      *prometheus.HistogramVec
	Hosts                *prometheus.GaugeVec
	Projects             *prometheus.GaugeVec
	ScheduleTotal        *prometheus.CounterVec
	CertsIssuedTotal     prometheus.Counter
	CertsRevokedTotal    prometheus.Counter
	RollupLagSeconds     prometheus.Gauge
	RollupDuration       prometheus.Histogram
	NotifyTotal          *prometheus.CounterVec
	OutboxDepth          prometheus.Gauge
	OutboxLagSeconds     prometheus.Gauge
	StripeUsagePushTotal *prometheus.CounterVec
	SnapshotAgeSeconds   prometheus.Gauge
	GRPCStreams          prometheus.Gauge
	OpsTotal             *prometheus.CounterVec
	OpsOpen              *prometheus.GaugeVec
	BuildDuration        *prometheus.HistogramVec
	SecretsOpsTotal      *prometheus.CounterVec
	CommandsTotal        *prometheus.CounterVec
	SamplesTotal         prometheus.Counter
	EventsTotal          *prometheus.CounterVec
	HostWarningsTotal    *prometheus.CounterVec
	EgressAlertProjects  prometheus.Gauge
	BillingGapMinutes    prometheus.Counter
	KeyVaultErrorsTotal  prometheus.Counter
}

// New registers every family on reg.
func New(reg prometheus.Registerer) *M {
	m := &M{
		RequestsTotal:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_requests_total", Help: "HTTP requests by route, method and status."}, []string{"route", "method", "status"}),
		RequestDuration:      prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "repose_api_request_duration_seconds", Help: "HTTP request latency by route.", Buckets: prometheus.DefBuckets}, []string{"route"}),
		Hosts:                prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "repose_api_hosts", Help: "Hosts by state."}, []string{"state"}),
		Projects:             prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "repose_api_projects", Help: "Projects by state and class."}, []string{"state", "class"}),
		ScheduleTotal:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_schedule_total", Help: "Placement attempts by result."}, []string{"result"}),
		CertsIssuedTotal:     prometheus.NewCounter(prometheus.CounterOpts{Name: "repose_api_certs_issued_total", Help: "User certificates issued."}),
		CertsRevokedTotal:    prometheus.NewCounter(prometheus.CounterOpts{Name: "repose_api_certs_revoked_total", Help: "Certificates revoked."}),
		RollupLagSeconds:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "repose_api_rollup_lag_seconds", Help: "Age of the newest rolled-up hour."}),
		RollupDuration:       prometheus.NewHistogram(prometheus.HistogramOpts{Name: "repose_api_rollup_duration_seconds", Help: "Hourly rollup duration.", Buckets: prometheus.ExponentialBuckets(0.01, 4, 8)}),
		NotifyTotal:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_notify_total", Help: "Notification deliveries by channel and result."}, []string{"channel", "result"}),
		OutboxDepth:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "repose_api_outbox_depth", Help: "Undelivered outbox rows."}),
		OutboxLagSeconds:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "repose_api_outbox_lag_seconds", Help: "Age of the oldest undelivered outbox row."}),
		StripeUsagePushTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_stripe_usage_push_total", Help: "Usage record pushes by result."}, []string{"result"}),
		SnapshotAgeSeconds:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "repose_api_snapshot_age_seconds", Help: "Oldest newest-snapshot age over running projects."}),
		GRPCStreams:          prometheus.NewGauge(prometheus.GaugeOpts{Name: "repose_api_grpc_streams", Help: "Connected host streams."}),
		OpsTotal:             prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_ops_total", Help: "Ops finished by kind and state."}, []string{"kind", "state"}),
		OpsOpen:              prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "repose_api_ops_open", Help: "Ops pending or running by kind."}, []string{"kind"}),
		BuildDuration:        prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "repose_api_build_duration_seconds", Help: "Build op duration by result.", Buckets: prometheus.ExponentialBuckets(1, 2, 12)}, []string{"result"}),
		SecretsOpsTotal:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_secrets_ops_total", Help: "Secret operations by op."}, []string{"op"}),
		CommandsTotal:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_commands_total", Help: "hostd commands by kind and result."}, []string{"kind", "result"}),
		SamplesTotal:         prometheus.NewCounter(prometheus.CounterOpts{Name: "repose_api_samples_total", Help: "Sample messages ingested."}),
		EventsTotal:          prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_events_total", Help: "Events ingested by kind."}, []string{"kind"}),
		HostWarningsTotal:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "repose_api_host_warnings_total", Help: "Host warnings by kind."}, []string{"kind"}),
		EgressAlertProjects:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "repose_api_egress_alert_projects", Help: "Projects over 1 TB egress in 24 h."}),
		BillingGapMinutes:    prometheus.NewCounter(prometheus.CounterOpts{Name: "repose_api_billing_gap_minutes_total", Help: "Minutes a running project had no sample."}),
		KeyVaultErrorsTotal:  prometheus.NewCounter(prometheus.CounterOpts{Name: "repose_api_keyvault_errors_total", Help: "Key Vault failures."}),
	}
	reg.MustRegister(m.RequestsTotal, m.RequestDuration, m.Hosts, m.Projects, m.ScheduleTotal, m.CertsIssuedTotal, m.CertsRevokedTotal,
		m.RollupLagSeconds, m.RollupDuration, m.NotifyTotal, m.OutboxDepth, m.OutboxLagSeconds, m.StripeUsagePushTotal, m.SnapshotAgeSeconds,
		m.GRPCStreams, m.OpsTotal, m.OpsOpen, m.BuildDuration, m.SecretsOpsTotal, m.CommandsTotal, m.SamplesTotal, m.EventsTotal,
		m.HostWarningsTotal, m.EgressAlertProjects, m.BillingGapMinutes, m.KeyVaultErrorsTotal)
	return m
}

// NewNop returns metrics on a private registry (tests).
func NewNop() *M { return New(prometheus.NewRegistry()) }
