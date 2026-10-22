// Package metric provides OpenTelemetry metrics initialization and helper
// functions for mk services. It supports Prometheus (pull) and OTLP (push)
// exporters and automatically wires HTTP and gRPC instrumentation hooks when
// Init is called.
//
// Use Init once during application startup. Call the package-level helper
// functions (Count, Gauge, StartTimer, etc.) anywhere in your code without
// passing a meter explicitly.
package metric

import (
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/pemako/forge/core"
)

// options holds the functional configuration for Init.
type options struct {
	serviceName      string
	prometheusAddr   string
	otlpEndpoint     string
	allowNoExporters bool
	readers          []sdkmetric.Reader
}

// WithReader registers an additional sdkmetric.Reader on the MeterProvider.
//
// It exists mainly for tests: pass a sdkmetric.NewManualReader() and call its
// Collect to assert on what instruments actually recorded. Without this there
// was no way to observe emitted metrics, so code calling the metric helpers
// could not be tested at all.
//
// A registered reader also counts as a configured exporter, so Init does not
// return ErrNoExporters.
func WithReader(r sdkmetric.Reader) Option {
	return func(o *options) { o.readers = append(o.readers, r) }
}

// Option is a functional option for configuring metric initialization.
type Option func(*options)

// WithServiceName sets the OpenTelemetry service.name resource attribute.
// Defaults to "service".
func WithServiceName(name string) Option { return func(o *options) { o.serviceName = name } }

// WithAllowNoExporters lets Init succeed when neither a Prometheus address nor
// an OTLP endpoint is configured. Without it, Init returns ErrNoExporters.
//
// By default that case is an error: a MeterProvider with no reader accepts every
// measurement and discards it, so a service whose metric config failed to load
// looked healthy while emitting nothing. Pass this when metrics are switched off
// deliberately, such as in tests or a CLI build.
func WithAllowNoExporters() Option {
	return func(o *options) { o.allowNoExporters = true }
}

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
