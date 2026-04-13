package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Provider wraps an OpenTelemetry TracerProvider and implements core.Provider.
// When no exporter is configured a no-op provider is used so code runs
// without a collector in development or test environments.
type Provider struct {
	tp       trace.TracerProvider
	shutdown func(context.Context) error
}

// New creates a Provider with the given options.
// If no exporters are configured (no WithOTLP / WithOTLPGRPC / WithExporter
// calls), a no-op provider is returned and no network connections are made.
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

// newOTLPExporter creates an OTLP/HTTP exporter for the given endpoint.
func newOTLPExporter(endpoint string) func(context.Context) (sdktrace.SpanExporter, error) {
	return func(ctx context.Context) (sdktrace.SpanExporter, error) {
		return otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(endpointHost(endpoint)),
			otlptracehttp.WithInsecure(),
		)
	}
}

// newOTLPGRPCExporter creates an OTLP/gRPC exporter for the given endpoint.
func newOTLPGRPCExporter(endpoint string) func(context.Context) (sdktrace.SpanExporter, error) {
	return func(ctx context.Context) (sdktrace.SpanExporter, error) {
		conn, err := grpc.NewClient(
			endpointHost(endpoint),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, err
		}
		return otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	}
}
