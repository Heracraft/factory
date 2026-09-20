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

type loggerKey struct{}

// WithLogger attaches a logger to a context, for the per-request fields the
// api adds once and every handler below it inherits.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// Logger returns the context's logger, or the fallback when there is none.
func Logger(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return fallback
}

// LogWithRequest returns a logger carrying the context's request_id, so a
// handler's own lines join the `request` line.
func LogWithRequest(ctx context.Context, log *slog.Logger) *slog.Logger {
	if id := RequestID(ctx); id != "" {
		return log.With("request_id", id)
	}
	return log
}
