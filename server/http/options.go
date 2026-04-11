package httpserver

import (
	"time"

	"github.com/pemako/mk/core"
)

type options struct {
	addr           string
	readTimeout    time.Duration
	writeTimeout   time.Duration
	logger         core.Logger
	serviceName    string
	serviceVersion string
}

type Option func(*options)

func WithAddr(addr string) Option             { return func(o *options) { o.addr = addr } }
func WithLogger(l core.Logger) Option         { return func(o *options) { o.logger = l } }
func WithServiceName(n string) Option         { return func(o *options) { o.serviceName = n } }
func WithServiceVersion(v string) Option      { return func(o *options) { o.serviceVersion = v } }
func WithReadTimeout(d time.Duration) Option  { return func(o *options) { o.readTimeout = d } }
func WithWriteTimeout(d time.Duration) Option { return func(o *options) { o.writeTimeout = d } }

// WithConfig reads from core.Config: server.http.addr, server.http.timeout, app.name, app.version
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("server.http.addr"); v != "" {
			o.addr = v
		}
		if v := cfg.GetDuration("server.http.timeout"); v > 0 {
			o.readTimeout = v
			o.writeTimeout = v
		}
		if v := cfg.GetString("app.name"); v != "" {
			o.serviceName = v
		}
		if v := cfg.GetString("app.version"); v != "" {
			o.serviceVersion = v
		}
	}
}
