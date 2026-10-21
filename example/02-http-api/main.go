// Package main demonstrates a layered HTTP API service using forge.
//
// File layout:
//
//	main.go                    — wire & start
//	internal/model/user.go     — domain types, request/response structs
//	internal/service/user.go   — business logic (in-memory store)
//	internal/handler/routes.go — route registration, JWT claims type
//	internal/handler/auth.go   — login handler
//	internal/handler/user.go   — user CRUD handlers
//
// Routes:
//
//	POST   /auth/login   no auth — returns JWT
//	GET    /users        auth required — paginated list
//	GET    /users/:id    auth required — get by ID
//	POST   /users        auth required — create
//	PUT    /users/:id    auth required — update
//	DELETE /users/:id    auth required — delete
package main

import (
	"context"
	"net"

	"github.com/pemako/forge/app"
	"github.com/pemako/forge/config"
	"github.com/pemako/forge/logger"
	httpserver "github.com/pemako/forge/server/http"

	"github.com/pemako/forge/example/02-http-api/internal/handler"
	"github.com/pemako/forge/example/02-http-api/internal/service"
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

	// Random port: port is known before Start, so we can log it in BeforeStart.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	srv, err := httpserver.New(
		httpserver.WithListener(ln),
		httpserver.WithServiceName("http-api"),
	)
	if err != nil {
		panic(err)
	}

	svc := service.NewUserService()
	handler.Register(srv.Engine(), svc, cfg)

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	base := "http://localhost:" + port

	application := app.New(
		app.WithName("http-api"),
		app.WithVersion("0.1.0"),
		app.WithServer(srv),
		app.WithBeforeStart(func(ctx context.Context) error {
			logger.Info(ctx, "http-api started", logger.F("addr", base))
			return nil
		}),
	)

	if err := application.Run(); err != nil {
		logger.Error(context.Background(), "application exited with error",
			logger.F("error", err),
		)
	}
}
