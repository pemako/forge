package metric_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pemako/mk/metric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_prometheusEndpoint(t *testing.T) {
	err := metric.Init(
		metric.WithServiceName("test_svc"),
		metric.WithPrometheusAddr(":19091"),
	)
	require.NoError(t, err)
	t.Cleanup(metric.Shutdown)

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
