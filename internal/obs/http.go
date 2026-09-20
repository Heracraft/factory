package obs

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// RequestIDHeader is the header the api echoes so a user's bug report and a
// Loki line can be joined by request_id.
const RequestIDHeader = "X-Request-Id"

type requestIDKey struct{}

// WithRequestID puts a request id on a context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request id on a context, or "".
func RequestID(ctx context.Context) string {
	s, _ := ctx.Value(requestIDKey{}).(string)
	return s
}

// LogWithRequest returns a logger carrying the context's request_id, so a
// handler's own lines join the `request` line.
func LogWithRequest(ctx context.Context, log *slog.Logger) *slog.Logger {
	if id := RequestID(ctx); id != "" {
		return log.With("request_id", id)
	}
	return log
}

// HTTPOptions configures HTTPMiddleware.
type HTTPOptions struct {
	Log     *slog.Logger
	Metrics *APIMetrics
	// Route names the request for the `route` label and log field. The
	// default uses the pattern http.ServeMux matched, which is a template
	// ("GET /projects/{id}") and therefore bounded; a path would put one
	// label value per project into Prometheus.
	Route func(*http.Request) string
	// Tracing wraps the handler with otelhttp when true. It costs one
	// context value per request while no OTLP endpoint is set.
	Tracing bool
}

// HTTPMiddleware is the api's request layer: a request id, the `request` log
// event with method, route, status and duration_ms, the
// repose_api_requests_total and repose_api_request_duration_seconds series,
// and an OpenTelemetry span.
//
// It logs no IP address, no user agent, no query string and no header
// beyond the request id, because docs/ops/OBSERVABILITY.md forbids the first
// two and a query string can carry the `?access_token=` of DECISIONS I-7.
func HTTPMiddleware(o HTTPOptions) func(http.Handler) http.Handler {
	route := o.Route
	if route == nil {
		route = func(r *http.Request) string {
			if r.Pattern != "" {
				return r.Pattern
			}
			return "unmatched"
		}
	}
	return func(next http.Handler) http.Handler {
		h := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if id == "" {
				id = uuid.NewString()
			}
			w.Header().Set(RequestIDHeader, id)
			r = r.WithContext(WithRequestID(r.Context(), id))

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			d := time.Since(start)

			// r.Pattern is set by http.ServeMux during ServeHTTP, so the
			// route is only known now.
			rt := route(r)
			status := strconv.Itoa(rec.status)
			if o.Metrics != nil {
				o.Metrics.RequestsTotal.WithLabelValues(rt, r.Method, status).Inc()
				o.Metrics.RequestDuration.WithLabelValues(rt).Observe(d.Seconds())
			}
			if o.Log != nil {
				lvl := slog.LevelInfo
				if rec.status >= 500 {
					lvl = slog.LevelError
				}
				o.Log.Log(r.Context(), lvl, "http request", "event", EventRequest,
					"request_id", id, "method", r.Method, "route", rt,
					"status", rec.status, "duration_ms", d.Milliseconds())
			}
		}))
		if o.Tracing {
			// The span name is the route template for the same reason the
			// label is.
			h = otelhttp.NewHandler(h, "http", otelhttp.WithSpanNameFormatter(
				func(_ string, r *http.Request) string { return r.Method + " " + r.URL.Path },
			))
		}
		return h
	}
}

// statusRecorder remembers the status code and whether anything was written.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.written = true
	return s.ResponseWriter.Write(b)
}

// Flush forwards to the wrapped writer when it can, so the SSE build-log
// route of docs/interfaces/api.md still streams through the middleware.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
