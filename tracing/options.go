// Package tracing provides an OpenTelemetry tracing provider for mk services.
// Multiple exporters can be registered simultaneously so spans are forwarded to
// more than one backend in a single process.
//
// When no exporter is configured, a no-op provider is returned so instrumented
// code works without requiring a collector in development or test environments.
//
// Use New to create a Provider, register its Shutdown method with
// app.WithAfterStop to ensure all spans are flushed on exit.
//
// Example — single OTLP backend:
//
//	p, _ := tracing.New(
//	    tracing.WithServiceName("order-svc"),
//	    tracing.WithOTLP("http://otel-collector:4318"),
//	)
//
// Example — OTLP + Zipkin simultaneously:
//
//	p, _ := tracing.New(
//	    tracing.WithOTLP("http://otel-collector:4318"),
//	    tracing.WithZipkin("http://zipkin:9411/api/v2/spans"),
//	)
package tracing

import (
	"context"
	"net/url"

	"github.com/pemako/mk/core"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// options holds the functional configuration for New.
type options struct {
	serviceName string
	sampler     float64
	exporters   []sdktrace.SpanExporter // built lazily in New
	exporterFns []func(ctx context.Context) (sdktrace.SpanExporter, error)
}

// Option is a functional option for configuring the tracing Provider.
type Option func(*options)

// WithServiceName sets the OpenTelemetry service.name resource attribute.
// Defaults to "service".
func WithServiceName(name string) Option { return func(o *options) { o.serviceName = name } }

// WithSampler sets the trace sampling ratio in the range [0.0, 1.0].
// 1.0 samples all traces; 0.0 samples none. Defaults to 1.0.
func WithSampler(ratio float64) Option { return func(o *options) { o.sampler = ratio } }

// WithOTLP registers an OTLP/HTTP exporter sending spans to endpoint.
// endpoint accepts a full URL ("http://host:4318") or host:port ("host:4318").
// Multiple WithOTLP calls register multiple exporters.
// This is the recommended exporter for most backends, including modern Jaeger
// (≥ 1.35) which accepts OTLP natively on port 4318.
func WithOTLP(endpoint string) Option {
	return func(o *options) {
		o.exporterFns = append(o.exporterFns, newOTLPExporter(endpoint))
	}
}

// WithEndpoint is an alias for WithOTLP, kept for backward compatibility.
// Prefer WithOTLP in new code.
func WithEndpoint(endpoint string) Option { return WithOTLP(endpoint) }

// WithJaeger registers a Jaeger Thrift/HTTP exporter sending spans to endpoint
// (e.g., "http://jaeger:14268/api/traces").
//
// Deprecated: Jaeger ≥ 1.35 accepts OTLP natively. Prefer:
//
//	tracing.WithOTLP("http://jaeger:4318")
//
// Use WithJaeger only when targeting a Jaeger instance that does not support OTLP.
func WithJaeger(endpoint string) Option {
	return func(o *options) {
		o.exporterFns = append(o.exporterFns, newJaegerExporter(endpoint))
	}
}

// WithZipkin registers a Zipkin exporter sending spans to endpoint
// (e.g., "http://zipkin:9411/api/v2/spans").
//
// Deprecated: the upstream OTel Zipkin exporter will be removed in early 2027.
// Use WithZipkin only when targeting a Zipkin backend that cannot be migrated to OTLP.
func WithZipkin(endpoint string) Option {
	return func(o *options) {
		o.exporterFns = append(o.exporterFns, newZipkinExporter(endpoint))
	}
}

// WithExporter registers a pre-built SpanExporter. Use this for custom or
// third-party exporters (e.g., Datadog, Lightstep) that are not directly
// supported by the built-in options.
func WithExporter(exp sdktrace.SpanExporter) Option {
	return func(o *options) {
		o.exporterFns = append(o.exporterFns, func(_ context.Context) (sdktrace.SpanExporter, error) {
			return exp, nil
		})
	}
}

// WithConfig reads tracing settings from core.Config:
//
//	tracing.endpoint — OTLP collector URL (sets one OTLP exporter, same as WithOTLP)
//	tracing.sampler  — sampling ratio 0.0–1.0
//	app.name         — service name
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("tracing.endpoint"); v != "" {
			WithOTLP(v)(o)
		}
		if v := cfg.GetFloat64("tracing.sampler"); v > 0 {
			o.sampler = v
		}
		if v := cfg.GetString("app.name"); v != "" {
			o.serviceName = v
		}
	}
}

// endpointHost extracts host:port from a full URL or returns the string as-is.
// otlptracehttp.WithEndpoint expects host:port, not a full URL.
func endpointHost(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
