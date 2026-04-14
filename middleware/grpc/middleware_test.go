package grpcMiddleware_test

import (
	"context"
	"net"
	"testing"
	"time"

	mkErrors "github.com/pemako/forge/errors"
	grpcMW "github.com/pemako/forge/middleware/grpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// startServer creates an in-process gRPC server with the given interceptors
// on a random port, registers the health check service, and returns the
// client connection plus a cleanup function.
func startServer(t *testing.T, interceptors ...grpc.UnaryServerInterceptor) (*grpc.ClientConn, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptors...))
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health.HealthCheckResponse_SERVING)
	grpc_health.RegisterHealthServer(srv, hs)

	go srv.Serve(lis) //nolint:errcheck

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		srv.GracefulStop()
	}
	return conn, cleanup
}

func healthCheck(t *testing.T, conn *grpc.ClientConn) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := grpc_health.NewHealthClient(conn).Check(ctx, &grpc_health.HealthCheckRequest{})
	return err
}

// ── RequestID ────────────────────────────────────────────────────────────────

func TestUnaryRequestID_setsMetadata(t *testing.T) {
	var capturedID string
	// Chain RequestID then a capturing interceptor.
	capture := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if ids := md.Get(grpcMW.MetadataRequestID); len(ids) > 0 {
			capturedID = ids[0]
		}
		return handler(ctx, req)
	}
	conn, cleanup := startServer(t, grpcMW.UnaryRequestID, capture)
	defer cleanup()

	require.NoError(t, healthCheck(t, conn))
	assert.NotEmpty(t, capturedID, "request_id should be set in metadata")
}

func TestUnaryRequestID_propagatesExisting(t *testing.T) {
	var capturedID string
	capture := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if ids := md.Get(grpcMW.MetadataRequestID); len(ids) > 0 {
			capturedID = ids[0]
		}
		return handler(ctx, req)
	}
	conn, cleanup := startServer(t, grpcMW.UnaryRequestID, capture)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, grpcMW.MetadataRequestID, "my-custom-id")
	_, err := grpc_health.NewHealthClient(conn).Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, "my-custom-id", capturedID)
}

// ── Recovery ─────────────────────────────────────────────────────────────────

func TestUnaryRecovery_catchesPanic(t *testing.T) {
	panicInterceptor := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		panic("test panic from interceptor")
	}
	conn, cleanup := startServer(t, grpcMW.UnaryRecovery, panicInterceptor)
	defer cleanup()

	err := healthCheck(t, conn)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

// ── ErrorHandler ─────────────────────────────────────────────────────────────

func TestUnaryErrorHandler_convertsCodedError(t *testing.T) {
	mkerr := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return nil, mkErrors.Define(404, "NOT_FOUND", "not found")
	}
	conn, cleanup := startServer(t, grpcMW.UnaryErrorHandler, mkerr)
	defer cleanup()

	err := healthCheck(t, conn)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestUnaryErrorHandler_passthroughOnSuccess(t *testing.T) {
	conn, cleanup := startServer(t, grpcMW.UnaryErrorHandler)
	defer cleanup()
	assert.NoError(t, healthCheck(t, conn))
}

// ── Timeout ───────────────────────────────────────────────────────────────────

func TestUnaryTimeout_cancelsSlowHandler(t *testing.T) {
	slow := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return handler(ctx, req)
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "timeout")
		}
	}
	conn, cleanup := startServer(t,
		grpcMW.UnaryTimeout(20*time.Millisecond),
		slow,
	)
	defer cleanup()

	err := healthCheck(t, conn)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.DeadlineExceeded, st.Code())
}

// ── RateLimit ─────────────────────────────────────────────────────────────────

func TestUnaryRateLimit_blocksExcess(t *testing.T) {
	conn, cleanup := startServer(t,
		grpcMW.UnaryRateLimit(grpcMW.GRPCRateLimitConfig{Rate: 1, Burst: 1}),
	)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hc := grpc_health.NewHealthClient(conn)

	// First request should pass.
	_, err1 := hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err1)

	// Second immediate request should be rate-limited.
	_, err2 := hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.Error(t, err2)
	st, _ := status.FromError(err2)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

// ── Logger ────────────────────────────────────────────────────────────────────

func TestUnaryLogger_doesNotBreakRequest(t *testing.T) {
	conn, cleanup := startServer(t, grpcMW.UnaryLogger(nil))
	defer cleanup()
	assert.NoError(t, healthCheck(t, conn))
}

// ── Metrics ───────────────────────────────────────────────────────────────────

func TestUnaryMetrics_doesNotBreakRequest(t *testing.T) {
	conn, cleanup := startServer(t, grpcMW.UnaryMetrics())
	defer cleanup()
	assert.NoError(t, healthCheck(t, conn))
}

// ── IPFilter ──────────────────────────────────────────────────────────────────

func TestUnaryIPFilter_whitelistAllowsLoopback(t *testing.T) {
	conn, cleanup := startServer(t,
		grpcMW.UnaryIPFilter(grpcMW.GRPCIPFilterConfig{
			Mode: "whitelist",
			IPs:  []string{"127.0.0.1", "::1"},
		}),
	)
	defer cleanup()
	assert.NoError(t, healthCheck(t, conn))
}

func TestUnaryIPFilter_blacklistBlocksLoopback(t *testing.T) {
	conn, cleanup := startServer(t,
		grpcMW.UnaryIPFilter(grpcMW.GRPCIPFilterConfig{
			Mode: "blacklist",
			IPs:  []string{"127.0.0.1", "::1"},
		}),
	)
	defer cleanup()

	err := healthCheck(t, conn)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}
