package obs

import (
	"context"
	"log/slog"
)

// RequestIDHeader is the header the api echoes so a user's bug report and a
// Loki line can be joined by request_id. The middleware that sets it is in
// internal/obs/instrument; the context helpers are here because they are
// stdlib-only and a handler anywhere may want them.
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
