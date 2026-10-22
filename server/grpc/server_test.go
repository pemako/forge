package grpcserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"

	grpcserver "github.com/pemako/forge/server/grpc"
)

// TestGRPCServer_healthCheck covers the readiness contract: the health service
// reports NOT_SERVING until something marks the server ready, so it cannot
// advertise itself before migrations and Lifecycle.Init have run. app drives
// SetReady automatically; this test does it by hand.
func TestGRPCServer_healthCheck(t *testing.T) {
	srv, err := grpcserver.New(grpcserver.WithAddr(":0"))
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	conn, err := grpc.NewClient(srv.Addr(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	hc := grpc_health.NewHealthClient(conn)

	resp, err := hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health.HealthCheckResponse_NOT_SERVING, resp.Status,
		"a freshly constructed server must not advertise itself as serving")

	srv.SetReady(true)
	resp, err = hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health.HealthCheckResponse_SERVING, resp.Status)

	srv.SetReady(false)
	resp, err = hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health.HealthCheckResponse_NOT_SERVING, resp.Status,
		"readiness must be revocable so clients stop picking a draining instance")
}
