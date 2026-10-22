# Forge

Forge is a Go toolkit for building production-grade backend services with batteries-included infrastructure components and consistent engineering conventions.

`forge` provides a cohesive set of packages — config, logger, tracing, metrics, database, cache, servers, and middleware — all wired together through a common interface layer and a lifecycle-managing `app` runner.

## Getting Started

**Task-oriented recipes**: See [`docs/getting-started.md`](./docs/getting-started.md) for four scenario-based guides:

- Minimal HTTP service (what's the least you need to know)
- HTTP + database + Redis (recommended setup)
- Pure worker service (cron / stream consumer, no HTTP)
- What to ignore until you need it

## Breaking Changes

Read this before upgrading. Each item is covered in detail in its package section below.

| Change | What breaks | What to do |
| ------ | ----------- | ---------- |
| `core.Config` gained `IsSet(key string) bool` | Any external implementation of `core.Config` stops compiling | Add the method; it distinguishes "absent" from "present but zero" |
| `core.Closer` runs in **reverse** registration order | `WithCloser(a, b)` now closes `b` first, not `a` | Usually a fix, not a regression — it matches `Lifecycle` nesting. Re-check ordering assumptions |
| `metric.Provider.Init` returns `ErrNoExporters` when no exporter is configured | A service with an empty `metric:` block now fails at startup | Configure `metric.prometheus_addr` / `metric.otlp_endpoint`, or pass `metric.WithAllowNoExporters()` |
| `metric.Provider.Init` binds the Prometheus listener synchronously | A port clash now fails startup with `ErrPrometheusListen` | Previously `/metrics` was quietly unserved — fix the clash |
| gRPC health service starts `NOT_SERVING` | A `server/grpc` used without `app` is never picked by health-aware clients | Let `app` drive it, or call `grpcSrv.SetReady(true)` yourself |
| gRPC codes for HTTP 408, 412, 413, 416, 422, 499, 501, 502, 503, 504 | Clients switching on gRPC codes see new values (these all used to be `Internal`) | Review client-side code switches |
| gRPC metric `status` label is the real code name | Dashboards and alerts matching `status="ERROR"` match nothing | Rewrite them against `"NotFound"`, `"Unavailable"`, … |
| `RegisterHTTPMetricHooks` / `RegisterGRPCMetricHooks` gained an `errorsInc` parameter | Direct callers stop compiling | Pass the extra callback (only `metric` calls these in-tree) |
| `tracing.New` installs the global W3C propagator unconditionally | Traces start connecting across services where they previously did not | Nothing — this was broken before. Call `tracing.New()` even when tracing is off |
| `Config.Watch` returns `ErrWatchDisabled` without `WithWatch()` | A `Watch` call that silently did nothing now errors | Add `config.WithWatch()` at Load, or stop calling `Watch` |
| `cache.New` / `database.New` return `ErrInvalidConfig` on a malformed block | A typo'd config block now fails instead of yielding zero backends | Fix the block; check the returned error |
| `app.WithHealthCheck` auto-registers probes | The old `BeforeStart` bridging hook registers every probe a second time | Delete the hook |
| `app` drives readiness | `httpSrv.SetReady(true)` in `BeforeStart` advertises readiness too early | Delete that call; `app` handles it |


## Design Philosophy

`forge` is **instance-first**: prefer creating explicit instances over package-level singletons.

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

`forge` organizes its packages into four tiers with different stability contracts:

| Tier                            | Packages                                                                                                                              | Stability                                                                                                                                     |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| **Core Stable**                 | `core`, `errors`, `app`, `config`, `logger`                                                                                           | Interfaces and behavior are frozen. Breaking changes require a major version bump.                                                            |
| **Infrastructure Integrations** | `server/*`, `middleware/*`, `database`, `cache`, `tracing`, `metric`, `httpclient`, `grpcclient`, `lock`, `stream`, `cron`, `storage` | Stable APIs; additive changes only. External dependencies (Redis, OTLP, gRPC) may constrain upgrades.                                         |
| **Opinionated Extensions**      | `auth`, `validate`, `pagination`, `ratelimit`, `notify`, `idgen`, `circuitbreaker`, `pprof`                                           | Usable in production but opinionated. Teams may prefer their own implementations. Minor breaking changes are possible between minor versions. |
| **Developer Utilities**         | `testutil`, `event`                                                                                                                   | Convenience helpers with no cross-service API surface. `event` is in-process only; use `stream` for cross-instance coordination.              |

> **Practical guidance:** Start with Core Stable packages. Add Infrastructure Integrations for external connections. Use Opinionated Extensions only if they fit your conventions — they are easy to replace.

## Features

### Core Stable

| Package  | Description                                                                                                                                                        |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `core`   | Shared interfaces: `Logger`, `Config`, `Server`, `Provider`, `HealthChecker`, `Metrics`, plus the lifecycle contracts `Migrator`, `Lifecycle`, `Closer`, `ReadinessToggler`, `ReadyCheckRegistrar` |
| `errors` | Structured `CodedError` with HTTP code, machine-readable reason, and cause chaining; generic `AsType[T]` extraction                                                |
| `app`    | Lifecycle runner: health-probe wiring → `Migrator.Up` → `Lifecycle.Init` → `BeforeStart` → concurrent servers → drain → `Stop` → `Lifecycle.Shutdown` → `Closer.Close` → `AfterStop`; panic-safe; `RunContext` / `Stop()` for programmatic shutdown |
| `config` | Viper-backed config loader with file, env-var, and hot-reload support; `WithRequired` fail-fast and `WithKnownKeys` typo detection                                 |
| `logger` | `log/slog`-backed structured logger with trace injection, field masking, and level-split file output                                                              |

### Infrastructure Integrations

| Package           | Description                                                                                                                                                                     |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `server/http`     | Gin HTTP server; default middleware stack pre-mounted; `/healthz`, `/readyz`, `/health` auto-registered; implements `core.ReadinessToggler` + `core.ReadyCheckRegistrar` so `app` drives readiness         |
| `server/grpc`     | gRPC server with interceptor chain, optional TLS/mTLS, and a health service that starts `NOT_SERVING` until `SetReady(true)`; `Stop(ctx)` force-closes past the deadline                                   |
| `middleware/http` | RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler, CORS, RateLimit, IPFilter, BodyLimit, Timeout; errors are recorded on the span, counted, and logged by status class                           |
| `middleware/grpc` | UnaryRequestID, UnaryLogger, UnaryMetrics, UnaryRecovery, UnaryErrorHandler, UnaryRateLimit, UnaryIPFilter, UnaryTimeout; 17-code HTTP→gRPC mapping with `Reason`/`Metadata` carried as `google.rpc.ErrorInfo` |
| `tracing`         | OpenTelemetry OTLP trace provider; implements `app.Lifecycle`; no exporter when endpoint is unset, but the W3C propagator is always installed                                                              |
| `metric`          | OTel metrics with Prometheus and OTLP exporters; implements `app.Lifecycle` and `core.Metrics`; automatic HTTP/gRPC instrumentation; `Init` errors when no exporter is configured                          |
| `database`        | GORM connection manager for MySQL, Postgres, SQLite; SQL tracing; goose migrations; `*Manager` implements `core.Migrator`; `CloseAll() error`; `Manager.HealthChecker(connName)`; `ErrInvalidConfig`       |
| `cache`           | `Cache` interface; `Manager` holds named backends (memory/Redis); `Manager` implements `core.Closer`; `Manager.HealthChecker(backendName)`; `ErrInvalidConfig`                                             |
| `httpclient`      | Outbound HTTP with OTel tracing, exponential-backoff retry, and named registry; `Register(name, *Client)` / `Replace` / `CloseAll() error`                                      |
| `grpcclient`      | Outbound gRPC with OTel tracing, per-call retry, named registry; `Register(name, *ClientConn)` / `Replace`; `Registry` implements `core.Closer`                                  |
| `lock`            | Redis distributed lock; atomic Lua acquire/release; optional auto-renew and wait-with-timeout                                                                                   |
| `stream`          | Redis Streams pub/sub `Bus`; work-queue (`WithGroup`) and broadcast (`WithBroadcast`) modes                                                                                     |
| `cron`            | Cron scheduler (`core.Server`); per-job retry, timeout, skip-if-running, sub-second tickers                                                                                     |
| `storage`         | Unified object storage interface for AWS S3, Alibaba OSS, and Tencent COS; `Register(name, Storage)` / `Replace` / `CloseAll() error`                                           |

### Opinionated Extensions

| Package          | Description                                                                                                                          |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `auth`           | JWT signing/verification; `Required`/`Optional` gin middleware; typed claims via generics                                            |
| `validate`       | go-playground/validator binding; structured 400 errors with field-level metadata                                                     |
| `pagination`     | Page-number `Params` and typed `Result[T]` envelope for list endpoints                                                               |
| `ratelimit`      | `Limiter` interface; in-process token-bucket or Redis fixed-window; backs HTTP/gRPC middleware                                       |
| `circuitbreaker` | Circuit breaker (Closed → Open → HalfOpen); `Execute(ctx, fn)`                                                                       |
| `notify`         | Multi-channel notifications: Email (SMTP), SMS (Aliyun), DingTalk, WeCom, Feishu                                                     |
| `idgen`          | Snowflake ID generator + UUIDv7/v4; `New()` → `Register("name", gen)` → `Get("name").Next()`; `CloseAll() error` (no-op, clears registry) |
| `pprof`          | pprof endpoints; standalone `core.Server` or mounted on an existing gin.Engine                                                       |

### Developer Utilities

| Package    | Description                                                                                 |
| ---------- | ------------------------------------------------------------------------------------------- |
| `event`    | In-process pub/sub `Bus`; synchronous default, optional async; no package-level state       |
| `testutil` | Test helpers: `FreePort`, `NewConfig` (stub `core.Config`), `NewLogger` (writes to `t.Log`) |

## Installation

```bash
go get github.com/pemako/forge
```

Requires Go 1.26 or later.

## Quick Start

```go
package main

import (
  "context"
  "os"
  "time"

  "github.com/pemako/forge/app"
  "github.com/pemako/forge/config"
  "github.com/pemako/forge/database"
  "github.com/pemako/forge/logger"
  "github.com/pemako/forge/metric"
  grpcserver "github.com/pemako/forge/server/grpc"
  httpserver "github.com/pemako/forge/server/http"
  "github.com/pemako/forge/tracing"
)

func main() {
  cfg, _ := config.Load(
    config.WithFile("config.yaml"),
    config.WithEnv("APP"),
    config.WithRequired("database.default.dsn"), // fail fast, naming every missing key
  )
  log, _ := logger.New(logger.WithConfig(cfg))
  logger.SetDefault(log)

  dbMgr, _ := database.New(database.WithConfig(cfg), database.WithLogger(log))
  database.Register("default", dbMgr)

  // tracing.Provider and metric.Provider implement app.Lifecycle:
  // Init() is called before servers start, Shutdown() after they stop.
  // tracing.New() also installs the global W3C trace propagator, even with no
  // exporter configured — call it unconditionally or cross-service traces break.
  tracingProvider, _ := tracing.New(tracing.WithConfig(cfg), tracing.WithLogger(log))
  // metric.Init fails with ErrNoExporters unless metric.prometheus_addr or
  // metric.otlp_endpoint is configured; pass WithAllowNoExporters to opt out.
  metricProvider, _ := metric.New(metric.WithConfig(cfg))

  httpSrv, _ := httpserver.New(httpserver.WithConfig(cfg), httpserver.WithLogger(log))
  grpcSrv, _ := grpcserver.New(grpcserver.WithConfig(cfg), grpcserver.WithLogger(log))

  // Register routes on httpSrv.Engine() and gRPC services on grpcSrv.Server().
  // httpserver.New already mounts: RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler.
  // Do NOT call r.Use(mw.ErrorHandler()) again — it would cause duplicate responses.

  if err := app.New(
    app.WithName(cfg.GetString("app.name")),
    app.WithVersion("v1.0.0"),
    app.WithServer(httpSrv, grpcSrv),
    app.WithLogger(log),
    app.WithMigration(dbMgr),
    app.WithLifecycle(tracingProvider, metricProvider),
    // Registered on every server that accepts readiness probes, before startup.
    app.WithHealthCheck(dbMgr.HealthChecker("default")),
    // Readiness itself is driven by app: servers are marked ready once they have
    // all started, and not-ready at the start of shutdown. Do NOT call
    // httpSrv.SetReady(true) in a BeforeStart hook.
    app.WithDrainDelay(5 * time.Second), // let load balancers notice /readyz failing
  ).Run(); err != nil {
    logger.Error(context.Background(), "fatal", logger.F("err", err))
    os.Exit(1)
  }
}
```

Place a `config.yaml` next to the binary and run. The app handles `SIGTERM`/`SIGINT` for graceful shutdown; a second signal abandons the remaining waits.

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
    ui_addr: '' # grpcui listen address; only used when WithGRPCUI is set

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
  endpoint: '' # e.g. http://jaeger:4318; leave empty to export nothing
  sampler: 1.0 # sampling ratio: 1.0 = 100%
  tls: false # enable TLS for the OTLP exporter

# Metrics — at least one exporter is REQUIRED unless you pass
# metric.WithAllowNoExporters(); otherwise Init returns metric.ErrNoExporters
metric:
  prometheus_addr: :9091 # expose /metrics; omit to disable Prometheus
  otlp_endpoint: '' # e.g. http://collector:4318; omit to disable OTLP metrics

# Database connections — scanned keys are database.default / .read / .write
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

# ID generation — see the idgen section; set this explicitly in production
idgen:
  worker_id: 42

# JWT signing and verification
auth:
  secret: 'change-me' # HMAC signing key
  expire: 24h # token lifetime
  issuer: my-service # "iss" claim

# Redis distributed lock — or pass cache.RedisClient("redis") to NewWithClient
# instead and skip these keys entirely
lock:
  addr: 'redis:6379'
  password: ''
  db: 0

# Rate limiter, when constructed with ratelimit.WithConfig. The HTTP and gRPC
# middleware do NOT read these — they take Rate/Burst from their own config structs.
ratelimit:
  rate: 100 # requests per second per key
  burst: 10 # burst above the sustained rate (in-process limiter)
  window: 1s # window for the distributed Redis limiter

# Object storage — one backend per storage.New call
storage:
  provider: s3 # s3 | oss | cos
  region: us-east-1
  bucket: my-bucket
  access_key: ''
  secret_key: ''
  endpoint: '' # required for OSS/COS; optional S3-compatible override

# Notification channels — configure only the ones you use
notify:
  email:
    host: smtp.example.com
    port: 587
    username: ''
    password: ''
    from: 'noreply@example.com'
  sms: # Aliyun SMS
    access_key: ''
    secret_key: ''
    region: cn-hangzhou
    sign_name: ''
    template_code: ''
  dingtalk_webhook: ''
  wecom_webhook: ''
  feishu_webhook: ''

# Outbound gRPC clients, read by grpcclient.Init(cfg, "user", "order").
# Each sub-key is a connection name; Init registers one *grpc.ClientConn per name.
grpcclient:
  user:
    addr: 'user-service:9090'
    insecure: true
    timeout: 5s
    retry: 3

# pprof profiling endpoints
pprof:
  addr: :6060 # standalone server address; or mount on an existing gin.Engine
  block_rate: 0 # runtime.SetBlockProfileRate; 0 disables
  mutex_fraction: 0 # runtime.SetMutexProfileFraction; 0 disables
```

Environment variables override any YAML key. With `WithEnv("APP")`, the variable `APP_SERVER_HTTP_ADDR` overrides `server.http.addr`.

`app` has no `WithConfig` — its options (`WithName`, `WithVersion`,
`WithShutdownTimeout`, `WithDrainDelay`, …) are set in code, not from YAML. Pass
`cfg.GetString("app.name")` yourself if you want them config-driven.

### Making missing configuration fail loudly

A missing key reads back as a zero value from every `Get*` method, so declare what
the service cannot start without:

```go
cfg, err := config.Load(
  config.WithFile("config.yaml"),
  config.WithEnv("APP"),
  // Load fails with ErrMissingRequiredKey naming EVERY missing key at once.
  // A key supplied only via the environment satisfies this; present-but-empty
  // counts as missing.
  config.WithRequired("database.default.dsn", "cache.redis.addr", "auth.secret"),
  // One Warn listing keys nothing declared — in practice a typo such as
  // "databse.default.dsn". A warning, never fatal.
  config.WithKnownKeys("app", "server", "logger", "tracing", "metric", "database", "cache", "idgen"),
)
if err != nil {
  log.Fatal(err) // one restart diagnoses the whole misconfiguration, not one key per restart
}

// Escalate undeclared keys to an error yourself if you want to:
if extra := cfg.UnknownKeys([]string{"app", "server"}); len(extra) > 0 { ... }

// Tell "absent" apart from "present but zero" — GetInt returns 0 for both a
// missing key and `port: 0`.
if cfg.IsSet("server.http.port") { ... }
```

## Package Reference

### `core`

Shared interfaces that prevent import cycles across packages.

| Interface               | Description                                                                 |
| ----------------------- | --------------------------------------------------------------------------- |
| `Logger`                | `Debug/Info/Warn/Error(ctx, msg, ...Field)` + `With(...Field) Logger`       |
| `Config`                | `Get*`, `IsSet`, `Scan`, `Watch`                                            |
| `Server`                | `Start(ctx) error`, `Stop(ctx) error`                                       |
| `Provider`              | `Tracer(name) trace.Tracer`, `Shutdown(ctx) error`                          |
| `Metrics`               | `Count(ctx, name, n, ...Field)`, `Gauge(ctx, name, v, ...Field)`, `Histogram(ctx, name, v, ...Field)` |
| `HealthChecker`         | `Name() string`, `Check(ctx context.Context) error`                         |
| `Migrator`              | `Up(ctx) error` — migrations, before any server starts                      |
| `Lifecycle`             | `Init(ctx) error`, `Shutdown(ctx) error`                                    |
| `Closer`                | `Close() error` — after every `Lifecycle.Shutdown`                          |
| `ReadinessToggler`      | `SetReady(bool)` — optional on a `Server`; `app` drives it                  |
| `ReadyCheckRegistrar`   | `AddReadyCheck(name, func(ctx) error)` — optional on a `Server`             |

`core.Field` is `{Key string, Value any}`. Use `logger.F(key, value)` as shorthand.

> ⚠️ **Breaking:** `core.Config` gained `IsSet(key string) bool`. Any external
> implementation of `core.Config` must add the method before it compiles again.
> It exists because every `Get*` method returns a zero value for a missing key,
> which makes "absent" and "configured as empty" indistinguishable.

**The lifecycle contracts live in `core`, not `app`.** `Migrator`, `Lifecycle`,
`Closer`, `ReadinessToggler` and `ReadyCheckRegistrar` are declared in
`core/lifecycle.go`. A library package may not import `app`, so only the `core`
spelling can be asserted at compile time:

```go
var _ core.Lifecycle = (*MyProvider)(nil)
```

`app` keeps type aliases (`app.Lifecycle = core.Lifecycle`, and likewise for the
rest), so existing call sites are unaffected.

`core.Metrics` closes the matching gap on the metrics side: `tracing` always had
`core.Provider` to depend on, while `metric` offered only package-level globals that
tests could neither observe nor reset. `*metric.Provider` implements `core.Metrics`.

---

### `config`

Viper-backed configuration loader.

```go
cfg, err := config.Load(
  config.WithFile("config.yaml"),
  config.WithEnv("APP"),    // APP_SERVER_HTTP_ADDR → server.http.addr
  config.WithWatch(),       // live-reload on file change
  config.WithRequired("database.default.dsn"),  // Load fails naming every missing key
  config.WithKnownKeys("app", "server", "database"), // Warn on undeclared keys
  config.WithLogger(log),   // logger used for that warning
)
cfg.GetString("app.name")
cfg.IsSet("server.http.port")      // "absent" vs "present but zero"
cfg.Scan("database.default", &dbCfg)
cfg.UnknownKeys([]string{"app", "server"})  // escalate typos to an error yourself

// Watch returns ErrWatchDisabled unless WithWatch() was passed — it used to
// accept a callback it could never invoke.
if err := cfg.Watch("logger.level", func(v any) { /* react */ }); err != nil { ... }
```

| Sentinel                       | Returned by | When                                                       |
| ------------------------------ | ----------- | ---------------------------------------------------------- |
| `config.ErrMissingRequiredKey` | `Load`      | a `WithRequired` key is absent; the message names them all |
| `config.ErrWatchDisabled`      | `Watch`     | loaded without `WithWatch()`                               |

`WithKnownKeys` warns rather than errors on purpose: config files legitimately carry
keys this library knows nothing about — per-deployment annotations, settings consumed
by application code, blocks staged for a future release.

---

### `logger`

Structured JSON logger backed by the standard library's `log/slog`. Auto-injects `trace_id`/`span_id` from an active OTel span.

```go
log, _ := logger.New(logger.WithConfig(cfg))
logger.SetDefault(log) // optional bootstrap convenience

log.Info(ctx, "user created", logger.F("user_id", 42))
log.With(logger.F("component", "auth")).Warn(ctx, "token expired")
```

When `output: file`, writes per-level files: `.debug.log`, `.info.log`, `.warn.log`, `.error.log`.

---

### `tracing`

OpenTelemetry OTLP trace provider. Implements `core.Provider` and `app.Lifecycle`.

```go
// New() does all setup; no exporter is created when none is configured.
p, _ := tracing.New(
  tracing.WithConfig(cfg),
  tracing.WithLogger(log),      // log exporter errors
  tracing.WithOTLP("http://jaeger:4318"),
  tracing.WithSampler(0.1),
  tracing.WithTLS(),            // enable TLS for the OTLP exporter
)
app.WithLifecycle(p)              // Init() no-op; Shutdown() flushes spans
```

> ⚠️ **Call `tracing.New()` even when tracing is "off".** `New` installs the global
> OTel text map propagator — W3C TraceContext (`traceparent` / `tracestate`) plus W3C
> Baggage — **unconditionally**, before the no-exporter early return. Previously
> nothing was installed unless an exporter existed, so inbound `traceparent` headers
> were discarded and nothing was injected outbound: distributed propagation was
> entirely non-functional.

Propagation and export are independent concerns. A process that exports nothing
itself (sampler 0, no collector, a thin proxy, a test binary) must still forward
inbound trace context to its downstream calls, or those downstream spans start
brand-new disconnected traces.

```go
// Override the set only to interoperate with a different wire format,
// e.g. B3 for a legacy Zipkin mesh. Passing none leaves the current global intact.
tracing.New(tracing.WithPropagators(propagation.TraceContext{}, b3.New()))
```

---

### `metric`

OTel metrics with Prometheus and OTLP exporters. Implements `app.Lifecycle` and `core.Metrics`.

```go
p, _ := metric.New(metric.WithConfig(cfg))
app.WithLifecycle(p)   // Init() starts exporters and wires middleware hooks; Shutdown() flushes

// Business helpers — package-level facade, takes metric.Label(...) options
metric.Count(ctx, "orders.created", 1, metric.Label("region", "us"))
metric.Gauge(ctx, "queue.depth", 42)
stop := metric.StartTimer(ctx, "db.query.duration")
defer stop()

// core.Metrics methods on the instance — these take core.Field labels
p.Count(ctx, "orders.created", 1, logger.F("region", "us"))
p.Gauge(ctx, "queue.depth", 42)
p.Histogram(ctx, "db.query.seconds", 0.042)
```

Options: `WithServiceName`, `WithPrometheusAddr`, `WithOTLPEndpoint`,
`WithAllowNoExporters`, `WithReader`, `WithConfig`. There is **no** `metric.WithLogger`.

`Gauge` uses **set** semantics — `Gauge(ctx, "q", 5)` twice reads 5, not 10. Use
`GaugeAdd`/`GaugeSub` for deltas. (Both `metric.Gauge` and `p.Gauge` previously
documented "sets" while accumulating.)

To assert on metrics in a test, register a manual reader:

```go
reader := sdkmetric.NewManualReader()
p, _ := metric.New(metric.WithReader(reader))
_ = p.Init(ctx)

p.Count(ctx, "jobs_total", 2)

var rm metricdata.ResourceMetrics
_ = reader.Collect(ctx, &rm)   // inspect rm.ScopeMetrics
```

> ⚠️ **Breaking: `Init` now fails when no exporter is configured.** A
> `MeterProvider` with no reader accepts every measurement and discards it, so a
> service whose metric config failed to load looked healthy while emitting nothing.
>
> ```go
> // Either configure metric.prometheus_addr / metric.otlp_endpoint, or opt out:
> p, _ := metric.New(metric.WithAllowNoExporters())  // tests, CLI builds
> ```

| Sentinel                    | When                                                                |
| --------------------------- | ------------------------------------------------------------------- |
| `metric.ErrNoExporters`     | neither Prometheus nor OTLP configured and no `WithAllowNoExporters` |
| `metric.ErrPrometheusListen`| the `/metrics` listener cannot bind its address                      |

The Prometheus listener now binds **synchronously** inside `Init`, so a port clash
fails startup instead of leaving `/metrics` permanently unserved while the service
reported itself healthy.

**Multiple providers now work.** HTTP and gRPC middleware metrics are wired when
`p.Init()` is called, and the wiring is no longer one-shot — the hook set is swapped
atomically and the most recent `Init` wins. Previously a `sync.Once` guard meant a
second provider (another app instance, or the next test in the same binary) was
silently ignored and kept reporting through the first one's instruments, under the
first one's service name.

Instruments installed per service name: `*_http_requests_total`,
`*_http_request_duration_seconds`, `*_http_requests_in_flight`, `*_http_errors_total`,
and the four `*_grpc_*` equivalents.

For tests, `middleware/http.ResetHTTPMetricHooks()` and
`middleware/grpc.ResetGRPCMetricHooks()` restore the no-op hooks.

**To record metrics from a library package, depend on `core.Metrics`** rather than
calling the package-level facade — that is what makes it stubbable in tests.

---

### `app`

Lifecycle manager. Coordinates component initialization and graceful shutdown.

```go
app.New(
  app.WithName("my-service"),
  app.WithVersion("v1.0.0"),          // both appear in the app: starting / shutdown log lines
  app.WithLogger(log),
  app.WithServer(httpSrv, grpcSrv, cronScheduler, streamWorker),
  app.WithMigration(dbManager),       // Up() called before Lifecycle.Init
  app.WithLifecycle(tracingProvider, metricProvider), // Init before servers, Shutdown after (reverse)
  app.WithCloser(cacheManager, grpcclient.NewRegistry()), // Close after Lifecycle.Shutdown (reverse)
  app.WithHealthCheck(               // auto-registered as /readyz probes on every
    dbMgr.HealthChecker("default"),  // server implementing core.ReadyCheckRegistrar
    cacheMgr.HealthChecker("redis"),
  ),
  app.WithBeforeStart(func(ctx) error { return warmCaches(ctx) }),
  app.WithAfterStop(func(ctx) error { return nil }),
  app.WithShutdownTimeout(30 * time.Second),
  app.WithDrainDelay(5 * time.Second), // pause between not-ready and stopping servers
).Run()
```

**Run() lifecycle order:**

| Phase | Action                                                                                           |
| ----- | ------------------------------------------------------------------------------------------------ |
| 1     | `WithHealthCheck` probes registered on every `core.ReadyCheckRegistrar` server — before any starts |
| 2     | `Migrator.Up()`                                                                                  |
| 3     | `Lifecycle.Init()` — sequential; on failure, reverse-rollback **and** run Closers                |
| 4     | `BeforeStart` hooks — sequential; first error aborts (same rollback)                              |
| 5     | `Server.Start()` — all servers concurrent; then all marked ready                                 |
| 6     | Block on SIGTERM/SIGINT, `App.Stop()`, ctx cancel, or server error                                |
| 7     | Servers marked **not** ready, then `WithDrainDelay` waited out                                    |
| 8     | `Server.Stop()` — concurrent, within the shutdown budget                                          |
| 9     | `Lifecycle.Shutdown()` — reverse registration order                                               |
| 10    | `Closer.Close()` — **reverse** registration order                                                 |
| 11    | `AfterStop` hooks                                                                                 |

**Programmatic shutdown:**

```go
a := app.New(app.WithServer(srv))
go func() { _ = a.RunContext(ctx) }()  // cancelling ctx requests graceful shutdown
a.Stop()                              // or ask directly; idempotent, safe before Run
```

`Run()` is `RunContext(context.Background())`. `Stop()` returns immediately and `Run`
returns once the shutdown completes. Cancelling the `RunContext` context requests the
same graceful shutdown a signal would — the shutdown sequence itself is **not** bound
by that context, so cancelling it does not cut the drain short.

**Sentinels:**

| Sentinel                | When                                                                           |
| ----------------------- | ------------------------------------------------------------------------------ |
| `app.ErrPanic`          | a lifecycle callback, hook, or server goroutine panicked                        |
| `app.ErrServersExited`  | every `Server.Start` returned with no shutdown requested (`Start` must block)   |

**Robustness guarantees:**

- **Readiness is driven by app.** Servers implementing `core.ReadinessToggler` are
  marked ready once all servers have started and not-ready at the start of shutdown.
  Do **not** call `httpSrv.SetReady(true)` in a `BeforeStart` hook — that advertises
  readiness before the listener is up and fights `app` for the flag.
- **Health checkers need no manual bridging.** `WithHealthCheck` used to store the
  checkers and wire nothing, so callers copied them across in a `BeforeStart` hook.
  That hook is now redundant — remove it, or every probe is registered twice.
- ⚠️ **Closers run in reverse registration order** (was forward), matching
  `Lifecycle`, so a component may depend on one registered before it. Each `Close` is
  bounded by the shutdown budget, so a hung `Close` leaks one goroutine instead of
  hanging the process forever.
- **Startup failure rolls back Lifecycles *and* runs Closers.** A `BeforeStart`
  failure previously returned with every pool and client already opened still held.
- **Signals are trapped before migrations.** A SIGTERM during a migration used to hit
  the default disposition and kill the process mid-migration.
- **A second SIGTERM/SIGINT abandons the remaining waits**, so an operator is never
  stuck with a process that ignores signals.
- **Each post-server phase gets its own deadline**, floored at 5s (capped by
  `WithShutdownTimeout`), so a slow drain cannot starve telemetry flushing.
- **Panics are recovered** in every callback, hook and server goroutine, logged with
  their stack, and reported as `app.ErrPanic`; the remaining shutdown phases still run.
- `app` has no `WithConfig` — these options are set in code, not from YAML.

Keep `WithDrainDelay + WithShutdownTimeout` below Kubernetes'
`terminationGracePeriodSeconds`, and set the drain delay to roughly
`periodSeconds × failureThreshold` of your readiness probe.

---

### `errors`

```go
// Define package-level sentinels
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")

// Wrap with context
return errors.Wrap(ErrUserNotFound, "get user %d: %w", id, dbErr)

// Check
if errors.Is(err, ErrUserNotFound) { ... }

// Extract — AsType avoids the declare-then-pass-a-pointer dance that As needs
if ce, ok := errors.AsType[*errors.CodedError](err); ok { fmt.Println(ce.Code, ce.Reason) }

// Still available
var ce *errors.CodedError
if errors.As(err, &ce) { ... }
```

`ErrorHandler` middleware (HTTP/gRPC) auto-converts `CodedError` to structured responses. Non-`CodedError` returns a generic 500.

Builtin sentinels: `ErrBadRequest` (400), `ErrUnauthorized` (401), `ErrForbidden` (403), `ErrNotFound` (404), `ErrConflict` (409), `ErrTooMany` (429), `ErrInternal` (500).

**The cause chain never reaches the client — but it is no longer lost.** `errors.Wrap`
context is withheld from both the HTTP body and the gRPC status message. It is now
recorded on the active span and written as `error` / `error_reason` log fields by the
Logger middleware. When debugging a 500, read the log line or the span.

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

// Add more ready checks after construction
srv.AddReadyCheck("cache", cacheMgr.HealthChecker("redis").Check)
```

Auto-registered endpoints: `/healthz`, `/readyz`, `/health`

`*ginServer` implements `core.ReadinessToggler` and `core.ReadyCheckRegistrar`, so
`app` marks it ready after all servers start, not-ready at the start of shutdown, and
registers `app.WithHealthCheck` probes before startup. **Do not call
`srv.SetReady(true)` from a `BeforeStart` hook** — that advertises readiness before
the listener is up and fights `app` for the flag. Call it yourself only when running
the server without `app`.

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

> ⚠️ **Breaking: the health service now starts `NOT_SERVING`.** It used to report
> `SERVING` from `New()` — i.e. before migrations and `Lifecycle.Init` had run, so
> clients were routed to an instance that could not yet serve.
>
> `*grpcServer` implements `core.ReadinessToggler`, so `app` flips it to `SERVING`
> once every server has started and back to `NOT_SERVING` at the start of shutdown.
> **A server used without `app` must call `grpcSrv.SetReady(true)` itself**, or
> health-aware load balancing will never pick it.

`Stop(ctx)` now honours its context: `GracefulStop` races the deadline, and remaining
connections are force-closed (returning `ctx.Err()`) once it expires. `GracefulStop`
alone has no deadline and never returns while a long-lived stream is open. The grpcui
teardown goroutine — previously dead code, since it received a context that was never
cancelled — now actually fires.

---

### `middleware/grpc`

Pre-chained by `server/grpc`: RequestID → Logger → Metrics → Recovery → ErrorHandler.

`UnaryErrorHandler` translates handler errors into gRPC statuses:

- errors already carrying a gRPC status pass through unchanged;
- `*errors.CodedError` goes through the HTTP→gRPC code table, with `Reason` and
  `Metadata` attached as a `google.rpc.ErrorInfo` detail under
  `grpcMiddleware.ErrorInfoDomain` (`"forge"`);
- `context.DeadlineExceeded` / `context.Canceled` map explicitly, preserving the
  retryability signal `codes.Internal` would lose;
- anything else becomes `codes.Internal` with a generic message.

```go
// Clients should dispatch on the stable reason token, not the message:
for _, d := range status.Convert(err).Details() {
  if info, ok := d.(*errdetails.ErrorInfo); ok && info.Domain == grpcMiddleware.ErrorInfoDomain {
    switch info.Reason {
    case "USER_NOT_FOUND": ...
    }
  }
}
```

> ⚠️ **Breaking: the code table grew from 7 entries to 17.** HTTP 408, 412, 413, 416,
> 422, 499, 501, 502, 503 and 504 previously all fell through to `codes.Internal`.
> They now map to `DeadlineExceeded`, `FailedPrecondition`, `ResourceExhausted`,
> `OutOfRange`, `InvalidArgument`, `Canceled`, `Unimplemented`, `Unavailable`,
> `Unavailable` and `DeadlineExceeded` respectively. Unlisted codes still fall back
> to `codes.Internal`.

> ⚠️ **Breaking: the metric `status` label is now the real code name** —
> `"NotFound"`, `"Unavailable"`, `"OK"` — not the old `"OK"` / `"ERROR"` pair.
> Dashboards and alerts matching `status="ERROR"` must be rewritten. A
> `*_grpc_errors_total{method,status}` counter is also recorded.

`UnaryLogger` picks its level from the outcome: INFO on `OK`, WARN for caller-fault
codes (`InvalidArgument`, `NotFound`, `AlreadyExists`, `PermissionDenied`,
`Unauthenticated`, `FailedPrecondition`, `OutOfRange`, `Canceled`), ERROR otherwise.
It adds a `code` field, plus `error` and `error_reason` on failure — previously it
logged the raw error value under `error` even when nil, and never surfaced `Reason`.

`ResetGRPCMetricHooks()` restores the no-op hooks for tests.

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
  mw.RateLimit(mw.RateLimitConfig{Rate: 100, Burst: 10}),
  mw.IPFilter(mw.IPFilterConfig{Mode: "whitelist", IPs: []string{"10.0.0.0/8"}}),
)
```

`ErrorHandler` does more than write the JSON body: it records the error on the active
span, sets the span status to `codes.Error` for 5xx, adds `error.code` /
`error.reason` attributes, and increments `*_http_errors_total{reason,code}`.
Previously a failed request produced a body and nothing else — a 500 was invisible in
both tracing and metrics.

`Logger` picks its level from the status class — INFO below 400, WARN for 4xx, ERROR
for 5xx — so a failing endpoint is visible without grepping for status codes. When the
handler called `c.Error`, the entry also carries `error` (the full `Error()` string,
i.e. `"message: cause"`) and `error_reason`. This is where the `errors.Wrap` cause
chain, deliberately withheld from the response body, survives.

`ResetHTTPMetricHooks()` restores the no-op metric hooks for tests.

---

### `database`

```go
// Create manager (no global side effects)
mgr, err := database.New(
  database.WithConfig(cfg),
  database.WithLogger(log),
  database.WithMigrationsDir("migrations"),
)
// err wraps database.ErrInvalidConfig when a database.{default,read,write} block
// fails to decode — e.g. max_open_conns is a string, or the block is a scalar.
// Such a block was previously skipped with no diagnostic. An absent block is fine.

// Register for global access (optional)
database.Register("default", mgr)

// Instance access
db := mgr.DB("default")      // *gorm.DB for the named connection

// Global convenience (delegates to Get("default").DB(connName))
db := database.DB("default")

// Migrations — mgr implements core.Migrator
app.WithMigration(mgr)

// Health check — hand it to app, which registers it on every server that
// accepts readiness probes, before startup
app.WithHealthCheck(mgr.HealthChecker("default"))   // probes SELECT 1
```

---

### `cache`

```go
// Create Manager (no global side effects)
mgr, err := cache.New(
  cache.WithMemory("default"),
  cache.WithRedis("redis", "redis:6379"),
)
// err wraps cache.ErrInvalidConfig when the "cache" block fails to decode — e.g.
// a backend entry is a scalar instead of a map. That previously produced zero
// backends with no diagnostic. An absent or empty block is not an error.

// Register for global access
cache.Register("default", mgr)

// Instance access
c := mgr.Client("default")
rdb := mgr.RedisClient("redis")    // *redis.Client for lock/stream

// Global convenience (delegates to Get("default"))
c := cache.Client("default")
rdb := cache.RedisClient("redis")

// mgr implements core.Closer — pass to app.WithCloser for automatic cleanup.
// Closers run in REVERSE registration order.
app.WithCloser(mgr)

c.Set(ctx, "key", "value", time.Hour)
v, _ := c.Get(ctx, "key")
c.Del(ctx, "key")

// Health check — hand it to app; memory backend is always healthy, redis PINGs
app.WithHealthCheck(mgr.HealthChecker("redis"))
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

// Implements core.Closer for automatic CloseAll on shutdown
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
  cron.WithRetries(3),
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
// Claims must implement auth.Claims; embedding jwt.RegisteredClaims satisfies it.
type Claims struct {
  jwt.RegisteredClaims
  UserID int64  `json:"user_id"`
  Role   string `json:"role"`
}

// Options are shared between signing and verification.
authOpts := []auth.Option{
  auth.WithSecret("my-secret"),
  auth.WithExpire(24 * time.Hour),
  auth.WithClaims(func() auth.Claims { return &Claims{} }),  // required for parsing
}

r.POST("/login", func(c *gin.Context) {
  token, err := auth.Generate(&Claims{UserID: 42, Role: "admin"}, authOpts...)
  if err != nil {
    c.Error(err)
    return
  }
  c.JSON(200, gin.H{"token": token})
})

// Required rejects a missing or invalid token with 401 before the handler runs.
// Optional parses the token when present and never blocks.
r.GET("/profile", auth.Required(authOpts...), func(c *gin.Context) {
  claims, ok := auth.FromContext[*Claims](c.Request.Context())
  if !ok {
    c.Error(errors.ErrUnauthorized)
    return
  }
  c.JSON(200, claims)
})
```

`WithClaims` must be supplied for verification — it tells `auth` what concrete
type to unmarshal into. `FromContext` takes a `context.Context` (not the
`*gin.Context`) and returns `(T, bool)`.

For gRPC, use `auth.UnaryAuth(authOpts...)` or `auth.OptionalUnaryAuth(authOpts...)`.

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

uuid := idgen.UUID()                       // time-sortable UUIDv7, no registration needed
token := idgen.UUIDv4()                    // fully random UUID, leaks no creation time
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
// Create instance first, then register. Provider is "s3", "oss" or "cos".
s, err := storage.New(
  storage.WithProvider("s3"),
  storage.WithRegion("us-east-1"),
  storage.WithBucket("my-avatars"),
  storage.WithAccessKey(
    cfg.GetString("storage.access_key"),
    cfg.GetString("storage.secret_key"),
  ),
)
storage.Register("avatars", s)

s := storage.Get("avatars")
url, err := s.Upload(ctx, "user/42/avatar.png", reader,
  storage.WithContentType("image/png"))
rc, err := s.Download(ctx, "user/42/avatar.png")   // caller closes rc
ok, err := s.Exists(ctx, "user/42/avatar.png")
signed, err := s.PresignURL(ctx, "user/42/avatar.png", time.Hour)
err = s.Delete(ctx, "user/42/avatar.png")          // missing keys are not an error
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
lim := ratelimit.New(ratelimit.WithRate(100), ratelimit.WithBurst(10))

// Distributed Redis fixed-window — share the client via cache
lim = ratelimit.New(
  ratelimit.WithRate(1000),
  ratelimit.WithWindow(time.Minute),
  ratelimit.WithRedis(cache.RedisClient("redis")),
)

if !lim.Allow(ctx, "user:42") {
  // over the limit
}
```

> The Redis-backed limiter **fails open** — if Redis is unreachable, `Allow`
> returns true rather than rejecting traffic.

The HTTP and gRPC middleware do not take a `Limiter`; they build an in-process
one from their own config, so middleware rate limiting is always per-instance:

```go
r.Use(mw.RateLimit(mw.RateLimitConfig{Rate: 100, Burst: 10}))
grpcserver.WithUnaryInterceptors(
  mwgrpc.UnaryRateLimit(mwgrpc.GRPCRateLimitConfig{Rate: 100, Burst: 10}),
)
```

For a cluster-wide limit, construct a Redis-backed `Limiter` as above and call
`Allow` yourself.

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
forge/
├── core/              # shared interfaces — zero deps on other forge packages
├── errors/            # CodedError — zero deps on other forge packages
├── app/               # lifecycle manager
├── config/            # Viper config loader
├── logger/            # slog structured logger
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
├── ARCHITECTURE.md    # Design guide
├── CLAUDE.md          # Claude Code guidance
└── Makefile
```
