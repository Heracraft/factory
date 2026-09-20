package obs

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// GRPCDialOptions are the options a gRPC client adds to take part in
// tracing: hostd's Session stream to the api, and the api's own clients.
// With no OTLP endpoint set the handler records into the noop provider, so
// the cost is one context value per RPC and no network traffic of its own.
func GRPCDialOptions() []grpc.DialOption {
	return []grpc.DialOption{grpc.WithStatsHandler(otelgrpc.NewClientHandler())}
}

// GRPCServerOptions are the matching options for the api's gRPC server, the
// other end of docs/interfaces/grpc-hostd.md.
func GRPCServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler())}
}
