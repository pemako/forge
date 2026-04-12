package tracing_test

import (
	"context"
	"testing"
	"time"

	"github.com/pemako/mk/config"
	"github.com/pemako/mk/core"
	"github.com/pemako/mk/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestNew_noopWhenNoEndpoint(t *testing.T) {
	p, err := tracing.New(tracing.WithServiceName("test"))
	require.NoError(t, err)
	assert.NotNil(t, p)

	tr := p.Tracer("test")
	ctx, span := tr.Start(context.Background(), "test-span")
	assert.NotNil(t, ctx)
	span.End()

	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestProvider_implementsCoreProvider(t *testing.T) {
	p, err := tracing.New()
	require.NoError(t, err)
	var _ core.Provider = p
}

func TestProvider_tracerIsTraceTracer(t *testing.T) {
	p, _ := tracing.New()
	tr := p.Tracer("svc")
	var _ trace.Tracer = tr
	assert.NotNil(t, tr)
}

func TestNew_withEndpoint_noConnection(t *testing.T) {
	// endpoint set but unreachable — New should still succeed
	// (connection is lazy in OTLP exporters)
	p, err := tracing.New(
		tracing.WithEndpoint("localhost:14317"),
		tracing.WithServiceName("test"),
		tracing.WithSampler(0.5),
	)
	require.NoError(t, err)
	assert.NotNil(t, p)
	// Shutdown should not hang even if no connection was made
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.Shutdown(ctx)
}

func TestWithConfig_reads(t *testing.T) {
	cfg, err := config.Load(config.WithFile("../config/testdata/config.yaml"))
	require.NoError(t, err)
	opt := tracing.WithConfig(cfg)
	assert.NotNil(t, opt)
}

func TestWithSampler_zero(t *testing.T) {
	p, err := tracing.New(tracing.WithSampler(0.0))
	require.NoError(t, err)
	assert.NotNil(t, p)
	_ = p.Shutdown(context.Background())
}

func TestWithServiceName(t *testing.T) {
	p, err := tracing.New(tracing.WithServiceName("my-svc"))
	require.NoError(t, err)
	tr := p.Tracer("my-svc")
	assert.NotNil(t, tr)
	_ = p.Shutdown(context.Background())
}

func TestProvider_multipleTracers(t *testing.T) {
	p, err := tracing.New()
	require.NoError(t, err)
	t1 := p.Tracer("svc1")
	t2 := p.Tracer("svc2")
	assert.NotNil(t, t1)
	assert.NotNil(t, t2)
	_ = p.Shutdown(context.Background())
}
