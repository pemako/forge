package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pemako/forge/core"
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
		insecure:    true, // default: no TLS; use WithTLS() or tracing.tls=true to enable
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
		resource.WithHost(),    // host.name — auto-detected hostname
		resource.WithProcess(), // process.pid, process.executable.name
		resource.WithAttributes(
			semconv.ServiceName(o.serviceName),
		),
	)

	// Append service.version, deployment.environment, and any user-supplied
	// extra attributes into a second resource, then merge both.
	var extraAttrs []attribute.KeyValue
	if o.serviceVersion != "" {
		extraAttrs = append(extraAttrs, semconv.ServiceVersion(o.serviceVersion))
	}
	if o.environment != "" {
		extraAttrs = append(extraAttrs, semconv.DeploymentEnvironment(o.environment))
	}
	extraAttrs = append(extraAttrs, o.extraAttrs...)
	if len(extraAttrs) > 0 {
		extra, _ := resource.New(ctx, resource.WithAttributes(extraAttrs...))
		res, _ = resource.Merge(res, extra)
	}

	sdkOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(o.sampler)),
	}
	for _, exp := range exporters {
		sdkOpts = append(sdkOpts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(sdkOpts...)
	otel.SetTracerProvider(tp)

	// Register a global error handler so exporter errors (e.g., connection
	// refused, TLS handshake failure) are visible instead of silently dropped.
	if o.logger != nil {
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
			o.logger.Error(context.Background(), "otel exporter error",
				core.Field{Key: "err", Value: err.Error()})
		}))
	}

	return &Provider{tp: tp, shutdown: tp.Shutdown}, nil
}

// Tracer returns a named trace.Tracer. Implements core.Provider.
func (p *Provider) Tracer(name string) trace.Tracer {
	return p.tp.Tracer(name)
}

// Init is a no-op for tracing providers; all initialization happens in New.
// Exists to satisfy the app.Lifecycle interface.
func (p *Provider) Init(_ context.Context) error { return nil }

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

func newOTLPGRPCExporterWithOpts(ctx context.Context, endpoint string, useInsecure bool) (sdktrace.SpanExporter, error) {
	var creds credentials.TransportCredentials
	if useInsecure {
		creds = insecure.NewCredentials()
	} else {
		creds = credentials.NewClientTLSFromCert(nil, "")
	}
	conn, err := grpc.NewClient(endpointHost(endpoint), grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	return otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
}
