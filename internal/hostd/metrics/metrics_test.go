package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Every metric named in docs/workstreams/03-hostd.md §5.14 must be exposed.
func TestDocumentedMetricsExposed(t *testing.T) {
	m := New()
	m.Guests.WithLabelValues("running", "large").Set(1)
	m.CommandsTotal.WithLabelValues("CreateGuest", "ok").Inc()
	m.CommandDuration.WithLabelValues("CreateGuest").Observe(1)
	m.BuildDuration.WithLabelValues("ok").Observe(1)
	m.SnapshotFreezeSeconds.Observe(0.1)
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	for _, name := range []string{
		"repose_host_guests", "repose_host_commands_total", "repose_host_command_duration_seconds",
		"repose_host_build_queue_depth", "repose_host_build_duration_seconds", "repose_host_snapshot_bytes_total",
		"repose_host_snapshot_freeze_seconds", "repose_host_stream_connected", "repose_host_stream_reconnects_total",
		"repose_host_samples_dropped_total", "repose_host_pool_free_bytes", "repose_host_store_bytes",
		"repose_host_builds_running", "repose_host_guestd_lost",
		"repose_host_mem_free_bytes", "repose_host_mem_reserved_bytes",
	} {
		if !strings.Contains(body, "# TYPE "+name+" ") {
			t.Errorf("metric %s not exposed", name)
		}
	}
}
