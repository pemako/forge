package metric_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/pemako/forge/metric"
)

// newTestProvider returns an initialised Provider wired to a manual reader, so
// a test can collect and assert on whatever the instruments recorded.
func newTestProvider(t *testing.T) (*sdkmetric.ManualReader, *metric.Provider) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	p, err := metric.New(
		metric.WithServiceName("test"),
		metric.WithReader(reader),
	)
	require.NoError(t, err)
	require.NoError(t, p.Init(context.Background()))
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	return reader, p
}

// findMetric collects once and returns the metric recorded under name.
func findMetric(t *testing.T, reader *sdkmetric.ManualReader, name string) metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var seen []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			seen = append(seen, m.Name)
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q not recorded; saw %v", name, seen)
	return metricdata.Metrics{}
}

func collectFloat64(t *testing.T, reader *sdkmetric.ManualReader, name string) float64 {
	t.Helper()
	m := findMetric(t, reader, name)
	switch d := m.Data.(type) {
	case metricdata.Gauge[float64]:
		require.Len(t, d.DataPoints, 1)
		return d.DataPoints[0].Value
	case metricdata.Sum[float64]:
		require.Len(t, d.DataPoints, 1)
		return d.DataPoints[0].Value
	default:
		t.Fatalf("metric %q has unexpected data type %T", name, m.Data)
		return 0
	}
}

func collectInt64(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	m := findMetric(t, reader, name)
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "metric %q should be an int64 sum, got %T", name, m.Data)
	require.Len(t, sum.DataPoints, 1)
	return sum.DataPoints[0].Value
}
