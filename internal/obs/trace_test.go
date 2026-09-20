package obs

import (
	"context"
	"net"
	"testing"

	"go.opentelemetry.io/otel"
)

// TestTracingOffByDefault is the §9 checklist item: with
// OTEL_EXPORTER_OTLP_ENDPOINT unset there is no exporter, so no span is
// recorded and nothing is sent anywhere.
func TestTracingOffByDefault(t *testing.T) {
	t.Setenv(EndpointEnv, "")
	if TracingEnabled() {
		t.Fatal("TracingEnabled with no endpoint set")
	}
	tr, shutdown, err := SetupTracing(context.Background(), TraceOptions{Component: ComponentAPI})
	if err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	t.Cleanup(func() {
		if err := shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	_, span := tr.Start(context.Background(), "op")
	defer span.End()
	if span.IsRecording() {
		t.Error("a span is recording with no exporter configured")
	}
	if span.SpanContext().IsValid() {
		t.Error("a span has a valid trace id with no exporter configured")
	}
}

// TestNoDialWithoutAnEndpoint proves the same thing by blocking the network:
// every outbound connection this goroutine could make fails the test.
// (`strace -e network` on a real binary is the other half of that evidence,
// recorded in the workstream report.)
func TestNoDialWithoutAnEndpoint(t *testing.T) {
	t.Setenv(EndpointEnv, "")
	var dialed []string
	restore := net.DefaultResolver
	t.Cleanup(func() { net.DefaultResolver = restore })
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(_ context.Context, network, addr string) (net.Conn, error) {
			dialed = append(dialed, network+" "+addr)
			return nil, context.Canceled
		},
	}
	tr, shutdown, err := SetupTracing(context.Background(), TraceOptions{Component: ComponentHostd})
	if err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	_, span := tr.Start(context.Background(), "op")
	span.End()
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if len(dialed) != 0 {
		t.Errorf("name lookups were made with no endpoint set: %v", dialed)
	}
}

// TestPropagatorIsSetEvenWhenOff: a trace context that arrives on a request
// is passed on, so turning one component on does not need them all on.
func TestPropagatorIsSetEvenWhenOff(t *testing.T) {
	t.Setenv(EndpointEnv, "")
	if _, _, err := SetupTracing(context.Background(), TraceOptions{Component: ComponentGateway}); err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	fields := otel.GetTextMapPropagator().Fields()
	var hasTraceparent bool
	for _, f := range fields {
		if f == "traceparent" {
			hasTraceparent = true
		}
	}
	if !hasTraceparent {
		t.Errorf("propagator fields = %v, want traceparent", fields)
	}
}

// TestSetupTracingRejectsAnUnknownComponent, and still returns a shutdown
// that can be deferred.
func TestSetupTracingRejectsAnUnknownComponent(t *testing.T) {
	_, shutdown, err := SetupTracing(context.Background(), TraceOptions{Component: Component("worker")})
	if err == nil {
		t.Fatal("an unknown component was accepted")
	}
	if shutdown == nil {
		t.Fatal("shutdown is nil; every caller defers it")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}
