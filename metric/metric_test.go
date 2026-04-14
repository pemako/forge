package metric_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pemako/forge/metric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_prometheusEndpoint(t *testing.T) {
	err := metric.Init(
		metric.WithServiceName("test_svc"),
		metric.WithPrometheusAddr(":19091"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { metric.Shutdown(context.Background()) })

	time.Sleep(80 * time.Millisecond)

	resp, err := http.Get("http://localhost:19091/metrics")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.True(t, strings.Contains(string(body), "go_goroutines"))
}

func TestCount_doesNotPanic(t *testing.T) {
	ctx := context.Background()
	assert.NotPanics(t, func() {
		metric.Count(ctx, "test_events", 1, metric.Label("env", "test"))
	})
}

func TestStartTimer_doesNotPanic(t *testing.T) {
	ctx := context.Background()
	end := metric.StartTimer(ctx, "test_op")
	assert.NotNil(t, end)
	assert.NotPanics(t, func() { end() })
}

func TestGauge_doesNotPanic(t *testing.T) {
	ctx := context.Background()
	assert.NotPanics(t, func() {
		metric.Gauge(ctx, "test_queue", 42.0)
		metric.GaugeAdd(ctx, "test_conn", 1)
		metric.GaugeSub(ctx, "test_conn", 1)
	})
}

func TestNew_ReturnsProvider(t *testing.T) {
	p, err := metric.New()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestProvider_Init_Idempotent(t *testing.T) {
	p1, _ := metric.New()
	p2, _ := metric.New()
	require.NoError(t, p1.Init(context.Background()))
	require.NotPanics(t, func() { p2.Init(context.Background()) }) //nolint:errcheck
}

func TestProvider_Shutdown(t *testing.T) {
	p, _ := metric.New()
	p.Init(context.Background()) //nolint:errcheck
	require.NoError(t, p.Shutdown(context.Background()))
}
