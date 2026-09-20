package gateway

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics are the repose_gateway_* series of 06-gateway-edge.md §5.5 plus
// the three of 10-observability.md that are not synonyms of them
// (auth_fail_total{reason}, sessions_total, route_duration_seconds; see
// DECISIONS I-49).
type Metrics struct {
	ConnectionsOpen    prometheus.Gauge
	SessionsTotal      prometheus.Counter
	AuthTotal          *prometheus.CounterVec
	AuthFailTotal      *prometheus.CounterVec
	RelayBytesTotal    *prometheus.CounterVec
	DialErrorsTotal    prometheus.Counter
	SessionSeconds     prometheus.Histogram
	RevocationCacheAge prometheus.Gauge
	RouteDuration      prometheus.Histogram
	WGSyncPeers        prometheus.Gauge
	WGSyncErrorsTotal  prometheus.Counter
	HookEventsTotal    *prometheus.CounterVec
}

// Auth results (the `result` label). Every failure is also counted in
// AuthFailTotal under the same word as `reason`.
const (
	ResultOK             = "ok"
	ResultNoCert         = "no_cert"
	ResultBadCA          = "bad_ca"
	ResultRevoked        = "revoked"
	ResultExpired        = "expired"
	ResultWrongPrincipal = "wrong_principal"
	ResultStopped        = "stopped"
	ResultRouteError     = "route_error"
	ResultRateLimited    = "rate_limited"
	ResultNotFound       = "not_found"
	ResultBadLogin       = "bad_login"
	ResultBusy           = "busy"
)

// AuthResults lists every value the `result` label takes, so the series
// exist from the first scrape.
var AuthResults = []string{
	ResultOK, ResultNoCert, ResultBadCA, ResultRevoked, ResultExpired, ResultWrongPrincipal,
	ResultStopped, ResultRouteError, ResultRateLimited, ResultNotFound, ResultBadLogin, ResultBusy,
}

// NewMetrics registers the series on reg (nil registers nothing).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		ConnectionsOpen: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "repose_gateway_connections_open", Help: "Relays currently open.",
		}),
		SessionsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "repose_gateway_sessions_total", Help: "Relays opened since start.",
		}),
		AuthTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "repose_gateway_auth_total", Help: "Authentication outcomes by result.",
		}, []string{"result"}),
		AuthFailTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "repose_gateway_auth_fail_total", Help: "Authentication failures by reason.",
		}, []string{"reason"}),
		RelayBytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "repose_gateway_relay_bytes_total", Help: "Bytes relayed by direction (client_to_guest, guest_to_client).",
		}, []string{"direction"}),
		DialErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "repose_gateway_dial_errors_total", Help: "Failed dials to a guest's sshd.",
		}),
		SessionSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "repose_gateway_session_seconds", Help: "Relay duration.",
			Buckets: []float64{1, 10, 60, 300, 1800, 3600, 4 * 3600, 12 * 3600, 24 * 3600},
		}),
		RevocationCacheAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "repose_gateway_revocation_cache_age_seconds", Help: "Seconds since the revocation list was last refreshed.",
		}),
		RouteDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "repose_gateway_route_duration_seconds", Help: "Latency of /internal/route lookups.",
			Buckets: prometheus.DefBuckets,
		}),
		WGSyncPeers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "repose_gateway_wgsync_peers", Help: "WireGuard peers wgsync last applied.",
		}),
		WGSyncErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "repose_gateway_wgsync_errors_total", Help: "wgsync cycles that failed.",
		}),
		HookEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "repose_gateway_hook_events_total", Help: "Hook events received on the ingest port by result.",
		}, []string{"result"}),
	}
	for _, r := range AuthResults {
		m.AuthTotal.WithLabelValues(r)
		if r != ResultOK {
			m.AuthFailTotal.WithLabelValues(r)
		}
	}
	m.RelayBytesTotal.WithLabelValues("client_to_guest")
	m.RelayBytesTotal.WithLabelValues("guest_to_client")
	for _, r := range []string{"forwarded", "rejected", "rate_limited", "api_error", "not_found"} {
		m.HookEventsTotal.WithLabelValues(r)
	}
	if reg != nil {
		reg.MustRegister(m.ConnectionsOpen, m.SessionsTotal, m.AuthTotal, m.AuthFailTotal, m.RelayBytesTotal,
			m.DialErrorsTotal, m.SessionSeconds, m.RevocationCacheAge, m.RouteDuration, m.WGSyncPeers,
			m.WGSyncErrorsTotal, m.HookEventsTotal)
	}
	return m
}

// auth records one outcome.
func (m *Metrics) auth(result string) {
	m.AuthTotal.WithLabelValues(result).Inc()
	if result != ResultOK {
		m.AuthFailTotal.WithLabelValues(result).Inc()
	}
}
