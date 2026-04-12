// Package httpserver provides a production-ready HTTP server built on Gin.
// It pre-mounts a standard middleware stack (request ID, tracing, structured
// logging, metrics, panic recovery, and error handling) and exposes /healthz,
// /readyz, and /health endpoints automatically.
//
// Use New to create a server, Engine to register application routes, and
// pass the returned *ginServer to app.WithServer.
package httpserver

import (
	"time"

	"github.com/pemako/mk/core"
)

// options holds the functional configuration for New.
type options struct {
	addr           string
	readTimeout    time.Duration
	writeTimeout   time.Duration
	logger         core.Logger
	serviceName    string
	serviceVersion string
}

// Option is a functional option for configuring the HTTP server.
type Option func(*options)

// WithAddr sets the TCP address the server listens on (e.g., ":8080").
// Defaults to ":8080".
func WithAddr(addr string) Option { return func(o *options) { o.addr = addr } }

// WithLogger sets the logger used for request logging. Defaults to logger.Default().
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

// WithServiceName sets the service name embedded in health check responses
// and used as the otel service name for tracing. Defaults to "service".
func WithServiceName(n string) Option { return func(o *options) { o.serviceName = n } }

// WithServiceVersion sets the service version embedded in health check responses.
// Defaults to "unknown".
func WithServiceVersion(v string) Option { return func(o *options) { o.serviceVersion = v } }

// WithReadTimeout sets the maximum duration for reading the full request,
// including the body. Defaults to 30 seconds.
func WithReadTimeout(d time.Duration) Option { return func(o *options) { o.readTimeout = d } }

// WithWriteTimeout sets the maximum duration before timing out writes of the
// response. Defaults to 30 seconds.
func WithWriteTimeout(d time.Duration) Option { return func(o *options) { o.writeTimeout = d } }

// WithConfig reads HTTP server settings from core.Config:
// server.http.addr (addr), server.http.timeout (read+write timeout),
// app.name (service name), and app.version (service version).
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
