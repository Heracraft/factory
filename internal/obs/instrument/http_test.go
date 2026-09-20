package instrument

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/obs"
	"github.com/heracraft/repose/internal/obs/metrics"
)

// line decodes the single JSON object a logger wrote, as obs's own tests do.
func line(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	s := strings.TrimSpace(buf.String())
	if s == "" {
		t.Fatal("nothing was logged")
	}
	if strings.ContainsRune(s, '\n') {
		t.Fatalf("more than one line: %q", s)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not JSON: %v: %s", err, s)
	}
	return m
}

// scrape renders a registry, as obs/metrics's own tests do.
func scrape(t *testing.T, m *metrics.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("/metrics returned %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// TestMiddlewareLogsAndCounts: one `request` line with the route template,
// and the two api series §5 names.
func TestMiddlewareLogsAndCounts(t *testing.T) {
	var buf bytes.Buffer
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentAPI, Writer: &buf})
	met := metrics.New(obs.ComponentAPI)
	m := metrics.NewAPIMetrics(met)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := HTTPMiddleware(HTTPOptions{Log: log, Metrics: m})(mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/projects/p-0199?access_token=shhh", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get(obs.RequestIDHeader) == "" {
		t.Error("no request id was returned")
	}
	out := buf.String()
	fields := line(t, &buf)
	if fields["event"] != obs.EventRequest {
		t.Errorf("event = %v, want %s", fields["event"], obs.EventRequest)
	}
	if fields["route"] != "GET /projects/{id}" {
		t.Errorf("route = %v, want the pattern, not the path", fields["route"])
	}
	if fields["status"].(float64) != http.StatusTeapot {
		t.Errorf("status = %v", fields["status"])
	}
	if _, ok := fields["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
	// The never-log list: no path, no query string, no address, no agent.
	for _, leak := range []string{"p-0199", "shhh", "192.0.2", "Go-http-client"} {
		if strings.Contains(out, leak) {
			t.Errorf("the request line carries %q: %s", leak, out)
		}
	}
	if got := scrape(t, met); !strings.Contains(got,
		`repose_api_requests_total{method="GET",route="GET /projects/{id}",status="418"} 1`) {
		t.Errorf("request counter missing:\n%s", got)
	}
}

// TestMiddlewareUnmatchedRoute: a 404 must not put one label value per
// unknown path into Prometheus.
func TestMiddlewareUnmatchedRoute(t *testing.T) {
	var buf bytes.Buffer
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentAPI, Writer: &buf})
	h := HTTPMiddleware(HTTPOptions{Log: log})(http.NewServeMux())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/nope/nope/nope", nil))
	if got := line(t, &buf)["route"]; got != "unmatched" {
		t.Errorf("route = %v, want unmatched", got)
	}
}

// TestRequestIDFlowsToHandlers: a handler's own lines join the request line.
func TestRequestIDFlowsToHandlers(t *testing.T) {
	var buf bytes.Buffer
	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentAPI, Writer: &buf})
	var inner string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /x", func(_ http.ResponseWriter, r *http.Request) {
		inner = obs.RequestID(r.Context())
		obs.LogWithRequest(r.Context(), log).Info("scheduled", "event", obs.EventSchedule, "host_id", "h-1")
	})
	h := HTTPMiddleware(HTTPOptions{Log: log})(mux)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if inner == "" {
		t.Fatal("the handler saw no request id")
	}
	if inner != rec.Header().Get(obs.RequestIDHeader) {
		t.Errorf("the handler's id %q is not the one returned %q", inner, rec.Header().Get(obs.RequestIDHeader))
	}
	if n := strings.Count(buf.String(), inner); n != 2 {
		t.Errorf("request id appears on %d lines, want 2:\n%s", n, buf.String())
	}
}

// TestRequestIDFromTheClientIsKept, so a CLI can quote the id it sent.
func TestRequestIDFromTheClientIsKept(t *testing.T) {
	h := HTTPMiddleware(HTTPOptions{})(http.NewServeMux())
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set(obs.RequestIDHeader, "cli-123")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(obs.RequestIDHeader); got != "cli-123" {
		t.Errorf("request id = %q, want cli-123", got)
	}
}
