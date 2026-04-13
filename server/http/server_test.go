package httpserver_test

import (
	"context"
	"errors"
	"fmt"
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

// testHealthChecker is a minimal core.HealthChecker for use in tests.
type testHealthChecker struct {
	name    string
	checkFn func(ctx context.Context) error
}

func (c *testHealthChecker) Name() string                    { return c.name }
func (c *testHealthChecker) Check(ctx context.Context) error { return c.checkFn(ctx) }

func TestWithHealthChecks_WiresCheckers(t *testing.T) {
	called := false
	checker := &testHealthChecker{
		name: "db:default",
		checkFn: func(_ context.Context) error {
			called = true
			return nil
		},
	}

	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv, err := httpserver.New(
		httpserver.WithListener(lis),
		httpserver.WithHealthChecks(checker),
	)
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	srv.SetReady(true)
	resp, err := http.Get("http://" + lis.Addr().String() + "/readyz")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, called, "checker should have been called during /readyz")
}

func TestAddReadyCheck_AfterConstruction(t *testing.T) {
	called := false
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv, err := httpserver.New(httpserver.WithListener(lis))
	require.NoError(t, err)

	srv.AddReadyCheck("late", func(_ context.Context) error {
		called = true
		return nil
	})

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	srv.SetReady(true)
	resp, err := http.Get("http://" + lis.Addr().String() + "/readyz")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, called, "late-registered check should have been called")
}

func TestWithHealthChecks_AllPass(t *testing.T) {
	checker1 := &testHealthChecker{name: "svc1", checkFn: func(_ context.Context) error { return nil }}
	checker2 := &testHealthChecker{name: "svc2", checkFn: func(_ context.Context) error { return nil }}

	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	srv, err := httpserver.New(
		httpserver.WithListener(ln),
		httpserver.WithHealthChecks(checker1, checker2),
	)
	require.NoError(t, err)
	srv.SetReady(true)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	defer srv.Stop(ctx)

	resp, err := http.Get("http://" + ln.Addr().String() + "/readyz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestWithHealthChecks_OneFails(t *testing.T) {
	checkerOK := &testHealthChecker{name: "db", checkFn: func(_ context.Context) error { return nil }}
	checkerFail := &testHealthChecker{name: "redis", checkFn: func(_ context.Context) error {
		return fmt.Errorf("redis unreachable")
	}}

	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	srv, err := httpserver.New(
		httpserver.WithListener(ln),
		httpserver.WithHealthChecks(checkerOK, checkerFail),
	)
	require.NoError(t, err)
	srv.SetReady(true)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	defer srv.Stop(ctx)

	resp, err := http.Get("http://" + ln.Addr().String() + "/readyz")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
