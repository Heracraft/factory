package obs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// LevelNotice sits between Info and Warn: the operator-visible events that
// are not problems (a guest started, an operator logged in). hostd used
// slog.Level(2) for this before obs existed; the name is here so the JSON
// says NOTICE instead of INFO+2.
const LevelNotice = slog.Level(2)

// Redacted replaces the value of a field whose name is on the never-log
// list. It is deliberately not the empty string: a reviewer reading a log
// line must be able to tell "we refused to log this" from "this was empty".
const Redacted = "[redacted]"

// redactedFields are replaced with Redacted wherever they appear as a log
// field name, at any nesting depth. The first six are the floor
// docs/ops/OBSERVABILITY.md promises; the rest are never-log entries from
// the same document that have an obvious field name, so that the floor
// covers the mistake as well as the malice.
//
// Matching is on the exact lowercased name, never a prefix, because the
// names next to them are legitimate: cert_serial is an id, key_id names a
// Key Vault key, token_used is a boolean.
var redactedFields = map[string]bool{
	"token": true, "secret": true, "password": true,
	"authorization": true, "cert": true, "key": true,
	"email": true, "handle": true, "remote_url": true,
	"prompt": true, "args": true, "argv": true, "env": true,
	"cmdline": true, "command_line": true, "user_agent": true,
}

// RedactedFields lists the field names NewLogger redacts, for tests and for
// the obslint rules that refuse them at build time.
func RedactedFields() []string {
	out := make([]string, 0, len(redactedFields))
	for k := range redactedFields {
		out = append(out, k)
	}
	return out
}

// LogOptions configures NewLogger. Only Component is required.
type LogOptions struct {
	// Component is the binary's name in the log and must be valid; an
	// invalid one panics, because a log line with the wrong component is
	// invisible to every query in docs/ops/OBSERVABILITY.md.
	Component Component
	// Level defaults to slog.LevelInfo. Use ParseLevel for a flag value.
	Level slog.Level
	// Writer defaults to os.Stdout. guestd passes os.Stderr, which is the
	// serial console hostd captures, so that logging keeps working while the
	// guest's root filesystem is frozen for a snapshot.
	Writer io.Writer
	// OnViolation, when set, is called with a description of every line that
	// breaks the §5 rules (no event field, or a field name that must never
	// be logged). NewTestLogger points it at t.Errorf; production leaves it
	// nil and relies on redaction plus obslint.
	OnViolation func(string)
}

// NewLogger builds the logger of docs/workstreams/10-observability.md §5:
// one JSON object per line with ts, level, component and msg always
// present, and the never-log fields redacted.
func NewLogger(o LogOptions) *slog.Logger {
	if err := o.Component.check(); err != nil {
		// As in NewMetrics: the component is a constant, and a log line with
		// the wrong component is invisible to every query in
		// docs/ops/OBSERVABILITY.md, which is worse than not starting.
		panic(err)
	}
	w := o.Writer
	if w == nil {
		w = os.Stdout
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       o.Level,
		ReplaceAttr: replaceAttr,
	})
	return slog.New(&handler{
		Handler:   h,
		component: o.Component,
		violation: o.OnViolation,
	})
}

// NewTestLogger writes to w (use io.Discard for a quiet test) and fails the
// test on any line that breaks the §5 rules, so a component's own unit
// tests are the first place an unnamed event shows up.
func NewTestLogger(t interface{ Errorf(string, ...any) }, c Component, w io.Writer) *slog.Logger {
	if w == nil {
		w = io.Discard
	}
	return NewLogger(LogOptions{
		Component:   c,
		Level:       slog.LevelDebug,
		Writer:      w,
		OnViolation: func(s string) { t.Errorf("log line breaks docs/workstreams/10-observability.md §5: %s", s) },
	})
}

// Nop is the logger a constructor falls back to when its caller passed
// none: it writes nowhere but keeps the type and the component, so a
// library never has to check for nil and never reaches slog's default
// handler, which no repose binary configures.
func Nop(c Component) *slog.Logger {
	return NewLogger(LogOptions{Component: c, Level: slog.LevelError + 1, Writer: io.Discard})
}

// ParseLevel maps a --log-level flag value to a slog level. The names are
// the JSON `level` values lowercased.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "notice":
		return LevelNotice, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q; use debug, info, notice, warn or error", s)
	}
}

// replaceAttr renames slog's built-in keys to the §5 names and formats the
// two that need it: ts as RFC 3339 UTC, level with NOTICE named.
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 {
		switch a.Key {
		case slog.TimeKey:
			return slog.String("ts", a.Value.Time().UTC().Format(time.RFC3339))
		case slog.MessageKey:
			return slog.String("msg", a.Value.String())
		case slog.LevelKey:
			if lv, ok := a.Value.Any().(slog.Level); ok && lv == LevelNotice {
				return slog.String(slog.LevelKey, "NOTICE")
			}
			return a
		}
	}
	if redactedFields[strings.ToLower(a.Key)] {
		return slog.String(a.Key, Redacted)
	}
	return a
}

// handler adds `component` to every line, redacts the never-log fields that
// arrive inside groups or LogValuers (where ReplaceAttr does not see them
// the same way), and reports §5 violations to OnViolation.
type handler struct {
	slog.Handler
	component Component
	violation func(string)
	// preset is true once component has been added by WithAttrs, so a
	// derived logger does not add it twice.
	preset bool
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	var hasEvent bool
	var bad []string
	r.Attrs(func(a slog.Attr) bool {
		switch {
		case a.Key == "event":
			hasEvent = true
		case redactedFields[strings.ToLower(a.Key)]:
			bad = append(bad, a.Key)
		case a.Key == "component":
			// A call site that sets component itself is harmless but
			// redundant; the handler's value wins on the wire because
			// WithAttrs runs first, and slog keeps both. Drop the caller's.
		}
		return true
	})
	if h.violation != nil {
		if !hasEvent && r.Level >= slog.LevelInfo {
			h.violation(fmt.Sprintf("no event field: %q", r.Message))
		}
		for _, k := range bad {
			h.violation(fmt.Sprintf("field %q is on the never-log list (%q)", k, r.Message))
		}
	}
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	if !h.preset {
		out.AddAttrs(slog.String("component", string(h.component)))
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != "component" {
			out.AddAttrs(a)
		}
		return true
	})
	return h.Handler.Handle(ctx, out)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	keep := make([]slog.Attr, 0, len(attrs)+1)
	if !h.preset {
		keep = append(keep, slog.String("component", string(h.component)))
	}
	for _, a := range attrs {
		if a.Key == "component" {
			continue
		}
		keep = append(keep, a)
	}
	return &handler{Handler: h.Handler.WithAttrs(keep), component: h.component, violation: h.violation, preset: true}
}

func (h *handler) WithGroup(name string) slog.Handler {
	// component belongs at the top level, so it is added before the group is
	// opened; otherwise the record attrs Handle adds would land inside it.
	base := h.Handler
	if !h.preset {
		base = base.WithAttrs([]slog.Attr{slog.String("component", string(h.component))})
	}
	return &handler{Handler: base.WithGroup(name), component: h.component, violation: h.violation, preset: true}
}
