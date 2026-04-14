// Package main demonstrates the minimal setup for an HTTP service using forge:
// config → logger → server/http → app lifecycle.
//
// File layout (single package, no sub-packages):
//
//	main.go    — wire & start
//	handler.go — HTTP handlers (gin + huma)
//	model.go   — request / response types
//
// Routes:
//
//	GET  /hello        plain gin handler
//	GET  /ping         plain gin handler
//	GET  /users/{id}   huma type-safe handler
//	POST /users        huma type-safe handler
//
// Swagger UI: http://<addr>/docs
package main

import (
	"context"
	"net"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"github.com/pemako/forge/app"
	"github.com/pemako/forge/config"
	"github.com/pemako/forge/logger"
	httpserver "github.com/pemako/forge/server/http"
)

func main() {
	cfg, err := config.Load(
		config.WithFile("config.yaml"),
		config.WithEnv("APP"),
	)
	if err != nil {
		panic(err)
	}

	log, err := logger.New(logger.WithConfig(cfg))
	if err != nil {
		panic(err)
	}
	logger.SetDefault(log)

	// Pre-create listener so the actual port is known before Start is called.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	srv, err := httpserver.New(
		httpserver.WithListener(ln),
		httpserver.WithConfig(cfg),
		httpserver.WithServiceName("quickstart"),
		httpserver.WithOpenAPI("Quickstart API", "0.1.0", "/docs"),
	)
	if err != nil {
		panic(err)
	}

	registerRoutes(srv.Engine(), srv.API())

	application := app.New(
		app.WithName("quickstart"),
		app.WithVersion("0.1.0"),
		app.WithServer(srv),
		app.WithBeforeStart(func(ctx context.Context) error {
			base := "http://localhost" + ln.Addr().String()
			logger.Info(ctx, "quickstart started",
				logger.F("addr", base),
				logger.F("docs", base+"/docs"),
			)
			return nil
		}),
	)

	if err := application.Run(); err != nil {
		logger.Error(context.Background(), "application exited with error",
			logger.F("error", err),
		)
	}
}

// registerRoutes mounts all gin and huma routes.
// In a real service this would live in an internal/handler package.
func registerRoutes(r *gin.Engine, api huma.API) {
	r.GET("/hello", helloHandler)
	r.GET("/ping", pingHandler)

	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/users/{id}",
		Summary: "Get user",
		Tags:    []string{"users"},
	}, getUserHandler)

	huma.Register(api, huma.Operation{
		Method:  http.MethodPost,
		Path:    "/users",
		Summary: "Create user",
		Tags:    []string{"users"},
	}, createUserHandler)
}
