package tracing

import (
	"net/url"

	"github.com/pemako/mk/core"
)

type options struct {
	serviceName string
	endpoint    string
	sampler     float64
}

type Option func(*options)

func WithServiceName(name string) Option { return func(o *options) { o.serviceName = name } }
func WithEndpoint(ep string) Option      { return func(o *options) { o.endpoint = ep } }
func WithSampler(ratio float64) Option   { return func(o *options) { o.sampler = ratio } }

// WithConfig reads tracing.endpoint, tracing.sampler, app.name from core.Config.
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
