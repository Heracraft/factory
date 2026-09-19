// Package metrics is hostd's Prometheus surface: every series named in
// docs/workstreams/03-hostd.md §5.14 and the host family in
// docs/workstreams/10-observability.md, under the repose_host_ prefix.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// M holds the registry and every instrument.
type M struct {
	Registry *prometheus.Registry

	Guests                *prometheus.GaugeVec
	CommandsTotal         *prometheus.CounterVec
	CommandDuration       *prometheus.HistogramVec
	BuildQueueDepth       prometheus.Gauge
	BuildsRunning         prometheus.Gauge
	BuildDuration         *prometheus.HistogramVec
	SnapshotBytesTotal    prometheus.Counter
	SnapshotBytes         prometheus.Gauge
	SnapshotFreezeSeconds prometheus.Histogram
	SnapshotDuration      *prometheus.HistogramVec
	StreamConnected       prometheus.Gauge
	StreamReconnectsTotal prometheus.Counter
	SamplesDroppedTotal   prometheus.Counter
	PoolFreeBytes         prometheus.Gauge
	PoolBytes             prometheus.Gauge
	StoreBytes            prometheus.Gauge
	MemFreeBytes          prometheus.Gauge
	MemReservedBytes      prometheus.Gauge
	GuestdUnreachable     *prometheus.GaugeVec
	GuestdLost            prometheus.Gauge
	GuestCPUSecondsTotal  *prometheus.CounterVec
	GuestNetBytesTotal    *prometheus.CounterVec
	EventsPending         prometheus.Gauge
}

// New registers every instrument on a fresh registry.
func New() *M {
	reg := prometheus.NewRegistry()
	f := promauto{reg}
	m := &M{
		Registry:              reg,
		Guests:                f.gaugeVec("guests", "Guests on this host by state and class.", "state", "class"),
		CommandsTotal:         f.counterVec("commands_total", "Commands handled by kind and result.", "kind", "result"),
		CommandDuration:       f.histVec("command_duration_seconds", "Command execution time by kind.", []float64{.1, .5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1800}, "kind"),
		BuildQueueDepth:       f.gauge("build_queue_depth", "Builds waiting for a worker."),
		BuildsRunning:         f.gauge("builds_running", "Builds executing now."),
		BuildDuration:         f.histVec("build_duration_seconds", "Build wall time by result.", []float64{5, 15, 30, 60, 120, 300, 600, 1200, 1800}, "result"),
		SnapshotBytesTotal:    f.counter("snapshot_bytes_total", "Bytes uploaded to the snapshot store."),
		SnapshotBytes:         f.gauge("snapshot_bytes", "Bytes of the most recent snapshot upload."),
		SnapshotFreezeSeconds: f.hist("snapshot_freeze_seconds", "Freeze window per snapshot.", []float64{.05, .1, .25, .5, 1, 2, 5, 10}),
		SnapshotDuration:      f.histVec("snapshot_duration_seconds", "Snapshot wall time by reason and result.", []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800}, "reason", "result"),
		StreamConnected:       f.gauge("stream_connected", "1 while the Session stream to the api is up."),
		StreamReconnectsTotal: f.counter("stream_reconnects_total", "Session stream reconnects."),
		SamplesDroppedTotal:   f.counter("samples_dropped_total", "Samples dropped from the outage buffer."),
		PoolFreeBytes:         f.gauge("pool_free_bytes", "Thin pool free bytes."),
		PoolBytes:             f.gauge("pool_bytes", "Thin pool size."),
		StoreBytes:            f.gauge("store_bytes", "Bytes used by the host store filesystem."),
		MemFreeBytes:          f.gauge("mem_free_bytes", "Host free memory after the reserve."),
		MemReservedBytes:      f.gauge("mem_reserved_bytes", "Memory reserved by running guests."),
		GuestdUnreachable:     f.gaugeVec("guestd_unreachable", "1 while a guest's guestd is unreachable.", "guest_id"),
		GuestdLost:            f.gauge("guestd_lost", "Count of guests with guestd_ok=false."),
		GuestCPUSecondsTotal:  f.counterVec("guest_cpu_seconds_total", "Guest CPU time summed by class.", "class"),
		GuestNetBytesTotal:    f.counterVec("guest_net_bytes_total", "Guest network bytes by direction.", "direction"),
		EventsPending:         f.gauge("events_pending", "Events awaiting an api ack."),
	}
	return m
}

// Handler serves the registry.
func (m *M) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

type promauto struct{ reg *prometheus.Registry }

const ns = "repose_host"

func (p promauto) gauge(name, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Namespace: ns, Name: name, Help: help})
	p.reg.MustRegister(g)
	return g
}

func (p promauto) gaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: ns, Name: name, Help: help}, labels)
	p.reg.MustRegister(g)
	return g
}

func (p promauto) counter(name, help string) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{Namespace: ns, Name: name, Help: help})
	p.reg.MustRegister(c)
	return c
}

func (p promauto) counterVec(name, help string, labels ...string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: ns, Name: name, Help: help}, labels)
	p.reg.MustRegister(c)
	return c
}

func (p promauto) hist(name, help string, buckets []float64) prometheus.Histogram {
	h := prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: ns, Name: name, Help: help, Buckets: buckets})
	p.reg.MustRegister(h)
	return h
}

func (p promauto) histVec(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: ns, Name: name, Help: help, Buckets: buckets}, labels)
	p.reg.MustRegister(h)
	return h
}
