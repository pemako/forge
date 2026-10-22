package tracing_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/pemako/forge/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// W3C trace-context spec example header.
const (
	specTraceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	specTraceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	specSpanID      = "00f067aa0ba902b7"
)

// restorePropagator snapshots the global propagator and restores it when the
// test finishes, so these tests do not leak global state into other tests.
func restorePropagator(t *testing.T) {
	t.Helper()
	prev := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })
}

// TestNew_installsW3CPropagator_withoutExporter is the regression test for the
// propagation fix: before it, otel.SetTextMapPropagator was never called, the
// global propagator stayed the delegating no-op, and a valid inbound
// traceparent was discarded (trace ID all zeros).
func TestNew_installsW3CPropagator_withoutExporter(t *testing.T) {
	restorePropagator(t)

	// No exporter configured — propagation must still work.
	p, err := tracing.New(tracing.WithServiceName("test"))
	require.NoError(t, err)
	require.NotNil(t, p)

	h := http.Header{}
	h.Set("traceparent", specTraceparent)

	ctx := otel.GetTextMapPropagator().Extract(
		context.Background(), propagation.HeaderCarrier(h))

	sc := trace.SpanContextFromContext(ctx)
	require.True(t, sc.IsValid(), "extracted SpanContext must be valid")
	assert.Equal(t, specTraceID, sc.TraceID().String())
	assert.Equal(t, specSpanID, sc.SpanID().String())
	assert.True(t, sc.IsSampled(), "sampled flag 01 must survive extraction")
	assert.True(t, sc.IsRemote(), "extracted context must be marked remote")
}

func TestNew_installsW3CPropagator_withExporter(t *testing.T) {
	restorePropagator(t)

	p, err := tracing.New(tracing.WithExporter(&noopExporter{}))
	require.NoError(t, err)
	defer func() { _ = p.Shutdown(context.Background()) }()

	h := http.Header{}
	h.Set("traceparent", specTraceparent)
	ctx := otel.GetTextMapPropagator().Extract(
		context.Background(), propagation.HeaderCarrier(h))

	assert.Equal(t, specTraceID,
		trace.SpanContextFromContext(ctx).TraceID().String())
}

// TestNew_propagatorInjects asserts the outbound half: otelhttp/otelgrpc rely
// on Inject writing a traceparent header.
func TestNew_propagatorInjects(t *testing.T) {
	restorePropagator(t)

	_, err := tracing.New()
	require.NoError(t, err)

	in := http.Header{}
	in.Set("traceparent", specTraceparent)
	ctx := otel.GetTextMapPropagator().Extract(
		context.Background(), propagation.HeaderCarrier(in))

	out := http.Header{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(out))

	assert.Equal(t, specTraceparent, out.Get("traceparent"))
}

// TestNew_propagatorCarriesBaggage covers the Baggage half of the composite.
func TestNew_propagatorCarriesBaggage(t *testing.T) {
	restorePropagator(t)

	_, err := tracing.New()
	require.NoError(t, err)

	in := http.Header{}
	in.Set("baggage", "tenant=acme")
	ctx := otel.GetTextMapPropagator().Extract(
		context.Background(), propagation.HeaderCarrier(in))

	out := http.Header{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(out))

	assert.Contains(t, out.Get("baggage"), "tenant=acme")
}

// TestWithPropagators_override asserts the option replaces the default set:
// with TraceContext omitted, a traceparent header must no longer be extracted.
func TestWithPropagators_override(t *testing.T) {
	restorePropagator(t)

	_, err := tracing.New(tracing.WithPropagators(propagation.Baggage{}))
	require.NoError(t, err)

	h := http.Header{}
	h.Set("traceparent", specTraceparent)
	h.Set("baggage", "tenant=acme")
	ctx := otel.GetTextMapPropagator().Extract(
		context.Background(), propagation.HeaderCarrier(h))

	assert.False(t, trace.SpanContextFromContext(ctx).IsValid(),
		"TraceContext was not registered, so traceparent must be ignored")

	out := http.Header{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(out))
	assert.Contains(t, out.Get("baggage"), "tenant=acme")
}

// TestWithPropagators_emptyLeavesGlobalUntouched documents the documented
// no-op behaviour of passing no propagators.
func TestWithPropagators_emptyLeavesGlobalUntouched(t *testing.T) {
	restorePropagator(t)

	sentinel := propagation.TraceContext{}
	otel.SetTextMapPropagator(sentinel)

	_, err := tracing.New(tracing.WithPropagators())
	require.NoError(t, err)

	assert.Equal(t, sentinel, otel.GetTextMapPropagator())
}
