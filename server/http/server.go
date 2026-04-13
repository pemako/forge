package httpserver

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	mw "github.com/pemako/mk/middleware/http"
)

type ginServer struct {
	opts  options
	r     *gin.Engine
	srv   *http.Server
	ready atomic.Bool
}

// New creates a new HTTP server with the default middleware stack pre-mounted.
// The middleware order is: RequestID -> Tracing -> Logger -> Metrics ->
// Recovery -> ErrorHandler. Health endpoints /healthz, /readyz, and /health
// are registered automatically before any business routes.
// Defaults: addr=":8080", readTimeout=30s, writeTimeout=30s.
func New(opts ...Option) (*ginServer, error) {
	o := options{
		addr:           ":8080",
		readTimeout:    30 * time.Second,
		writeTimeout:   30 * time.Second,
		serviceName:    "service",
		serviceVersion: "unknown",
	}
	for _, opt := range opts {
		opt(&o)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Default middleware stack (order is significant):
	// RequestID -> Tracing -> Logger -> Metrics -> Recovery -> ErrorHandler
	r.Use(
		mw.RequestID(),
		mw.Tracing(o.serviceName),
		mw.Logger(o.logger),
		mw.Metrics(),
		mw.Recovery(),
		mw.ErrorHandler(),
	)

	// Health routes — no auth, registered before business routes
	r.GET("/healthz", healthzHandler(o.serviceName, o.serviceVersion))
	r.GET("/health", healthzHandler(o.serviceName, o.serviceVersion))

	s := &ginServer{
		opts: o,
		r:    r,
		srv: &http.Server{
			Addr:         o.addr,
			Handler:      r,
			ReadTimeout:  o.readTimeout,
			WriteTimeout: o.writeTimeout,
		},
	}

	// Register /readyz after s is created so the closure captures the instance.
	r.GET("/readyz", readyzHandler(o.serviceName, o.serviceVersion, func() bool {
		return s.ready.Load()
	}, func() []readyCheck {
		return s.opts.readyChecks
	}))

	return s, nil
}

// AddReadyCheck adds a named readiness check to the server after construction.
// Call this in app.WithBeforeStart to bridge health checkers from app.HealthCheckers():
//
//	app.WithBeforeStart(func(ctx context.Context) error {
//	    for _, hc := range myApp.HealthCheckers() {
//	        httpSrv.AddReadyCheck(hc.Name(), hc.Check)
//	    }
//	    return nil
//	})
func (s *ginServer) AddReadyCheck(name string, fn func(context.Context) error) {
	s.opts.readyChecks = append(s.opts.readyChecks, readyCheck{name: name, fn: fn})
}

// SetReady marks this server instance as ready to serve traffic.
// Call after database migrations and startup checks complete.
// Affects the /readyz endpoint on this server instance only.
func (s *ginServer) SetReady(ready bool) { s.ready.Store(ready) }

// Engine returns the underlying *gin.Engine so callers can register
// application-specific routes and middleware after server creation.
func (s *ginServer) Engine() *gin.Engine { return s.r }

// Addr returns the address the server is listening on.
func (s *ginServer) Addr() string { return s.srv.Addr }

// Start begins listening on the configured address and blocks until the server
// is stopped. It implements core.Server.
func (s *ginServer) Start(_ context.Context) error {
	var err error
	if s.opts.listener != nil {
		s.srv.Addr = s.opts.listener.Addr().String()
		err = s.srv.Serve(s.opts.listener)
	} else {
		err = s.srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the HTTP server, waiting for in-flight requests
// to complete until the deadline carried by ctx is exceeded. It implements core.Server.
func (s *ginServer) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
