// Package metric provides OpenTelemetry metrics initialization and helper
// functions for mk services. It supports Prometheus (pull) and OTLP (push)
// exporters and automatically wires HTTP and gRPC instrumentation hooks when
// Init is called.
//
// Use Init once during application startup. Call the package-level helper
// functions (Count, Gauge, StartTimer, etc.) anywhere in your code without
// passing a meter explicitly.
package metric

import "github.com/pemako/mk/core"

// options holds the functional configuration for Init.
type options struct {
	serviceName    string
	prometheusAddr string
	otlpEndpoint   string
}

// Option is a functional option for configuring metric initialization.
type Option func(*options)

// WithServiceName sets the OpenTelemetry service.name resource attribute.
// Defaults to "service".
func WithServiceName(name string) Option { return func(o *options) { o.serviceName = name } }

// WithPrometheusAddr sets the TCP address for the Prometheus /metrics HTTP
// endpoint (e.g., ":9090"). When set, a separate HTTP server is started to
// serve Prometheus metrics. Leave empty to disable Prometheus export.
func WithPrometheusAddr(addr string) Option { return func(o *options) { o.prometheusAddr = addr } }

// WithOTLPEndpoint sets the OTLP collector endpoint for push-based metric
// export (e.g., "http://localhost:4318"). Accepts full URLs or host:port strings.
// Leave empty to disable OTLP export.
func WithOTLPEndpoint(ep string) Option { return func(o *options) { o.otlpEndpoint = ep } }

// WithConfig reads metric settings from core.Config:
// metric.prometheus_addr, metric.otlp_endpoint, and app.name.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("metric.prometheus_addr"); v != "" {
			o.prometheusAddr = v
		}
		if v := cfg.GetString("metric.otlp_endpoint"); v != "" {
			o.otlpEndpoint = v
		}
		if v := cfg.GetString("app.name"); v != "" {
			o.serviceName = v
		}
	}
}
