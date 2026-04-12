// Package tracing provides an OpenTelemetry tracing provider for mk services.
// When an OTLP endpoint is configured, spans are exported via OTLP/HTTP.
// When no endpoint is configured, a no-op provider is returned so instrumented
// code works without requiring a collector in development or test environments.
//
// Use New to create a Provider, and register its Shutdown method with
// app.WithAfterStop to ensure all spans are flushed on exit.
package tracing

import (
	"net/url"

	"github.com/pemako/mk/core"
)

// options holds the functional configuration for New.
type options struct {
	serviceName string
	endpoint    string
	sampler     float64
}

// Option is a functional option for configuring the tracing Provider.
type Option func(*options)

// WithServiceName sets the OpenTelemetry service.name resource attribute.
// Defaults to "service".
func WithServiceName(name string) Option { return func(o *options) { o.serviceName = name } }

// WithEndpoint sets the OTLP collector endpoint (e.g., "http://localhost:4318").
// When empty, a no-op provider is used. Accepts full URLs or host:port strings.
func WithEndpoint(ep string) Option { return func(o *options) { o.endpoint = ep } }

// WithSampler sets the trace sampling ratio in the range [0.0, 1.0].
// 1.0 samples all traces; 0.0 samples none. Defaults to 1.0.
func WithSampler(ratio float64) Option { return func(o *options) { o.sampler = ratio } }

// WithConfig reads tracing settings from core.Config:
// tracing.endpoint (collector URL), tracing.sampler (sampling ratio 0.0–1.0),
// and app.name (service name).
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("tracing.endpoint"); v != "" {
			o.endpoint = v
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
