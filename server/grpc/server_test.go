package grpcserver_test

import (
	"context"
	"testing"
	"time"

	grpcserver "github.com/pemako/mk/server/grpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCServer_healthCheck(t *testing.T) {
	srv, err := grpcserver.New(grpcserver.WithAddr(":19090"))
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	conn, err := grpc.NewClient("localhost:19090",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	hc := grpc_health.NewHealthClient(conn)
	resp, err := hc.Check(ctx, &grpc_health.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health.HealthCheckResponse_SERVING, resp.Status)
}
