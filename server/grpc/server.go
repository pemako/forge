package grpcserver

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/fullstorydev/grpcui/standalone"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/pemako/forge/logger"
	grpcMW "github.com/pemako/forge/middleware/grpc"
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
		o.grpcUIEnabled = false
	}

	var serverOpts []grpc.ServerOption

	// Tracing via stats handler (otelgrpc v0.68+ API)
	serverOpts = append(serverOpts, grpc.StatsHandler(grpcMW.ServerTracingHandler()))

	// Default interceptor chain + any extra interceptors from WithUnaryInterceptors.
	chain := []grpc.UnaryServerInterceptor{
		grpcMW.UnaryRequestID,
		grpcMW.UnaryLogger(o.logger),
		grpcMW.UnaryMetrics(),
		grpcMW.UnaryRecovery,
		grpcMW.UnaryErrorHandler,
	}
	chain = append(chain, o.extraUnaryInterceptors...)
	serverOpts = append(serverOpts, grpc.ChainUnaryInterceptor(chain...))

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

	var lis net.Listener
	if o.listener != nil {
		lis = o.listener
	} else {
		var err error
		lis, err = net.Listen("tcp", o.addr)
		if err != nil {
			return nil, err
		}
	}

	return &grpcServer{opts: o, s: s, lis: lis}, nil
}

// Server returns the underlying *grpc.Server so callers can register gRPC
// service implementations after server creation.
func (g *grpcServer) Server() *grpc.Server { return g.s }

// Addr returns the address the server is listening on.
// Useful in tests when using ":0" for a random port.
func (g *grpcServer) Addr() string {
	if g.lis != nil {
		return g.lis.Addr().String()
	}
	return g.opts.addr
}

// Start begins accepting gRPC connections and blocks until the server is stopped.
// When WithGRPCUI is configured, also starts the embedded grpcui HTTP server.
// It implements core.Server.
func (g *grpcServer) Start(ctx context.Context) error {
	if g.opts.grpcUIEnabled {
		go g.startGRPCUI(ctx)
	}
	return g.s.Serve(g.lis)
}

// Stop gracefully stops the gRPC server, waiting for in-flight RPCs to complete.
// It implements core.Server.
func (g *grpcServer) Stop(_ context.Context) error { g.s.GracefulStop(); return nil }

// startGRPCUI launches an HTTP server that serves the grpcui web interface.
// It retries connecting to the gRPC server (via reflection) until ready.
func (g *grpcServer) startGRPCUI(ctx context.Context) {
	target := g.Addr()

	cc, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error(ctx, "grpcui: failed to create client", logger.F("error", err))
		return
	}
	defer cc.Close()

	// Retry until the gRPC server is ready to serve reflection requests.
	var h http.Handler
	for i := range 20 {
		h, err = standalone.HandlerViaReflection(ctx, cc, target)
		if err == nil {
			break
		}
		if i == 19 {
			logger.Error(ctx, "grpcui: failed to connect via reflection", logger.F("error", err))
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	uiAddr := g.opts.grpcUIAddr
	if uiAddr == "" {
		uiAddr = ":0"
	}
	lis, err := net.Listen("tcp", uiAddr)
	if err != nil {
		logger.Error(ctx, "grpcui: failed to listen", logger.F("error", err))
		return
	}

	_, port, _ := net.SplitHostPort(lis.Addr().String())
	logger.Info(ctx, "grpcui started", logger.F("addr", "http://localhost:"+port))

	srv := &http.Server{Handler: h}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	_ = srv.Serve(lis)
}
