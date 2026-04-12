// Package grpcserver provides a production-ready gRPC server for mk services.
// It pre-configures a standard interceptor chain (request ID, tracing via
// otelgrpc stats handler, structured logging, metrics, panic recovery, and
// error mapping) and registers the gRPC health protocol automatically.
// Reflection is enabled by default and automatically disabled in production
// environments to prevent service introspection.
//
// Use New to create a server, Server to register gRPC services, and pass the
// returned *grpcServer to app.WithServer.
package grpcserver

import "github.com/pemako/mk/core"

// options holds the functional configuration for New.
type options struct {
	addr       string
	logger     core.Logger
	reflection bool
	env        string
	tlsCert    string
	tlsKey     string
}

// Option is a functional option for configuring the gRPC server.
type Option func(*options)

// WithAddr sets the TCP address the server listens on (e.g., ":9090").
// Defaults to ":9090".
func WithAddr(addr string) Option { return func(o *options) { o.addr = addr } }

// WithLogger sets the logger used for request logging. Defaults to logger.Default().
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

// WithReflection enables or disables gRPC server reflection.
// Reflection is enabled by default but automatically disabled when the
// environment is "production". Use this to override that behavior explicitly.
func WithReflection(on bool) Option { return func(o *options) { o.reflection = on } }

// WithTLS configures mutual TLS using the given certificate and key files.
// When set, the server requires clients to connect with TLS.
func WithTLS(cert, key string) Option {
	return func(o *options) { o.tlsCert = cert; o.tlsKey = key }
}

// WithConfig reads gRPC server settings from core.Config:
// server.grpc.addr (listen address) and app.env (environment name).
// When app.env is "production", reflection is disabled automatically.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("server.grpc.addr"); v != "" {
			o.addr = v
		}
		if v := cfg.GetString("app.env"); v != "" {
			o.env = v
		}
	}
}
