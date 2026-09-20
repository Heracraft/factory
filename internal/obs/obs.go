// Package obs is the shared observability floor: a structured JSON logger
// that carries `component` on every line and redacts the field names
// docs/ops/OBSERVABILITY.md forbids, and a Prometheus registry in the
// repose_ namespace. Workstream 10 owns the naming rules; this is the
// subset the api needs.
package obs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Redacted replaces the value of any forbidden field.
const Redacted = "[redacted]"

// forbidden are substrings of attribute keys whose values are never
// logged. The reviewer is the real check; this is the floor.
var forbidden = []string{
	"token", "secret", "password", "authorization", "cert", "key",
	"email", "handle", "remote_url", "argv", "environ", "prompt", "summary",
	"fragment", "ntfy_url", "github_login", "public_key", "ciphertext",
}

// allowed are exact keys that contain a forbidden substring but carry no
// tenant data: bounded enums and identifiers.
var allowed = map[string]bool{
	"kind": true, "key_version": true, "cert_serial": true, "certs": true,
	"keys": true, "secrets": true, "secret_count": true, "secret_names_count": true,
	"cert_kind": true,
}

// IsForbidden reports whether an attribute key must be redacted.
func IsForbidden(key string) bool {
	k := strings.ToLower(key)
	if allowed[k] {
		return false
	}
	for _, f := range forbidden {
		if strings.Contains(k, f) {
			return true
		}
	}
	return false
}

func redact(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.String("ts", a.Value.Time().UTC().Format("2006-01-02T15:04:05.000Z07:00"))
	}
	if IsForbidden(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	return a
}

// NewLogger returns the JSON logger for one component.
func NewLogger(component string, w io.Writer, level slog.Level) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: redact})
	return slog.New(h).With("component", component)
}

// LevelNotice sits between Info and Warn for audited operator actions.
const LevelNotice = slog.Level(2)

// Registry is a Prometheus registry with the process and Go collectors.
func Registry() *prometheus.Registry {
	r := prometheus.NewRegistry()
	r.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return r
}

type ctxKey struct{}

// WithLogger attaches a logger to a context (per-request fields).
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// Logger returns the context's logger or the fallback.
func Logger(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return fallback
}
