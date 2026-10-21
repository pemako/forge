# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Module

`github.com/pemako/forge` — Go 1.26 internal component library.

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

# Build and run the quickstart example (random port, no external deps)
cd example/quickstart && go build ./... && go run .

# Build and run the full example (requires config.yaml with database/cache/etc.)
cd example/full && go build ./... && go run .
```

## After completing any package work

After finishing any task that adds or modifies a package — before considering the work done — always run this checklist without being asked:

1. **`CLAUDE.md`** — does the Package inventory list need updating? Are there new patterns, conventions, or cross-package rules introduced by this change?
2. **`README.md`** — does the Features table or Package Reference section need a new row or updated description?
3. **`ARCHITECTURE.md`** — do any architecture rules or patterns need updating?
4. **`example/`** — does `example/full/` demonstrate the new or changed API? For significant API changes, also check if `example/quickstart/` needs updating.

If any are stale, update them as part of the same task. Do not wait to be asked.

## Adding a new package

Every new package requires three steps — always do all three before committing:
1. Add complete godoc (package-level comment + all exported identifiers)
2. Update `README.md` (Features table + Package Reference section)
3. Update `example/full/` to demonstrate the new package's key API

### New package checklist

Before writing any code, verify:
- [ ] Does the package need a `core/` interface? (yes if other packages will depend on it, or if it needs to be mockable in tests)
- [ ] Does it manage shared named resources? → use named registry pattern (`Register` / `Get` / `Replace` / `CloseAll`)
- [ ] Does it hold a long-running goroutine? → implement `core.Server` (`Start` blocks, `Stop` unblocks)
- [ ] Does it need Redis? → get client from `cache.RedisClient("name")`, never open a new connection directly
- [ ] Does it have init/shutdown lifecycle? → implement `app.Lifecycle`
- [ ] Does it hold closeable resources? → implement `app.Closer`

### Mandatory file layout

```
newpkg/
  newpkg.go    ← package doc comment + main types + constructors
  options.go   ← options struct, Option type, all WithXxx funcs
  newpkg_test.go
```

Split into additional files only when a single file exceeds ~300 lines.

### Constructor shape — copy this exactly

```go
// newpkg/options.go

type options struct {
  cfg    core.Config
  log    core.Logger
  // package-specific fields ...
}

type Option func(*options)

// WithConfig reads package settings from cfg under the "newpkg.*" key prefix.
// Explicit options applied after WithConfig override config values.
func WithConfig(cfg core.Config) Option {
  return func(o *options) {
    o.cfg = cfg
    if v := cfg.GetString("newpkg.xxx"); v != "" {
      o.xxx = v
    }
  }
}

// WithLogger sets the logger. Defaults to logger.Default() when not provided.
func WithLogger(l core.Logger) Option {
  return func(o *options) { o.log = l }
}

func defaultOptions() options {
  return options{log: logger.Default()}
}
```

```go
// newpkg/newpkg.go

// New creates a Xxx with the given options.
// Returns an error if required configuration (e.g. address) is missing.
func New(opts ...Option) (*Xxx, error) {
  o := defaultOptions()
  for _, opt := range opts {
    opt(&o)
  }
  // validate required fields here
  return &Xxx{opts: o}, nil
}
```

### Named registry — use when multiple named instances are needed

```go
var ErrAlreadyRegistered = errors.Define(409, "NEWPKG_ALREADY_REGISTERED", "already registered")
var (
  registry   = map[string]*Xxx{}
  registryMu sync.RWMutex
)

// Register stores x under name. Returns ErrAlreadyRegistered if name is taken.
func Register(name string, x *Xxx) error {
  registryMu.Lock()
  defer registryMu.Unlock()
  if _, ok := registry[name]; ok {
    return ErrAlreadyRegistered
  }
  registry[name] = x
  return nil
}

// Replace stores x under name, closing and releasing the old instance first.
func Replace(name string, x *Xxx) error {
  registryMu.Lock()
  old := registry[name]
  registry[name] = x
  registryMu.Unlock()
  if old != nil {
    old.Close() //nolint:errcheck
  }
  return nil
}

func Get(name string) *Xxx {
  registryMu.RLock()
  defer registryMu.RUnlock()
  return registry[name]
}

// reset clears the registry. For use in tests only (_test.go).
func reset() {
  registryMu.Lock()
  clear(registry)
  registryMu.Unlock()
}
```

### core.Server — implement when the package owns a blocking goroutine

```go
func (x *Xxx) Start(_ context.Context) error {
  // start background work
  <-x.done          // block until Stop is called
  return nil
}

func (x *Xxx) Stop(ctx context.Context) error {
  close(x.done)
  // wait for in-flight work with ctx deadline
  return nil
}
```

### Sentinel errors — define at package level

```go
var (
  ErrNotFound = errors.Define(404, "NEWPKG_NOT_FOUND", "resource not found")
  ErrConflict = errors.Define(409, "NEWPKG_CONFLICT", "resource conflict")
)
```

Never return raw `fmt.Errorf` from public functions — always wrap a sentinel or use `errors.Wrap`.

### Test requirements for every new package

- Minimum 60% statement coverage
- Use `github.com/stretchr/testify/assert` and `require`
- For packages with network I/O: use an in-process fake or `testcontainers` — never skip or mock the real protocol
- Tests that start a server must use a random port (`":0"`) to avoid conflicts:

```go
ln, err := net.Listen("tcp", ":0")
addr := ln.Addr().String()
```

- Tests must not rely on or mutate package-level globals. If the package has global state, provide an unexported `reset()` helper in `_test.go` only.

### Import rules

- `core/` packages may not import any other `mk` package
- `errors/` may not import any other `mk` package
- Infrastructure packages (`cache`, `lock`, `stream`, …) may import `core/`, `errors/`, `logger/`
- Never introduce a circular import — check with `go build ./...` before committing
- New packages must not import `server/http`, `server/grpc`, `middleware/*`, or `app` (those are consumers, not libraries)

## Architecture

### Package inventory

```
core/            ← interfaces only (Logger, Config, Server, Provider, HealthChecker) — zero deps on other mk packages
          core.HealthChecker: Name() string / Check(ctx context.Context) error
errors/          ← CodedError (HTTP code + reason + cause chain), sentinel vars
config/          ← viper implementation of core.Config
logger/          ← log/slog + lumberjack; per-level log files; sensitive field masking
app/             ← lifecycle manager: Migrator.Up → Lifecycle.Init → BeforeStart → Server.Start → signal
          → Server.Stop → Lifecycle.Shutdown (reverse) → Closer.Close → AfterStop
          Options: WithLifecycle(components...) / WithCloser(closers...)
          Interfaces: Lifecycle{Init,Shutdown}, Closer{Close}, Migrator{Up}
          WithHealthCheck(checkers ...core.HealthChecker) / HealthCheckers() []core.HealthChecker
server/http/     ← gin server implementing core.Server; auto-mounts default middleware stack:
          RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler
          Do NOT re-add these middlewares manually — causes duplicate responses
          WithHealthChecks(...core.HealthChecker) — registers /readyz probes at construction
          AddReadyCheck(name, fn) — add /readyz probe after construction
server/grpc/     ← gRPC server implementing core.Server; auto-chains interceptors
middleware/http/ ← RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler
          + optional: CORS, Timeout, BodyLimit, RateLimit, IPFilter
middleware/grpc/ ← UnaryRequestID, UnaryLogger, UnaryMetrics, UnaryRecovery, UnaryErrorHandler
          + optional: UnaryTimeout, UnaryRateLimit, UnaryIPFilter
database/        ← gorm multi-driver (mysql/postgres/sqlite); SQL tracing plugin; goose migrations
          database.New() → *Manager (no global side effects)
          database.Register("name", mgr) / database.Get("name") / database.DB("conn")
          database.CloseAll() error — closes and releases all registered managers
          *Manager implements app.Migrator; Manager.Close() releases all connections
          Manager.HealthChecker(connName) core.HealthChecker — probes SELECT 1
cache/           ← Cache interface; Manager holds named backends (memory/Redis)
          cache.New() → *Manager (no global side effects)
          cache.Register("name", mgr) / cache.Client("name") / cache.RedisClient("name")
          *Manager implements app.Closer; Manager.CloseBackend("name") / Manager.CloseAll()
          Manager.HealthChecker(backendName) core.HealthChecker — memory: always healthy; redis: PING
httpclient/      ← outbound HTTP; otelhttp tracing; exponential-backoff retry; named registry
          client, err := httpclient.New(opts...) — create instance
          httpclient.Register("name", client) / httpclient.Replace("name", client) / httpclient.Get("name")
          httpclient.CloseAll() error — closes and clears all registered clients
grpcclient/      ← outbound gRPC; otelgrpc; retry interceptor; named registry; grpc.NewClient
          conn, err := grpcclient.Dial(target, opts...) — create connection
          grpcclient.Register("name", conn) / grpcclient.Replace("name", conn) / grpcclient.Get("name")
          grpcclient.NewRegistry() implements app.Closer (calls CloseAll on shutdown)
lock/            ← Redis distributed lock; Lua atomic acquire/release; optional auto-renew
cron/            ← robfig/cron scheduler implementing core.Server; WithDistributedLock option
event/           ← in-process pub/sub; Bus struct only — no package-level state
          bus := event.New() / bus.Subscribe / bus.Publish / bus.Unsubscribe
          event.New(event.WithAsync()) for async delivery
stream/          ← Redis Streams pub/sub; Bus struct only — no package-level state
          bus, err := stream.New(redisClient, opts...) — returns error if client is nil
          bus.Subscribe(topic, stream.WithGroup(...), stream.WithHandler(...))
          bus.Subscribe(topic, stream.WithBroadcast(), stream.WithHandler(...))
          app.WithServer(bus.Worker()) — Worker implements core.Server
tracing/         ← OTel OTLP provider; implements core.Provider and app.Lifecycle
          tracing.New() → *Provider; Init() is no-op; Shutdown() flushes spans
metric/          ← Prometheus + OTLP; implements app.Lifecycle
          metric.New() → *Provider; p.Init(ctx) starts exporters + wires middleware hooks
          metric.Count / Gauge / StartTimer — package-level facade (idempotent sync.Once wiring)
auth/            ← JWT signing/verification; Required/Optional gin middleware; FromContext[T]
validate/        ← go-playground/validator; BindJSON/BindForm/BindQuery/BindURI helpers
pagination/      ← Params, Result[T], FromQuery, Offset, New[T]
circuitbreaker/  ← three-state circuit breaker (Closed/Open/HalfOpen); Execute(ctx, fn)
storage/         ← Storage interface; S3 / Alibaba OSS / Tencent COS backends
          s, err := storage.New(opts...) — create instance
          storage.Register("name", s) / storage.Replace("name", s) / storage.Get("name")
          storage.CloseAll() error — clears all registered backends
notify/          ← Email(SMTP) / SMS(Aliyun) / DingTalk / WeCom / Feishu notification channels
pprof/           ← pprof profiling endpoints; standalone server (core.Server) or Register on gin.Engine
idgen/           ← Snowflake ID generator + UUID
          gen, err := idgen.New(idgen.WithWorkerID(n)) — warns if worker_id not set explicitly
          idgen.Register("name", gen) / idgen.Get("name").Next() / idgen.Get("name").NextString()
          idgen.CloseAll() error — no-op (no connections to close), clears registry
          idgen.UUID() — time-sortable UUIDv7 string, no registration needed
          idgen.UUIDv4() — fully random UUID, not sortable
          Production: always set idgen.worker_id in config (IP-derived default is unstable in containers)
testutil/        ← test helpers: FreePort, NewConfig (stub core.Config), NewLogger (writes to t.Log)
ratelimit/       ← Limiter interface; in-process token-bucket or distributed Redis fixed-window; used by middleware
```

### Key patterns used everywhere

**Functional options** — every package constructor follows the same shape:
```go
func New(opts ...Option) (*T, error)
type Option func(*options)
func WithConfig(cfg core.Config) Option  // present on all infrastructure packages
func WithLogger(l core.Logger) Option    // present on all infrastructure packages
```

**Named registry** — all registry packages use this consistent API:
```go
Register(name string, instance *T) error   // ErrAlreadyRegistered on duplicate
Replace(name string, instance *T) error    // closes old, stores new
Get(name string) *T                        // nil if not found
CloseAll() error                           // close all + clear; nil for resource-free packages
reset()                                    // unexported; _test.go only
```

Note: `Register` always accepts a **pre-built instance**, never options. Create with `New()` (or `Dial()` for grpcclient) first, then pass the instance to `Register`.

**core.Server contract** — `Start` must block; `Stop` must be called to unblock it. `app.WithServer` runs each server in a goroutine. `cron.Scheduler`, `stream.Bus.Worker()`, and both network servers implement this interface.

**app.Lifecycle contract** — `Init` is called before servers start (sequential), `Shutdown` is called after servers stop (reverse order). If `Init[i]` fails, all previously-Init'd components are Shutdown in reverse before `Run` returns. Implemented by `tracing.Provider` and `metric.Provider`.

**app.Closer contract** — `Close` is called after all `Lifecycle.Shutdown` calls complete. Implemented by `cache.Manager` and `grpcclient.Registry`.

**Metrics stub + late wiring** — `middleware/http` and `middleware/grpc` expose hook registration functions. These are no-ops until `metric.Provider.Init()` is called, breaking the import cycle between middleware and metric. Wire functions are idempotent via `sync.Once`.

### HealthChecker integration

Infrastructure packages that connect to external services provide a `HealthChecker` factory:

```go
// database.Manager and cache.Manager implement this pattern:
hc := dbMgr.HealthChecker("default")     // returns core.HealthChecker; probes SELECT 1
hc := cacheMgr.HealthChecker("redis")   // returns core.HealthChecker; probes PING (memory always healthy)
// hc.Name() → "database:default" or "cache:redis"
```

Wire into `/readyz` via:
- `httpserver.WithHealthChecks(checkers...)` — at construction time
- `httpSrv.AddReadyCheck(name, fn)` — after construction (e.g. in BeforeStart)
- `app.WithHealthCheck(checkers...)` + `myApp.HealthCheckers()` — centralized holding with manual bridging

### Error handling convention

```go
// Define a sentinel (package-level var)
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")

// Return with context
return nil, errors.Wrap(ErrUserNotFound, "query user: %w", dbErr)

// Check type
if errors.Is(err, ErrUserNotFound) { ... }

// HTTP handler — use c.Error(); ErrorHandler middleware converts to JSON
c.Error(err)   // NOT c.JSON(500, gin.H{"error": err.Error()})
```

Non-`CodedError` values returned from handlers produce a generic 500 (message not forwarded).

### Context discipline

Every log call receives `ctx`. The logger extracts `trace_id` / `span_id` automatically from any active OTel span. All outbound clients (`httpclient`, `grpcclient`, `stream`) propagate trace context through headers/metadata.

### Redis connection sharing

`cache` is the Redis connection registry. Packages that need a `*redis.Client` (`lock`, `stream`, `ratelimit`) call `cache.RedisClient("name")`. Configure one named Redis backend in cache; all packages share it — no duplicate Redis addresses.

```yaml
cache:
  default:
  backend: memory          # regular caching
  redis:
  backend: redis           # shared for lock + stream
  addr: "redis:6379"
```

```go
cacheMgr, _ := cache.New(cache.WithConfig(cfg))
cache.Register("default", cacheMgr)

locker    := lock.NewWithClient(cache.RedisClient("redis"))
streamBus, _ := stream.New(cache.RedisClient("redis"))
```

### event vs stream

- `event` — in-process pub/sub; zero deps; synchronous by default; use for module decoupling within one process.
- `stream` — cross-instance pub/sub via Redis Streams; requires Redis 5.0+; use when multiple service replicas must coordinate. Two modes: work-queue (`WithGroup`) and broadcast (`WithBroadcast`).

### HTTP middleware stack warning

`server/http.New()` mounts the full default stack automatically. **Never add the same middleware again**:

```go
srv, _ := httpserver.New(...)
r := srv.Engine()
// ❌ Wrong — ErrorHandler is already in the stack; this causes duplicate JSON responses
r.Use(mw.RequestID(), mw.Logger(log), mw.Recovery(), mw.ErrorHandler())
// ✅ Correct — just register routes
r.GET("/users", handler.ListUsers)
```

### cron distributed execution

`cron.WithDistributedLock(locker)` ensures only one instance across the cluster runs a job per tick. TTL is derived from `WithTimeout` if set, otherwise defaults to 30s. Always set TTL ≥ max expected job duration to avoid premature lock expiry.

### Logger level-split files

When `output: file`, each level writes to a separate file: `{filename}.debug.log`, `.info.log`, `.warn.log`, `.error.log`. The `filename` config key is the base path (e.g. `logs/app`). If `filename` is empty, no files are created even when `output: file`.

### app.Migrator interface

Defined in `app/options.go` (not in database) to avoid an import cycle. `*database.Manager` implements it. Pass via `app.WithMigration(manager)` to run goose migrations before `BeforeStart` hooks fire.

### cron sub-second intervals

`robfig/cron/v3` truncates `@every` durations below 1 second. `Scheduler.Add` detects sub-second specs and routes them through a `time.Ticker` goroutine instead.

### idgen worker ID in production

`idgen.New()` without `WithWorkerID` auto-derives the worker ID from the last byte of the host's primary IPv4 address and emits a Warn log. This is unstable in containerized environments (IP can change, collisions possible). Always configure `idgen.worker_id` explicitly in production:

```yaml
idgen:
  worker_id: 42   # unique per deployed instance
```

### example service

`example/` contains two separate Go modules:
- `example/quickstart/` — minimal HTTP service demonstrating config, logger, gin routes, and app lifecycle. Uses `net.Listen(":0")` for a random port to avoid bind conflicts. Replace directive: `../../`.
- `example/full/` — full-featured service demonstrating every mk package. Replace directive: `../../`.

Run `go mod tidy` inside the relevant subdirectory after adding new dependencies to the parent module.
