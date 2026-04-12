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
| `cache` | Cache client manager with in-memory (default) and Redis backends; unified `Cache` interface |
| `errors` | Structured `CodedError` with HTTP code, reason string, and cause chaining |
| `middleware/http` | RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler, CORS, RateLimit, IPFilter, BodyLimit, Timeout |
| `middleware/grpc` | UnaryRequestID, UnaryLogger, UnaryMetrics, UnaryRecovery, UnaryErrorHandler, UnaryRateLimit, UnaryIPFilter, UnaryTimeout |
| `httpclient` | Outbound HTTP client with OTel tracing, exponential-backoff retry, and a named global registry |
| `grpcclient` | Outbound gRPC client with OTel tracing, per-call retry, and a named global registry |
| `lock` | Distributed lock backed by Redis with atomic acquire/release, auto-renew, and wait-with-timeout |
| `cron` | Cron scheduler (`core.Server`) with retries, per-job timeouts, skip-if-running, and sub-second tickers |
| `auth` | JWT signing/verification and gin middleware (`Required`, `Optional`) with typed claims extraction via generics |
| `validate` | Request binding + struct validation backed by go-playground/validator; returns structured 400 errors with field-level metadata |
| `pagination` | Standard page-number pagination params and a typed `Result[T]` response envelope for list endpoints |

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

# Cache — in-memory by default; Redis is optional
cache:
  default:
    backend: memory        # in-memory, no Redis required
  session:
    backend: redis
    addr: "redis:6379"     # host:port
    password: ""           # optional auth password
    db: 0                  # Redis DB index
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

Cache client manager with a unified `Cache` interface. All backends are stored in a package-level registry and retrieved by name. The default backend is in-memory (no external dependencies). Redis is optional and configured via `WithRedis`/`WithRedisAuth` or a `backend: redis` config entry. A slow-command hook logs a warning for Redis commands taking longer than 100 ms.

```go
// In-memory (default, no Redis needed)
cache.Init(cache.WithMemory("default"))

// Redis (optional)
cache.Init(cache.WithRedis("session", "redis:6379"))

// Use the Cache interface — same API regardless of backend
c := cache.Client("default")

ctx := context.Background()
c.Set(ctx, "user:1", userData, time.Hour)

val, err := c.Get(ctx, "user:1")
if errors.Is(err, cache.ErrCacheMiss) {
    // key not found or expired
}

c.Del(ctx, "user:1")

exists, err := c.Exists(ctx, "user:1")
```

**Options**

| Option | Description |
|---|---|
| `WithMemory(name)` | Register a named in-memory backend (no external deps) |
| `WithRedis(name, addr)` | Register a named Redis backend by `host:port` |
| `WithRedisAuth(name, addr, password, db)` | Register a named Redis backend with auth and DB index |
| `WithConfig(cfg)` | Read all backends from config (see YAML below) |
| `WithLogger(log)` | Logger for slow Redis command warnings |

**Config**

```yaml
cache:
  default:
    backend: memory        # in-memory, no Redis required
  session:
    backend: redis
    addr: "redis:6379"
    password: ""
    db: 0
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

### `httpclient`

Outbound HTTP client with OTel tracing, exponential-backoff retry, JSON marshal/unmarshal helpers, and a named global registry. All methods are safe for concurrent use.

**Key types / functions**

| Symbol | Description |
|---|---|
| `New(opts...) *Client` | Create a client |
| `Client.Get/Post/Put/Delete` | Typed HTTP verbs; body and response are JSON-marshalled automatically |
| `Client.Do` | Raw request returning `*Response{StatusCode, Body}` |
| `Register(name, opts...)` | Create and store a named client in the global registry |
| `Get(name) *Client` | Retrieve a named client |
| `Init(cfg, names...)` | Bulk-register clients from config |

**Usage**

```go
import "github.com/pemako/mk/httpclient"

// One-off client
c := httpclient.New(
    httpclient.WithBaseURL("https://api.example.com"),
    httpclient.WithTimeout(10 * time.Second),
    httpclient.WithHeader("Authorization", "Bearer "+token),
    httpclient.WithRetry(3),
)

var result MyResponse
if err := c.Get(ctx, "/v1/users/42", &result); err != nil {
    return err
}

if err := c.Post(ctx, "/v1/orders", orderReq, &result); err != nil {
    return err
}

// Named registry — register once at startup
httpclient.Register("payments",
    httpclient.WithBaseURL("https://payments.internal"),
    httpclient.WithRetry(3),
    httpclient.WithRetryBaseDelay(200 * time.Millisecond),
)

// Use anywhere
c := httpclient.Get("payments")
c.Put(ctx, "/v1/charge/"+id, req, &resp)
```

**Options**

| Option | Default | Description |
|---|---|---|
| `WithBaseURL(url)` | `""` | Base URL prepended to every request path |
| `WithTimeout(d)` | `30s` | Per-request context deadline |
| `WithRetry(n)` | `0` | Max retry attempts on network error, 429, or 5xx |
| `WithRetryBaseDelay(d)` | `100ms` | Exponential-backoff base delay |
| `WithRetryMaxDelay(d)` | `2s` | Backoff cap |
| `WithHeader(k, v)` | — | Default header sent on every request |
| `WithLogger(log)` | default logger | Logger for retry debug messages |

**Config**

```yaml
httpclient:
  default:
    base_url: "https://api.example.com"
    timeout: 30s
    retry: 3
  payments:
    base_url: "https://payments.internal"
    timeout: 10s
    retry: 2
```

---

### `grpcclient`

Outbound gRPC client with OTel tracing (stats handler), per-call logging, exponential-backoff retry on `Unavailable`/`DeadlineExceeded`/`ResourceExhausted`, and a named global registry.

**Key types / functions**

| Symbol | Description |
|---|---|
| `Dial(target, opts...) (*grpc.ClientConn, error)` | Open a lazy gRPC connection |
| `Register(name, target, opts...) error` | Dial and store in the global registry |
| `Get(name) *grpc.ClientConn` | Retrieve a named connection |
| `CloseAll()` | Close all registered connections; call in `app.WithAfterStop` |
| `Init(cfg, names...) error` | Bulk-register from config |

**Usage**

```go
import "github.com/pemako/mk/grpcclient"

// Register at startup
if err := grpcclient.Register("user-service", "user-service:9090",
    grpcclient.WithInsecure(),
    grpcclient.WithTimeout(5 * time.Second),
    grpcclient.WithRetry(3),
); err != nil {
    panic(err)
}

// Use anywhere
conn := grpcclient.Get("user-service")
client := pb.NewUserServiceClient(conn)
resp, err := client.GetUser(ctx, &pb.GetUserRequest{Id: id})

// Graceful shutdown — wire into app lifecycle
app.New(
    app.WithAfterStop(func(ctx context.Context) error {
        grpcclient.CloseAll()
        return nil
    }),
)
```

**Options**

| Option | Default | Description |
|---|---|---|
| `WithInsecure()` | — | Plaintext transport (no TLS) |
| `WithTLS(caCert)` | — | One-way TLS with CA certificate file |
| `WithMTLS(cert, key, caCert)` | — | Mutual TLS |
| `WithTimeout(d)` | `30s` | Per-call context deadline |
| `WithRetry(n)` | `0` | Max retry attempts for retryable status codes |
| `WithRetryBaseDelay(d)` | `100ms` | Exponential-backoff base delay |
| `WithLogger(log)` | default logger | Logger for RPC call events |

**Config**

```yaml
grpcclient:
  user-service:
    addr: "user-service:9090"
    insecure: true
    timeout: 5s
    retry: 3
  order-service:
    addr: "order-service:9090"
    insecure: true
    timeout: 10s
```

---

### `lock`

Distributed lock backed by Redis. Uses atomic Lua scripts for both acquire (SET NX PX) and release (compare-and-delete), preventing accidental release of a lock owned by another instance. All methods are safe for concurrent use.

**Key types / functions**

| Symbol | Description |
|---|---|
| `New(addr, opts...) *Locker` | Create a Locker with its own Redis connection |
| `NewWithClient(rdb, opts...) *Locker` | Create a Locker using an existing `*redis.Client` |
| `Locker.Acquire(ctx, key, ttl)` | Low-level: attempt once, returns `(bool, error)` |
| `Locker.Release(ctx, key)` | Low-level: release if this Locker holds the token |
| `Locker.TryLock(ctx, key, ttl, fn, opts...)` | Acquire, run fn, release; handles wait and auto-renew |
| `ErrLockNotAcquired` | Returned when lock is already held and `WithWait` is not set |
| `ErrLockExpired` | Returned when auto-renew detects the lock was lost mid-execution |

**Usage**

```go
import "github.com/pemako/mk/lock"

// Share an existing Redis client (e.g. from the cache package)
locker := lock.NewWithClient(rdb, lock.WithLogger(log))

// Simple try-once pattern
err := locker.TryLock(ctx, "invoice:process:42", 30*time.Second, func(ctx context.Context) error {
    return processInvoice(ctx, 42)
})
if errors.Is(err, lock.ErrLockNotAcquired) {
    // Another instance is processing — skip silently
    return nil
}

// Wait up to 5s for the lock, then auto-renew the TTL while processing
err = locker.TryLock(ctx, "report:generate", 10*time.Second,
    func(ctx context.Context) error {
        return generateReport(ctx) // may take longer than 10s
    },
    lock.WithWait(5*time.Second, 200*time.Millisecond),
    lock.WithAutoRenew(),
)
if errors.Is(err, lock.ErrLockExpired) {
    log.Error(ctx, "lock lost during report generation")
}
```

**Options**

| Option | Description |
|---|---|
| `WithRedisPassword(pw)` | Redis AUTH password (for `New` only) |
| `WithRedisDB(db)` | Redis logical DB index (for `New` only, default `0`) |
| `WithLogger(log)` | Logger for acquire/release/renewal events |

**TryLock options**

| Option | Description |
|---|---|
| `WithWait(timeout, interval)` | Poll until the lock is free or timeout elapses |
| `WithAutoRenew()` | Renew the lock TTL every `ttl/3` in the background |

---

### `cron`

Cron scheduler that implements `core.Server` and integrates with the `app` lifecycle via `app.WithServer`. Supports standard 5-field cron syntax, robfig shorthands (`@hourly`, `@every 5m`, etc.), and sub-second intervals via a ticker. Each job gets panic recovery, optional timeout, optional retry with exponential backoff, and optional skip-if-running protection.

**Key types / functions**

| Symbol | Description |
|---|---|
| `New(opts...) *Scheduler` | Create a scheduler |
| `Scheduler.Add(name, spec, fn, opts...)` | Register a named job |
| `Scheduler.Remove(name)` | Unregister a job (in-flight execution finishes) |
| `Scheduler.Start(ctx) error` | Implements `core.Server`; blocks until `Stop` |
| `Scheduler.Stop(ctx) error` | Graceful shutdown; waits for running jobs up to ctx deadline |

**Usage**

```go
import "github.com/pemako/mk/cron"

scheduler := cron.New(cron.WithLogger(log))

// Simple job — runs every 5 minutes
scheduler.Add("cleanup", "@every 5m", func(ctx context.Context) error {
    return cleanupExpiredSessions(ctx)
})

// Job with timeout, retries, and overlap protection
scheduler.Add("report", "0 3 * * *",  // daily at 03:00 UTC
    func(ctx context.Context) error {
        return generateDailyReport(ctx)
    },
    cron.WithTimeout(10*time.Minute),
    cron.WithRetries(2),
    cron.WithRetryDelay(30*time.Second),
    cron.WithSkipIfRunning(),
)

// Sub-second ticker (uses time.Ticker, not robfig)
scheduler.Add("heartbeat", "@every 500ms", func(ctx context.Context) error {
    return emitHeartbeat(ctx)
})

// Wire into app lifecycle — Start/Stop managed automatically
app.New(
    app.WithServer(httpSrv, grpcSrv, scheduler),
)
```

**Scheduler options**

| Option | Default | Description |
|---|---|---|
| `WithLogger(log)` | default logger | Logger for job lifecycle events |
| `WithLocation(loc)` | `time.UTC` | Timezone for cron expression evaluation |

**Per-job options**

| Option | Default | Description |
|---|---|---|
| `WithTimeout(d)` | none | Cancel the job context after d |
| `WithRetries(n)` | `0` | Max retry attempts on error |
| `WithRetryDelay(d)` | `1s` | Base delay for exponential backoff between retries |
| `WithSkipIfRunning()` | false | Skip the tick if a previous execution is still running |

**Log output**

```json
{"level":"info","msg":"cron start","job":"report"}
{"level":"info","msg":"cron done","job":"report","elapsed":"2.341s"}
{"level":"error","msg":"cron failed","job":"report","err":"context deadline exceeded","retry":0}
{"level":"warn","msg":"cron skip","job":"report","reason":"still running"}
```

---

### `auth`

JWT signing and verification with gin middleware. The framework handles HS256 signing, token parsing, and context storage. Business code defines its own `Claims` struct and manages its own secret.

**Key API**

| Symbol | Description |
|---|---|
| `Generate(claims, opts...) (string, error)` | Sign and return a JWT string using HS256 |
| `Required(opts...) gin.HandlerFunc` | Middleware that requires a valid Bearer token; aborts with 401 otherwise |
| `Optional(opts...) gin.HandlerFunc` | Middleware that parses the token if present; requests without a token continue normally |
| `FromContext[T](ctx) (T, bool)` | Extract typed claims stored by `Required` or `Optional` |

**Options**

| Option | Description |
|---|---|
| `WithSecret(secret)` | HMAC signing secret for HS256 |
| `WithKeyFunc(fn)` | Custom `jwt.Keyfunc` for key rotation |
| `WithExpire(d)` | Token expiration duration (default 24h) |
| `WithIssuer(iss)` | `iss` claim added to generated tokens |
| `WithClaims(fn)` | Factory that returns a new empty claims instance for parsing |

**Usage**

```go
// 1. Define your claims struct — embed jwt.RegisteredClaims to satisfy auth.Claims.
type UserClaims struct {
    jwt.RegisteredClaims
    UserID int64  `json:"uid"`
    Role   string `json:"role"`
}

// 2. Generate a token (e.g. in a login handler)
token, err := auth.Generate(&UserClaims{
    RegisteredClaims: jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
    },
    UserID: 42,
    Role:   "admin",
}, auth.WithSecret(cfg.GetString("auth.secret")))

// 3. Protect a route group with middleware
protected := r.Group("/api/v1")
protected.Use(auth.Required(
    auth.WithSecret(cfg.GetString("auth.secret")),
    auth.WithClaims(func() auth.Claims { return &UserClaims{} }),
))

// 4. Extract claims inside a handler
func GetMe(c *gin.Context) {
    claims, ok := auth.FromContext[*UserClaims](c.Request.Context())
    if !ok {
        c.Error(errors.ErrUnAuth)
        return
    }
    c.JSON(200, gin.H{"user_id": claims.UserID, "role": claims.Role})
}
```

> Business code defines the `Claims` struct by embedding `jwt.RegisteredClaims`. The `auth` package never imports the struct — it receives a factory function via `WithClaims`.

---

### `validate`

Request binding and struct validation for gin handlers, backed by [go-playground/validator v10](https://github.com/go-playground/validator). All functions return a structured 400 `CodedError` with field-level details on failure — no raw `err.Error()` strings leak to callers.

**Key API**

| Function | Description |
|---|---|
| `Bind(c, obj)` | Bind by Content-Type (JSON or form) and validate |
| `BindJSON(c, obj)` | Bind JSON body and validate |
| `BindForm(c, obj)` | Bind form data and validate |
| `BindQuery(c, obj)` | Bind query string parameters and validate |
| `BindURI(c, obj)` | Bind URI path parameters and validate |
| `Struct(obj)` | Validate a struct directly (no gin context needed) |
| `Register(tag, fn)` | Register a custom validation tag |

**Error response format**

On validation failure, `BindJSON` (and all `Bind*` variants) return a `CodedError` that the `ErrorHandler` middleware serialises as:

```json
{
  "code": 400,
  "reason": "VALIDATION_FAILED",
  "message": "validation failed",
  "metadata": {
    "email": "required",
    "name": "min"
  }
}
```

**Usage**

```go
// Struct tags: `json`/`form`/`uri` for binding, `validate` for rules.
type CreateUserReq struct {
    Name  string `json:"name"  validate:"required,min=2"`
    Email string `json:"email" validate:"required,email"`
}

// In a handler — replaces c.ShouldBindJSON + manual validation.
func (h *UserHandler) Create(c *gin.Context) {
    var req CreateUserReq
    if err := validate.BindJSON(c, &req); err != nil {
        c.Error(err)
        return
    }
    // req is valid here
}

// Query params for a list endpoint.
type ListReq struct {
    Status string `form:"status" validate:"omitempty,oneof=active inactive"`
}
func (h *UserHandler) List(c *gin.Context) {
    var q ListReq
    if err := validate.BindQuery(c, &q); err != nil {
        c.Error(err)
        return
    }
}

// URI path params.
type GetReq struct {
    ID int64 `uri:"id" validate:"required,min=1"`
}
func (h *UserHandler) Get(c *gin.Context) {
    var p GetReq
    if err := validate.BindURI(c, &p); err != nil {
        c.Error(err)
        return
    }
}
```

---

### `pagination`

Standard page-number pagination helpers for list endpoints. Parses and normalises `page` and `page_size` query parameters, enforces defaults (page=1, page_size=20) and a hard cap (page_size≤100), and wraps results in a typed `Result[T]` envelope.

**Key API**

| Symbol | Description |
|---|---|
| `FromQuery(c) Params` | Parse pagination params from the gin query string |
| `FromParams(page, pageSize) Params` | Construct `Params` from explicit values (service layer) |
| `Offset(p) int` | Return the DB query offset: `(page - 1) * page_size` |
| `New[T](total, data, p) Result[T]` | Wrap a page of results in the standard envelope |

**Response envelope**

```json
{
  "total": 142,
  "page": 2,
  "page_size": 20,
  "data": [ ... ]
}
```

**Usage**

```go
func (h *UserHandler) List(c *gin.Context) {
    p := pagination.FromQuery(c)   // parses ?page=2&page_size=20, applies defaults

    users, err := svc.List(ctx, pagination.Offset(p), p.PageSize)
    if err != nil {
        c.Error(err)
        return
    }
    total, err := svc.Count(ctx)
    if err != nil {
        c.Error(err)
        return
    }

    c.JSON(200, pagination.New(total, users, p))
}
```

---

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
├── cache/                  # Cache client manager (memory + Redis)
│   ├── cache.go            # Cache interface, Init, Client, registry
│   ├── memory.go           # In-memory backend with TTL and cleanup
│   ├── redis.go            # Redis backend with slow-command hook
│   └── options.go          # WithMemory, WithRedis, WithRedisAuth, WithConfig
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
├── httpclient/             # Outbound HTTP client with OTel tracing and retry
│   ├── client.go           # New, Get/Post/Put/Delete, Do, Register, Get, Init
│   ├── options.go          # WithBaseURL, WithTimeout, WithRetry, WithHeader, WithLogger
│   └── retry.go            # shouldRetry, backoffDelay
├── grpcclient/             # Outbound gRPC client with OTel tracing and retry
│   ├── client.go           # Dial, Register, Get, CloseAll, Init
│   ├── options.go          # WithInsecure, WithTLS, WithMTLS, WithTimeout, WithRetry
│   └── interceptors.go     # unaryLoggerInterceptor, unaryRetryInterceptor
├── lock/                   # Distributed lock backed by Redis
│   ├── lock.go             # Locker, New, NewWithClient, Acquire, Release, TryLock
│   ├── options.go          # WithRedisPassword, WithRedisDB, WithLogger, WithWait, WithAutoRenew
│   └── errors.go           # ErrLockNotAcquired, ErrLockExpired
├── cron/                   # Cron scheduler (core.Server)
│   ├── cron.go             # Scheduler, New, Add, Remove, Start, Stop
│   ├── options.go          # WithLogger, WithLocation, WithTimeout, WithRetries, WithSkipIfRunning
│   └── job.go              # executeJob, runOnce, cronBackoff
├── auth/                   # JWT signing/verification + gin middleware
│   ├── auth.go             # Generate, Required, Optional, FromContext
│   └── options.go          # WithSecret, WithKeyFunc, WithExpire, WithIssuer, WithClaims
├── validate/               # Request binding + struct validation
│   └── validate.go         # Bind, BindJSON, BindForm, BindQuery, BindURI, Struct, Register
├── pagination/             # Page-number pagination helpers
│   └── pagination.go       # Params, Result[T], FromQuery, FromParams, Offset, New
└── example/                # Runnable reference application
    ├── main.go
    ├── config.yaml
    └── internal/
        ├── handler/http/   # HTTP route handlers
        ├── model/          # Data models and JWT claims
        ├── repository/     # Database access layer
        └── service/        # Business logic
```
