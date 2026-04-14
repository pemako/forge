// Package httpserver provides a production-ready HTTP server built on Gin.
// It pre-mounts a standard middleware stack (request ID, tracing, structured
// logging, metrics, panic recovery, and error handling) and exposes /healthz,
// /readyz, and /health endpoints automatically.
//
// Use New to create a server, Engine to register application routes, and
// pass the returned *ginServer to app.WithServer.
package httpserver

import (
	"context"
	"net"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/core"
)

// ReadyCheckFunc is a custom readiness check function.
// Return nil when the component is ready; return an error to signal not ready.
// The check name is used as the key in the /readyz response body.
type ReadyCheckFunc func(ctx context.Context) error

// readyCheck pairs a name with its check function.
type readyCheck struct {
	name string
	fn   ReadyCheckFunc
}

// options holds the functional configuration for New.
type options struct {
	addr            string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	logger          core.Logger
	serviceName     string
	serviceVersion  string
	listener        net.Listener
	readyChecks     []readyCheck
	middlewareStack []gin.HandlerFunc // nil = use default stack; non-nil = use as-is (may be empty)
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

// WithListener sets an explicit net.Listener for the server to use instead of
// opening a new TCP socket. When set, Start calls srv.Serve(lis) instead of
// srv.ListenAndServe, which allows using ":0" (random port) in tests.
func WithListener(lis net.Listener) Option {
	return func(o *options) { o.listener = lis }
}

// WithReadyCheck registers a named readiness check function.
// The /readyz endpoint calls all registered checks in addition to the
// SetReady flag. If any check returns an error, /readyz returns 503 with
// a JSON body listing each check's status.
//
// Multiple checks can be registered; all must pass for the server to be ready:
//
//	httpserver.New(
//	    httpserver.WithReadyCheck("database", func(ctx context.Context) error {
//	        return database.DB("default").WithContext(ctx).Exec("SELECT 1").Error
//	    }),
//	    httpserver.WithReadyCheck("cache", func(ctx context.Context) error {
//	        c := cache.Client("default")
//	        if c == nil { return errors.New("not initialized") }
//	        _, err := c.Exists(ctx, "__health")
//	        return err
//	    }),
//	)
func WithReadyCheck(name string, fn ReadyCheckFunc) Option {
	return func(o *options) {
		o.readyChecks = append(o.readyChecks, readyCheck{name: name, fn: fn})
	}
}

// WithHealthChecks registers multiple core.HealthChecker instances as readiness
// checks. This is equivalent to calling WithReadyCheck for each checker.
// Use this when you have a slice of checkers built from database.Manager or
// cache.Manager:
//
//	httpserver.New(
//	    httpserver.WithHealthChecks(dbMgr.HealthChecker("default"), cacheMgr.HealthChecker("redis")),
//	)
func WithHealthChecks(checkers ...core.HealthChecker) Option {
	return func(o *options) {
		for _, c := range checkers {
			o.readyChecks = append(o.readyChecks, readyCheck{
				name: c.Name(),
				fn:   c.Check,
			})
		}
	}
}

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

// WithMiddlewareStack completely replaces the built-in default middleware stack
// with the provided handlers. New() will mount exactly these handlers in the
// given order — nothing more, nothing less.
//
// Three modes:
//
//   - Not called: New() mounts the default stack
//     (RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler).
//   - Called with handlers: those handlers become the entire middleware stack.
//   - Called with no handlers (or use WithNoMiddleware): no request middleware
//     is mounted; health endpoints are still registered.
//
// This is NOT an append — it replaces the default stack entirely.
// To extend the default stack without copying its internals, use
// DefaultMiddlewareStack:
//
//	stack := httpserver.DefaultMiddlewareStack("svc", log)
//	stack = append(stack, myAuthMiddleware)
//	srv, _ := httpserver.New(httpserver.WithMiddlewareStack(stack...))
//
// WARNING: omitting Recovery() means panics will crash the server process.
func WithMiddlewareStack(middlewares ...gin.HandlerFunc) Option {
	return func(o *options) {
		// Store as non-nil slice so New() can distinguish "explicitly set to empty"
		// from "never set" (nil). A nil stack means "use default".
		if middlewares == nil {
			o.middlewareStack = []gin.HandlerFunc{}
		} else {
			o.middlewareStack = middlewares
		}
	}
}

// WithNoMiddleware mounts no request middleware on the server.
// Health endpoints (/healthz, /readyz, /health) are still registered.
// This is equivalent to WithMiddlewareStack() with no arguments.
//
// WARNING: without Recovery(), panics in route handlers will crash the process.
// Without ErrorHandler(), c.Error() calls will not produce JSON responses.
func WithNoMiddleware() Option {
	return WithMiddlewareStack()
}
