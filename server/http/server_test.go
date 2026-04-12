package httpserver_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpserver "github.com/pemako/mk/server/http"
)

func TestHTTPServer_healthEndpoints(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv, err := httpserver.New(
		httpserver.WithListener(lis),
		httpserver.WithServiceName("test-svc"),
		httpserver.WithServiceVersion("v1"),
	)
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	base := "http://" + lis.Addr().String()

	for _, tc := range []struct {
		path       string
		wantStatus int
	}{
		{"/healthz", 200},
		{"/health", 200},
		{"/readyz", 503},
	} {
		resp, err := http.Get(base + tc.path)
		require.NoError(t, err, tc.path)
		assert.Equal(t, tc.wantStatus, resp.StatusCode, tc.path)
	}

	srv.SetReady(true)
	defer srv.SetReady(false)
	resp, err := http.Get(base + "/readyz")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestHTTPServer_readyChecks_allPass(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv, err := httpserver.New(
		httpserver.WithListener(lis),
		httpserver.WithReadyCheck("db", func(_ context.Context) error { return nil }),
		httpserver.WithReadyCheck("cache", func(_ context.Context) error { return nil }),
	)
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	srv.SetReady(true)
	resp, err := http.Get("http://" + lis.Addr().String() + "/readyz")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestHTTPServer_readyChecks_oneFails(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv, err := httpserver.New(
		httpserver.WithListener(lis),
		httpserver.WithReadyCheck("db", func(_ context.Context) error { return nil }),
		httpserver.WithReadyCheck("cache", func(_ context.Context) error {
			return errors.New("connection refused")
		}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	srv.SetReady(true)
	resp, err := http.Get("http://" + lis.Addr().String() + "/readyz")
	require.NoError(t, err)
	assert.Equal(t, 503, resp.StatusCode)
}

func TestHTTPServer_readyChecks_flagNotSet(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	// Both checks pass but SetReady not called — should still return 503
	srv, err := httpserver.New(
		httpserver.WithListener(lis),
		httpserver.WithReadyCheck("db", func(_ context.Context) error { return nil }),
	)
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	resp, err := http.Get("http://" + lis.Addr().String() + "/readyz")
	require.NoError(t, err)
	assert.Equal(t, 503, resp.StatusCode)
}
