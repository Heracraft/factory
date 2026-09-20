package otel

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestUnsetEndpointIsNoop(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, enabled, err := Setup(context.Background(), "api", "test")
	if err != nil || enabled {
		t.Fatalf("enabled=%v err=%v", enabled, err)
	}
	_, span := otel.Tracer("t").Start(context.Background(), "op")
	if span.SpanContext().IsValid() {
		t.Fatal("noop provider produced a recording span")
	}
	span.End()
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
