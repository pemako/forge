package httpserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	httpserver "github.com/pemako/mk/server/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPServer_healthEndpoints(t *testing.T) {
	srv, err := httpserver.New(
		httpserver.WithAddr(":18080"),
		httpserver.WithServiceName("test-svc"),
		httpserver.WithServiceVersion("v1"),
	)
	require.NoError(t, err)

	ctx := context.Background()
	go srv.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	defer srv.Stop(ctx)

	for _, tc := range []struct {
		path       string
		wantStatus int
	}{
		{"/healthz", 200},
		{"/health", 200},
		{"/readyz", 503},
	} {
		resp, err := http.Get("http://localhost:18080" + tc.path)
		require.NoError(t, err, tc.path)
		assert.Equal(t, tc.wantStatus, resp.StatusCode, tc.path)
	}

	httpserver.SetReady(true)
	defer httpserver.SetReady(false)
	resp, err := http.Get("http://localhost:18080/readyz")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
