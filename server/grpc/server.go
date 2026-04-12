package grpcserver

import (
	"context"
	"net"

	grpcMW "github.com/pemako/mk/middleware/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

type grpcServer struct {
	opts options
	s    *grpc.Server
	lis  net.Listener
}

// New creates a new gRPC server with the default interceptor chain pre-configured.
// The interceptor order is: RequestID -> Logger -> Metrics -> Recovery -> ErrorHandler.
// Tracing is applied via an otelgrpc stats handler that covers both unary and streaming RPCs.
// The gRPC health protocol is registered automatically.
// Reflection is enabled by default and disabled when env is "production".
// Defaults: addr=":9090", reflection=true.
func New(opts ...Option) (*grpcServer, error) {
	o := options{
		addr:       ":9090",
		reflection: true,
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.env == "production" {
		o.reflection = false
	}

	var serverOpts []grpc.ServerOption

	// Tracing via stats handler (otelgrpc v0.68+ API)
	serverOpts = append(serverOpts, grpc.StatsHandler(grpcMW.ServerTracingHandler()))

	// Default interceptor chain:
	// RequestID -> Logger -> Metrics -> Recovery -> ErrorHandler
	serverOpts = append(serverOpts,
		grpc.ChainUnaryInterceptor(
			grpcMW.UnaryRequestID,
			grpcMW.UnaryLogger(o.logger),
			grpcMW.UnaryMetrics(),
			grpcMW.UnaryRecovery,
			grpcMW.UnaryErrorHandler,
		),
	)

	if o.tlsCert != "" {
		creds, err := credentials.NewServerTLSFromFile(o.tlsCert, o.tlsKey)
		if err != nil {
			return nil, err
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	s := grpc.NewServer(serverOpts...)

	// Health check — no auth, always registered
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health.HealthCheckResponse_SERVING)
	grpc_health.RegisterHealthServer(s, hs)

	if o.reflection {
		reflection.Register(s)
	}

	lis, err := net.Listen("tcp", o.addr)
	if err != nil {
		return nil, err
	}

	return &grpcServer{opts: o, s: s, lis: lis}, nil
}

// Server returns the underlying *grpc.Server so callers can register gRPC
// service implementations after server creation.
func (g *grpcServer) Server() *grpc.Server { return g.s }

// Start begins accepting gRPC connections and blocks until the server is stopped.
// It implements core.Server.
func (g *grpcServer) Start(_ context.Context) error { return g.s.Serve(g.lis) }

// Stop gracefully stops the gRPC server, waiting for in-flight RPCs to complete.
// It implements core.Server.
func (g *grpcServer) Stop(_ context.Context) error { g.s.GracefulStop(); return nil }
