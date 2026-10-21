// Package main demonstrates database integration using forge:
// SQLite + GORM + goose migrations + health check + HTTP API.
//
// File layout:
//
//	main.go                         — wire & start
//	config.yaml                     — database DSN, logger settings
//	migrations/00001_create_users.sql — goose migration
//	internal/model/user.go          — GORM model + request types
//	internal/handler/user.go        — CRUD handlers
//	internal/handler/routes.go      — route registration
//
// Startup sequence (managed by app):
//  1. database.Manager.Up() applies pending goose migrations
//  2. BeforeStart: HTTP server marks itself ready, logs address
//  3. server/http starts listening
//
// Routes:
//
//	GET    /users          — paginated list
//	GET    /users/:id      — get by ID
//	POST   /users          — create
//	PUT    /users/:id      — update
//	DELETE /users/:id      — delete
//	GET    /readyz         — health check (probes DB with SELECT 1)
package main

import (
	"context"
	"net"

	"github.com/pemako/forge/app"
	"github.com/pemako/forge/config"
	"github.com/pemako/forge/database"
	"github.com/pemako/forge/logger"
	httpserver "github.com/pemako/forge/server/http"

	"github.com/pemako/forge/example/04-database/internal/handler"
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

	// ── Database ──────────────────────────────────────────────────────────────
	dbMgr, err := database.New(
		database.WithConfig(cfg),
		database.WithLogger(log),
		database.WithMigrationsDir("migrations"),
	)
	if err != nil {
		panic(err)
	}
	if err := database.Register("default", dbMgr); err != nil {
		panic(err)
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	srv, err := httpserver.New(
		httpserver.WithListener(ln),
		httpserver.WithServiceName("database-example"),
		// Wire DB health check into /readyz
		httpserver.WithHealthChecks(dbMgr.HealthChecker("default")),
	)
	if err != nil {
		panic(err)
	}

	// Register CRUD routes, passing the default connection.
	handler.Register(srv.Engine(), database.DB("default"))

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	base := "http://localhost:" + port

	// ── App lifecycle ─────────────────────────────────────────────────────────
	application := app.New(
		app.WithName("database-example"),
		app.WithVersion("0.1.0"),
		app.WithLogger(log),
		app.WithServer(srv),
		// app.WithMigration runs dbMgr.Up() before BeforeStart hooks.
		app.WithMigration(dbMgr),
		app.WithBeforeStart(func(ctx context.Context) error {
			srv.SetReady(true)
			logger.Info(ctx, "database-example started",
				logger.F("addr", base),
				logger.F("health", base+"/readyz"),
			)
			return nil
		}),
		app.WithAfterStop(func(_ context.Context) error {
			return database.CloseAll()
		}),
	)

	if err := application.Run(); err != nil {
		logger.Error(context.Background(), "application exited with error",
			logger.F("error", err),
		)
	}
}
