package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"  //nolint:staticcheck
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/zipkin"  //nolint:staticcheck
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Provider wraps an OpenTelemetry TracerProvider and implements core.Provider.
// Supports zero or more exporters simultaneously. When no exporter is configured,
// it uses a no-op provider so code runs without a collector.
type Provider struct {
	tp       trace.TracerProvider
	shutdown func(context.Context) error
}

// New creates a Provider. If no exporters are configured (no WithOTLP / WithJaeger /
// WithZipkin / WithExporter / WithEndpoint calls), a no-op provider is returned
// and no network connections are made.
// Defaults: serviceName="service", sampler=1.0.
func New(opts ...Option) (*Provider, error) {
	o := options{
		serviceName: "service",
		sampler:     1.0,
	}
	for _, opt := range opts {
		opt(&o)
	}

	ctx := context.Background()

	var exporters []sdktrace.SpanExporter
	for _, fn := range o.exporterFns {
		exp, err := fn(ctx)
		if err != nil {
			return nil, err
		}
		exporters = append(exporters, exp)
	}

	if len(exporters) == 0 {
		return &Provider{
			tp:       noop.NewTracerProvider(),
			shutdown: func(context.Context) error { return nil },
		}, nil
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(o.serviceName)),
	)

	sdkOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(o.sampler)),
	}
	for _, exp := range exporters {
		sdkOpts = append(sdkOpts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(sdkOpts...)
	otel.SetTracerProvider(tp)

	return &Provider{tp: tp, shutdown: tp.Shutdown}, nil
}

// Tracer returns a named trace.Tracer. Implements core.Provider.
func (p *Provider) Tracer(name string) trace.Tracer {
	return p.tp.Tracer(name)
}

// Shutdown flushes pending spans and releases exporter resources.
// Register with app.WithAfterStop.
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.shutdown(ctx)
}

// --- exporter constructors (package-private, called by options.go) ---

func newOTLPExporter(endpoint string) func(context.Context) (sdktrace.SpanExporter, error) {
	return func(ctx context.Context) (sdktrace.SpanExporter, error) {
		return otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(endpointHost(endpoint)),
			otlptracehttp.WithInsecure(),
		)
	}
}

func newJaegerExporter(endpoint string) func(context.Context) (sdktrace.SpanExporter, error) {
	return func(_ context.Context) (sdktrace.SpanExporter, error) {
		return jaeger.New( //nolint:staticcheck
			jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(endpoint)), //nolint:staticcheck
		)
	}
}

func newZipkinExporter(endpoint string) func(context.Context) (sdktrace.SpanExporter, error) {
	return func(_ context.Context) (sdktrace.SpanExporter, error) {
		return zipkin.New(endpoint) //nolint:staticcheck
	}
}
