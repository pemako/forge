# mk

An internal Go component library for building production-grade HTTP and gRPC services. `mk` provides a cohesive set of packages — config, logger, tracing, metrics, database, cache, servers, and middleware — all wired together through a common interface layer and a lifecycle-managing `app` runner.

## Getting Started

**Task-oriented recipes**: See [`docs/getting-started.md`](./docs/getting-started.md) for four scenario-based guides:
- Minimal HTTP service (what's the least you need to know)
- HTTP + database + Redis (recommended setup)
- Pure worker service (cron / stream consumer, no HTTP)
- What to ignore until you need it

**Quick Start** (5 minutes): See [`example/quickstart`](./example/quickstart) for a minimal HTTP service with graceful shutdown.

**Full Example**: See [`example/full`](./example/full) for database, cache, tracing, metrics, stream, cron, auth, storage, notifications, and pprof.

## Design Philosophy

`mk` is **instance-first**: prefer creating explicit instances over package-level singletons.

```go
// Preferred: explicit instance
mgr, err := cache.New(cache.WithMemory("default"), cache.WithRedis("redis", "redis:6379"))
cache.Register("default", mgr)                    // make globally accessible if needed
c := mgr.Client("default")                        // instance access

// Convenience: package-level accessor (delegates to Get("default"))
c := cache.Client("default")
```

Registry packages (`database`, `cache`, `httpclient`, `grpcclient`, `storage`, `idgen`) follow a consistent contract:
`Register` errors on duplicate, `Replace` closes the old resource, `Get` returns nil if not found.

See [ARCHITECTURE.md](./ARCHITECTURE.md) for the full design guide.

## Package Tiers

`mk` organizes its packages into four tiers with different stability contracts:

| Tier | Packages | Stability |
|------|----------|-----------|
| **Core Stable** | `core`, `errors`, `app`, `config`, `logger` | Interfaces and behavior are frozen. Breaking changes require a major version bump. |
| **Infrastructure Integrations** | `server/*`, `middleware/*`, `database`, `cache`, `tracing`, `metric`, `httpclient`, `grpcclient`, `lock`, `stream`, `cron`, `storage` | Stable APIs; additive changes only. External dependencies (Redis, OTLP, gRPC) may constrain upgrades. |
| **Opinionated Extensions** | `auth`, `validate`, `pagination`, `ratelimit`, `notify`, `idgen`, `circuitbreaker`, `pprof` | Usable in production but opinionated. Teams may prefer their own implementations. Minor breaking changes are possible between minor versions. |
| **Developer Utilities** | `testutil`, `event` | Convenience helpers with no cross-service API surface. `event` is in-process only; use `stream` for cross-instance coordination. |

> **Practical guidance:** Start with Core Stable packages. Add Infrastructure Integrations for external connections. Use Opinionated Extensions only if they fit your conventions — they are easy to replace.

## Features

### Core Stable

| Package  | Description                                                                                                    |
| -------- | -------------------------------------------------------------------------------------------------------------- |
| `core`   | Shared interfaces: `Logger`, `Config`, `Server`, `Provider`, `HealthChecker`                                   |
| `errors` | Structured `CodedError` with HTTP code, machine-readable reason, and cause chaining                            |
| `app`    | Lifecycle runner: Lifecycle.Init → BeforeStart → concurrent servers → Stop → Lifecycle.Shutdown → Closer.Close → AfterStop; `WithHealthCheck` / `HealthCheckers()` |
| `config` | Viper-backed config loader with file, env-var, and hot-reload support                                          |
| `logger` | Zap-backed structured logger with trace injection, field masking, and level-split file output                  |

### Infrastructure Integrations

| Package           | Description                                                                                                                |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `server/http`     | Gin HTTP server; default middleware stack pre-mounted; `/healthz`, `/readyz`, `/health` auto-registered; `WithHealthChecks` / `AddReadyCheck` for /readyz integration |
| `server/grpc`     | gRPC server with interceptor chain, health check service, and optional TLS/mTLS                                            |
| `middleware/http` | RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler, CORS, RateLimit, IPFilter, BodyLimit, Timeout                 |
| `middleware/grpc` | UnaryRequestID, UnaryLogger, UnaryMetrics, UnaryRecovery, UnaryErrorHandler, UnaryRateLimit, UnaryIPFilter, UnaryTimeout   |
| `tracing`         | OpenTelemetry OTLP trace provider; implements `app.Lifecycle`; no-op when endpoint is unset                                |
| `metric`          | OTel metrics with Prometheus and OTLP exporters; implements `app.Lifecycle`; automatic HTTP/gRPC instrumentation           |
| `database`        | GORM connection manager for MySQL, Postgres, SQLite; SQL tracing; goose migrations; `*Manager` implements `app.Migrator`; `CloseAll() error`; `Manager.HealthChecker(connName)` |
| `cache`           | `Cache` interface; `Manager` holds named backends (memory/Redis); `Manager` implements `app.Closer`; `Manager.HealthChecker(backendName)` |
| `httpclient`      | Outbound HTTP with OTel tracing, exponential-backoff retry, and named registry; `Register(name, *Client)` / `Replace` / `CloseAll() error` |
| `grpcclient`      | Outbound gRPC with OTel tracing, per-call retry, named registry; `Register(name, *ClientConn)` / `Replace`; `Registry` implements `app.Closer` |
| `lock`            | Redis distributed lock; atomic Lua acquire/release; optional auto-renew and wait-with-timeout                              |
| `stream`          | Redis Streams pub/sub `Bus`; work-queue (`WithGroup`) and broadcast (`WithBroadcast`) modes                                |
| `cron`            | Cron scheduler (`core.Server`); per-job retry, timeout, skip-if-running, sub-second tickers                                |
| `storage`         | Unified object storage interface for AWS S3, Alibaba OSS, and Tencent COS; `Register(name, Storage)` / `Replace` / `CloseAll() error` |

### Opinionated Extensions

| Package          | Description                                                                                                    |
| ---------------- | -------------------------------------------------------------------------------------------------------------- |
| `auth`           | JWT signing/verification; `Required`/`Optional` gin middleware; typed claims via generics                      |
| `validate`       | go-playground/validator binding; structured 400 errors with field-level metadata                               |
| `pagination`     | Page-number `Params` and typed `Result[T]` envelope for list endpoints                                         |
| `ratelimit`      | `Limiter` interface; in-process token-bucket or Redis fixed-window; backs HTTP/gRPC middleware                 |
| `circuitbreaker` | Circuit breaker (Closed → Open → HalfOpen); `Execute(ctx, fn)`                                                 |
| `notify`         | Multi-channel notifications: Email (SMTP), SMS (Aliyun), DingTalk, WeCom, Feishu                               |
| `idgen`          | Snowflake ID generator + ULID; `New()` → `Register("name", gen)` → `Get("name").Next()`; `CloseAll() error` (no-op, clears registry) |
| `pprof`          | pprof endpoints; standalone `core.Server` or mounted on an existing gin.Engine                                 |

### Developer Utilities

| Package    | Description                                                                                      |
| ---------- | ------------------------------------------------------------------------------------------------ |
| `event`    | In-process pub/sub `Bus`; synchronous default, optional async; no package-level state            |
| `testutil` | Test helpers: `FreePort`, `NewConfig` (stub `core.Config`), `NewLogger` (writes to `t.Log`)      |

## Installation

```bash
go get github.com/pemako/mk
```

Requires Go 1.26 or later.

## Quick Start

```go
package main

import (
  "context"
  "os"

  "github.com/pemako/mk/app"
  "github.com/pemako/mk/config"
  "github.com/pemako/mk/logger"
  "github.com/pemako/mk/metric"
  grpcserver "github.com/pemako/mk/server/grpc"
  httpserver "github.com/pemako/mk/server/http"
  "github.com/pemako/mk/tracing"
)

func main() {
  cfg, _ := config.Load(config.WithFile("config.yaml"), config.WithEnv("APP"))
  log, _ := logger.New(logger.WithConfig(cfg))
  logger.SetDefault(log)

  // tracing.Provider and metric.Provider implement app.Lifecycle:
  // Init() is called before servers start, Shutdown() after they stop.
  tracingProvider, _ := tracing.New(tracing.WithConfig(cfg), tracing.WithLogger(log))
  metricProvider, _ := metric.New(metric.WithConfig(cfg))

  httpSrv, _ := httpserver.New(httpserver.WithConfig(cfg), httpserver.WithLogger(log))
  grpcSrv, _ := grpcserver.New(grpcserver.WithConfig(cfg), grpcserver.WithLogger(log))

  // Register routes on httpSrv.Engine() and gRPC services on grpcSrv.Server().
  // httpserver.New already mounts: RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler.
  // Do NOT call r.Use(mw.ErrorHandler()) again — it would cause duplicate responses.

  if err := app.New(
    app.WithName(cfg.GetString("app.name")),
    app.WithServer(httpSrv, grpcSrv),
    app.WithLogger(log),
    app.WithLifecycle(tracingProvider, metricProvider),
    app.WithBeforeStart(func(ctx context.Context) error {
      httpSrv.SetReady(true)
      return nil
    }),
  ).Run(); err != nil {
    logger.Error(context.Background(), "fatal", logger.F("err", err))
    os.Exit(1)
  }
}
```

Place a `config.yaml` next to the binary and run. The app handles `SIGTERM`/`SIGINT` for graceful shutdown.

## Complete config.yaml Reference

```yaml
# Application identity
app:
  name: my-service # propagated to OTel resource, /healthz response
  version: v1.0.0 # echoed in /healthz and /readyz responses
  env: development # "production" disables gRPC server reflection

# HTTP server
server:
  http:
  addr: :8080 # listen address (default :8080)
  timeout: 30s # read and write timeout (default 30s)
  grpc:
  addr: :9090 # listen address (default :9090)

# Structured logger
logger:
  level: info # debug | info | warn | error (live-reloadable)
  output: stdout # stdout | file | both
  filename: /var/log/app # base path; level-split files appended as .info.log etc.
  max_size: 100 # MB before rotation (default 100)
  max_age: 7 # days to retain rotated files (default 7)
  max_backups: 10 # max number of rotated files to keep (default 10)
  mask_fields: # field names partially redacted in logs
  - password
  - token

# Distributed tracing (OpenTelemetry OTLP)
tracing:
  endpoint: '' # e.g. http://jaeger:4318; leave empty to disable
  sampler: 1.0 # sampling ratio: 1.0 = 100%

# Metrics
metric:
  prometheus_addr: :9091 # expose /metrics; omit to disable Prometheus
  otlp_endpoint: '' # e.g. http://collector:4318; omit to disable OTLP metrics

# Database connections
database:
  default:
  driver: mysql # mysql | postgres | sqlite
  dsn: 'user:pass@tcp(localhost:3306)/mydb?parseTime=true'
  max_open_conns: 100
  max_idle_conns: 10
  conn_max_lifetime: 1h
  trace_all_sql: false # emit OTel spans for every SQL statement
  slow_threshold: 200ms # log warning for queries exceeding this

# Cache — in-memory by default; Redis is optional
cache:
  default:
  backend: memory
  redis:
  backend: redis
  addr: 'redis:6379'
  password: ''
  db: 0
```

Environment variables override any YAML key. With `WithEnv("APP")`, the variable `APP_SERVER_HTTP_ADDR` overrides `server.http.addr`.

## Package Reference

### `core`

Shared interfaces that prevent import cycles across packages.

| Interface       | Description                                                           |
| --------------- | --------------------------------------------------------------------- |
| `Logger`        | `Debug/Info/Warn/Error(ctx, msg, ...Field)` + `With(...Field) Logger` |
| `Config`        | `Get*`, `Scan`, `Watch`                                               |
| `Server`        | `Start(ctx) error`, `Stop(ctx) error`                                 |
| `Provider`      | `Tracer(name) trace.Tracer`, `Shutdown(ctx) error`                    |
| `HealthChecker` | `Name() string`, `Check(ctx context.Context) error`                   |

`core.Field` is `{Key string, Value any}`. Use `logger.F(key, value)` as shorthand.

---

### `config`

Viper-backed configuration loader.

```go
cfg, _ := config.Load(
  config.WithFile("config.yaml"),
  config.WithEnv("APP"),    // APP_SERVER_HTTP_ADDR → server.http.addr
  config.WithWatch(),       // live-reload on file change
)
cfg.GetString("app.name")
cfg.Scan("database.default", &dbCfg)
cfg.Watch("logger.level", func(v any) { /* react to change */ })
```

---

### `logger`

Zap-backed structured JSON logger. Auto-injects `trace_id`/`span_id` from an active OTel span.

```go
log, _ := logger.New(logger.WithConfig(cfg), logger.WithLogger(zap.New(...)))
logger.SetDefault(log) // optional bootstrap convenience

log.Info(ctx, "user created", logger.F("user_id", 42))
log.With(logger.F("component", "auth")).Warn(ctx, "token expired")
```

When `output: file`, writes per-level files: `.debug.log`, `.info.log`, `.warn.log`, `.error.log`.

---

### `tracing`

OpenTelemetry OTLP trace provider. Implements `core.Provider` and `app.Lifecycle`.

```go
// New() does all setup; a no-op provider is returned when no exporter is configured.
p, _ := tracing.New(
  tracing.WithConfig(cfg),
  tracing.WithLogger(log),      // log exporter errors
  tracing.WithOTLP("http://jaeger:4318"),
  tracing.WithSampler(0.1),
  tracing.WithTLS(),            // enable TLS for the OTLP exporter
)
app.WithLifecycle(p)              // Init() no-op; Shutdown() flushes spans
```

---

### `metric`

OTel metrics with Prometheus and OTLP exporters. Implements `app.Lifecycle`.

```go
p, _ := metric.New(metric.WithConfig(cfg))
app.WithLifecycle(p)   // Init() starts exporters and wires middleware hooks; Shutdown() flushes

// Business helpers (delegate to default provider — safe to call after Init)
metric.Count(ctx, "orders.created", 1, metric.Label("region", "us"))
metric.Gauge(ctx, "queue.depth", 42)
stop := metric.StartTimer(ctx, "db.query.duration")
defer stop()

// Multi-provider / test isolation: use instance directly
p.Init(ctx)
meter := p.Meter()
```

HTTP and gRPC middleware metrics are wired automatically when `p.Init()` is called. Wire functions are idempotent (`sync.Once`) — calling `Init()` on multiple providers is safe.

---

### `app`

Lifecycle manager. Coordinates component initialization and graceful shutdown.

```go
app.New(
  app.WithName("my-service"),
  app.WithVersion("v1.0.0"),
  app.WithLogger(log),
  app.WithServer(httpSrv, grpcSrv, cronScheduler, streamWorker),
  app.WithMigration(dbManager),       // Up() called before Lifecycle.Init
  app.WithLifecycle(tracingProvider, metricProvider), // Init before servers, Shutdown after (reverse)
  app.WithCloser(cacheManager, grpcclient.NewRegistry()), // Close after Lifecycle.Shutdown
  app.WithHealthCheck(                // register health checkers for centralized access
    dbMgr.HealthChecker("default"),
    cacheMgr.HealthChecker("redis"),
  ),
  app.WithBeforeStart(func(ctx) error { httpSrv.SetReady(true); return nil }),
  app.WithAfterStop(func(ctx) error { return nil }),
  app.WithShutdownTimeout(30 * time.Second),
).Run()
```

**Run() lifecycle order:**

| Phase | Action                                                                            |
| ----- | --------------------------------------------------------------------------------- |
| 1     | `Migrator.Up()`                                                                   |
| 2     | `Lifecycle.Init()` — sequential; failure reverses already-init'd in reverse order |
| 3     | `BeforeStart` hooks — sequential; first error aborts                              |
| 4     | `Server.Start()` — all servers concurrent                                         |
| 5     | Block on SIGTERM/SIGINT or server error                                           |
| 6     | `Server.Stop()` — concurrent; errors logged                                       |
| 7     | `Lifecycle.Shutdown()` — reverse registration order                               |
| 8     | `Closer.Close()` — registration order                                             |
| 9     | `AfterStop` hooks                                                                 |

---

### `errors`

```go
// Define package-level sentinels
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")

// Wrap with context
return errors.Wrap(ErrUserNotFound, "get user %d: %w", id, dbErr)

// Check
if errors.Is(err, ErrUserNotFound) { ... }
var ce *errors.CodedError
if errors.As(err, &ce) { fmt.Println(ce.Code, ce.Reason) }
```

`ErrorHandler` middleware (HTTP/gRPC) auto-converts `CodedError` to structured responses. Non-`CodedError` returns a generic 500.

Builtin sentinels: `ErrBadRequest` (400), `ErrUnauthorized` (401), `ErrForbidden` (403), `ErrNotFound` (404), `ErrConflict` (409), `ErrInternal` (500).

---

### `server/http`

Gin-based HTTP server. `New()` mounts the full middleware stack automatically.

**Default middleware stack (in order):** RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler

> ⚠️ Do NOT call `r.Use(mw.ErrorHandler())` after `httpserver.New()`. The middleware is already in the stack; adding it again causes duplicate JSON responses.

```go
srv, _ := httpserver.New(
  httpserver.WithConfig(cfg),
  httpserver.WithLogger(log),
  httpserver.WithAddr(":8080"),          // or WithListener(ln) for :0 random port
  httpserver.WithHealthChecks(           // probe external deps in /readyz
    dbMgr.HealthChecker("default"),    // SELECT 1
    cacheMgr.HealthChecker("redis"),   // PING
  ),
  httpserver.WithReadyCheck("db", func(ctx context.Context) error {
    return database.DB("default").WithContext(ctx).Exec("SELECT 1").Error
  }),
)
r := srv.Engine()                          // access underlying *gin.Engine for route registration
r.GET("/users", handler.ListUsers)

srv.SetReady(true)                         // call in app.WithBeforeStart; affects /readyz

// Add more ready checks after construction (e.g. in BeforeStart)
srv.AddReadyCheck("cache", cacheMgr.HealthChecker("redis").Check)
```

Auto-registered endpoints: `/healthz`, `/readyz`, `/health`

---

### `server/grpc`

```go
grpcSrv, _ := grpcserver.New(
  grpcserver.WithConfig(cfg),
  grpcserver.WithLogger(log),
  grpcserver.WithAddr(":9090"),
  grpcserver.WithTLS("cert.pem", "key.pem"),
)
pb.RegisterMyServiceServer(grpcSrv.Server(), &myServiceImpl{})
```

gRPC health check and reflection are registered automatically. Reflection is disabled when `app.env: production`.

---

### `middleware/http`

Pre-mounted by `server/http`. Available for standalone use on a plain `*gin.Engine`:

```go
r.Use(
  mw.RequestID(),
  mw.Tracing("my-service"),
  mw.Logger(log),
  mw.Metrics(),
  mw.Recovery(),
  mw.ErrorHandler(),
  // Optional:
  mw.CORS(),
  mw.Timeout(10 * time.Second),
  mw.BodyLimit(4 * 1024 * 1024),        // 4 MB
  mw.RateLimit(ratelimit.NewTokenBucket(100, 10)),
  mw.IPFilter(mw.WithCIDRWhitelist("10.0.0.0/8")),
)
```

---

### `database`

```go
// Create manager (no global side effects)
mgr, _ := database.New(
  database.WithConfig(cfg),
  database.WithLogger(log),
  database.WithMigrationsDir("migrations"),
)
// Register for global access (optional)
database.Register("default", mgr)

// Instance access
db := mgr.DB("default")      // *gorm.DB for the named connection

// Global convenience (delegates to Get("default").DB(connName))
db := database.DB("default")

// Migrations — mgr implements app.Migrator
app.WithMigration(mgr)

// Health check for /readyz integration
hc := mgr.HealthChecker("default")   // probes SELECT 1 on the named connection
httpSrv.AddReadyCheck(hc.Name(), hc.Check)
```

---

### `cache`

```go
// Create Manager (no global side effects)
mgr, _ := cache.New(
  cache.WithMemory("default"),
  cache.WithRedis("redis", "redis:6379"),
)
// Register for global access
cache.Register("default", mgr)

// Instance access
c := mgr.Client("default")
rdb := mgr.RedisClient("redis")    // *redis.Client for lock/stream

// Global convenience (delegates to Get("default"))
c := cache.Client("default")
rdb := cache.RedisClient("redis")

// mgr implements app.Closer — pass to app.WithCloser for automatic cleanup
app.WithCloser(mgr)

c.Set(ctx, "key", "value", time.Hour)
v, _ := c.Get(ctx, "key")
c.Del(ctx, "key")

// Health check for /readyz integration
hc := mgr.HealthChecker("redis")   // probes PING; memory backend is always healthy
httpSrv.AddReadyCheck(hc.Name(), hc.Check)
```

---

### `httpclient`

```go
// Create instance first, then register
client, err := httpclient.New(httpclient.WithBaseURL("https://api.payments.io"))
httpclient.Register("payments", client)
// Or: explicitly check for duplicate
if err := httpclient.Register("payments", client); errors.Is(err, httpclient.ErrAlreadyRegistered) { ... }

// Replace closes the old client and stores the new one
httpclient.Replace("payments", newClient)

client := httpclient.Get("payments")
resp, body, err := client.GetJSON(ctx, "/v1/charges/123")
resp, body, err := client.PostJSON(ctx, "/v1/charges", payload)

// Release all clients on shutdown
httpclient.CloseAll()
```

---

### `grpcclient`

```go
// Dial first, then register
conn, err := grpcclient.Dial("inventory-svc:9090",
  grpcclient.WithTLS(),
  grpcclient.WithRetry(3),
)
grpcclient.Register("inventory", conn)
conn := grpcclient.Get("inventory")
svc := pb.NewInventoryClient(conn)

// Implements app.Closer for automatic CloseAll on shutdown
app.WithCloser(grpcclient.NewRegistry())
```

---

### `lock`

```go
locker := lock.NewWithClient(cache.RedisClient("redis"), lock.WithLogger(log))

// Try once
if err := locker.Lock(ctx, "job:process", 30*time.Second); err != nil { ... }
defer locker.Unlock(ctx, "job:process")

// Wait up to 5s for the lock
if err := locker.LockWait(ctx, "job:process", 30*time.Second, 5*time.Second); err != nil { ... }
```

---

### `cron`

```go
scheduler, _ := cron.New(cron.WithLogger(log), cron.WithLocation(time.UTC))

scheduler.Add("cleanup", "@every 1h",
  func(ctx context.Context) error { return cleanupOldRecords(ctx) },
  cron.WithTimeout(5*time.Minute),
  cron.WithRetry(3),
  cron.WithSkipIfRunning(),
  cron.WithDistributedLock(locker), // only one cluster node runs each tick
)
app.WithServer(scheduler)            // scheduler implements core.Server
```

---

### `event`

```go
// Always use instance — no package-level state
bus := event.New()                       // synchronous
bus := event.New(event.WithAsync())      // async (errors logged, not returned)

bus.Subscribe("user.registered", func(ctx context.Context, payload any) error {
  e := payload.(UserRegisteredEvent)
  return mailer.SendWelcome(ctx, e.Email)
})

err := bus.Publish(ctx, "user.registered", UserRegisteredEvent{Email: "a@b.com"})
bus.Unsubscribe("user.registered")
```

---

### `stream`

```go
// New returns (*Bus, error) — nil client is rejected
bus, _ := stream.New(
  cache.RedisClient("redis"),
  stream.WithLogger(log),
  stream.WithMaxLen(10_000),
)

// Register subscriptions before starting Worker
bus.Subscribe("order.created",
  stream.WithGroup("order-service"),     // work-queue: one instance processes each message
  stream.WithHandler(func(ctx context.Context, msg stream.Message) error {
    var order Order
    msg.Decode(&order)
    return processOrder(ctx, order)
  }),
  stream.WithRetries(3),
)
bus.Subscribe("audit.event",
  stream.WithBroadcast(),                // every instance receives each message
  stream.WithHandler(auditHandler),
)

app.WithServer(bus.Worker())              // Worker implements core.Server
bus.Publish(ctx, "order.created", order)
```

---

### `auth`

```go
type Claims struct {
  jwt.RegisteredClaims
  UserID int64  `json:"user_id"`
  Role   string `json:"role"`
}

signer, _ := auth.NewSigner[Claims](auth.WithSecret("my-secret"), auth.WithTTL(24*time.Hour))
token, _ := signer.Sign(Claims{UserID: 42, Role: "admin"})

r.POST("/login", func(c *gin.Context) {
  c.JSON(200, gin.H{"token": token})
})
r.GET("/profile", auth.Required[Claims](signer), func(c *gin.Context) {
  claims := auth.FromContext[Claims](c)
  c.JSON(200, claims)
})
```

---

### `validate`

```go
type CreateUserReq struct {
  Name  string `json:"name"  validate:"required,min=2,max=64"`
  Email string `json:"email" validate:"required,email"`
}

r.POST("/users", func(c *gin.Context) {
  var req CreateUserReq
  if err := validate.BindJSON(c, &req); err != nil {
    return // ErrorHandler middleware writes 400 with field-level details
  }
  // req is valid
})
```

---

### `pagination`

```go
r.GET("/users", func(c *gin.Context) {
  p := pagination.FromQuery(c)          // ?page=2&page_size=20
  offset := pagination.Offset(p)       // (page-1) * page_size
  var users []User
  db.Limit(p.PageSize).Offset(offset).Find(&users)
  total := db.Model(&User{}).Count()
  c.JSON(200, pagination.New(users, total, p))
})
```

---

### `idgen`

```go
// Explicit worker ID (required in production — auto-derived ID is unstable in containers)
gen, _ := idgen.New(idgen.WithWorkerID(cfg.GetInt64("idgen.worker_id")))
idgen.Register("default", gen)

id := idgen.Get("default").Next()         // int64 snowflake ID
idStr := idgen.Get("default").NextString() // decimal string

ulid := idgen.ULID()                       // time-sortable string, no registration needed
```

> ⚠️ In production, always set `idgen.worker_id` in config. Without it, the worker ID is derived from the host's primary IP, which is unstable in containerized environments and may cause ID collisions across instances.

---

### `circuitbreaker`

```go
cb := circuitbreaker.New(
  circuitbreaker.WithThreshold(5),           // trips after 5 consecutive failures
  circuitbreaker.WithTimeout(30*time.Second), // half-open probe after 30s
  circuitbreaker.WithLogger(log),
)

err := cb.Execute(ctx, func(ctx context.Context) error {
  return externalService.Call(ctx, payload)
})
// err == circuitbreaker.ErrOpen when the breaker is tripped
```

---

### `storage`

```go
// Create instance first, then register
s, err := storage.New(storage.WithS3(storage.S3Config{
  Bucket: "my-avatars", Region: "us-east-1",
  AccessKey: cfg.GetString("storage.access_key"),
  SecretKey: cfg.GetString("storage.secret_key"),
}))
storage.Register("avatars", s)

s := storage.Get("avatars")
err := s.Put(ctx, "user/42/avatar.png", reader, size, "image/png")
url, _ := s.GetURL(ctx, "user/42/avatar.png", time.Hour)
err = s.Delete(ctx, "user/42/avatar.png")
```

---

### `notify`

```go
n := notify.New(notify.WithConfig(cfg))

// Email
n.SendEmail(ctx, notify.Email{
  To:      []string{"alice@example.com"},
  Subject: "Welcome",
  Body:    "<h1>Hello</h1>",
})

// DingTalk webhook
n.SendDingTalk(ctx, notify.DingTalk{Content: "Deploy succeeded"})

// WeCom webhook
n.SendWeCom(ctx, notify.WeCom{Content: "Deploy succeeded"})

// Feishu webhook
n.SendFeishu(ctx, notify.Feishu{Content: "Deploy succeeded"})
```

---

### `pprof`

```go
// Option A: standalone server on a dedicated port
pprofSrv := pprof.New(pprof.WithAddr(":6060"), pprof.WithLogger(log))
app.WithServer(pprofSrv)   // implements core.Server

// Option B: mount on existing gin.Engine
pprof.Register(r)          // adds /debug/pprof/* routes
```

---

### `ratelimit`

```go
// In-process token bucket
lim := ratelimit.NewTokenBucket(100, 10) // 100 req/s, burst 10

// Distributed Redis fixed-window
lim := ratelimit.NewRedisFixedWindow(cache.RedisClient("redis"), "api", 1000, time.Minute)

// Use as middleware
r.Use(mw.RateLimit(lim))
```

---

### `testutil`

```go
port := testutil.FreePort()                            // random available TCP port
cfg  := testutil.NewConfig(map[string]any{             // stub core.Config
  "server.http.addr": ":8081",
})
log  := testutil.NewLogger(t)                          // writes to t.Log
```

## Middleware Reference

### HTTP Error Handling Pattern

```go
// ✅ Correct — use c.Error(); ErrorHandler writes the response
r.GET("/users/:id", func(c *gin.Context) {
  user, err := svc.GetUser(ctx, id)
  if err != nil {
    c.Error(err)   // err should be a *errors.CodedError
    return
  }
  c.JSON(200, user)
})

// ❌ Wrong — bypasses ErrorHandler, no structured response
r.GET("/users/:id", func(c *gin.Context) {
  c.JSON(500, gin.H{"error": err.Error()})
})
```

### HTTP Middleware Stack Warning

`httpserver.New()` mounts the full default middleware stack. Never add the same middleware again:

```go
srv, _ := httpserver.New(...)
r := srv.Engine()
// ❌ Wrong — ErrorHandler already in the stack; this causes duplicate JSON responses
r.Use(mw.ErrorHandler())
// ✅ Correct — just register routes
r.GET("/hello", handler)
```

## Error Handling Convention

```go
// Package-level sentinels (exported vars)
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")
var ErrDuplicate    = errors.Define(409, "USER_DUPLICATE", "user already exists")

// Service layer
func (s *UserService) Get(ctx context.Context, id int64) (*User, error) {
  u, err := s.repo.FindByID(ctx, id)
  if err != nil {
    return nil, errors.Wrap(ErrUserNotFound, "get user %d: %w", id, err)
  }
  return u, nil
}

// Handler
func (h *UserHandler) Get(c *gin.Context) {
  id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
  user, err := h.svc.Get(c.Request.Context(), id)
  if err != nil {
    c.Error(err)   // ErrorHandler middleware converts to JSON
    return
  }
  c.JSON(200, user)
}
```

## Project Structure

```
mk/
├── core/              # shared interfaces — zero deps on other mk packages
├── errors/            # CodedError — zero deps on other mk packages
├── app/               # lifecycle manager
├── config/            # Viper config loader
├── logger/            # Zap structured logger
├── tracing/           # OTel trace provider
├── metric/            # OTel metrics
├── server/
│   ├── http/          # Gin HTTP server
│   └── grpc/          # gRPC server
├── middleware/
│   ├── http/          # HTTP middleware
│   └── grpc/          # gRPC unary interceptors
├── database/          # GORM wrapper
├── cache/             # Cache manager (memory + Redis)
├── httpclient/        # Outbound HTTP client
├── grpcclient/        # Outbound gRPC client
├── lock/              # Redis distributed lock
├── cron/              # Cron scheduler
├── event/             # In-process pub/sub
├── stream/            # Redis Streams pub/sub
├── auth/              # JWT authentication
├── validate/          # Request validation
├── pagination/        # Pagination helpers
├── circuitbreaker/    # Circuit breaker
├── storage/           # Object storage
├── notify/            # Notification dispatch
├── pprof/             # Profiling endpoints
├── idgen/             # ID generation
├── ratelimit/         # Rate limiting
├── testutil/          # Test helpers
├── example/
│   ├── quickstart/    # Minimal HTTP service (start here)
│   └── full/          # Full-featured demo service
├── ARCHITECTURE.md    # Design guide
├── CLAUDE.md          # Claude Code guidance
└── Makefile
```
