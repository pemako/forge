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

# Build and run the example service
cd example && go build ./... && go run .
```

## Architecture

### Layer structure

```
core/          ← interfaces only (Logger, Config, Server, Provider) — zero deps on other mk packages
├── config/    ← viper implementation of core.Config
├── logger/    ← zap + lumberjack implementation of core.Logger
├── errors/    ← CodedError (HTTP code + reason + cause chain), sentinel vars
├── tracing/   ← otel OTLP provider implementing core.Provider
├── metric/    ← Prometheus + OTLP dual-channel, business helpers (Count/Gauge/StartTimer)
├── app/       ← lifecycle manager: migrations → BeforeStart → Start servers → signal → Stop → AfterStop
├── server/
│   ├── http/  ← gin server implementing core.Server, auto-mounts default middleware stack
│   └── grpc/  ← gRPC server implementing core.Server, auto-chains interceptors
├── middleware/
│   ├── http/  ← RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler + optional CORS/RateLimit/etc.
│   └── grpc/  ← same set as unary interceptors; Metrics stub wired by metric.Init()
├── database/  ← gorm multi-driver, SQL tracing plugin, goose migrations; Manager implements app.Migrator
├── cache/     ← Cache interface; memory (default) or Redis backend; named registry
├── httpclient/← outbound HTTP with otelhttp tracing, exponential-backoff retry, named registry
├── grpcclient/← outbound gRPC with otelgrpc, retry interceptor, named registry; uses grpc.NewClient
├── lock/      ← Redis distributed lock; Lua atomic acquire/release; optional auto-renew
└── cron/      ← robfig/cron-backed scheduler implementing core.Server; sub-second via time.Ticker
```

### Key patterns used everywhere

**Functional options** — every package constructor follows the same shape:
```go
func New(opts ...Option) (*T, error)   // or func Init(opts ...Option) error
type Option func(*options)
func WithXxx(...) Option { return func(o *options) { ... } }
func WithConfig(cfg core.Config) Option  // always present on infrastructure packages
func WithLogger(l core.Logger) Option    // always present
```

**Named registry** — database, cache, httpclient, grpcclient all follow this pattern:
```go
Register("name", opts...)   // or Init / New that stores internally
Get("name")                 // returns typed value or nil
```

**core.Server contract** — `Start` must block; `Stop` must be called to unblock it. `app.WithServer` runs each server in a goroutine. The cron Scheduler and both network servers all implement this.

**Metrics stub + late wiring** — `middleware/http` and `middleware/grpc` expose `RegisterHTTPMetricHooks` / `RegisterGRPCMetricHooks`. These are no-ops until `metric.Init()` calls them. This breaks the import cycle between middleware and metric.

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

Non-`CodedError` values returned from handlers produce a generic 500 response (message is not forwarded).

### Context discipline

Every log call receives `ctx`. The logger extracts `trace_id` / `span_id` automatically from any active otel span in the context. All outbound clients (httpclient, grpcclient) propagate trace context through headers/metadata.

### Logger level-split files

When `output: file`, each level writes to a separate file: `{filename}.debug.log`, `.info.log`, `.warn.log`, `.error.log`. The `filename` config key is the base path (e.g. `logs/app`). If `filename` is empty, no files are created even when `output: file`.

### app.Migrator interface

Defined in `app/options.go` (not in database) to avoid an import cycle. `*database.Manager` implements it. Pass via `app.WithMigration(manager)` to run migrations before `BeforeStart` hooks fire.

### cron sub-second intervals

`robfig/cron/v3` truncates `@every` durations below 1 second. The `Scheduler.Add` implementation detects sub-second specs and routes them through a `time.Ticker` goroutine instead.

### example service

`example/` is a separate Go module with a `replace` directive pointing at `../`. It demonstrates every mk package. Run `go mod tidy` inside `example/` after adding new dependencies to the parent module.
