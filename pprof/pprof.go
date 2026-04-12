// Package pprof exposes Go's built-in profiling endpoints for diagnosing
// performance issues such as CPU hotspots, memory leaks, goroutine leaks,
// and lock contention. It supports two usage modes:
//
// Mode 1 — standalone server (public-facing services, strict isolation):
//
//	app.WithServer(pprof.New())                  // listens on localhost:6060
//	app.WithServer(pprof.New(
//	    pprof.WithAddr("localhost:6060"),
//	    pprof.WithMutexFraction(1),              // enable mutex profiling
//	))
//
// Mode 2 — mount on an existing gin.Engine (internal services, no extra port):
//
//	pprof.Register(httpSrv.Engine())             // adds /debug/pprof/* routes
//	pprof.Register(httpSrv.Engine(),
//	    pprof.WithMutexFraction(1))
//
// Security: the standalone server binds to localhost by default and is never
// reachable from outside the host. Use kubectl port-forward or an SSH tunnel
// to access it in production. When using Register, ensure the gin.Engine is
// not exposed to the public internet.
//
// Profiling endpoints available at /debug/pprof/:
//
//	/debug/pprof/          — index of all profiles
//	/debug/pprof/cmdline   — process command line
//	/debug/pprof/profile   — 30-second CPU profile (blocking)
//	/debug/pprof/symbol    — symbol lookup
//	/debug/pprof/trace     — execution trace
//	/debug/pprof/heap      — heap allocations
//	/debug/pprof/goroutine — all goroutine stacks
//	/debug/pprof/mutex     — mutex contention (requires WithMutexFraction)
//	/debug/pprof/block     — goroutine blocking (requires WithBlockRate)
package pprof

import (
	"context"
	"net/http"
	netpprof "net/http/pprof"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/core"
)

// Server is a standalone HTTP server that exposes pprof endpoints.
// Create with New and register with app.WithServer to integrate with the
// application lifecycle. Implements core.Server.
type Server struct {
	opts options
	srv  *http.Server
}

// New creates a pprof Server that listens on a dedicated address.
// Default address is "localhost:6060". The server only starts when
// app.Run calls Start; it blocks until Stop is called.
//
// Use this mode when the business HTTP server may be public-facing and you
// need strict isolation of profiling endpoints. Access via:
//
//	kubectl port-forward pod/xxx 6060:6060
//	go tool pprof http://localhost:6060/debug/pprof/heap
func New(opts ...Option) *Server {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	mux := http.NewServeMux()
	registerRoutes(mux)
	applyRuntimeSettings(o)
	return &Server{
		opts: o,
		srv:  &http.Server{Addr: o.addr, Handler: mux},
	}
}

// Start begins listening on the configured address and blocks until Stop is
// called. It implements core.Server.
func (s *Server) Start(_ context.Context) error {
	s.opts.log.Info(context.Background(), "pprof server started",
		core.Field{Key: "addr", Value: s.srv.Addr})
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the pprof server within the deadline carried by
// ctx. It implements core.Server.
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Register mounts pprof handlers on an existing gin.Engine without starting
// a new server. All /debug/pprof/* routes are added immediately.
//
// Use this mode when the service is purely internal (e.g., a Kubernetes
// cluster-internal service) and you want to avoid managing an extra port.
// Ensure the gin.Engine is not exposed to the public internet.
//
//	pprof.Register(httpSrv.Engine())
//	pprof.Register(httpSrv.Engine(), pprof.WithMutexFraction(1))
func Register(engine *gin.Engine, opts ...Option) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	applyRuntimeSettings(o)

	engine.GET("/debug/pprof/", gin.WrapF(netpprof.Index))
	engine.GET("/debug/pprof/cmdline", gin.WrapF(netpprof.Cmdline))
	engine.GET("/debug/pprof/profile", gin.WrapF(netpprof.Profile))
	engine.GET("/debug/pprof/symbol", gin.WrapF(netpprof.Symbol))
	engine.GET("/debug/pprof/trace", gin.WrapF(netpprof.Trace))
	// heap, goroutine, allocs, block, mutex, threadcreate are served by Index
	engine.GET("/debug/pprof/:profile", gin.WrapF(netpprof.Index))
}

// registerRoutes adds all pprof handlers to mux.
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", netpprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", netpprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", netpprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", netpprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", netpprof.Trace)
}
