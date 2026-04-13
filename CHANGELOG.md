# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

---

## [0.3.0] - 2026-04-13

### Breaking Changes

#### All registry packages — `Register`/`Replace` now accept pre-built instances

All registry packages now require a pre-built instance, not options:

| Package | Before | After |
|---------|--------|-------|
| `httpclient` | `Register("x", WithBaseURL(...))` | `c, _ := New(WithBaseURL(...)); Register("x", c)` |
| `storage` | `Register("x", WithS3(...))` | `s, _ := New(WithS3(...)); Register("x", s)` |
| `grpcclient` | `Register("x", "host:port", opts...)` | `conn, _ := Dial("host:port", opts...); Register("x", conn)` |

`database`, `cache`, and `idgen` already accepted instances — no change for them.

#### `httpclient.CloseAll()` now returns `error`

```go
// Before: func CloseAll()
// After:  func CloseAll() error  (always returns nil; consistent with other packages)
```

### Added

#### `core.HealthChecker` interface

```go
type HealthChecker interface {
  Name() string
  Check(ctx context.Context) error
}
```

#### `server/http` — `WithHealthChecks` and `AddReadyCheck`

```go
httpserver.New(httpserver.WithHealthChecks(
  dbMgr.HealthChecker("default"),    // probes SELECT 1
  cacheMgr.HealthChecker("redis"),   // probes PING
))
// Or after construction:
httpSrv.AddReadyCheck("cache", cacheMgr.HealthChecker("redis").Check)
```

#### `app` — `WithHealthCheck` and `HealthCheckers()`

```go
myApp := app.New(app.WithHealthCheck(dbMgr.HealthChecker("default")))
// myApp.HealthCheckers() returns []core.HealthChecker for manual bridging in BeforeStart
```

#### `database.Manager.HealthChecker(connName)` and `cache.Manager.HealthChecker(backendName)`

Return `core.HealthChecker` for `/readyz` integration. Memory backend is always healthy; Redis backend issues PING.

#### `database.CloseAll() error` and `idgen.CloseAll() error`

Complete the base registry contract for all packages. `idgen.CloseAll()` is a no-op (no connections to close) and always returns nil.

### Changed

#### godoc: primary vs convenience paths documented

Each infrastructure package now has explicit "Instance-first (recommended)" and "Convenience (small services)" sections in its package doc.

#### `metric` helpers documented as facade

`Count`/`Gauge`/`StartTimer` are now explicitly documented as facades over the default `Provider`.

#### `logger.Default()` documented as bootstrap convenience

`logger.Default()` / `logger.SetDefault()` are documented as bootstrap helpers; constructor injection is preferred in production.

---

## [0.2.0] - 2026-04-13

This release is a **breaking refactor** moving the library from a convenience-first, global-state-heavy design to an instance-first, lifecycle-governed architecture. There are no external users at the time of this release.

### Breaking Changes

#### `event` — package-level default bus removed

```go
// Before (removed)
event.Subscribe("user.registered", handler)
event.Publish(ctx, "user.registered", payload)

// After
bus := event.New()
bus.Subscribe("user.registered", handler)
bus.Publish(ctx, "user.registered", payload)
```

#### `stream` — package-level global functions removed; `New()` returns `(*Bus, error)`

```go
// Before (removed)
stream.Init(cache.RedisClient("redis"))
stream.Subscribe("order.created", stream.WithHandler(h))
app.WithServer(stream.Worker(log))

// After
bus, err := stream.New(cache.RedisClient("redis"), stream.WithLogger(log))
bus.Subscribe("order.created", stream.WithHandler(h))
app.WithServer(bus.Worker())
```

`stream.New()` returns an error if the Redis client is nil.

#### `cache` — `Init()` removed; `Manager` type introduced

```go
// Before (removed)
cache.Init(cache.WithMemory("default"), cache.WithRedis("redis", addr))

// After
mgr, err := cache.New(cache.WithMemory("default"), cache.WithRedis("redis", addr))
cache.Register("default", mgr)
```

`*cache.Manager` implements `app.Closer`. `Manager.CloseBackend(name)` closes a single backend; `Manager.CloseAll()` closes all.

#### `database` — `New()` no longer writes to global registry; `Register` accepts `*Manager`

```go
// Before (removed — New() wrote to global registry automatically)
_, err := database.New(database.WithDriver("default", "sqlite", dsn))
db := database.DB("default")

// After
mgr, err := database.New(database.WithDriver("default", "sqlite", dsn))
database.Register("default", mgr)
db := mgr.DB("default")           // instance access
db := database.DB("default")      // convenience: Get("default").DB("default")
```

The old `database.Register(name, driver, dsn, opts...)` convenience wrapper is removed.
`*database.Manager` gains `DB(name string) *gorm.DB` and `Close() error` methods.

#### `idgen` — `Next()` / `NextString()` removed; `Register` accepts `*Generator`

```go
// Before (removed)
idgen.Init("default", idgen.WithWorkerID(1))
id := idgen.Next()

// After
gen, err := idgen.New(idgen.WithWorkerID(1))
idgen.Register("default", gen)
id := idgen.Get("default").Next()
```

The lazy-initialized `_default` generator and `sync.Once` are removed.

#### `grpcclient` — `CloseAll()` now returns `error`

#### All registry packages — `Register()` returns `ErrAlreadyRegistered` on duplicate

Previously, duplicate registration silently overwrote the existing entry (risking connection leaks).
Now `Register()` returns `ErrAlreadyRegistered`. Use `Replace()` for intentional overwrites.

Affected packages: `database`, `cache`, `httpclient`, `grpcclient`, `storage`, `idgen`

```go
if err := cache.Register("default", mgr); errors.Is(err, cache.ErrAlreadyRegistered) {
  // already registered — skip or Replace
}
cache.Replace("default", newMgr)  // closes old Manager, stores new one
```

### Added

#### `app` — `Lifecycle` and `Closer` interfaces; `WithLifecycle` / `WithCloser` options

```go
type Lifecycle interface {
  Init(ctx context.Context) error
  Shutdown(ctx context.Context) error
}
type Closer interface {
  Close() error
}
app.WithLifecycle(tracingProvider, metricProvider)
app.WithCloser(cacheManager, grpcclient.NewRegistry())
```

- `Lifecycle.Init()` is called sequentially before servers start; if `Init[i]` fails, already-Init'd components `[0..i-1]` are Shutdown in reverse order before `Run` returns
- `Lifecycle.Shutdown()` is called in reverse registration order after all servers stop
- `Closer.Close()` is called after all `Lifecycle.Shutdown` calls
- Server `Stop()` errors are now logged (previously silently dropped); first stop error is returned if no earlier error occurred

#### `metric` — `Provider` struct implementing `app.Lifecycle`

```go
p, _ := metric.New(metric.WithConfig(cfg))
app.WithLifecycle(p)   // Init() starts exporters + wires middleware hooks; Shutdown() flushes
```

Wire functions (`wireHTTPMetrics`, `wireGRPCMetrics`) are now idempotent via `sync.Once`.
Global `metric.Init()` / `metric.Shutdown()` remain as convenience wrappers.

#### `tracing` — `Init()` added to satisfy `app.Lifecycle`

`*tracing.Provider` now implements `app.Lifecycle` (Init is a no-op; Shutdown flushes spans).

#### `grpcclient` — `Registry` type implementing `app.Closer`

```go
app.WithCloser(grpcclient.NewRegistry())  // calls CloseAll() on shutdown
```

#### All registry packages — `Replace()` function

`Replace(name, instance)` stores a new instance, closing the old resource first (for packages that hold connections).

#### `ARCHITECTURE.md`

New file documenting package layers, dependency direction, global vs instance principles, registry semantics, lifecycle roles, Redis connection sharing, import cycle resolution, and the new package checklist.

### Changed

#### `example/` — split into `quickstart/` and `full/`

- `example/quickstart/` — minimal HTTP service; random port via `net.Listen(":0")`; no external dependencies; demonstrates config, logger, gin routes, and graceful shutdown
- `example/full/` — comprehensive service using all packages with instance-first APIs and `app.WithLifecycle`/`app.WithCloser`

Both are separate Go modules with `replace github.com/pemako/mk => ../../`.

#### `Makefile` — updated example targets

| Target | Action |
|--------|--------|
| `make example-build` | Build both `quickstart` and `full` |
| `make example-run` | Run `quickstart` (random port, no deps) |
| `make example-run-full` | Run `full` example |

---

## [0.1.0] - 2026-04-12

### Added

**Foundation**
- `core` — interface definitions: `Logger`, `Config`, `Server`, `Provider`
- `errors` — typed `CodedError` with HTTP status code, machine-readable reason, and cause chaining; sentinel errors (`ErrBadRequest`, `ErrNotFound`, `ErrInternal`, etc.)
- `config` — multi-format configuration (YAML/TOML/JSON), environment variable override, hot-reload via `Watch`
- `logger` — structured JSON logging (zap + lumberjack); automatic `trace_id`/`span_id` injection from OpenTelemetry context; per-level log file splitting; configurable sensitive field masking
- `app` — application lifecycle manager: database migrations → `BeforeStart` hooks → concurrent server start → OS signal handling → graceful shutdown → `AfterStop` hooks

**Serving**
- `server/http` — Gin-based HTTP server with default middleware stack (RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler); `/healthz`, `/readyz`, `/health` endpoints
- `server/grpc` — gRPC server with interceptor chain; health check service; optional reflection and TLS/mTLS
- `middleware/http` — default stack + optional: CORS, Timeout, BodyLimit, RateLimit (token bucket), IPFilter
- `middleware/grpc` — matching interceptor set for gRPC

**Data**
- `database` — gorm wrapper for MySQL, PostgreSQL, SQLite; SQL tracing plugin; goose migration runner
- `cache` — unified `Cache` interface; in-memory backend (default); Redis backend (optional); named registry

**Observability**
- `tracing` — OpenTelemetry OTLP HTTP provider; no-op when endpoint not configured; implements `core.Provider`
- `metric` — Prometheus + OTLP dual export; automatic HTTP/gRPC metrics; `Count`, `Gauge`, `StartTimer` helpers

**Clients & utilities**
- `httpclient` — outbound HTTP with otelhttp tracing, exponential-backoff retry, named registry
- `grpcclient` — gRPC connection manager; otelgrpc; per-call retry; TLS/mTLS; named registry
- `lock` — Redis distributed lock; atomic Lua acquire/release; auto-renewal; wait-with-timeout
- `cron` — cron scheduler implementing `core.Server`; per-job retry, timeout, skip-if-running, sub-second tickers
- `auth` — JWT signing/verification; `Required`/`Optional` gin middleware; typed claims via generics
- `validate` — request binding + struct validation; structured 400 errors with field-level metadata
- `pagination` — page-number `Params` and typed `Result[T]` envelope
- `event` — in-process pub/sub; synchronous (default) or async delivery
- `stream` — Redis Streams pub/sub; work-queue and broadcast modes
- `circuitbreaker` — three-state circuit breaker; `Execute(ctx, fn)`
- `storage` — unified object storage: AWS S3, Alibaba OSS, Tencent COS
- `notify` — multi-channel notifications: Email (SMTP), SMS, DingTalk, WeCom, Feishu
- `pprof` — pprof endpoints; standalone server or mounted on gin.Engine
- `idgen` — Snowflake ID generator + ULID; named registry; auto-derives worker ID from host IP
- `ratelimit` — `Limiter` interface; in-process token-bucket or Redis fixed-window
- `testutil` — test helpers: `FreePort`, `NewConfig`, `NewLogger`

**Tooling**
- `example/` — runnable demo service demonstrating all packages
- `CLAUDE.md` — Claude Code guidance for this repository
- `Makefile` — development commands (`test`, `test-race`, `lint`, `vet`, `fmt`, `tidy`)
- `.golangci.yml` — golangci-lint configuration
- `.github/workflows/ci.yml` — GitHub Actions CI (test + lint + example build)

---

[Unreleased]: https://github.com/pemako/mk/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/pemako/mk/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/pemako/mk/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/pemako/mk/releases/tag/v0.1.0
