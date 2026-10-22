package metric_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/pemako/forge/core"
)

// TestProvider_Gauge_SetsNotAccumulates pins core.Metrics' gauge contract.
//
// Both this method and the package-level Gauge facade used to be implemented
// with Float64UpDownCounter.Add while documenting "sets", so two calls of 5
// reported 10 instead of 5 — a silently wrong number on every dashboard built
// on them. They now use OTel's synchronous gauge.
func TestProvider_Gauge_SetsNotAccumulates(t *testing.T) {
	reader, p := newTestProvider(t)
	ctx := context.Background()

	p.Gauge(ctx, "queue_depth", 5)
	p.Gauge(ctx, "queue_depth", 5)

	got := collectFloat64(t, reader, "queue_depth")
	assert.Equal(t, 5.0, got, "a gauge set twice to 5 must read 5, not 10")
}

func TestProvider_Count_Accumulates(t *testing.T) {
	reader, p := newTestProvider(t)
	ctx := context.Background()

	p.Count(ctx, "jobs_total", 2)
	p.Count(ctx, "jobs_total", 3)

	assert.Equal(t, int64(5), collectInt64(t, reader, "jobs_total"))
}

func TestProvider_Histogram_RecordsWithLabels(t *testing.T) {
	reader, p := newTestProvider(t)
	ctx := context.Background()

	p.Histogram(ctx, "latency_seconds", 0.25, core.Field{Key: "route", Value: "/users"})

	m := findMetric(t, reader, "latency_seconds")
	h, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "latency_seconds should be a float64 histogram, got %T", m.Data)
	require.Len(t, h.DataPoints, 1)
	assert.Equal(t, uint64(1), h.DataPoints[0].Count)

	route, found := h.DataPoints[0].Attributes.Value("route")
	require.True(t, found, "route label should be attached")
	assert.Equal(t, "/users", route.AsString())
}

// TestProvider_ImplementsCoreMetrics guards the interface wiring, so metric can
// be faked in tests the way tracing already could.
func TestProvider_ImplementsCoreMetrics(t *testing.T) {
	reader, p := newTestProvider(t)
	var m core.Metrics = p
	m.Count(context.Background(), "iface_total", 1)
	assert.Equal(t, int64(1), collectInt64(t, reader, "iface_total"))
}
