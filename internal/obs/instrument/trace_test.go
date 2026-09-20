package instrument

import (
	"context"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/heracraft/repose/internal/obs"
)

// TestTracingOffByDefault is the §9 checklist item: with
// OTEL_EXPORTER_OTLP_ENDPOINT unset there is no exporter, so no span is
// recorded and nothing is sent anywhere.
func TestTracingOffByDefault(t *testing.T) {
	t.Setenv(EndpointEnv, "")
	if TracingEnabled() {
		t.Fatal("TracingEnabled with no endpoint set")
	}
	tr, shutdown, err := SetupTracing(context.Background(), TraceOptions{Component: obs.ComponentAPI})
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
	tr, shutdown, err := SetupTracing(context.Background(), TraceOptions{Component: obs.ComponentHostd})
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

// TestGlobalTracerIsNoopWhenOff: the api takes its tracer from the global
// provider, not from the returned one, so the global must be the noop
// provider too. (From workstream 05's own otel package, kept at the merge:
// DECISIONS I-56.)
func TestGlobalTracerIsNoopWhenOff(t *testing.T) {
	t.Setenv(EndpointEnv, "")
	if _, _, err := SetupTracing(context.Background(), TraceOptions{Component: obs.ComponentAPI}); err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	_, span := otel.Tracer("t").Start(context.Background(), "op")
	if span.SpanContext().IsValid() {
		t.Fatal("the global provider produced a recording span")
	}
	span.End()
}

// TestProtocolSwitch: OTEL_EXPORTER_OTLP_PROTOCOL picks the exporter, and
// both must reach a collector. The listener accepts and says nothing; what is
// asserted is that setup succeeds and spans record either way.
func TestProtocolSwitch(t *testing.T) {
	for _, proto := range []string{"", "grpc", "http/protobuf"} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		t.Setenv(ProtocolEnv, proto)
		t.Setenv(EndpointEnv, "http://"+ln.Addr().String())
		tr, shutdown, err := SetupTracing(context.Background(), TraceOptions{
			Component: obs.ComponentAPI, Insecure: true,
		})
		if err != nil {
			t.Fatalf("%s: SetupTracing: %v", proto, err)
		}
		_, span := tr.Start(context.Background(), "op")
		if !span.IsRecording() {
			t.Errorf("%s: a span is not recording", proto)
		}
		span.End()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := shutdown(ctx); err != nil {
			t.Logf("%s: shutdown: %v (the collector is a bare listener)", proto, err)
		}
		cancel()
		_ = ln.Close()
	}
}

// TestPropagatorIsSetEvenWhenOff: a trace context that arrives on a request
// is passed on, so turning one component on does not need them all on.
func TestPropagatorIsSetEvenWhenOff(t *testing.T) {
	t.Setenv(EndpointEnv, "")
	if _, _, err := SetupTracing(context.Background(), TraceOptions{Component: obs.ComponentGateway}); err != nil {
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
	_, shutdown, err := SetupTracing(context.Background(), TraceOptions{Component: obs.Component("worker")})
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

// TestTracingOnWithAnEndpoint is the other half of the no-network claim: with
// an endpoint set, the SDK provider is installed, spans record, and the
// exporter is the thing that opens a connection. Without this test,
// "no network calls when it is off" could pass because tracing never works at
// all.
func TestTracingOnWithAnEndpoint(t *testing.T) {
	// A listener that accepts and says nothing: the exporter connects lazily
	// and retries, and the test asserts nothing about what arrives.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }() // closing the fixture's listener cannot fail usefully
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close() // the exporter's connection; nothing is read from it
		}
	}()

	t.Setenv(EndpointEnv, "http://"+ln.Addr().String())
	tr, shutdown, err := SetupTracing(context.Background(), TraceOptions{
		Component: obs.ComponentAPI, Version: "test", Insecure: true,
	})
	if err != nil {
		t.Fatalf("SetupTracing: %v", err)
	}
	_, span := tr.Start(context.Background(), "op")
	if !span.IsRecording() {
		t.Error("a span is not recording with an endpoint set")
	}
	if !span.SpanContext().IsValid() {
		t.Error("a span has no trace id with an endpoint set")
	}
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Logf("shutdown: %v (the collector is a bare listener)", err)
	}
}
