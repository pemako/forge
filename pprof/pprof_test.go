package pprof_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/mk/pprof"
	"github.com/pemako/mk/testutil"
)

// freePort returns a random available TCP port on localhost.
func freePort(t *testing.T) string {
	t.Helper()
	return testutil.FreePort(t)
}

func TestServer_startsAndResponds(t *testing.T) {
	addr := freePort(t)
	srv := pprof.New(pprof.WithAddr(addr))

	ctx := context.Background()
	go srv.Start(ctx) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	defer srv.Stop(ctx) //nolint:errcheck

	resp, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/", addr))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_profileEndpoints(t *testing.T) {
	addr := freePort(t)
	srv := pprof.New(pprof.WithAddr(addr))

	ctx := context.Background()
	go srv.Start(ctx) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	defer srv.Stop(ctx) //nolint:errcheck

	for _, path := range []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
	} {
		resp, err := http.Get(fmt.Sprintf("http://%s%s", addr, path))
		require.NoError(t, err, path)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

func TestServer_stopGracefully(t *testing.T) {
	addr := freePort(t)
	srv := pprof.New(pprof.WithAddr(addr))

	ctx := context.Background()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	require.NoError(t, srv.Stop(stopCtx))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestServer_mutexProfiling(t *testing.T) {
	addr := freePort(t)
	srv := pprof.New(pprof.WithAddr(addr), pprof.WithMutexFraction(1))

	ctx := context.Background()
	go srv.Start(ctx) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	defer srv.Stop(ctx) //nolint:errcheck

	resp, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/mutex", addr))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_withConfig(t *testing.T) {
	addr := freePort(t)
	cfg := testutil.NewConfig(map[string]string{"pprof.addr": addr})
	srv := pprof.New(pprof.WithConfig(cfg))

	ctx := context.Background()
	go srv.Start(ctx) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	defer srv.Stop(ctx) //nolint:errcheck

	resp, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/", addr))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRegister_mountsRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	pprof.Register(engine)

	srv := &http.Server{Addr: freePort(t), Handler: engine}
	go srv.ListenAndServe() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	for _, path := range []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		"/debug/pprof/heap",
		"/debug/pprof/goroutine",
	} {
		resp, err := http.Get(fmt.Sprintf("http://%s%s", srv.Addr, path))
		require.NoError(t, err, path)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

func TestRegister_withMutexFraction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	pprof.Register(engine, pprof.WithMutexFraction(1))

	srv := &http.Server{Addr: freePort(t), Handler: engine}
	go srv.ListenAndServe() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	resp, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/mutex", srv.Addr))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// stubConfig and time import removed — now using testutil.NewConfig and testutil.FreePort
