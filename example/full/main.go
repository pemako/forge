package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pemako/forge/app"
	"github.com/pemako/forge/auth"
	"github.com/pemako/forge/cache"
	"github.com/pemako/forge/circuitbreaker"
	"github.com/pemako/forge/config"
	"github.com/pemako/forge/core"
	"github.com/pemako/forge/cron"
	"github.com/pemako/forge/database"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/event"
	"github.com/pemako/forge/httpclient"
	"github.com/pemako/forge/lock"
	"github.com/pemako/forge/logger"
	"github.com/pemako/forge/metric"
	"github.com/pemako/forge/notify"
	"github.com/pemako/forge/pprof"
	grpcserver "github.com/pemako/forge/server/grpc"
	httpserver "github.com/pemako/forge/server/http"
	"github.com/pemako/forge/storage"
	"github.com/pemako/forge/stream"
	"github.com/pemako/forge/tracing"
	"go.opentelemetry.io/otel/attribute"

	httphandler "github.com/pemako/forge/example/full/internal/handler/http"
	"github.com/pemako/forge/example/full/internal/model"
	"github.com/pemako/forge/example/full/internal/service"
)

var (
	storageNotConfiguredErr = errors.Define(503, "STORAGE_NOT_CONFIGURED", "storage not configured")
	uploadErr               = errors.Define(500, "UPLOAD_FAILED", "upload failed")
	tokenGenErr             = errors.Define(500, "TOKEN_GEN_FAILED", "token generation failed")
)

func main() {
	// ── Config ────────────────────────────────────────────────────────────
	cfg, err := config.Load(
		config.WithFile("config.yaml"),
		config.WithEnv("APP"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	// ── Logger ────────────────────────────────────────────────────────────
	log, err := logger.New(logger.WithConfig(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: %v\n", err)
		os.Exit(1)
	}
	logger.SetDefault(log)

	// ── Tracing ───────────────────────────────────────────────────────────
	// tracing.Provider implements app.Lifecycle (Init is a no-op; Shutdown
	// flushes pending spans). Pass it to app.WithLifecycle for automatic teardown.
	//
	// WithAttributes appends custom resource attributes that appear on every span.
	// Use this for dimensions that are specific to your deployment and not covered
	// by the standard options (service.name / service.version / deployment.environment).
	tracingProvider, err := tracing.New(
		tracing.WithConfig(cfg),
		tracing.WithLogger(log),
		tracing.WithAttributes(
			attribute.String("region", cfg.GetString("app.region")),
			attribute.String("cluster", cfg.GetString("app.cluster")),
			attribute.Bool("atls.full.collect", true),
		),
	)
	if err != nil {
		log.Error(context.Background(), "tracing init failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── Metrics ───────────────────────────────────────────────────────────
	// metric.Provider implements app.Lifecycle (Init starts exporters; Shutdown
	// flushes them). Pass it to app.WithLifecycle for automatic init and teardown.
	metricProvider, err := metric.New(metric.WithConfig(cfg))
	if err != nil {
		log.Error(context.Background(), "metric init failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── Cache (memory default + Redis shared backend) ─────────────────────
	// cache.Manager implements app.Closer (Close calls CloseAll).
	cacheMgr, err := cache.New(cache.WithConfig(cfg), cache.WithLogger(log))
	if err != nil {
		log.Error(context.Background(), "cache init failed", logger.F("err", err))
		os.Exit(1)
	}
	if err := cache.Register("default", cacheMgr); err != nil {
		log.Error(context.Background(), "cache register failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── Database ──────────────────────────────────────────────────────────
	dbMgr, err := database.New(
		database.WithConfig(cfg),
		database.WithLogger(log),
		database.WithMigrationsDir("migrations"),
	)
	if err != nil {
		log.Error(context.Background(), "database init failed", logger.F("err", err))
		os.Exit(1)
	}
	if err := database.Register("default", dbMgr); err != nil {
		log.Error(context.Background(), "database register failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── Distributed Lock — reuses the Redis client from cache (no new connection)
	// cache.RedisClient("redis") returns the *redis.Client registered under "redis".
	locker := lock.NewWithClient(cache.RedisClient("redis"), lock.WithLogger(log))

	// ── Circuit Breaker ───────────────────────────────────────────────────
	cb := circuitbreaker.New("jsonplaceholder",
		circuitbreaker.WithConfig(cfg),
	)

	// ── HTTP Client ───────────────────────────────────────────────────────
	httpclient.Register("jsonplaceholder", httpclient.New( //nolint:errcheck
		httpclient.WithBaseURL("https://jsonplaceholder.typicode.com"),
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithRetry(2),
	))

	// ── Storage (S3/OSS/COS — only when credentials are set) ─────────────
	if cfg.GetString("storage.access_key") != "" {
		if storageBackend, err := storage.New(storage.WithConfig(cfg)); err != nil {
			log.Warn(context.Background(), "storage init failed", logger.F("err", err))
		} else if err := storage.Register("default", storageBackend); err != nil {
			log.Warn(context.Background(), "storage register failed", logger.F("err", err))
		}
	}

	// ── Notify ────────────────────────────────────────────────────────────
	notifier, err := notify.New(notify.WithConfig(cfg))
	if err != nil {
		log.Warn(context.Background(), "notify init failed", logger.F("err", err))
	}

	// ── Event Bus (in-process) ────────────────────────────────────────────
	bus := event.New(event.WithAsync(), event.WithLogger(log))
	bus.Subscribe("user.created", func(ctx context.Context, payload any) error {
		u, ok := payload.(*model.User)
		if !ok {
			return nil
		}
		log.Info(ctx, "event: user created",
			logger.F("user_id", u.ID),
			logger.F("email", u.Email),
		)
		return nil
	})
	bus.Subscribe("user.created", func(ctx context.Context, payload any) error {
		metric.Count(ctx, "user_created_total", 1)
		return nil
	})

	// ── Stream (cross-instance, Redis Streams) ────────────────────────────
	rdb := cache.RedisClient("redis")
	var streamBus *stream.Bus
	if rdb != nil {
		var streamErr error
		streamBus, streamErr = stream.New(rdb, stream.WithLogger(log))
		if streamErr != nil {
			log.Error(context.Background(), "stream init failed", logger.F("err", streamErr))
			os.Exit(1)
		}

		// Work queue: only one instance processes each order event
		streamBus.Subscribe("order.created",
			stream.WithGroup("full-service"),
			stream.WithRetries(3),
			stream.WithHandler(func(ctx context.Context, msg stream.Message) error {
				log.Info(ctx, "stream: processing order event", logger.F("id", msg.ID))
				return nil
			}),
		)

		// Broadcast: every instance reloads config on this event
		streamBus.Subscribe("config.reload",
			stream.WithBroadcast(),
			stream.WithHandler(func(ctx context.Context, msg stream.Message) error {
				log.Info(ctx, "stream: config reload triggered")
				return nil
			}),
		)
	}

	// ── Cron Scheduler ────────────────────────────────────────────────────
	scheduler := cron.New(cron.WithLogger(log))

	// Cleanup job: distributed lock ensures only one instance runs it
	if err := scheduler.Add("cleanup-expired-sessions", "@every 1m",
		func(ctx context.Context) error {
			log.Info(ctx, "cron: running cleanup")
			metric.Count(ctx, "cron_cleanup_total", 1)
			return nil
		},
		cron.WithTimeout(30*time.Second),
		cron.WithSkipIfRunning(),
		cron.WithDistributedLock(locker),
	); err != nil {
		log.Error(context.Background(), "cron add cleanup failed", logger.F("err", err))
		os.Exit(1)
	}

	// Stats job: all instances run this independently (no lock)
	if err := scheduler.Add("report-stats", "@every 5m",
		func(ctx context.Context) error {
			end := metric.StartTimer(ctx, "cron_stats_duration")
			defer end()
			log.Info(ctx, "cron: reporting stats")
			return nil
		},
		cron.WithTimeout(1*time.Minute),
		cron.WithRetries(2),
		cron.WithRetryDelay(5*time.Second),
	); err != nil {
		log.Error(context.Background(), "cron add report-stats failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── HTTP Server ───────────────────────────────────────────────────────
	// Health checkers are wired directly into the HTTP server via WithHealthChecks.
	// Each checker is registered as a named /readyz sub-check: database:default
	// performs SELECT 1 and cache:default verifies the memory backend is present.
	httpSrv, err := httpserver.New(
		httpserver.WithConfig(cfg),
		httpserver.WithLogger(log),
		httpserver.WithHealthChecks(
			dbMgr.HealthChecker("default"),
			cacheMgr.HealthChecker("default"),
		),
		// Enable Huma: OpenAPI spec + Swagger UI at /docs.
		httpserver.WithOpenAPI("Full Example API", cfg.GetString("app.version"), "/docs"),
	)
	if err != nil {
		log.Error(context.Background(), "http server init failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── gRPC Server ───────────────────────────────────────────────────────
	grpcSrv, err := grpcserver.New(
		grpcserver.WithConfig(cfg),
		grpcserver.WithLogger(log),
	)
	if err != nil {
		log.Error(context.Background(), "grpc server init failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── Service & Handlers ────────────────────────────────────────────────
	userSvc := service.NewUserService(bus, cb, locker)

	// Register all user-domain routes via Huma so they appear in the
	// OpenAPI spec at /docs. Huma runs on top of Gin — the default
	// middleware stack (RequestID, Logger, Metrics, Recovery, ErrorHandler)
	// still applies to every request.
	httphandler.RegisterHumaRoutes(httpSrv.API(), userSvc, "example-jwt-secret-change-in-production")

	r := httpSrv.Engine()

	// Demo: publish a stream event endpoint
	r.POST("/api/v1/demo/stream", func(c *gin.Context) {
		if streamBus != nil {
			_ = streamBus.Publish(c.Request.Context(), "order.created", map[string]any{"demo": true})
		}
		c.JSON(200, gin.H{"ok": true})
	})

	// Demo: notify endpoint
	r.POST("/api/v1/demo/notify", func(c *gin.Context) {
		if notifier != nil {
			_ = notifier.Send(c.Request.Context(), notify.Message{
				Subject:  "Test notification",
				Body:     "Test notification from full example service",
				Channels: []notify.Channel{notify.DingTalk},
			})
		}
		c.JSON(200, gin.H{"ok": true})
	})

	// Demo: storage upload endpoint
	r.POST("/api/v1/demo/upload", func(c *gin.Context) {
		s := storage.Get("default")
		if s == nil {
			c.Error(storageNotConfiguredErr) //nolint:errcheck
			return
		}
		url, err := s.Upload(c.Request.Context(), "demo/test.txt",
			strings.NewReader("hello world"),
			storage.WithContentType("text/plain"),
		)
		if err != nil {
			c.Error(uploadErr) //nolint:errcheck
			return
		}
		c.JSON(200, gin.H{"url": url})
	})

	// Demo: generate JWT token for testing
	r.POST("/api/v1/demo/token", func(c *gin.Context) {
		token, err := auth.Generate(&model.UserClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			},
			UserID: 1,
			Role:   "admin",
		}, auth.WithConfig(cfg))
		if err != nil {
			c.Error(tokenGenErr) //nolint:errcheck
			return
		}
		c.JSON(200, gin.H{"token": token})
	})

	// ── App Lifecycle ──────────────────────────────────────────────────────
	servers := []core.Server{httpSrv, grpcSrv, scheduler}
	if streamBus != nil {
		servers = append(servers, streamBus.Worker())
	}

	// pprof: two modes depending on deployment scenario.
	//
	// Mode 1 — standalone server on localhost:6060 (public-facing services,
	// strict isolation; access via `kubectl port-forward` or SSH tunnel):
	//   servers = append(servers, pprof.New())
	//
	// Mode 2 — mounted on the existing HTTP server (internal-only services,
	// no extra port needed):
	//   pprof.Register(httpSrv.Engine())
	//
	// Enable mutex/block profiling when diagnosing lock contention:
	//   pprof.New(pprof.WithMutexFraction(1), pprof.WithBlockRate(1))
	servers = append(servers, pprof.New())

	if err := app.New(
		app.WithName(cfg.GetString("app.name")),
		app.WithVersion(cfg.GetString("app.version")),
		app.WithServer(servers...),
		app.WithLogger(log),
		app.WithMigration(dbMgr),
		// tracing.Provider and metric.Provider implement app.Lifecycle:
		//   Init() is called before BeforeStart hooks; Shutdown() is called after
		//   servers stop, in reverse registration order.
		app.WithLifecycle(tracingProvider, metricProvider),
		// cache.Manager implements app.Closer: Close() is called after all
		// Lifecycle.Shutdown() calls have completed.
		app.WithCloser(cacheMgr),
		app.WithBeforeStart(func(ctx context.Context) error {
			httpSrv.SetReady(true)
			log.Info(ctx, "service ready",
				logger.F("http", cfg.GetString("server.http.addr")),
				logger.F("grpc", cfg.GetString("server.grpc.addr")),
				logger.F("metrics", cfg.GetString("metric.prometheus_addr")),
			)
			return nil
		}),
	).Run(); err != nil {
		logger.Error(context.Background(), "service exit", logger.F("err", err))
		os.Exit(1)
	}
}
