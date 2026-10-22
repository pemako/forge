package metric

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// The functions in this file are convenience facades over the default metric
// Provider. The default Provider must be initialized via metric.Init() or
// app.WithLifecycle(provider) before any of these functions are called.
//
// For tests or scenarios requiring multiple providers, use [Provider] directly:
//
//	p, _ := metric.New(opts...)
//	p.Init(ctx)
//	meter := p.Meter() // use OTel instruments directly

// LabelOption is a key-value label (attribute) attached to a metric recording.
// Use the Label constructor to create one.
type LabelOption struct{ k, v string }

// Label creates a LabelOption with the given key and value.
// Pass multiple Label calls to Count, Gauge, StartTimer, or Duration to
// add dimensions to the recorded metric.
func Label(key, value string) LabelOption { return LabelOption{k: key, v: value} }

func toAttrs(labels []LabelOption) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, len(labels))
	for i, l := range labels {
		attrs[i] = attribute.String(l.k, l.v)
	}
	return attrs
}

func noopMeter() otelmetric.Meter {
	return noop.NewMeterProvider().Meter("")
}

// getMeter returns the global meter or a noop if Init() not called.
// Uses boolean flag — otelmetric.Meter zero-value comparison is unsafe.
func getMeter() otelmetric.Meter {
	globalMu.Lock()
	init := initialized
	m := globalMeter
	globalMu.Unlock()
	if !init {
		return noopMeter()
	}
	return m
}

// Count is a convenience facade that delegates to the default metric Provider.
// It increments the named integer counter by n. The counter is created on first
// use. If Init has not been called, this is a no-op.
func Count(ctx context.Context, name string, n int64, labels ...LabelOption) {
	c, _ := getMeter().Int64Counter(name)
	c.Add(ctx, n, otelmetric.WithAttributes(toAttrs(labels)...))
}

// Gauge is a convenience facade that delegates to the default metric Provider.
// It sets the named gauge to value, replacing whatever was recorded before.
// Use it for a point-in-time reading (e.g., queue depth, pool size).
// If Init has not been called, this is a no-op.
//
// This previously shared GaugeAdd's body — Float64UpDownCounter.Add — so despite
// documenting "sets" it accumulated, and the two functions were identical. It
// now uses OTel's synchronous gauge, whose Record replaces the value. For
// increment/decrement semantics use GaugeAdd and GaugeSub.
func Gauge(ctx context.Context, name string, value float64, labels ...LabelOption) {
	g, _ := getMeter().Float64Gauge(name)
	g.Record(ctx, value, otelmetric.WithAttributes(toAttrs(labels)...))
}

// GaugeAdd is a convenience facade that delegates to the default metric Provider.
// It adds delta to the named float64 up-down counter.
// If Init has not been called, this is a no-op.
func GaugeAdd(ctx context.Context, name string, delta float64, labels ...LabelOption) {
	g, _ := getMeter().Float64UpDownCounter(name)
	g.Add(ctx, delta, otelmetric.WithAttributes(toAttrs(labels)...))
}

// GaugeSub is a convenience facade that delegates to the default metric Provider.
// It subtracts delta from the named float64 up-down counter.
// It is equivalent to GaugeAdd(ctx, name, -delta, labels...).
func GaugeSub(ctx context.Context, name string, delta float64, labels ...LabelOption) {
	GaugeAdd(ctx, name, -delta, labels...)
}

// StartTimer is a convenience facade that delegates to the default metric Provider.
// It returns a function that, when called, records the elapsed time since
// StartTimer was called in the histogram named "<name>_duration_seconds".
// Use the returned function as a deferred call to measure function duration:
//
//	defer metric.StartTimer(ctx, "db_query", metric.Label("op", "select"))()
func StartTimer(ctx context.Context, name string, labels ...LabelOption) func() {
	h, _ := getMeter().Float64Histogram(name + "_duration_seconds")
	start := time.Now()
	return func() {
		h.Record(ctx, time.Since(start).Seconds(), otelmetric.WithAttributes(toAttrs(labels)...))
	}
}

// Duration is a convenience facade that delegates to the default metric Provider.
// It records a pre-measured duration d in the histogram named
// "<name>_duration_seconds". Use this when the duration is already known
// rather than measuring it inline with StartTimer.
func Duration(ctx context.Context, name string, d time.Duration, labels ...LabelOption) {
	h, _ := getMeter().Float64Histogram(name + "_duration_seconds")
	h.Record(ctx, d.Seconds(), otelmetric.WithAttributes(toAttrs(labels)...))
}
