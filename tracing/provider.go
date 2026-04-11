package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Provider wraps an otel TracerProvider and adds Shutdown.
// Implements core.Provider.
type Provider struct {
	tp       trace.TracerProvider
	shutdown func(context.Context) error
}

// New creates a Provider. If endpoint is empty, a no-op provider is used.
func New(opts ...Option) (*Provider, error) {
	o := options{
		serviceName: "service",
		sampler:     1.0,
	}
	for _, opt := range opts {
		opt(&o)
	}

	if o.endpoint == "" {
		return &Provider{
			tp:       noop.NewTracerProvider(),
			shutdown: func(context.Context) error { return nil },
		}, nil
	}

	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpoint(endpointHost(o.endpoint)),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, _ := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName(o.serviceName)),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(o.sampler)),
	)

	otel.SetTracerProvider(tp)

	return &Provider{tp: tp, shutdown: tp.Shutdown}, nil
}

// Tracer returns a named trace.Tracer. Implements core.Provider.
func (p *Provider) Tracer(name string) trace.Tracer {
	return p.tp.Tracer(name)
}

// Shutdown flushes pending spans. Register with app.WithAfterStop.
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.shutdown(ctx)
}
