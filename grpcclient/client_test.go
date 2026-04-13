// grpcclient/client_test.go
package grpcclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pemako/mk/grpcclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

func startTestServer(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health.HealthCheckResponse_SERVING)
	grpc_health.RegisterHealthServer(srv, hs)
	go srv.Serve(lis)
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

func TestDial_insecure(t *testing.T) {
	addr := startTestServer(t)
	conn, err := grpcclient.Dial(addr, grpcclient.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()

	hc := grpc_health.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health.HealthCheckResponse_SERVING, resp.Status)
}

func TestRegistry_RegisterGet(t *testing.T) {
	addr := startTestServer(t)
	err := grpcclient.Register("grpc_health_test", addr, grpcclient.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { grpcclient.CloseAll() }) //nolint:errcheck

	conn := grpcclient.Get("grpc_health_test")
	require.NotNil(t, conn)

	hc := grpc_health.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health.HealthCheckResponse_SERVING, resp.Status)
}

func TestGet_unknownName(t *testing.T) {
	assert.Nil(t, grpcclient.Get("nonexistent_grpc_xyz_abc"))
}
