# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Module

`github.com/pemako/mk` — Go 1.26 internal component library.

## Commands

```bash
# Run all tests with race detector
go test -race ./... -timeout 120s

# Run tests for a single package
go test -race ./logger/... -v
go test -race ./cache/... -v -run TestMemoryCache

# Build all packages
go build ./...

# Vet
go vet ./...

# Lint (golangci-lint v1.x required)
golangci-lint run ./...

# Build and run the example service
cd example && go build ./... && go run .
```

## Adding a new package

Every new package requires three steps — always do all three before committing:
1. Add complete godoc (package-level comment + all exported identifiers)
2. Update `README.md` (Features table + Package Reference section)
3. Update `example/` to demonstrate the new package's key API

## Architecture

### Package inventory

```
core/            ← interfaces only (Logger, Config, Server, Provider) — zero deps on other mk packages
errors/          ← CodedError (HTTP code + reason + cause chain), sentinel vars
config/          ← viper implementation of core.Config
logger/          ← zap + lumberjack; per-level log files; sensitive field masking
app/             ← lifecycle manager: migrations → BeforeStart → Start → signal → Stop → AfterStop
server/http/     ← gin server implementing core.Server; auto-mounts default middleware stack
server/grpc/     ← gRPC server implementing core.Server; auto-chains interceptors
middleware/http/ ← RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler + optional CORS/RateLimit/etc.
middleware/grpc/ ← same set as unary interceptors; Metrics stub wired by metric.Init()
database/        ← gorm multi-driver (mysql/postgres/sqlite), SQL tracing plugin, goose migrations
cache/           ← Cache interface; memory (default) or Redis backend; named registry
httpclient/      ← outbound HTTP; otelhttp tracing; exponential-backoff retry; named registry
grpcclient/      ← outbound gRPC; otelgrpc; retry interceptor; named registry; grpc.NewClient
lock/            ← Redis distributed lock; Lua atomic acquire/release; optional auto-renew
cron/            ← robfig/cron scheduler implementing core.Server; WithDistributedLock option
event/           ← in-process pub/sub; sync (default) or async delivery
stream/          ← Redis Streams distributed pub/sub; work-queue and broadcast modes
tracing/         ← otel OTLP provider implementing core.Provider
metric/          ← Prometheus + OTLP dual-channel; Count/Gauge/StartTimer helpers
auth/            ← JWT signing/verification; Required/Optional gin middleware; FromContext[T]
validate/        ← go-playground/validator; BindJSON/BindForm/BindQuery/BindURI helpers
pagination/      ← Params, Result[T], FromQuery, Offset, New[T]
circuitbreaker/  ← three-state circuit breaker (Closed/Open/HalfOpen); Execute(ctx, fn)
storage/         ← Storage interface; S3 / Alibaba OSS / Tencent COS backends
notify/          ← Email(SMTP) / SMS(Aliyun) / DingTalk / WeCom / Feishu notification channels
```

### Key patterns used everywhere

**Functional options** — every package constructor follows the same shape:
```go
func New(opts ...Option) (*T, error)   // or func Init(opts ...Option) error
type Option func(*options)
func WithXxx(...) Option { return func(o *options) { ... } }
func WithConfig(cfg core.Config) Option  // present on all infrastructure packages
func WithLogger(l core.Logger) Option    // present on all infrastructure packages
```

**Named registry** — database, cache, httpclient, grpcclient follow this pattern:
```go
Register("name", opts...)   // or Init / New that stores internally
Get("name")                 // returns typed value or nil
```

**core.Server contract** — `Start` must block; `Stop` must be called to unblock it. `app.WithServer` runs each server in a goroutine. `cron.Scheduler`, `stream.Worker`, and both network servers implement this interface.

**Metrics stub + late wiring** — `middleware/http` and `middleware/grpc` expose `RegisterHTTPMetricHooks` / `RegisterGRPCMetricHooks`. These are no-ops until `metric.Init()` calls them, breaking the import cycle between middleware and metric.

### Error handling convention

```go
// Define a sentinel (package-level var)
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")

// Return with context
return nil, errors.Wrap(ErrUserNotFound, "query user: %w", dbErr)

// Check type
if errors.Is(err, ErrUserNotFound) { ... }

// HTTP/gRPC middleware auto-converts CodedError to JSON / gRPC status
```

Non-`CodedError` values returned from handlers produce a generic 500 (message not forwarded).

### Context discipline

Every log call receives `ctx`. The logger extracts `trace_id` / `span_id` automatically from any active otel span. All outbound clients (httpclient, grpcclient, stream) propagate trace context through headers/metadata.

### Redis connection sharing

`cache` is the Redis connection registry. Packages that need a `*redis.Client` (lock, stream) get it from `cache.RedisClient("name")`. Configure one named Redis backend in cache; all packages share it — no duplicate Redis addresses.

```yaml
cache:
  default:
    backend: memory          # regular caching
  redis:
    backend: redis           # shared for lock + stream
    addr: "redis:6379"
```

```go
locker := lock.NewWithClient(cache.RedisClient("redis"))
stream.Init(cache.RedisClient("redis"))
```

### event vs stream

- `event` — in-process pub/sub; zero deps; synchronous by default; use for module decoupling within one process.
- `stream` — cross-instance pub/sub via Redis Streams; requires Redis 5.0+; use when multiple service replicas must coordinate. Two modes: work-queue (`WithGroup`) and broadcast (`WithBroadcast`).

### cron distributed execution

`cron.WithDistributedLock(locker)` ensures only one instance across the cluster runs a job per tick. TTL is derived from `WithTimeout` if set, otherwise defaults to 30s. Always set TTL ≥ max expected job duration to avoid premature lock expiry.

### Logger level-split files

When `output: file`, each level writes to a separate file: `{filename}.debug.log`, `.info.log`, `.warn.log`, `.error.log`. The `filename` config key is the base path (e.g. `logs/app`). If `filename` is empty, no files are created even when `output: file`.

### app.Migrator interface

Defined in `app/options.go` (not in database) to avoid an import cycle. `*database.Manager` implements it. Pass via `app.WithMigration(manager)` to run goose migrations before `BeforeStart` hooks fire.

### cron sub-second intervals

`robfig/cron/v3` truncates `@every` durations below 1 second. `Scheduler.Add` detects sub-second specs and routes them through a `time.Ticker` goroutine instead.

### example service

`example/` is a separate Go module with a `replace` directive pointing at `../`. It demonstrates every mk package. Run `go mod tidy` inside `example/` after adding new dependencies to the parent module.
