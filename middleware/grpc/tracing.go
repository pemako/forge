package grpcMiddleware

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc/stats"
)

// ServerTracingHandler returns an otelgrpc stats.Handler for use with
// grpc.StatsHandler(ServerTracingHandler()). otelgrpc v0.46+ replaced
// the old UnaryServerInterceptor/StreamServerInterceptor with a stats-based
// handler that covers both unary and streaming RPCs in one call.
func ServerTracingHandler(opts ...otelgrpc.Option) stats.Handler {
	return otelgrpc.NewServerHandler(opts...)
}
