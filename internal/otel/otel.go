// Package otel wires OpenTelemetry traces and metrics to an OTLP endpoint
// when OTEL_EXPORTER_OTLP_ENDPOINT is set and installs no-op providers
// otherwise, so the cost of an unset endpoint is one context value per
// request and no network call.
package otel

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Setup installs the global providers. Enabled reports whether an OTLP
// endpoint was configured. The shutdown function flushes exporters.
func Setup(ctx context.Context, service, version string) (shutdown func(context.Context) error, enabled bool, err error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, false, nil
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName(service), semconv.ServiceVersion(version)))
	if err != nil {
		return nil, false, fmt.Errorf("otel resource: %w", err)
	}
	texp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("otlp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(texp), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	mexp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("otlp metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mexp, sdkmetric.WithInterval(30*time.Second))), sdkmetric.WithResource(res))
	otel.SetMeterProvider(mp)
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		terr := tp.Shutdown(ctx)
		merr := mp.Shutdown(ctx)
		if terr != nil {
			return terr
		}
		return merr
	}, true, nil
}

// Tracer returns the global tracer for a component.
func Tracer(name string) trace.Tracer { return otel.Tracer(name) }
