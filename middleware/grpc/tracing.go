// Package grpcMiddleware provides gRPC unary server interceptors for mk gRPC servers.
// The interceptors are pre-mounted by server/grpc.New in the standard order:
// RequestID, Logger, Metrics, Recovery, ErrorHandler. Tracing is handled via
// the otelgrpc stats handler returned by ServerTracingHandler.
// Additional interceptors (Timeout, RateLimit, IPFilter) must be configured explicitly.
package grpcMiddleware

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc/stats"
)

// ServerTracingHandler returns an otelgrpc stats.Handler that instruments both
// unary and streaming gRPC RPCs with OpenTelemetry tracing. Pass the result to
// grpc.StatsHandler when constructing the server.
//
// otelgrpc v0.46+ replaced the old unary/stream interceptors with a single
// stats-based handler that covers all RPC types in one call.
func ServerTracingHandler(opts ...otelgrpc.Option) stats.Handler {
	return otelgrpc.NewServerHandler(opts...)
}
