// Package main demonstrates the minimal setup for an HTTP service using forge:
// config → logger → server/http → app lifecycle.
//
// File layout (single package, no sub-packages):
//
//	main.go    — wire & start
//	handler.go — HTTP handlers
//	model.go   — request / response types
//
// Routes:
//
//	GET  /hello        greeting, name from query string
//	GET  /ping         liveness probe
//	GET  /users/:id    fetch a user by ID
//	POST /users        create a user
package main

import (
	"context"
	"net"

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
	)
	if err != nil {
		panic(err)
	}

	registerRoutes(srv.Engine())

	application := app.New(
		app.WithName("quickstart"),
		app.WithVersion("0.1.0"),
		app.WithServer(srv),
		app.WithBeforeStart(func(ctx context.Context) error {
			base := "http://localhost" + ln.Addr().String()
			logger.Info(ctx, "quickstart started", logger.F("addr", base))
			return nil
		}),
	)

	if err := application.Run(); err != nil {
		logger.Error(context.Background(), "application exited with error",
			logger.F("error", err),
		)
	}
}

// registerRoutes mounts all routes.
// In a real service this would live in an internal/handler package.
func registerRoutes(r *gin.Engine) {
	r.GET("/hello", helloHandler)
	r.GET("/ping", pingHandler)
	r.GET("/users/:id", getUserHandler)
	r.POST("/users", createUserHandler)
}
