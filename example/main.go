package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pemako/mk/app"
	"github.com/pemako/mk/auth"
	"github.com/pemako/mk/cache"
	"github.com/pemako/mk/circuitbreaker"
	"github.com/pemako/mk/config"
	"github.com/pemako/mk/core"
	"github.com/pemako/mk/cron"
	"github.com/pemako/mk/database"
	"github.com/pemako/mk/event"
	httphandler "github.com/pemako/mk/example/internal/handler/http"
	"github.com/pemako/mk/example/internal/model"
	"github.com/pemako/mk/example/internal/service"
	"github.com/pemako/mk/httpclient"
	"github.com/pemako/mk/lock"
	"github.com/pemako/mk/logger"
	"github.com/pemako/mk/metric"
	"github.com/pemako/mk/notify"
	grpcserver "github.com/pemako/mk/server/grpc"
	httpserver "github.com/pemako/mk/server/http"
	"github.com/pemako/mk/storage"
	"github.com/pemako/mk/stream"
	"github.com/pemako/mk/tracing"

	"github.com/pemako/mk/pprof"
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
	provider, err := tracing.New(tracing.WithConfig(cfg), tracing.WithLogger(log))
	if err != nil {
		log.Error(context.Background(), "tracing init failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── Metrics ───────────────────────────────────────────────────────────
	if err := metric.Init(metric.WithConfig(cfg)); err != nil {
		log.Error(context.Background(), "metric init failed", logger.F("err", err))
		os.Exit(1)
	}

	// ── Cache (memory default + Redis shared backend) ─────────────────────
	if err := cache.Init(cache.WithConfig(cfg), cache.WithLogger(log)); err != nil {
		log.Error(context.Background(), "cache init failed", logger.F("err", err))
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

	// ── Distributed Lock (reads lock.addr from config) ────────────────────
	locker := lock.NewFromConfig(cfg, lock.WithLogger(log))

	// ── Circuit Breaker ───────────────────────────────────────────────────
	cb := circuitbreaker.New("jsonplaceholder",
		circuitbreaker.WithConfig(cfg),
	)

	// ── HTTP Client ───────────────────────────────────────────────────────
	httpclient.Register("jsonplaceholder",
		httpclient.WithBaseURL("https://jsonplaceholder.typicode.com"),
		httpclient.WithTimeout(10*time.Second),
		httpclient.WithRetry(2),
	)

	// ── Storage (S3/OSS/COS — only when credentials are set) ─────────────
	if cfg.GetString("storage.access_key") != "" {
		if err := storage.Register("default", storage.WithConfig(cfg)); err != nil {
			log.Warn(context.Background(), "storage init failed", logger.F("err", err))
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
	if rdb != nil {
		stream.Init(rdb, stream.WithLogger(log))

		// Work queue: only one instance processes each order event
		stream.Subscribe("order.created",
			stream.WithGroup("example-service"),
			stream.WithRetries(3),
			stream.WithHandler(func(ctx context.Context, msg stream.Message) error {
				log.Info(ctx, "stream: processing order event", logger.F("id", msg.ID))
				return nil
			}),
		)

		// Broadcast: every instance reloads config on this event
		stream.Subscribe("config.reload",
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
	httpSrv, err := httpserver.New(
		httpserver.WithConfig(cfg),
		httpserver.WithLogger(log),
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
	userHandler := httphandler.NewUserHandler(userSvc)

	r := httpSrv.Engine()

	// Public routes
	userHandler.RegisterRoutes(r)
	r.POST("/api/v1/login", userHandler.Login)

	// Protected routes (JWT required)
	protected := r.Group("/api/v1")
	protected.Use(auth.Required(
		auth.WithConfig(cfg),
		auth.WithClaims(func() auth.Claims { return &model.UserClaims{} }),
	))
	protected.GET("/me", userHandler.GetMe)

	// Demo: publish a stream event endpoint
	r.POST("/api/v1/demo/stream", func(c *gin.Context) {
		if rdb != nil {
			_ = stream.Publish(c.Request.Context(), "order.created", map[string]any{"demo": true})
		}
		c.JSON(200, gin.H{"ok": true})
	})

	// Demo: notify endpoint
	r.POST("/api/v1/demo/notify", func(c *gin.Context) {
		if notifier != nil {
			_ = notifier.Send(c.Request.Context(), notify.Message{
				Subject:  "Test notification",
				Body:     "Test notification from example service",
				Channels: []notify.Channel{notify.DingTalk},
			})
		}
		c.JSON(200, gin.H{"ok": true})
	})

	// Demo: storage upload endpoint
	r.POST("/api/v1/demo/upload", func(c *gin.Context) {
		s := storage.Get("default")
		if s == nil {
			c.JSON(503, gin.H{"error": "storage not configured"})
			return
		}
		url, err := s.Upload(c.Request.Context(), "demo/test.txt",
			strings.NewReader("hello world"),
			storage.WithContentType("text/plain"),
		)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
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
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"token": token})
	})

	// ── App Lifecycle ──────────────────────────────────────────────────────
	servers := []core.Server{httpSrv, grpcSrv, scheduler}
	if rdb != nil {
		servers = append(servers, stream.Worker(log))
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
		app.WithBeforeStart(func(ctx context.Context) error {
			httpSrv.SetReady(true)
			log.Info(ctx, "service ready",
				logger.F("http", cfg.GetString("server.http.addr")),
				logger.F("grpc", cfg.GetString("server.grpc.addr")),
				logger.F("metrics", cfg.GetString("metric.prometheus_addr")),
			)
			return nil
		}),
		app.WithAfterStop(func(ctx context.Context) error {
			cache.CloseAll() //nolint:errcheck
			if err := metric.Shutdown(ctx); err != nil {
				return err
			}
			return provider.Shutdown(ctx)
		}),
	).Run(); err != nil {
		logger.Error(context.Background(), "service exit", logger.F("err", err))
		os.Exit(1)
	}
}
