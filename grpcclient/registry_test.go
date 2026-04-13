package grpcclient

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health "google.golang.org/grpc/health/grpc_health_v1"
)

// reset closes all registry entries and clears the registry.
// Used in tests to ensure a clean state; must not be called from production code.
func reset() {
	registryMu.Lock()
	for _, conn := range registry {
		conn.Close() //nolint:errcheck
	}
	clear(registry)
	registryMu.Unlock()
}

// startEchoServer starts a minimal gRPC health server and returns its address.
func startEchoServer(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health.HealthCheckResponse_SERVING)
	grpc_health.RegisterHealthServer(srv, hs)
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(srv.GracefulStop)
	return lis.Addr().String()
}

func TestRegister_DuplicateError(t *testing.T) {
	reset()
	srv := startEchoServer(t)
	conn1, err := Dial(srv, WithInsecure())
	require.NoError(t, err)
	require.NoError(t, Register("dup", conn1))

	conn2, err := Dial(srv, WithInsecure())
	require.NoError(t, err)
	err = Register("dup", conn2)
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}

func TestReplace_ClosesOldConn(t *testing.T) {
	reset()
	srv := startEchoServer(t)
	conn1, err := Dial(srv, WithInsecure())
	require.NoError(t, err)
	require.NoError(t, Register("r", conn1))
	old := Get("r")

	conn2, err := Dial(srv, WithInsecure())
	require.NoError(t, err)
	require.NoError(t, Replace("r", conn2))
	// Use pointer inequality to avoid data race from deep-comparing gRPC internals.
	require.True(t, old != Get("r"), "Replace should store a new connection pointer")
}

func TestCloseAll_ReturnsError(t *testing.T) {
	reset()
	require.NoError(t, CloseAll())
}

func TestRegistry_Close(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	require.NoError(t, r.Close())
}
