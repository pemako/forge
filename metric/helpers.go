package metric

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

type LabelOption struct{ k, v string }

func Label(key, value string) LabelOption { return LabelOption{k: key, v: value} }

func toAttrs(labels []LabelOption) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, len(labels))
	for i, l := range labels {
		attrs[i] = attribute.String(l.k, l.v)
	}
	return attrs
}

// getMeter returns the global meter or a noop if Init() not called.
// Uses boolean flag — otelmetric.Meter zero-value comparison is unsafe.
func getMeter() otelmetric.Meter {
	if !initialized {
		return noop.NewMeterProvider().Meter("")
	}
	return globalMeter
}

func Count(ctx context.Context, name string, n int64, labels ...LabelOption) {
	c, _ := getMeter().Int64Counter(name)
	c.Add(ctx, n, otelmetric.WithAttributes(toAttrs(labels)...))
}

func Gauge(ctx context.Context, name string, value float64, labels ...LabelOption) {
	g, _ := getMeter().Float64UpDownCounter(name)
	g.Add(ctx, value, otelmetric.WithAttributes(toAttrs(labels)...))
}

func GaugeAdd(ctx context.Context, name string, delta float64, labels ...LabelOption) {
	g, _ := getMeter().Float64UpDownCounter(name)
	g.Add(ctx, delta, otelmetric.WithAttributes(toAttrs(labels)...))
}

func GaugeSub(ctx context.Context, name string, delta float64, labels ...LabelOption) {
	GaugeAdd(ctx, name, -delta, labels...)
}

func StartTimer(ctx context.Context, name string, labels ...LabelOption) func() {
	h, _ := getMeter().Float64Histogram(name + "_duration_seconds")
	start := time.Now()
	return func() {
		h.Record(ctx, time.Since(start).Seconds(), otelmetric.WithAttributes(toAttrs(labels)...))
	}
}

func Duration(ctx context.Context, name string, d time.Duration, labels ...LabelOption) {
	h, _ := getMeter().Float64Histogram(name + "_duration_seconds")
	h.Record(ctx, d.Seconds(), otelmetric.WithAttributes(toAttrs(labels)...))
}
