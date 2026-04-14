# Forge Architecture Guide

## 1. Project Scope

`forge` is an **instance-first** Go service framework for building production-grade HTTP and gRPC services. It solves recurring cross-cutting concerns — lifecycle management, configuration, logging, distributed tracing, metrics, HTTP/gRPC servers, database, cache, messaging — through a cohesive, composable package set with a common interface layer.

## 2. Package Layers and Dependency Direction

```
core/             ← interfaces only (Logger, Config, Server, Provider, HealthChecker); zero deps on other mk packages
errors/           ← CodedError; zero deps on other mk packages
│
config/logger/    ← infrastructure leaf; import core/ only
tracing/metric/   ← infrastructure leaf; import core/ only
│
middleware/*      ← import core/, errors/; late-wire metric via hooks to break import cycle
server/http+grpc  ← import core/, errors/, middleware/*
│
database/cache/   ← import core/, errors/, logger/
lock/stream/      ← import core/, errors/, logger/, cache/ (Redis via cache.RedisClient)
auth/validate/    ← import core/, errors/, server/http (via gin)
│
app/              ← import core/ only; all other types passed in as interface values
```

**Hard rules:**
- `core/` and `errors/` MUST NOT import any other `mk` package
- `app/` MUST NOT import `database`, `cache`, `metric`, `tracing`, or any infrastructure package
- New packages MUST NOT import `server/http`, `server/grpc`, `middleware/*`, or `app`
- Circular imports detected via `go build ./...` must be resolved before committing

## 2.1 Package Tiers and Stability Contracts

`mk` has grown beyond a minimal framework into an internal platform library. Packages are grouped into four tiers with different stability and maintenance expectations. **When adding a new package, assign it to a tier before merging.**

### Tier 1 — Core Stable

`core`, `errors`, `app`, `config`, `logger`

- Interfaces and exported behavior are **frozen**. No breaking changes without a major version bump.
- All other tiers may depend on these.
- Keep dependencies minimal; avoid pulling in large third-party libraries.

### Tier 2 — Infrastructure Integrations

`server/http`, `server/grpc`, `middleware/http`, `middleware/grpc`, `tracing`, `metric`, `database`, `cache`, `httpclient`, `grpcclient`, `lock`, `stream`, `cron`, `storage`

- APIs are stable; additive changes only within a minor version.
- Each package wraps an external system (Redis, OTLP, gRPC, SQL, S3). Upgrades to underlying libraries may surface as non-breaking behavior changes.
- Breaking changes require a deprecation notice in at least one minor release before removal.

### Tier 3 — Opinionated Extensions

`auth`, `validate`, `pagination`, `ratelimit`, `circuitbreaker`, `notify`, `idgen`, `pprof`

- Usable in production but **opinionated** about conventions (e.g., JWT format, pagination envelope shape, notification channels).
- Teams may prefer their own implementations; these packages are not required by the framework core.
- Minor breaking changes are acceptable between minor versions with a CHANGELOG entry.
- A package graduates from Tier 3 to Tier 2 when it has been used in 3+ production services without API complaints.

### Developer Utilities

`event`, `testutil`

- `event`: In-process pub/sub only. Use `stream` for cross-instance coordination. No API stability guarantee — it exists for process-local decoupling during development and testing.
- `testutil`: Test scaffolding; never import in production code. API may change freely.

### Tier assignment checklist for new packages

Before opening a PR for a new package, answer:

1. Does it wrap an external system? → Tier 2
2. Is it opinionated about a business-level convention? → Tier 3
3. Is it purely test scaffolding or in-process utilities? → Developer Utilities
4. Does it define interfaces consumed by other tiers? → Tier 1 (requires team consensus)

## 3. Global vs Instance

### Always instance-first (no package-level state)

These packages have zero global state. Always create an explicit instance:

| Package | Constructor |
|---------|------------|
| `stream` | `bus, err := stream.New(redisClient, opts...)` |
| `event` | `bus := event.New(opts...)` |
| `tracing` | `p, err := tracing.New(opts...)` |
| `metric` | `p, err := metric.New(opts...)` |

### Instance-first with named registry convenience

Create via `New()`, then optionally register for global access. Package-level convenience functions delegate to `Get("default")`:

```go
// database
mgr, err := database.New(database.WithConfig(cfg))
database.Register("default", mgr)
db := mgr.DB("main")           // instance access (preferred)
db := database.DB("main")      // convenience (delegates to Get("default").DB("main"))

// cache
mgr, err := cache.New(cache.WithMemory("default"), cache.WithRedis("redis", addr))
cache.Register("default", mgr)
c := mgr.Client("default")     // instance access
c := cache.Client("default")   // convenience

// same pattern: httpclient, grpcclient, storage, idgen
```

### Global facade (intentional exceptions)

These retain package-level helpers to break import cycles or serve genuinely stateless ops:

- `logger.Default()` / `logger.SetDefault()` — bootstrap convenience; prefer constructor injection
- `metric.Count()` / `Gauge()` / `StartTimer()` — facade needed by `middleware/*` (import cycle prevention); wire functions are idempotent via `sync.Once`

## 4. Registry Semantics

All named registry packages follow a two-tier contract:

### Base contract (all packages)

| Function | Behavior |
|----------|----------|
| `Register(name, instance)` | Returns `ErrAlreadyRegistered` if name exists; does NOT overwrite |
| `Replace(name, instance)` | Closes old resource (if any), stores new instance |
| `Get(name)` | Returns nil if not found |
| `CloseAll() error` | Releases resources (if any) and clears the registry; returns nil for resource-free packages |

> **`Register` always accepts a pre-built instance, never options.** Create the instance
> with `New()` first, then pass it to `Register`. This makes construction errors explicit
> and separates the "build" step from the "register" step.

### Optional extensions (package-specific)

| Function | Provided by |
|----------|------------|
| `CloseBackend(name string) error` | `cache.Manager` (close a single backend) |
| `Close() error` (implements `app.Closer`) | `cache.Manager`, `grpcclient.Registry` |
| `HealthChecker(name string) core.HealthChecker` | `database.Manager`, `cache.Manager` |

### Why `Close(name)` is NOT part of the base contract

Single-entry close has inconsistent semantics across packages:
- `database` manages a whole Manager (multiple connections) per registry entry
- `cache` manages a whole Manager (multiple backends) per registry entry
- `storage`/`idgen` have no connections to close at all

Using `Replace` to swap out an entry is the standard pattern when a single
entry needs to be refreshed. `CloseAll()` handles full teardown.

Registry packages: `database`, `cache`, `httpclient`, `grpcclient`, `storage`, `idgen`

## 5. Lifecycle Roles

Three interfaces govern component lifecycle, all coordinated by `app.Run()`:

```go
// core.Server — owns a blocking goroutine
// Start must block until Stop is called.
type Server interface {
  Start(ctx context.Context) error
  Stop(ctx context.Context) error
}
// Implementations: server/http, server/grpc, cron.Scheduler, stream.Bus.Worker()

// app.Lifecycle — non-blocking resource with explicit init/shutdown
// Init called before servers start; Shutdown called after servers stop (reverse order).
// If Init[i] fails, Init[0..i-1].Shutdown() are called before Run returns.
type Lifecycle interface {
  Init(ctx context.Context) error
  Shutdown(ctx context.Context) error
}
// Implementations: tracing.Provider, metric.Provider

// app.Closer — resource released after all Lifecycle.Shutdown calls
// Close called in registration order; errors collected but do not stop subsequent Closers.
type Closer interface {
  Close() error
}
// Implementations: cache.Manager, grpcclient.Registry
```

### app.Run() Lifecycle Order

```
1. Migrator.Up()                   database migrations
2. Lifecycle.Init()                sequential; abort + reverse-rollback on failure
3. BeforeStart hooks               sequential; first error aborts
4. Server.Start()                  all servers concurrent
5. Block                           until SIGTERM/SIGINT or server error
6. Server.Stop()                   concurrent; stop errors logged (not returned if runErr set)
7. Lifecycle.Shutdown()            reverse registration order
8. Closer.Close()                  registration order
9. AfterStop hooks                 sequential
```

### app orchestrator principle

`app` is an **orchestrator**, not a container or service locator. It sequences the startup and shutdown of components that are passed into it. It does not own, discover, or vend those components.

**Legitimate responsibilities** (current and future-safe):

| Category | Examples |
|----------|---------|
| Startup sequencing | Migrations → Lifecycle.Init → BeforeStart hooks → Server.Start |
| Shutdown sequencing | Server.Stop → Lifecycle.Shutdown → Closer.Close → AfterStop hooks |
| Timing and policy | `WithShutdownTimeout`, deadline propagation |
| Identity metadata | `WithName`, `WithVersion` (used in health check responses) |
| Observability wiring | `WithLogger`, `WithHealthCheck` (centralised collection for bridging into server/http) |

**Will never be added to app:**

| Pattern | Why not |
|---------|---------|
| `app.Get("database")` — dependency lookup | Turns app into a service locator; hides coupling, breaks testability |
| `app.WithConfig(cfg)` distributing config to components | Config belongs at construction time, not through the orchestrator |
| `app.RegisterModule("user", m)` — business routing | Business logic is outside the orchestrator's scope |
| Dynamic component registration after `Run()` starts | Orchestration requires a fixed, known set of components at startup |
| `app.Provide` / `app.Invoke` — DI container | Use explicit constructor injection; avoid implicit wiring |

**Adding a new option to app: decision checklist**

Before proposing a new `WithXxx` option, answer:

1. Does it control when or how a component starts or stops? → probably fine
2. Does it let app hold a reference that other code will later retrieve? → likely a service locator, reject
3. Does it push data or configuration into components on their behalf? → constructor injection is better
4. Does it add business-domain awareness to the orchestrator? → always reject



### HTTP Default Stack

`httpserver.New()` mounts this stack automatically before any routes are registered:

```
RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler
```

> **Do NOT call `r.Use(mw.ErrorHandler())` after `httpserver.New()`** — it is already mounted, and adding it again produces duplicate JSON error responses.

### gRPC Default Interceptor Chain

`grpcserver.New()` chains these unary interceptors automatically:

```
RequestID → Logger → Metrics → Recovery → ErrorHandler → (Tracing via stats handler)
```

## 7. Error Handling Convention

```go
// 1. Define package-level sentinels (exported var)
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")

// 2. Service layer: wrap for context
return nil, errors.Wrap(ErrUserNotFound, "get user %d: %w", id, dbErr)

// 3. Check type
if errors.Is(err, ErrUserNotFound) { ... }
var ce *errors.CodedError
if errors.As(err, &ce) { fmt.Println(ce.Code, ce.Reason) }

// 4. HTTP handler: use c.Error() — ErrorHandler middleware writes the response
func (h *Handler) Get(c *gin.Context) {
  user, err := h.svc.Get(c.Request.Context(), id)
  if err != nil {
    c.Error(err)   // do NOT call c.JSON(500, ...) — bypasses ErrorHandler
    return
  }
  c.JSON(200, user)
}
```

Non-`CodedError` values produce a generic 500 (message not forwarded to client).

## 8. Redis Connection Sharing

`cache` is the Redis connection registry. Packages needing `*redis.Client` (`lock`, `stream`, `ratelimit`) call `cache.RedisClient("name")` — never open a second Redis connection to the same address.

```yaml
# config.yaml — one Redis backend shared by cache, lock, stream
cache:
  app:
  backend: memory          # in-memory cache for application data
  redis:
  backend: redis           # shared Redis for lock + stream
  addr: "redis:6379"
```

```go
// All three share the same *redis.Client
cacheMgr, _ := cache.New(cache.WithConfig(cfg))
cache.Register("default", cacheMgr)

locker    := lock.NewWithClient(cache.RedisClient("redis"))
streamBus, _ := stream.New(cache.RedisClient("redis"))
```

## 9. event vs stream

| | `event` | `stream` |
|---|---|---|
| Transport | In-process goroutines | Redis Streams (cross-process) |
| Dependency | Zero (no network) | Redis 5.0+ |
| Delivery | Synchronous (default) or async | At-least-once via consumer groups |
| Modes | Single bus per process | Work-queue (`WithGroup`) or broadcast (`WithBroadcast`) |
| Use when | Decoupling modules within one service | Multiple service replicas must coordinate |
| Instance | `bus := event.New()` | `bus, _ := stream.New(redisClient)` |

## 10. Import Cycle: metric ↔ middleware

`middleware/http` and `middleware/grpc` must not import `metric` (would create a cycle). Instead they expose registration hooks:

```
metric.Provider.Init()
  └─ wireHTTPMetrics()  // calls middleware/http.RegisterHTTPMetricHooks()
  └─ wireGRPCMetrics()  // calls middleware/grpc.RegisterGRPCMetricHooks()
```

Both wire functions are guarded by `sync.Once` — calling `Init()` on multiple `metric.Provider` instances is safe; only the first takes effect for middleware hooks.

## 11. New Package Checklist

Before writing any code for a new package, confirm:

- [ ] Does it need a `core/` interface? → yes if other packages will import it, or it needs to be mockable
- [ ] Does it manage shared named resources? → use the named registry pattern (`Register`/`Get`/`Replace`/`CloseAll`)
- [ ] Does it own a blocking goroutine? → implement `core.Server` (`Start` blocks, `Stop` unblocks)
- [ ] Does it need Redis? → call `cache.RedisClient("name")`, never open a new connection
- [ ] Does it have init/shutdown behavior? → implement `app.Lifecycle`
- [ ] Does it hold closeable resources? → implement `app.Closer`
- [ ] Does it import `server/http`, `server/grpc`, `middleware/*`, or `app`? → **it should not**

### Mandatory file layout

```
newpkg/
  newpkg.go        ← package doc comment + main types + constructors
  options.go       ← options struct, Option type, all WithXxx funcs
  newpkg_test.go
```

### Constructor shape

```go
// options.go
type options struct {
  cfg core.Config
  log core.Logger
}
type Option func(*options)

func WithConfig(cfg core.Config) Option { return func(o *options) { o.cfg = cfg; /* scan keys */ } }
func WithLogger(l core.Logger) Option   { return func(o *options) { o.log = l } }
func defaultOptions() options           { return options{log: logger.Default()} }

// newpkg.go
func New(opts ...Option) (*T, error) {
  o := defaultOptions()
  for _, opt := range opts { opt(&o) }
  // validate, return
}
```

### Registry pattern (when multiple named instances needed)

```go
var ErrAlreadyRegistered = errors.Define(409, "NEWPKG_ALREADY_REGISTERED", "already registered")
var (
  registry   = map[string]*T{}
  registryMu sync.RWMutex
)

func Register(name string, t *T) error {
  registryMu.Lock(); defer registryMu.Unlock()
  if _, ok := registry[name]; ok { return ErrAlreadyRegistered }
  registry[name] = t; return nil
}
func Replace(name string, t *T) error {
  registryMu.Lock()
  old := registry[name]; registry[name] = t
  registryMu.Unlock()
  if old != nil { old.Close() }   // or equivalent cleanup
  return nil
}
func Get(name string) *T { registryMu.RLock(); defer registryMu.RUnlock(); return registry[name] }
func reset() { /* unexported; _test.go only */ registryMu.Lock(); clear(registry); registryMu.Unlock() }
```

### Sentinel errors

```go
var (
  ErrNotFound = errors.Define(404, "NEWPKG_NOT_FOUND", "resource not found")
  ErrConflict = errors.Define(409, "NEWPKG_CONFLICT",  "resource conflict")
)
```

Never return raw `fmt.Errorf` from public functions — always wrap a sentinel or use `errors.Wrap`.

### Test requirements

- Minimum 60% statement coverage
- Use `testify/assert` and `require`
- Network I/O: use in-process fake or `testcontainers` — never skip or mock the real protocol
- Tests that start a server: use a random port (`":0"`) to avoid conflicts
- Global state: provide unexported `reset()` in `_test.go`; tests must not leak state to other tests

## 12. HealthChecker Integration

Infrastructure packages that connect to external services provide a `HealthChecker` factory method on their `Manager` type:

```go
// database.Manager and cache.Manager implement this pattern:
hc := dbMgr.HealthChecker("default")     // core.HealthChecker; probes SELECT 1
hc := cacheMgr.HealthChecker("redis")    // core.HealthChecker; probes PING
// memory backend is always healthy (no network probe)

// hc.Name() returns a stable name like "database:default" or "cache:redis"
// hc.Check(ctx) returns nil on healthy, non-nil on failure
```

Wire into `/readyz` via one of three patterns:

```go
// Pattern A: at construction time (preferred)
srv, _ := httpserver.New(
  httpserver.WithHealthChecks(
    dbMgr.HealthChecker("default"),
    cacheMgr.HealthChecker("redis"),
  ),
)

// Pattern B: after construction in BeforeStart
app.WithBeforeStart(func(ctx context.Context) error {
  httpSrv.AddReadyCheck("db", dbMgr.HealthChecker("default").Check)
  return nil
})

// Pattern C: centralized via app, bridged manually
myApp := app.New(app.WithHealthCheck(dbMgr.HealthChecker("default"), cacheMgr.HealthChecker("redis")))
app.WithBeforeStart(func(ctx context.Context) error {
  for _, hc := range myApp.HealthCheckers() {
    httpSrv.AddReadyCheck(hc.Name(), hc.Check)
  }
  return nil
})
```

`core.HealthChecker` is defined in `core/` to avoid import cycles — infrastructure packages implement it without importing `server/http` or `app`.

## 13. Multi-backend Abstraction Semantics

Several packages in mk expose a single interface over multiple backends (`cache`, `ratelimit`, `storage`, `notify`). This pattern provides a convenient unified API but carries a risk: the interface hides semantic differences that affect correctness, not just performance. **Switching backends is not always a drop-in change.**

### Semantic layers

Every multi-backend package should be understood at three levels:

| Layer | Meaning |
|-------|---------|
| **Portable minimum** | Operations guaranteed to behave identically across all backends |
| **Backend-specific** | Operations available on one backend but absent or broken on others |
| **Non-interchangeable semantics** | Operations with the same signature but materially different observable behaviour |

The third layer is the most dangerous. Silent behavioural changes after a backend switch can be very hard to diagnose.

### `ratelimit`: scope is not portable

This is the highest-risk package. The two backends share the `Limiter` interface but enforce limits with fundamentally different scopes:

| Property | In-process (token-bucket) | Redis (fixed-window) |
|----------|--------------------------|----------------------|
| Enforcement scope | Per process instance | Cluster-wide |
| Algorithm | Token bucket (smooth) | Fixed window (bursty at boundary) |
| Multi-instance behaviour | Each instance has independent quota | All instances share one counter |
| Redis failure | N/A | Fails open (allows the request) |

**Consequence:** A service running 4 replicas with in-process rate limiting allows 4× more requests than intended. Switching to Redis in production resets all in-flight counters.

### `cache`: TTL precision is not portable

| Property | Memory (go-cache) | Redis |
|----------|------------------|-------|
| TTL enforcement | Lazy sweep (background goroutine) | Exact, server-side |
| Sub-second TTL reliability | No — keys may survive past TTL until sweep fires | Yes |
| Zero TTL | No expiry | No expiry |
| Redis-specific ops (INCR, EXPIRE, Lua) | Not available | Use `cache.RedisClient("name")` |

**Consequence:** Code relying on TTL=1s for short-lived tokens (e.g., rate limit windows, OTP expiry) behaves incorrectly with the memory backend. Use Redis in production for any TTL-sensitive logic.

### `storage`: capability surface is not uniform

S3, OSS, and COS expose different metadata fields, ACL models, and server-side encryption options. The `Storage` interface exposes the lowest common denominator (`Put`, `Get`, `Delete`, `URL`).

| Operation | S3 | Alibaba OSS | Tencent COS |
|-----------|----|-------------|-------------|
| ContentType on Put | ✓ | ✓ | ✓ |
| ACL on Put | ✓ | ✓ (different enum) | ✓ (different enum) |
| Server-side encryption | ✓ | ✓ | ✓ |
| Bucket lifecycle rules | ✓ | ✓ | ✓ |

The interface contract only guarantees `Put`/`Get`/`Delete`/`URL`. For provider-specific features, access the underlying SDK client directly; do not add them to the `Storage` interface.

### `notify`: delivery semantics vary by channel

All channels share `Send(ctx, Message) error`. A nil return means the API accepted the message, **not** that it was delivered:

| Channel | What nil error means |
|---------|---------------------|
| Email (SMTP) | SMTP server accepted for relay; not guaranteed to reach inbox |
| SMS (Aliyun) | API returned code "OK"; carrier delivery is async |
| DingTalk / WeCom / Feishu | Webhook returned HTTP 200; platform delivery is async |

`Send` stops on the first channel error and does not retry. For delivery guarantees, wrap `Send` in your own retry loop or use a message queue.

### Guidance for new multi-backend packages

When adding a new package with multiple backends:

1. **Document the portable minimum** in the interface godoc.
2. **Call out non-interchangeable semantics** explicitly — do not leave them to be discovered by the caller.
3. **Never add backend-specific methods to the shared interface** — keep the interface at the lowest common denominator and expose backend-specific features via type assertion or a separate constructor.
4. **Test with both backends** — the presence of tests for both backends is the best defence against unintentional semantic drift.

