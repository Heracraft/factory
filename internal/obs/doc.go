// Package obs is the one observability surface every repose binary uses:
// structured logs, Prometheus metrics and OpenTelemetry traces, with the
// naming rules of docs/workstreams/10-observability.md §5 built into the
// types rather than left to each call site.
//
// The workstream doc calls the three entry points obs.Logger, obs.Metrics and
// obs.Tracer. In Go they are a constructor each, and the two that carry heavy
// dependencies live in subpackages so that a guest links only what it uses
// (DECISIONS I-56):
//
//	log := obs.NewLogger(obs.LogOptions{Component: obs.ComponentHostd})   // this package, stdlib only
//	met := metrics.New(obs.ComponentHostd)                                 // internal/obs/metrics
//	tr, shutdown, err := instrument.SetupTracing(ctx, instrument.TraceOptions{Component: obs.ComponentHostd})
//
// What the types guarantee, so that no call site has to remember it:
//
//   - every log line carries ts (RFC 3339 UTC), level, component and msg,
//     because the handler adds them; the caller supplies event and context
//     fields.
//   - field names on the never-log list of docs/ops/OBSERVABILITY.md are
//     replaced with "[redacted]" as a floor. The real check is the reviewer
//     and the source rules in obslint, which refuse a forbidden field name
//     at build time; redaction is what catches the one that slips through
//     into a release.
//   - every metric registered through a metrics.Metrics is in the repose_
//     namespace and carries only labels from the low-cardinality list.
//     Registration fails otherwise, so a metric labelled by project_id cannot
//     ship.
//   - with OTEL_EXPORTER_OTLP_ENDPOINT unset there is no exporter and no
//     connection: SetupTracing returns the noop tracer provider.
package obs
