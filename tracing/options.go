// Package tracing provides an OpenTelemetry tracing provider for mk services.
//
// Supports OTLP/HTTP and OTLP/gRPC exporters. When no exporter is configured
// a no-op provider is returned so instrumented code works without a collector
// in development or test environments.
//
// Use New to create a Provider and register its Shutdown with app.WithAfterStop
// to flush all pending spans on exit.
//
// Example — OTLP/HTTP (port 4318):
//
//	p, _ := tracing.New(
//	    tracing.WithServiceName("order-svc"),
//	    tracing.WithServiceVersion("1.2.0"),
//	    tracing.WithEnvironment("production"),
//	    tracing.WithOTLP("http://otel-collector:4318"),
//	)
//
// Example — OTLP/gRPC (port 4317 or 14317):
//
//	p, _ := tracing.New(
//	    tracing.WithServiceName("order-svc"),
//	    tracing.WithOTLPGRPC("otel-collector:14317"),
//	)
//
// Example — config-driven (protocol auto-detected by port):
//
//	p, _ := tracing.New(tracing.WithConfig(cfg))
package tracing

import (
	"context"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/pemako/mk/core"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// options holds the functional configuration for New.
type options struct {
	serviceName    string
	serviceVersion string
	environment    string
	sampler        float64
	insecure       bool // default true (no TLS); set false via WithTLS
	exporterFns    []func(ctx context.Context) (sdktrace.SpanExporter, error)
	logger         core.Logger
	extraAttrs     []attribute.KeyValue // extra resource attributes
}

// Option is a functional option for configuring the tracing Provider.
type Option func(*options)

// WithServiceName sets the OpenTelemetry service.name resource attribute.
// Defaults to "service".
func WithServiceName(name string) Option { return func(o *options) { o.serviceName = name } }

// WithServiceVersion sets the service.version resource attribute.
// Appears in every exported span. Defaults to empty (omitted).
func WithServiceVersion(v string) Option { return func(o *options) { o.serviceVersion = v } }

// WithEnvironment sets the deployment.environment resource attribute
// (e.g., "production", "staging", "development").
// Defaults to empty (omitted).
func WithEnvironment(env string) Option { return func(o *options) { o.environment = env } }

// WithSampler sets the trace sampling ratio in the range [0.0, 1.0].
// 1.0 samples all traces; 0.0 samples none. Defaults to 1.0.
func WithSampler(ratio float64) Option { return func(o *options) { o.sampler = ratio } }

// WithTLS enables TLS for gRPC and HTTP exporters.
// Use this when the collector requires a secure connection (most production
// environments). By default connections are insecure (no TLS).
func WithTLS() Option { return func(o *options) { o.insecure = false } }

// WithLogger sets the logger used to surface exporter errors (e.g., connection
// failures, TLS handshake errors). Without this option, exporter errors are
// silently dropped.
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

// WithAttributes appends arbitrary key-value pairs to the OTel resource.
// These appear on every span exported by this provider. Use this for custom
// dimensions that are not covered by the standard semconv options
// (e.g., cluster name, region, tenant ID).
//
//	tracing.New(
//	    tracing.WithAttributes(
//	        attribute.String("k8s.cluster.name", "prod-us-east-1"),
//	        attribute.String("tenant",           "acme"),
//	    ),
//	)
func WithAttributes(attrs ...attribute.KeyValue) Option {
	return func(o *options) { o.extraAttrs = append(o.extraAttrs, attrs...) }
}

// WithOTLP registers an OTLP/HTTP exporter sending spans to endpoint.
// endpoint accepts a full URL ("http://host:4318") or bare host:port ("host:4318").
// Use this for collectors listening on the HTTP port (4318 / 14318).
func WithOTLP(endpoint string) Option {
	return func(o *options) {
		o.exporterFns = append(o.exporterFns, newOTLPExporter(endpoint))
	}
}

// WithOTLPGRPC registers an OTLP/gRPC exporter sending spans to endpoint.
// Use this for collectors listening on the gRPC port (4317 / 14317).
// TLS is controlled by WithTLS() option or tracing.tls config key.
//
//	tracing.WithOTLPGRPC("otel-collector:14317")
func WithOTLPGRPC(endpoint string) Option {
	return func(o *options) {
		o.exporterFns = append(o.exporterFns, func(ctx context.Context) (sdktrace.SpanExporter, error) {
			return newOTLPGRPCExporterWithOpts(ctx, endpoint, o.insecure)
		})
	}
}

// WithEndpoint is an alias for WithOTLP kept for backward compatibility.
// Prefer WithOTLP or WithOTLPGRPC in new code.
func WithEndpoint(endpoint string) Option { return WithOTLP(endpoint) }

// WithExporter registers a pre-built SpanExporter for custom or third-party
// backends (e.g., Datadog, Lightstep) not covered by the built-in options.
func WithExporter(exp sdktrace.SpanExporter) Option {
	return func(o *options) {
		o.exporterFns = append(o.exporterFns, func(_ context.Context) (sdktrace.SpanExporter, error) {
			return exp, nil
		})
	}
}

// WithConfig reads tracing settings from core.Config:
//
//	tracing.endpoint   — collector URL or host:port
//	tracing.sampler    — sampling ratio 0.0–1.0
//	tracing.tls        — bool, default false; set true to enable TLS
//	app.name           — service.name resource attribute
//	app.version        — service.version resource attribute
//	app.env            — deployment.environment resource attribute
//
// Protocol is selected automatically by port:
//   - port 4317 or 14317 → OTLP gRPC
//   - anything else       → OTLP HTTP
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("tracing.endpoint"); v != "" {
			if isGRPCPort(v) {
				WithOTLPGRPC(v)(o)
			} else {
				WithOTLP(v)(o)
			}
		}
		if v := cfg.GetFloat64("tracing.sampler"); v > 0 {
			o.sampler = v
		}
		if v := cfg.GetString("app.name"); v != "" {
			o.serviceName = v
		}
		if v := cfg.GetString("app.version"); v != "" {
			o.serviceVersion = v
		}
		if v := cfg.GetString("app.env"); v != "" {
			o.environment = v
		}
		// tracing.insecure defaults to true; explicitly set false to enable TLS
		if cfg.GetBool("tracing.tls") {
			o.insecure = false
		}
	}
}

// isGRPCPort reports whether the endpoint uses the OTLP gRPC port (4317 or 14317).
func isGRPCPort(endpoint string) bool {
	h := endpointHost(endpoint)
	return strings.HasSuffix(h, ":4317") || strings.HasSuffix(h, ":14317")
}

// endpointHost extracts host:port from a full URL or returns the string as-is.
func endpointHost(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
