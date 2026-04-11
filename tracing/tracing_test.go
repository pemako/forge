package tracing_test

import (
	"context"
	"testing"

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
