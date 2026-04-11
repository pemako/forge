# mk

An internal Go component library for building production-grade HTTP and gRPC services. `mk` provides a cohesive set of packages — config, logger, tracing, metrics, database, cache, servers, and middleware — all wired together through a common interface layer and a lifecycle-managing `app` runner.

## Features

| Package | Description |
|---|---|
| `core` | Shared interfaces: `Logger`, `Config`, `Server`, `Provider` |
| `app` | Application lifecycle runner: migrations, server start/stop, signal handling |
| `config` | Viper-backed config loader with file, env-var, and hot-reload support |
| `logger` | Zap-backed structured logger with trace injection, masking, and level-split files |
| `tracing` | OpenTelemetry OTLP trace provider; no-op when endpoint is unset |
| `metric` | OpenTelemetry metrics with Prometheus and OTLP exporters |
| `server/http` | Gin HTTP server with a default middleware stack pre-mounted |
| `server/grpc` | gRPC server with interceptor chain, health check, and optional TLS |
| `database` | GORM connection manager for MySQL, Postgres, and SQLite with goose migrations |
| `cache` | go-redis client manager with slow-command logging |
| `errors` | Structured `CodedError` with HTTP code, reason string, and cause chaining |
| `middleware/http` | RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler, CORS, RateLimit, IPFilter, BodyLimit, Timeout |
| `middleware/grpc` | UnaryRequestID, UnaryLogger, UnaryMetrics, UnaryRecovery, UnaryErrorHandler, UnaryRateLimit, UnaryIPFilter, UnaryTimeout |

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
    cfg, err := config.Load(config.WithFile("config.yaml"), config.WithEnv("APP"))
    if err != nil {
        panic(err)
    }

    log, err := logger.New(logger.WithConfig(cfg))
    if err != nil {
        panic(err)
    }
    logger.SetDefault(log)

    provider, _ := tracing.New(tracing.WithConfig(cfg))
    metric.Init(metric.WithConfig(cfg))

    httpSrv, _ := httpserver.New(httpserver.WithConfig(cfg), httpserver.WithLogger(log))
    grpcSrv, _ := grpcserver.New(grpcserver.WithConfig(cfg), grpcserver.WithLogger(log))

    // Register your routes here:
    // httpSrv.Engine().GET("/hello", ...)
    // pb.RegisterMyServiceServer(grpcSrv.Server(), ...)

    if err := app.New(
        app.WithName(cfg.GetString("app.name")),
        app.WithServer(httpSrv, grpcSrv),
        app.WithLogger(log),
        app.WithBeforeStart(func(ctx context.Context) error {
            httpserver.SetReady(true)
            return nil
        }),
        app.WithAfterStop(func(ctx context.Context) error {
            metric.Shutdown()
            return provider.Shutdown(ctx)
        }),
    ).Run(); err != nil {
        logger.Error(context.Background(), "fatal", logger.F("err", err))
        os.Exit(1)
    }
}
```

Place a `config.yaml` next to the binary (see the complete reference below) and run the service. It handles `SIGTERM`/`SIGINT` for graceful shutdown.

## Complete config.yaml Reference

```yaml
# Application identity — read by server/http, server/grpc, tracing, and metric
app:
  name: my-service          # service name propagated to OTel resource
  version: v1.0.0           # echoed in /healthz and /readyz responses
  env: development          # "production" disables gRPC server reflection

# HTTP server
server:
  http:
    addr: :8080             # listen address (default :8080)
    timeout: 30s            # read and write timeout (default 30s)

# gRPC server
  grpc:
    addr: :9090             # listen address (default :9090)

# Structured logger
logger:
  level: info               # debug | info | warn | error (live-reloadable)
  output: stdout            # stdout | file | both
  filename: /var/log/app    # base path; level-split files are appended as .info.log etc.
  max_size: 100             # max size in MB before rotation (default 100)
  max_age: 7                # max age in days to retain rotated files (default 7)
  max_backups: 10           # max number of rotated files to keep (default 10)
  mask_fields:              # field names whose values are partially redacted in logs
    - password
    - token

# Distributed tracing (OpenTelemetry OTLP over HTTP)
tracing:
  endpoint: ""              # e.g. http://jaeger:4318 — leave empty to disable tracing
  sampler: 1.0              # tail-sampling ratio: 1.0 = 100%, 0.1 = 10%

# Metrics
metric:
  prometheus_addr: :9091    # address to expose /metrics — omit to disable Prometheus
  otlp_endpoint: ""         # e.g. http://collector:4318 — omit to disable OTLP metrics

# Database connections — default, read, and write names are scanned automatically
database:
  default:
    driver: mysql           # mysql | postgres | sqlite
    dsn: "user:pass@tcp(localhost:3306)/mydb?parseTime=true"
    max_open_conns: 100     # default 100
    max_idle_conns: 10      # default 10
    conn_max_lifetime: 1h   # connection lifetime (0 = unlimited)
    trace_all_sql: false    # emit OTel spans for every SQL statement
    slow_threshold: 200ms   # log a warning for queries exceeding this duration
  read:
    driver: mysql
    dsn: "user:pass@tcp(replica:3306)/mydb?parseTime=true"
  write:
    driver: mysql
    dsn: "user:pass@tcp(primary:3306)/mydb?parseTime=true"

# Redis cache — default and session instances are scanned automatically
cache:
  default:
    addr: localhost:6379    # host:port
    password: ""            # optional auth password
    db: 0                   # Redis DB index
  session:
    addr: localhost:6380
    password: ""
    db: 1
```

Environment variables override any YAML key. When `WithEnv("APP")` is passed to `config.Load`, the variable `APP_SERVER_HTTP_ADDR` overrides `server.http.addr` (prefix stripped, underscores converted to dots).

## Package Reference

### `core`

Defines shared interfaces used across all packages to avoid import cycles.

| Interface | Description |
|---|---|
| `Logger` | `Debug/Info/Warn/Error(ctx, msg, ...Field)` plus `With(...Field) Logger` |
| `Config` | `Get*`, `Scan`, `Watch` — see config package |
| `Server` | `Start(ctx) error`, `Stop(ctx) error` |
| `Provider` | `Tracer(name) trace.Tracer`, `Shutdown(ctx) error` |

`core.Field` is a `{Key string, Value any}` pair. Use `logger.F(key, value)` as a shorthand.

---

### `config`

Viper-backed configuration loader.

**Key functions**

```go
cfg, err := config.Load(
    config.WithFile("config.yaml"),  // YAML, TOML, or JSON — format auto-detected
    config.WithEnv("APP"),           // enable env-var overrides with prefix APP_
    config.WithWatch(),              // enable hot-reload on file change (fsnotify)
)

cfg.GetString("app.name")
cfg.GetInt("server.http.port")
cfg.GetDuration("server.http.timeout")

// Unmarshal a sub-tree into a struct
var dbCfg database.DSNConfig
cfg.Scan("database.default", &dbCfg)

// Watch a key for runtime changes (only logger.level reacts at runtime)
cfg.Watch("logger.level", func(v any) {
    // v is the new value
})
```

**Options**

| Option | Description |
|---|---|
| `WithFile(path)` | Load config from the given file path |
| `WithEnv(prefix)` | Enable `PREFIX_KEY_NAME` env-var overrides |
| `WithWatch()` | Enable live reload via fsnotify |

---

### `logger`

Zap-backed JSON structured logger. Every log call takes a `context.Context`; if an active OTel span is in the context, `trace_id` and `span_id` are automatically appended to the log entry.

**Key functions**

```go
log, err := logger.New(
    logger.WithConfig(cfg),                        // read from config file
    logger.WithMaskFields("password", "token"),    // partial-mask these fields
)
logger.SetDefault(log)

// Package-level helpers (use the default logger)
logger.Info(ctx, "user created", logger.F("user_id", 42))
logger.Warn(ctx, "slow query", logger.F("elapsed", "350ms"))

// Scoped logger carrying permanent fields
reqLog := log.With(logger.F("request_id", rid))
reqLog.Debug(ctx, "parsed request")
```

**Options**

| Option | Description |
|---|---|
| `WithLevel(level)` | `debug`, `info`, `warn`, `error` |
| `WithOutput(output)` | `stdout`, `file`, `both` |
| `WithFilename(path)` | Base path for level-split log files |
| `WithMaxSizeMB(n)` | Max file size in MB before rotation |
| `WithMaxAgeDays(n)` | Days to retain rotated files |
| `WithMaxBackups(n)` | Max number of backup files |
| `WithMaskFields(...)` | Field names to partially redact |
| `WithConfig(cfg)` | Read all options from `core.Config` |

**Log output format**

```json
{"ts":"2026-04-11T10:00:00.000Z","level":"info","caller":"main.go:42","msg":"user created","trace_id":"abc123","span_id":"def456","user_id":42}
```

---

### `tracing`

OpenTelemetry trace provider backed by OTLP over HTTP. When `endpoint` is empty, a no-op provider is returned — no spans are emitted and no connections are made.

```go
provider, err := tracing.New(
    tracing.WithConfig(cfg),
    // or explicitly:
    tracing.WithEndpoint("http://jaeger:4318"),
    tracing.WithServiceName("my-service"),
    tracing.WithSampler(0.1), // 10% sampling
)

// Use in a handler
tracer := provider.Tracer("users")
ctx, span := tracer.Start(ctx, "create-user")
defer span.End()

// Shutdown flushes pending spans; register with app.WithAfterStop
provider.Shutdown(ctx)
```

The HTTP and gRPC server middleware automatically propagate trace context and create per-request spans. The logger automatically reads `trace_id` and `span_id` from the context, so all log lines emitted inside a traced handler carry those fields without any explicit wiring.

---

### `metric`

OpenTelemetry metrics with a Prometheus scrape endpoint and/or OTLP push. Call `metric.Init` once during startup. HTTP and gRPC middleware hooks are wired automatically.

```go
metric.Init(
    metric.WithConfig(cfg),
    // or explicitly:
    metric.WithServiceName("my-service"),
    metric.WithPrometheusAddr(":9091"),          // exposes GET /metrics
    metric.WithOTLPEndpoint("http://otel:4318"), // push to collector
)

// Helpers — safe to call before Init (no-op until Init runs)
metric.Count(ctx, "orders.created", 1, metric.Label("region", "us-east"))
metric.Gauge(ctx, "queue.depth", float64(len(q)))
stop := metric.StartTimer(ctx, "db.query", metric.Label("table", "users"))
defer stop() // records duration_seconds histogram

// Shutdown in app.WithAfterStop
metric.Shutdown()
```

**Helpers**

| Function | Description |
|---|---|
| `Count(ctx, name, n, labels...)` | Increment an int64 counter |
| `Gauge(ctx, name, value, labels...)` | Set a float64 up-down counter |
| `GaugeAdd / GaugeSub` | Delta operations on a gauge |
| `StartTimer(ctx, name, labels...)` | Returns a `func()` that records elapsed seconds |
| `Duration(ctx, name, d, labels...)` | Record a pre-measured duration |

---

### `app`

Lifecycle runner. Runs migrations, calls before-start hooks, starts all servers concurrently, blocks until `SIGTERM`/`SIGINT` or a server error, stops servers within the shutdown timeout, then calls after-stop hooks.

```go
a := app.New(
    app.WithName("my-service"),
    app.WithVersion("v1.0.0"),
    app.WithServer(httpSrv, grpcSrv),    // any core.Server
    app.WithLogger(log),
    app.WithMigration(dbMgr),            // run migrations before start
    app.WithShutdownTimeout(30 * time.Second),
    app.WithBeforeStart(func(ctx context.Context) error {
        httpserver.SetReady(true)
        return nil
    }),
    app.WithAfterStop(func(ctx context.Context) error {
        metric.Shutdown()
        return provider.Shutdown(ctx)
    }),
)

if err := a.Run(); err != nil {
    log.Fatal(err)
}
```

**Startup sequence**

1. `WithMigration.Up(ctx)` — run pending database migrations
2. `WithBeforeStart` hooks — in registration order
3. Start all servers concurrently in goroutines
4. Block until `SIGTERM`, `SIGINT`, or a server error
5. `Stop` all servers concurrently (bounded by `ShutdownTimeout`)
6. `WithAfterStop` hooks — in registration order

---

### `server/http`

Gin HTTP server. `New` returns a `*ginServer` that implements `core.Server`. The full default middleware stack is pre-mounted in the order shown below. Additional routes and middleware are added via `Engine()`.

**Default middleware stack**

```
RequestID -> Tracing -> Logger -> Metrics -> Recovery -> ErrorHandler
```

**Built-in routes** (no auth, always available)

- `GET /healthz` — liveness probe
- `GET /readyz` — readiness probe (controlled by `httpserver.SetReady(bool)`)
- `GET /health` — alias for `/healthz`

```go
httpSrv, err := httpserver.New(
    httpserver.WithConfig(cfg),
    httpserver.WithLogger(log),
    // or explicitly:
    httpserver.WithAddr(":8080"),
    httpserver.WithReadTimeout(30 * time.Second),
    httpserver.WithWriteTimeout(30 * time.Second),
    httpserver.WithServiceName("my-service"),
    httpserver.WithServiceVersion("v1.0.0"),
)

r := httpSrv.Engine()
r.GET("/api/v1/users", listUsersHandler)
r.Use(httpMiddleware.CORS(httpMiddleware.CORSConfig{
    AllowOrigins: []string{"https://example.com"},
    AllowMethods: []string{"GET", "POST"},
    AllowHeaders: []string{"Authorization", "Content-Type"},
    MaxAge:       12 * time.Hour,
}))
```

---

### `server/grpc`

gRPC server. `New` returns a `*grpcServer` that implements `core.Server`. Reflection is enabled by default and automatically disabled when `app.env` is `production`.

**Default interceptor chain**

```
RequestID -> Logger -> Metrics -> Recovery -> ErrorHandler
```
Tracing is wired as a `StatsHandler` (otelgrpc v0.68+ API).

**Built-in service**

- `grpc.health.v1.Health/Check` — standard gRPC health check, always `SERVING`

```go
grpcSrv, err := grpcserver.New(
    grpcserver.WithConfig(cfg),
    grpcserver.WithLogger(log),
    // optional TLS:
    grpcserver.WithTLS("/path/to/cert.pem", "/path/to/key.pem"),
)

pb.RegisterMyServiceServer(grpcSrv.Server(), &myServiceImpl{})
```

---

### `database`

GORM connection manager for MySQL, Postgres, and SQLite. All open connections are stored in a package-level registry and retrieved by name.

```go
dbMgr, err := database.New(
    database.WithConfig(cfg),            // reads database.default, .read, .write
    database.WithLogger(log),
    database.WithMigrationsDir("migrations"),
    // or add connections explicitly:
    database.WithDriver("replica", "postgres", "host=replica user=app dbname=mydb"),
)

// Retrieve a connection anywhere in the codebase
db := database.DB("default")  // *gorm.DB
db.First(&user, id)

// Run migrations manually (app.WithMigration calls this automatically)
dbMgr.Up(ctx)
```

`WithConfig` scans the keys `database.default`, `database.read`, and `database.write`. `WithDriver` adds an additional named connection programmatically.

When `trace_all_sql: true` is set, every SQL statement is emitted as an OTel span. Queries exceeding `slow_threshold` emit a warning log regardless.

---

### `cache`

go-redis client manager. All clients are stored in a package-level registry and retrieved by name. A slow-command hook logs a warning for any command taking longer than 100 ms.

```go
err := cache.Init(
    cache.WithConfig(cfg),             // reads cache.default and cache.session
    cache.WithLogger(log),
    // or add instances explicitly:
    cache.WithAddr("default", "localhost:6379"),
    cache.WithAddrAuth("session", "localhost:6380", "secret", 1),
)

rdb := cache.Client("default")  // *redis.Client
rdb.Set(ctx, "key", "value", time.Hour)
```

---

### `errors`

Structured errors with an HTTP status code and a machine-readable reason string.

```go
// Define sentinels at package level
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")

// Attach context before returning
func getUser(id int) (*User, error) {
    u, err := db.Find(id)
    if err == gorm.ErrRecordNotFound {
        return nil, errors.Wrap(ErrUserNotFound, "id=%d", id)
    }
    return u, err
}

// Inspect at the handler layer
if err != nil {
    code   := errors.Code(err)    // 404
    reason := errors.Reason(err)  // "USER_NOT_FOUND"
    // errors.Is works across Wrap boundaries:
    if errors.Is(err, ErrUserNotFound) { ... }
}
```

**Predefined sentinels**

| Variable | Code | Reason |
|---|---|---|
| `ErrBadRequest` | 400 | `BAD_REQUEST` |
| `ErrUnAuth` | 401 | `UNAUTHORIZED` |
| `ErrForbidden` | 403 | `FORBIDDEN` |
| `ErrNotFound` | 404 | `NOT_FOUND` |
| `ErrConflict` | 409 | `CONFLICT` |
| `ErrTooMany` | 429 | `TOO_MANY_REQUEST` |
| `ErrInternal` | 500 | `INTERNAL` |

## Middleware Reference

### HTTP Middleware (`middleware/http`)

All middleware in the table below is available as `gin.HandlerFunc`. The first six are mounted automatically by `httpserver.New`; the rest are opt-in.

| Middleware | Function | Notes |
|---|---|---|
| RequestID | `RequestID()` | Sets `X-Request-ID`; generates UUID if absent |
| Tracing | `Tracing(serviceName)` | Creates OTel span per request via otelgin |
| Logger | `Logger(log)` | Logs method, path, status, latency; injects request_id |
| Metrics | `Metrics()` | Records `http_requests_total`, `http_request_duration_seconds`, `http_requests_in_flight`; no-op until `metric.Init` |
| Recovery | `Recovery()` | Recovers panics, returns 500 |
| ErrorHandler | `ErrorHandler()` | Converts `*errors.CodedError` to JSON response |
| CORS | `CORS(CORSConfig)` | Configurable origin, methods, headers, max-age |
| RateLimit | `RateLimit(RateLimitConfig)` | Per-key token bucket; default key is client IP |
| IPFilter | `IPFilter(IPFilterConfig)` | Whitelist or blacklist by IP or CIDR |
| BodyLimit | `BodyLimit(size)` | Limit request body size, e.g. `"8MB"` |
| Timeout | `Timeout(duration)` | Context deadline per request; 503 on expiry |

**CORS example**

```go
r.Use(httpMiddleware.CORS(httpMiddleware.CORSConfig{
    AllowOrigins: []string{"https://app.example.com"},
    AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders: []string{"Authorization", "Content-Type"},
    MaxAge:       12 * time.Hour,
}))
```

**Rate limit example**

```go
r.Use(httpMiddleware.RateLimit(httpMiddleware.RateLimitConfig{
    Rate:  100,  // requests per second
    Burst: 20,
    // optional: custom key function
    KeyFunc: func(c *gin.Context) string { return c.GetHeader("X-API-Key") },
}))
```

### gRPC Middleware (`middleware/grpc`)

All interceptors are `grpc.UnaryServerInterceptor` unless noted. The first five are mounted automatically by `grpcserver.New`; tracing is a `StatsHandler`.

| Middleware | Function | Notes |
|---|---|---|
| Tracing | `ServerTracingHandler()` | OTel span per RPC via otelgrpc StatsHandler |
| RequestID | `UnaryRequestID` | Propagates or generates a request ID in metadata |
| Logger | `UnaryLogger(log)` | Logs method, status, latency |
| Metrics | `UnaryMetrics()` | Records `grpc_requests_total`, `grpc_request_duration_seconds`, `grpc_requests_in_flight` |
| Recovery | `UnaryRecovery` | Recovers panics, returns `codes.Internal` |
| ErrorHandler | `UnaryErrorHandler` | Converts `*errors.CodedError` to gRPC status |
| RateLimit | `UnaryRateLimit(GRPCRateLimitConfig)` | Per-key token bucket; default key is peer address |
| IPFilter | `UnaryIPFilter(GRPCIPFilterConfig)` | Whitelist or blacklist by IP or CIDR |
| Timeout | `UnaryTimeout(duration)` | Context deadline per RPC |

**Adding optional interceptors**

```go
grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        grpcMiddleware.UnaryRequestID,
        grpcMiddleware.UnaryLogger(log),
        grpcMiddleware.UnaryMetrics(),
        grpcMiddleware.UnaryRecovery,
        grpcMiddleware.UnaryErrorHandler,
        grpcMiddleware.UnaryRateLimit(grpcMiddleware.GRPCRateLimitConfig{
            Rate: 500, Burst: 50,
        }),
        grpcMiddleware.UnaryTimeout(10 * time.Second),
    ),
)
```

## Error Handling

Define package-level sentinel errors with `errors.Define`, then use `errors.Wrap` to attach call-site context before returning. The HTTP and gRPC error-handler middleware automatically converts `*errors.CodedError` to the appropriate response format.

```go
package users

import "github.com/pemako/mk/errors"

var (
    ErrNotFound  = errors.Define(404, "USER_NOT_FOUND",  "user not found")
    ErrDuplicate = errors.Define(409, "USER_DUPLICATE",  "user already exists")
)

func Create(ctx context.Context, req *CreateRequest) (*User, error) {
    if exists(req.Email) {
        return nil, errors.Wrap(ErrDuplicate, "email=%s", req.Email)
    }
    // ...
}

// In a Gin handler — no explicit status code needed:
func createHandler(c *gin.Context) {
    user, err := users.Create(c.Request.Context(), req)
    if err != nil {
        c.Error(err) // ErrorHandler middleware picks this up
        return
    }
    c.JSON(200, user)
}
```

The HTTP error handler writes:

```json
{"code": 409, "reason": "USER_DUPLICATE", "message": "user already exists"}
```

`errors.Is(err, ErrDuplicate)` returns `true` across `Wrap` boundaries because `CodedError.Is` matches on `Reason`.

## Log Masking

Fields whose names appear in `logger.mask_fields` (case-insensitive) have their string values partially redacted before writing. Short values (four characters or fewer) become `***`; longer values keep the first and last quarter, with `***` in the middle.

```yaml
logger:
  mask_fields:
    - password
    - token
    - api_key
```

```go
log.Info(ctx, "auth", logger.F("password", "mypassword"))
// output: {..., "password": "myp***ord"}

log.Info(ctx, "auth", logger.F("token", "ab"))
// output: {..., "token": "***"}
```

Masking is applied at log time inside the logger — the original value in your struct is never modified.

## Log Level File Splitting

When `output` is `file` or `both`, the logger writes one file per level instead of a single combined file. Given `filename: /var/log/app`, the following files are created:

| File | Contains |
|---|---|
| `/var/log/app.debug.log` | `DEBUG` entries only |
| `/var/log/app.info.log` | `INFO` entries only |
| `/var/log/app.warn.log` | `WARN` entries only |
| `/var/log/app.error.log` | `ERROR`, `DPANIC`, `PANIC`, `FATAL` entries |

Files below the configured minimum level are not created. For example, with `level: warn`, only `warn.log` and `error.log` are written.

Each file is managed by lumberjack and rotated according to `max_size`, `max_age`, and `max_backups`.

```yaml
logger:
  level: info
  output: both          # write to stdout AND files simultaneously
  filename: /var/log/myapp
  max_size: 200         # MB
  max_age: 14           # days
  max_backups: 5
```

## Database Migrations

`database.Manager` implements `app.Migrator`. When passed to `app.WithMigration`, migrations run automatically before any server starts. The migrator uses [goose v3](https://github.com/pressly/goose) and supports MySQL, Postgres, and SQLite.

**Directory layout**

```
migrations/
  00001_create_users.sql
  00002_add_email_index.sql
```

**SQL migration file format**

```sql
-- +goose Up
CREATE TABLE users (
    id   BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL
);

-- +goose Down
DROP TABLE users;
```

**Using the migrator directly**

```go
dbMgr, _ := database.New(
    database.WithDriver("default", "postgres", dsn),
    database.WithMigrationsDir("migrations"),
)

// Run all pending migrations
dbMgr.Up(ctx)

// Use the lower-level Migrator for more control
m := database.NewMigrator(database.DB("default"), "migrations")
m.Up(ctx)                  // apply all pending
m.Down(ctx)                // roll back the last migration
m.UpTo(ctx, 3)             // apply up to version 3
version, _ := m.Status(ctx) // current applied version
```

When `app.WithMigration(dbMgr)` is used, `Up` is called on every named connection that has a migrations directory configured.

## Project Structure

```
github.com/pemako/mk/
├── app/                    # Application lifecycle runner
│   ├── app.go              # App struct and Run method
│   └── options.go          # WithServer, WithMigration, WithBeforeStart, etc.
├── cache/                  # Redis client manager (go-redis)
│   ├── cache.go            # Init, Client, slow-command hook
│   └── options.go          # WithAddr, WithAddrAuth, WithConfig
├── config/                 # Viper-backed config loader
│   ├── config.go           # Load, viperConfig implementation
│   └── options.go          # WithFile, WithEnv, WithWatch
├── core/                   # Shared interfaces (no external deps)
│   ├── config.go           # Config interface
│   ├── logger.go           # Logger interface and Field type
│   ├── server.go           # Server interface
│   └── tracer.go           # Provider interface
├── database/               # GORM connection manager + goose migrations
│   ├── config.go           # DSNConfig struct
│   ├── database.go         # Manager, New, DB registry
│   ├── migrate.go          # Migrator: Up, Down, UpTo, Status
│   ├── options.go          # WithDriver, WithConfig, WithMigrationsDir
│   └── plugin.go           # OTel tracing plugin for GORM
├── errors/                 # Structured CodedError
│   └── errors.go           # Define, Wrap, Code, Reason, predefined sentinels
├── logger/                 # Zap-backed structured logger
│   ├── logger.go           # New, zapLogger, level-split file cores, masking
│   └── options.go          # WithLevel, WithOutput, WithMaskFields, WithConfig
├── metric/                 # OTel metrics (Prometheus + OTLP)
│   ├── metric.go           # Init, Shutdown, Prometheus server
│   ├── helpers.go          # Count, Gauge, StartTimer, Duration
│   └── options.go          # WithPrometheusAddr, WithOTLPEndpoint, WithConfig
├── middleware/
│   ├── http/               # Gin middleware
│   │   ├── requestid.go
│   │   ├── tracing.go
│   │   ├── logger.go
│   │   ├── metrics.go      # Metrics() + RegisterHTTPMetricHooks
│   │   ├── recovery.go
│   │   ├── error_handler.go
│   │   ├── cors.go
│   │   ├── rate_limit.go
│   │   ├── ip_filter.go
│   │   ├── body_limit.go
│   │   └── timeout.go
│   └── grpc/               # gRPC unary interceptors
│       ├── requestid.go
│       ├── tracing.go
│       ├── logger.go
│       ├── metrics.go      # UnaryMetrics() + RegisterGRPCMetricHooks
│       ├── recovery.go
│       ├── error_handler.go
│       ├── rate_limit.go
│       ├── ip_filter.go
│       └── timeout.go
├── server/
│   ├── http/               # Gin HTTP server
│   │   ├── server.go       # New, Engine, Start, Stop, health routes
│   │   └── options.go      # WithAddr, WithLogger, WithConfig
│   └── grpc/               # gRPC server
│       ├── server.go       # New, Server, Start, Stop, health service
│       └── options.go      # WithAddr, WithTLS, WithConfig
├── tracing/                # OTel OTLP trace provider
│   ├── provider.go         # New, Provider.Tracer, Provider.Shutdown
│   └── options.go          # WithEndpoint, WithSampler, WithConfig
└── example/                # Runnable reference application
    ├── main.go
    ├── config.yaml
    └── internal/
        ├── handler/http/   # HTTP route handlers
        └── service/        # Business logic
```
