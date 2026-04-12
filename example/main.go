package main

import (
	"context"
	"os"
	"time"

	"github.com/pemako/mk/app"
	"github.com/pemako/mk/auth"
	"github.com/pemako/mk/cache"
	"github.com/pemako/mk/config"
	"github.com/pemako/mk/cron"
	"github.com/pemako/mk/database"
	"github.com/pemako/mk/grpcclient"
	"github.com/pemako/mk/httpclient"
	"github.com/pemako/mk/logger"
	"github.com/pemako/mk/metric"
	grpcserver "github.com/pemako/mk/server/grpc"
	httpserver "github.com/pemako/mk/server/http"
	"github.com/pemako/mk/tracing"

	httphandler "github.com/pemako/mk/example/internal/handler/http"
	"github.com/pemako/mk/example/internal/model"
	"github.com/pemako/mk/example/internal/service"
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

	// HTTP client — register named client for external API calls
	httpclient.Register("jsonplaceholder",
		httpclient.WithBaseURL("https://jsonplaceholder.typicode.com"),
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithRetry(2),
	)

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
	r := httpSrv.Engine()

	// Public routes
	userHandler.RegisterRoutes(r)

	// Protected routes — require a valid JWT
	protected := r.Group("/api/v1/protected")
	protected.Use(auth.Required(
		auth.WithSecret(cfg.GetString("auth.secret")),
		auth.WithClaims(func() auth.Claims { return &model.UserClaims{} }),
	))
	protected.GET("/me", userHandler.GetMe)

	// Cron scheduler — register background jobs
	scheduler := cron.New(cron.WithLogger(log))
	if err := scheduler.Add("cleanup-expired-sessions", "@every 1m",
		func(ctx context.Context) error {
			log.Info(ctx, "running scheduled cleanup")
			// In production: delete expired sessions, purge old records, etc.
			return nil
		},
		cron.WithTimeout(30*time.Second),
		cron.WithSkipIfRunning(),
	); err != nil {
		panic("cron: " + err.Error())
	}

	// Run
	if err := app.New(
		app.WithName(cfg.GetString("app.name")),
		app.WithVersion(cfg.GetString("app.version")),
		app.WithServer(httpSrv, grpcSrv, scheduler),
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
			grpcclient.CloseAll()
			metric.Shutdown()
			return provider.Shutdown(ctx)
		}),
	).Run(); err != nil {
		logger.Error(context.Background(), "service stopped with error", logger.F("err", err))
		os.Exit(1)
	}
}
