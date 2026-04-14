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

import (
	"net"

	"google.golang.org/grpc"

	"github.com/pemako/forge/core"
)

// options holds the functional configuration for New.
type options struct {
	addr                   string
	logger                 core.Logger
	reflection             bool
	env                    string
	tlsCert                string
	tlsKey                 string
	listener               net.Listener
	extraUnaryInterceptors []grpc.UnaryServerInterceptor
	grpcUIEnabled          bool
	grpcUIAddr             string // HTTP listen address for grpcui, e.g. ":0"
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

// WithListener sets an explicit net.Listener instead of opening a new TCP
// socket. Useful for tests and random-port scenarios (net.Listen("tcp", ":0")).
// When set, WithAddr is ignored.
func WithListener(lis net.Listener) Option {
	return func(o *options) { o.listener = lis }
}

// WithUnaryInterceptors appends additional unary server interceptors to the
// default chain (RequestID → Logger → Metrics → Recovery → ErrorHandler).
// Interceptors are appended in the order provided and run after the defaults.
func WithUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) Option {
	return func(o *options) {
		o.extraUnaryInterceptors = append(o.extraUnaryInterceptors, interceptors...)
	}
}

// WithGRPCUI enables the embedded grpcui web interface.
// addr is the HTTP listen address (e.g. ":8080" or ":0" for a random port).
// grpcui connects to the gRPC server via reflection and provides a browser UI
// for testing — equivalent to running `grpcui -plaintext <addr>` externally.
// Automatically disabled when env is "production" (same as reflection).
//
// Also readable from config key "server.grpc.ui_addr".
func WithGRPCUI(addr string) Option {
	return func(o *options) {
		o.grpcUIEnabled = true
		o.grpcUIAddr = addr
	}
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
		if v := cfg.GetString("server.grpc.ui_addr"); v != "" {
			o.grpcUIEnabled = true
			o.grpcUIAddr = v
		}
	}
}
