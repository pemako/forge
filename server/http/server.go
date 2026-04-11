package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/pemako/mk/middleware/http"
)

type ginServer struct {
	opts options
	r    *gin.Engine
	srv  *http.Server
}

// New creates a new HTTP server with the default middleware stack pre-mounted.
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
	r.GET("/readyz", readyzHandler(o.serviceName, o.serviceVersion))
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
	return s, nil
}

// Engine returns the underlying *gin.Engine for business route registration.
func (s *ginServer) Engine() *gin.Engine { return s.r }

func (s *ginServer) Start(_ context.Context) error {
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *ginServer) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
