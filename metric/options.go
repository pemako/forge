package metric

import "github.com/pemako/mk/core"

type options struct {
	serviceName    string
	prometheusAddr string
	otlpEndpoint   string
}

type Option func(*options)

func WithServiceName(name string) Option    { return func(o *options) { o.serviceName = name } }
func WithPrometheusAddr(addr string) Option { return func(o *options) { o.prometheusAddr = addr } }
func WithOTLPEndpoint(ep string) Option     { return func(o *options) { o.otlpEndpoint = ep } }

// WithConfig reads metric.prometheus_addr, metric.otlp_endpoint, app.name.
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
