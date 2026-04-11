package main

import (
	"context"
	"os"

	"github.com/pemako/mk/app"
	"github.com/pemako/mk/cache"
	"github.com/pemako/mk/config"
	"github.com/pemako/mk/database"
	httphandler "github.com/pemako/mk/example/internal/handler/http"
	"github.com/pemako/mk/example/internal/service"
	"github.com/pemako/mk/logger"
	"github.com/pemako/mk/metric"
	grpcserver "github.com/pemako/mk/server/grpc"
	httpserver "github.com/pemako/mk/server/http"
	"github.com/pemako/mk/tracing"
)

func main() {
	// Config
	cfg, err := config.Load(config.WithFile("config.yaml"), config.WithEnv("APP"))
	if err != nil {
		panic("config: " + err.Error())
	}

	// Logger
	log, err := logger.New(logger.WithConfig(cfg))
	if err != nil {
		panic("logger: " + err.Error())
	}
	logger.SetDefault(log)

	// Tracing (noop when endpoint is empty)
	provider, err := tracing.New(tracing.WithConfig(cfg))
	if err != nil {
		panic("tracing: " + err.Error())
	}

	// Metrics
	if err := metric.Init(metric.WithConfig(cfg)); err != nil {
		panic("metric: " + err.Error())
	}

	// Database
	dbMgr, err := database.New(
		database.WithConfig(cfg),
		database.WithLogger(log),
		database.WithMigrationsDir("migrations"),
	)
	if err != nil {
		panic("database: " + err.Error())
	}

	// Cache (optional — skip if not configured)
	_ = cache.Init(cache.WithConfig(cfg))

	// HTTP server
	httpSrv, err := httpserver.New(
		httpserver.WithConfig(cfg),
		httpserver.WithLogger(log),
	)
	if err != nil {
		panic("http server: " + err.Error())
	}

	// gRPC server
	grpcSrv, err := grpcserver.New(
		grpcserver.WithConfig(cfg),
		grpcserver.WithLogger(log),
	)
	if err != nil {
		panic("grpc server: " + err.Error())
	}

	// Register HTTP routes
	userSvc := service.NewUserService()
	userHandler := httphandler.NewUserHandler(userSvc)
	userHandler.RegisterRoutes(httpSrv.Engine())

	// Run
	if err := app.New(
		app.WithName(cfg.GetString("app.name")),
		app.WithVersion(cfg.GetString("app.version")),
		app.WithServer(httpSrv, grpcSrv),
		app.WithLogger(log),
		app.WithMigration(dbMgr),
		app.WithBeforeStart(func(ctx context.Context) error {
			httpserver.SetReady(true)
			log.Info(ctx, "service starting",
				logger.F("name", cfg.GetString("app.name")),
				logger.F("http", cfg.GetString("server.http.addr")),
				logger.F("grpc", cfg.GetString("server.grpc.addr")),
			)
			return nil
		}),
		app.WithAfterStop(func(ctx context.Context) error {
			metric.Shutdown()
			return provider.Shutdown(ctx)
		}),
	).Run(); err != nil {
		logger.Error(context.Background(), "service stopped with error", logger.F("err", err))
		os.Exit(1)
	}
}
