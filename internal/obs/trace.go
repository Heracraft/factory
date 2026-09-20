package obs

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	// The same schema version the SDK's own resource.Default() uses; a
	// different one makes resource.Merge fail with "conflicting Schema URL",
	// which TestTracingOnWithAnEndpoint caught.
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// EndpointEnv is the one environment variable that turns tracing on. It is
// unset everywhere today: DESIGN §15 says traces are exported to nothing yet,
// and this workstream does not choose a backend.
const EndpointEnv = "OTEL_EXPORTER_OTLP_ENDPOINT"

// TraceOptions configures SetupTracing.
type TraceOptions struct {
	Component Component
	// Version goes on the resource as service.version.
	Version string
	// Endpoint defaults to $OTEL_EXPORTER_OTLP_ENDPOINT. Empty means no
	// exporter at all: no connection is made, no goroutine is started, and
	// the returned tracer is the noop one, so the cost of instrumented code
	// is one context value per request.
	Endpoint string
	// Insecure sends over plaintext gRPC. The endpoint is reached over
	// WireGuard when it exists, so this is the normal setting; TLS is used
	// when Insecure is false.
	Insecure bool
	// SampleRatio is the head sampling ratio; 0 means 1.0 (sample
	// everything), which is right until there is a backend to overload.
	SampleRatio float64
}

// Shutdown flushes and stops the exporter. It is never nil, so a caller can
// always `defer shutdown(ctx)`.
type Shutdown func(context.Context) error

// SetupTracing returns the component's tracer, a shutdown function and an
// error. With no endpoint configured it installs the noop provider globally
// and returns immediately, which is what
// docs/workstreams/10-observability.md §5 means by "the exporter is a no-op
// when OTEL_EXPORTER_OTLP_ENDPOINT is unset".
func SetupTracing(ctx context.Context, o TraceOptions) (trace.Tracer, Shutdown, error) {
	if err := o.Component.check(); err != nil {
		return nil, func(context.Context) error { return nil }, err
	}
	name := "github.com/heracraft/repose/" + string(o.Component)
	endpoint := o.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv(EndpointEnv)
	}
	// Propagation is set either way: a trace context that arrives on a
	// request is passed on even when this process records nothing, so
	// turning one component on does not require turning them all on.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if endpoint == "" {
		tp := noop.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp.Tracer(name), func(context.Context) error { return nil }, nil
	}

	version := o.Version
	if version == "" {
		version = "dev"
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("repose-"+string(o.Component)),
		semconv.ServiceVersion(version),
		attribute.String("repose.component", string(o.Component)),
	))
	if err != nil {
		return nil, func(context.Context) error { return nil }, fmt.Errorf("build the trace resource: %w", err)
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL(endpoint)}
	if o.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exp, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, func(context.Context) error { return nil }, fmt.Errorf("start the OTLP exporter for %s: %w", endpoint, err)
	}
	ratio := o.SampleRatio
	if ratio == 0 {
		ratio = 1
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	otel.SetTracerProvider(tp)
	return tp.Tracer(name), func(c context.Context) error {
		if err := tp.Shutdown(c); err != nil {
			return fmt.Errorf("shut the tracer provider down: %w", err)
		}
		return nil
	}, nil
}

// TracingEnabled reports whether an OTLP endpoint is configured, for a log
// line at startup that says which of the two modes the binary is in.
func TracingEnabled() bool { return os.Getenv(EndpointEnv) != "" }
