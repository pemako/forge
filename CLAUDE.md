# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Module

`github.com/pemako/forge` — Go 1.26 internal component library.

## Breaking changes to know about

Read this before touching `core`, `metric`, `server/grpc`, or anything that
implements a forge interface.

| Change | Impact |
| ------ | ------ |
| `core.Config` gained `IsSet(key string) bool` | Any external implementation of `core.Config` no longer compiles until it adds the method. In-tree: `config.viperConfig`, `testutil` stub, `database` test mock. |
| `core.Closer` runs in **reverse** registration order | Was forward order. If you relied on `WithCloser(a, b)` closing `a` first, the order is now `b` then `a` — which is the correct nesting, matching `Lifecycle`. |
| `metric.Provider.Init` returns `ErrNoExporters` when no exporter is configured | A service that previously started with an empty `metric:` block now fails at startup. Either configure `metric.prometheus_addr` / `metric.otlp_endpoint`, or pass `metric.WithAllowNoExporters()`. |
| `metric.Provider.Init` binds the Prometheus listener synchronously | A port clash now returns `ErrPrometheusListen` instead of leaving `/metrics` quietly unserved. |
| gRPC health service starts `NOT_SERVING` | It used to claim `SERVING` from `New()`, before migrations ran. `app` flips it via `SetReady`; a `server/grpc` used without `app` must call `SetReady(true)` itself. |
| `middleware/grpc` HTTP→gRPC code mapping grew from 7 to 17 entries | Clients switching on gRPC codes see different codes for HTTP 408, 412, 413, 416, 422, 499, 501, 502, 503 and 504 — all of which previously fell through to `codes.Internal`. |
| `middleware/grpc` metric `status` label | Now the real code name (`"NotFound"`, `"Unavailable"`), not `"OK"` / `"ERROR"`. Dashboards and alerts matching `status="ERROR"` must be rewritten. |
| `RegisterHTTPMetricHooks` / `RegisterGRPCMetricHooks` gained an `errorsInc` parameter | Any direct caller (only `metric` in-tree) must pass the extra callback. |
| `tracing.New` installs the global text map propagator unconditionally | Previously nothing was installed unless an exporter existed, so inbound `traceparent` headers were dropped and nothing was injected outbound. Expect traces to start connecting across services. |
| `config.Watch` returns `ErrWatchDisabled` without `WithWatch()` | It used to accept a callback it could never invoke. |
| `cache.New` / `database.New` return `ErrInvalidConfig` on a malformed block | Previously a decode failure silently produced zero backends / zero connections. |

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
```

Runnable examples live in a separate repository,
[`github.com/pemako/forge-example`](https://github.com/pemako/forge-example),
cloned as a sibling checkout. See "example repository" below.

## After completing any package work

After finishing any task that adds or modifies a package — before considering the work done — always run this checklist without being asked:

1. **`CLAUDE.md`** — does the Package inventory list need updating? Are there new patterns, conventions, or cross-package rules introduced by this change?
2. **`README.md`** — does the Features table or Package Reference section need a new row or updated description?
3. **`ARCHITECTURE.md`** — do any architecture rules or patterns need updating?
4. **`forge-example`** — if the change alters a public API an example uses, the example repo needs a matching update. It is a separate repo, so that is a follow-up change there, not part of this commit; call it out when finishing the task.

If any are stale, update them as part of the same task. Do not wait to be asked.

## Adding a new package

Every new package requires three steps — always do all three before committing:
1. Add complete godoc (package-level comment + all exported identifiers)
2. Update `README.md` (Features table + Package Reference section)
3. Note whether `forge-example` should gain or update an example (separate repo; follow-up change)

### New package checklist

Before writing any code, verify:
- [ ] Does the package need a `core/` interface? (yes if other packages will depend on it, or if it needs to be mockable in tests)
- [ ] Does it manage shared named resources? → use named registry pattern (`Register` / `Get` / `Replace` / `CloseAll`)
- [ ] Does it hold a long-running goroutine? → implement `core.Server` (`Start` blocks, `Stop` unblocks)
- [ ] Does it need Redis? → get client from `cache.RedisClient("name")`, never open a new connection directly
- [ ] Does it have init/shutdown lifecycle? → implement `core.Lifecycle`
- [ ] Does it hold closeable resources? → implement `core.Closer`
- [ ] Does it record metrics? → depend on `core.Metrics`, not the `metric` package's globals

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

- `core/` packages may not import any other `forge` package
- `errors/` may not import any other `forge` package
- Infrastructure packages (`cache`, `lock`, `stream`, …) may import `core/`, `errors/`, `logger/`
- Never introduce a circular import — check with `go build ./...` before committing
- New packages must not import `server/http`, `server/grpc`, `middleware/*`, or `app` (those are consumers, not libraries)
- Implement `core.Lifecycle` / `core.Closer` / `core.Migrator`, never the `app` aliases — a
  library package must not import `app`, and the `core` spelling can be compile-time asserted

## Architecture

### Package inventory

```
core/            ← interfaces only — zero deps on other forge packages
          core.go/config.go/...: Logger, Config, Server, Provider, HealthChecker
          core/lifecycle.go: Migrator, Lifecycle, Closer, ReadinessToggler, ReadyCheckRegistrar
          core/metric.go: Metrics — Count / Gauge / Histogram(ctx, name, value, ...core.Field)
          core.HealthChecker: Name() string / Check(ctx context.Context) error
          core.ReadinessToggler: SetReady(bool) — server/http + server/grpc implement it
          core.ReadyCheckRegistrar: AddReadyCheck(name, func(ctx) error) — same two servers
          *metric.Provider implements core.Metrics (compile-time asserted in metric/core.go)
          BREAKING: core.Config now also requires IsSet(key string) bool
errors/          ← CodedError (HTTP code + reason + cause chain), sentinel vars
          errors.AsType[T](err) (T, bool) — generic wrapper over stdlib errors.AsType
          prefer `ce, ok := errors.AsType[*errors.CodedError](err)` over the As dance
config/          ← viper implementation of core.Config
          config.Load(opts...) — WithFile / WithEnv / WithWatch / WithRequired /
          WithKnownKeys / WithLogger
          WithRequired(keys...) — Load fails with ErrMissingRequiredKey naming EVERY
          missing key at once (fail fast instead of zero values at runtime)
          WithKnownKeys(prefixes...) — Load logs one Warn listing undeclared keys
          (typo detection); warning only, never fatal
          cfg.UnknownKeys(declared) []string — programmatic access, to escalate yourself
          cfg.IsSet(key) bool — distinguishes "absent" from "present but zero"
          Watch returns ErrWatchDisabled when loaded without WithWatch
          Sentinels live in config/errors.go
logger/          ← log/slog + lumberjack; per-level log files; sensitive field masking
          logger.Default() is a DISCARD logger until logger.SetDefault is called
app/             ← lifecycle manager; see "app.Run phases" below for the full order
          Run() == RunContext(context.Background())
          RunContext(ctx) — cancelling ctx requests the same graceful shutdown a
          signal would; the shutdown sequence itself is not bound by ctx
          App.Stop() — programmatic shutdown; idempotent, safe before Run
          Options: WithName / WithVersion (both now appear in log output) /
          WithLogger / WithServer / WithMigration / WithLifecycle / WithCloser /
          WithBeforeStart / WithAfterStop / WithHealthCheck /
          WithShutdownTimeout(30s default) / WithDrainDelay(0 default)
          Interfaces are type ALIASES onto core: app.Lifecycle = core.Lifecycle etc.
          Sentinels: ErrPanic (any phase panicked), ErrServersExited (every
          Server.Start returned with no shutdown requested)
          WithHealthCheck checkers are auto-registered on every server implementing
          core.ReadyCheckRegistrar, before servers start — no manual bridging
          HealthCheckers() []core.HealthChecker — still there for non-server wiring
server/http/     ← gin server implementing core.Server; auto-mounts default middleware stack:
          RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler
          Do NOT re-add these middlewares manually — causes duplicate responses
          WithHealthChecks(...core.HealthChecker) — registers /readyz probes at construction
          AddReadyCheck(name, fn) — add /readyz probe after construction
          SetReady(bool) — implements core.ReadinessToggler; app drives it
server/grpc/     ← gRPC server implementing core.Server; auto-chains interceptors
          SetReady(bool) drives the gRPC health protocol ("" service)
          BREAKING: the health service now starts NOT_SERVING. It used to report
          SERVING from New(), i.e. before migrations and Lifecycle.Init had run.
          app flips it to SERVING once all servers started, and back on shutdown;
          a server used WITHOUT app must call SetReady(true) itself or health-aware
          clients will never route to it.
          Stop(ctx) now honours ctx: GracefulStop races the deadline and remaining
          connections are force-closed (ctx.Err() returned) instead of hanging
          forever on a long-lived stream. The grpcui teardown goroutine now fires.
middleware/http/ ← RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler
          + optional: CORS, Timeout, BodyLimit, RateLimit, IPFilter
          ErrorHandler also records the error on the active span, sets span status
          for 5xx, and increments *_http_errors_total{reason,code}
          Logger picks level by status class: <400 INFO, 4xx WARN, 5xx ERROR, and
          adds "error" + "error_reason" when the handler called c.Error — this is
          where the errors.Wrap cause chain (withheld from the body) survives
          ResetHTTPMetricHooks() — exported for tests; unwires metric.Init hooks
middleware/grpc/ ← UnaryRequestID, UnaryLogger, UnaryMetrics, UnaryRecovery, UnaryErrorHandler
          + optional: UnaryTimeout, UnaryRateLimit, UnaryIPFilter
          UnaryLogger: INFO on OK, WARN for caller-fault codes, ERROR otherwise;
          adds "code", and "error" + "error_reason" on failure
          UnaryMetrics labels with the real code name ("NotFound", "Unavailable"),
          not the old "OK"/"ERROR"; also feeds *_grpc_errors_total{method,status}
          UnaryErrorHandler maps 17 HTTP codes (was 7) and carries Reason +
          Metadata over the wire as google.rpc.ErrorInfo under ErrorInfoDomain
          ("forge"); context.DeadlineExceeded / Canceled map explicitly
          ResetGRPCMetricHooks() — exported for tests
database/        ← gorm multi-driver (mysql/postgres/sqlite); SQL tracing plugin; goose migrations
          database.New() → *Manager (no global side effects)
          database.Register("name", mgr) / database.Get("name") / database.DB("conn")
          database.CloseAll() error — closes and releases all registered managers
          *Manager implements core.Migrator; Manager.Close() releases all connections
          Manager.HealthChecker(connName) core.HealthChecker — probes SELECT 1
          New() returns ErrInvalidConfig when a WithConfig block fails to decode
          (previously the block was skipped silently, yielding zero connections)
cache/           ← Cache interface; Manager holds named backends (memory/Redis)
          cache.New() → *Manager (no global side effects)
          cache.Register("name", mgr) / cache.Client("name") / cache.RedisClient("name")
          *Manager implements core.Closer; Manager.CloseBackend("name") / Manager.CloseAll()
          Manager.HealthChecker(backendName) core.HealthChecker — memory: always healthy; redis: PING
          New() returns ErrInvalidConfig when the "cache" block fails to decode
          (previously produced zero backends with no diagnostic); an absent or
          empty block is still not an error
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
          New() ALWAYS installs the global text map propagator (W3C TraceContext +
          Baggage), even with no exporter configured — this is what connects
          traces across process boundaries. WithPropagators(...) overrides the set.
metric/          ← Prometheus + OTLP; implements app.Lifecycle and core.Metrics
          metric.New() → *Provider; p.Init(ctx) starts exporters + wires middleware hooks
          metric.Count / Gauge / GaugeAdd / GaugeSub / StartTimer — package-level
          facade taking metric.Label(...) options (NOT core.Field)
          p.Count / p.Gauge / p.Histogram — the core.Metrics methods, taking core.Field
          BREAKING: Init returns ErrNoExporters when neither metric.prometheus_addr
          nor metric.otlp_endpoint is set. Opt out with WithAllowNoExporters().
          ErrPrometheusListen — the Prometheus listener now binds synchronously, so
          a port clash fails Init instead of leaving /metrics silently unserved.
          Middleware hook wiring is no longer one-shot: the most recent Init wins.
          Gauge uses SET semantics (OTel sync gauge), NOT accumulation — both
          metric.Gauge and p.Gauge previously documented "sets" while adding.
          Use GaugeAdd / GaugeSub for deltas.
          WithReader(sdkmetric.Reader) — register an extra reader; pass
          sdkmetric.NewManualReader() in tests to assert on recorded instruments.
          A reader counts as a configured exporter, so Init won't return ErrNoExporters.
          No WithLogger option — use WithServiceName / WithPrometheusAddr /
          WithOTLPEndpoint / WithAllowNoExporters / WithConfig.
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

**core.Server contract** — `Start` must block; `Stop` must be called to unblock it. `app.WithServer` runs each server in a goroutine. `cron.Scheduler`, `stream.Bus.Worker()`, and both network servers implement this interface. If every registered server's `Start` returns while the app is still meant to be serving, `app.Run` returns `app.ErrServersExited` rather than blocking forever.

**Lifecycle contracts live in `core`** — `Migrator`, `Lifecycle`, `Closer`, `ReadinessToggler` and `ReadyCheckRegistrar` are declared in `core/lifecycle.go`, not in `app`. A library package (which must not import `app`) can therefore assert `var _ core.Lifecycle = (*T)(nil)` at compile time. `app` keeps type aliases, so `app.Lifecycle`, `app.Closer` and `app.Migrator` still compile unchanged.

**core.Lifecycle contract** — `Init` is called before servers start (sequential), `Shutdown` is called after servers stop (reverse order). If `Init[i]` fails, all previously-Init'd components are Shutdown in reverse **and every Closer is Closed** before `Run` returns. The same rollback now also happens when a `BeforeStart` hook or a migration fails — previously those paths leaked every pool already opened. Implemented by `tracing.Provider` and `metric.Provider`.

**core.Closer contract** — `Close` is called after all `Lifecycle.Shutdown` calls complete, in **reverse registration order** (it used to be forward order, which tore down a dependency before the Closer that used it). `Close` takes no context, so `app` runs each one in a goroutine and bounds the wait by the shutdown budget: a hung `Close` leaks that goroutine but no longer hangs the process. Implemented by `cache.Manager` and `grpcclient.Registry`.

**Metrics stub + late wiring** — `middleware/http` and `middleware/grpc` expose hook registration functions. These are no-ops until `metric.Provider.Init()` is called, breaking the import cycle between middleware and metric. The hook set is held in an `atomic.Pointer` and swapped as one immutable value, so wiring is race-free and repeatable: a second `Init` replaces the first set instead of being silently ignored (the old `sync.Once` guard meant a second Provider kept reporting through the first one's instruments, under the first one's service name). `ResetHTTPMetricHooks()` / `ResetGRPCMetricHooks()` restore the no-ops for tests.

### app.Run phases

```
 1. Readiness probes from WithHealthCheck registered on every server implementing
    core.ReadyCheckRegistrar — before anything serves traffic
 2. Migrator.Up()
 3. Lifecycle.Init()          sequential
 4. BeforeStart hooks         sequential
 5. Server.Start() × N        concurrent; then every server marked ready
 6. Block                     until signal / Stop() / ctx cancel / server failure
 7. Servers marked NOT ready, then WithDrainDelay is waited out
 8. Server.Stop() × N         concurrent, within the shutdown budget
 9. Lifecycle.Shutdown()      reverse registration order
10. Closer.Close()            reverse registration order
11. AfterStop hooks           sequential
```

Rules that are easy to get wrong:

- **Signals are trapped before migrations.** Previously `signal.Notify` ran only after servers launched, so a SIGTERM during a migration killed the process mid-migration.
- **A second SIGTERM/SIGINT abandons the remaining waits**, so an operator is never stuck with a process that ignores signals.
- **Each post-server phase gets its own deadline**, floored at 5s (`minPhaseTimeout`, capped by `WithShutdownTimeout`), so a slow server drain cannot starve telemetry flushing.
- **Panics are recovered** in every lifecycle callback, hook and server goroutine, logged with their stack, and reported as `app.ErrPanic`; the remaining shutdown phases still run.
- **Startup failure at phase 2, 3 or 4 rolls back** the already-init'd Lifecycles *and* runs the Closers.

### HealthChecker integration

Infrastructure packages that connect to external services provide a `HealthChecker` factory:

```go
// database.Manager and cache.Manager implement this pattern:
hc := dbMgr.HealthChecker("default")     // returns core.HealthChecker; probes SELECT 1
hc := cacheMgr.HealthChecker("redis")   // returns core.HealthChecker; probes PING (memory always healthy)
// hc.Name() → "database:default" or "cache:redis"
```

Wire into `/readyz` via:
- `app.WithHealthCheck(checkers...)` — **preferred**. `Run` registers every checker on
  each server implementing `core.ReadyCheckRegistrar` (both built-in servers do),
  before any server starts. No bridging hook is needed.
- `httpserver.WithHealthChecks(checkers...)` — at construction time, when you are not using `app`.
- `httpSrv.AddReadyCheck(name, fn)` — ad-hoc probe that is not a `core.HealthChecker`.

```go
// Correct, and all that is needed:
app.New(
    app.WithServer(httpSrv, grpcSrv),
    app.WithHealthCheck(dbMgr.HealthChecker("default"), cacheMgr.HealthChecker("redis")),
).Run()
```

> **Obsolete recipe — do not reintroduce it.** The old pattern below is now
> redundant; `WithHealthCheck` used to store the checkers and wire nothing.
>
> ```go
> // ❌ no longer necessary
> app.WithBeforeStart(func(ctx context.Context) error {
>     for _, hc := range a.HealthCheckers() {
>         httpSrv.AddReadyCheck(hc.Name(), hc.Check)
>     }
>     return nil
> })
> ```

If no registered server accepts probes, `Run` logs that the checkers went nowhere
instead of discarding them silently.

### Readiness is driven by app

Do **not** call `httpSrv.SetReady(true)` from a `BeforeStart` hook any more. `app`
marks every server implementing `core.ReadinessToggler` ready once all servers
have started, and not-ready at the very start of shutdown. Setting it true in
`BeforeStart` advertises readiness before the listener is up; combined with
`WithDrainDelay`, letting `app` own the flag is what gives a load balancer time to
stop routing into a draining instance.

`WithDrainDelay(d)` (default 0) is the pause between "marked not-ready" and
"servers stopped". Set it to roughly `periodSeconds × failureThreshold` of your
Kubernetes readiness probe, and keep `WithDrainDelay + WithShutdownTimeout` below
`terminationGracePeriodSeconds`.

### Error handling convention

```go
// Define a sentinel (package-level var)
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")

// Return with context
return nil, errors.Wrap(ErrUserNotFound, "query user: %w", dbErr)

// Check type
if errors.Is(err, ErrUserNotFound) { ... }

// Extract — prefer AsType over the As declare-then-point dance
if ce, ok := errors.AsType[*errors.CodedError](err); ok { _ = ce.Reason }

// HTTP handler — use c.Error(); ErrorHandler middleware converts to JSON
c.Error(err)   // NOT c.JSON(500, gin.H{"error": err.Error()})
```

Non-`CodedError` values returned from handlers produce a generic 500 (message not forwarded).

**The cause chain is never in the response body — but it is no longer lost.**
`errors.Wrap` context stays out of both the HTTP body and the gRPC status message.
It is now recorded on the active span by `middleware/http.ErrorHandler` and written
as the `error` / `error_reason` fields by `middleware/http.Logger` and
`middleware/grpc.UnaryLogger`. When debugging a 500, read the log line or the span,
not the response.

**gRPC clients should dispatch on `Reason`, not the message.**
`middleware/grpc.UnaryErrorHandler` attaches `CodedError.Reason` and `.Metadata` as a
`google.rpc.ErrorInfo` detail with `Domain == grpcMiddleware.ErrorInfoDomain` ("forge"):

```go
for _, d := range status.Convert(err).Details() {
    if info, ok := d.(*errdetails.ErrorInfo); ok && info.Domain == grpcMiddleware.ErrorInfoDomain {
        _ = info.Reason   // "USER_NOT_FOUND"
    }
}
```

### Context discipline

Every log call receives `ctx`. The logger extracts `trace_id` / `span_id` automatically from any active OTel span. All outbound clients (`httpclient`, `grpcclient`, `stream`) propagate trace context through headers/metadata.

### Trace propagation

`tracing.New()` installs the global OTel text map propagator — W3C TraceContext
(`traceparent` / `tracestate`) plus W3C Baggage — **unconditionally**, before the
no-exporter early return. Propagation and export are independent concerns: a
process that exports nothing itself (sampler 0, no collector, a thin proxy, a test
binary) must still forward inbound trace context downstream, or those downstream
spans start brand-new disconnected traces.

This means `tracing.New()` must be called even when tracing is "off", otherwise
the global propagator stays the delegating no-op and the trace chain breaks.
`tracing.WithPropagators(...)` replaces the set (combined with
`propagation.NewCompositeTextMapPropagator`) — only needed to interoperate with a
different wire format, e.g. B3 for a legacy Zipkin mesh. Passing no propagators
leaves whatever is already global untouched.

### Configuration fail-fast

Declare what a service cannot start without, instead of discovering zero values at
runtime:

```go
cfg, err := config.Load(
    config.WithFile("config.yaml"),
    config.WithEnv("APP"),
    config.WithRequired("database.default.dsn", "cache.redis.addr"),
    config.WithKnownKeys("app", "server", "logger", "database", "cache", "metric"),
)
```

- `WithRequired` → `Load` fails with `ErrMissingRequiredKey` naming **every** missing
  key at once, so a misconfigured deployment needs one restart to diagnose, not one
  per key. A key supplied only via `WithEnv` satisfies it; a key present but empty
  counts as missing.
- `WithKnownKeys` → one Warn listing keys nothing declared (in practice a typo like
  `databse.default.dsn`). Warning only, deliberately: config files legitimately carry
  keys this library knows nothing about. Use `cfg.UnknownKeys(declared)` to escalate
  it to an error yourself.
- `cfg.IsSet(key)` distinguishes "absent" from "present but zero" — `GetInt` returns
  0 for both a missing key and `port: 0`. It is part of `core.Config`, so no type
  assertion is needed.

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

### core.Migrator interface

Defined in `core/lifecycle.go` (not in `database`, and no longer in `app`) to avoid an
import cycle while still letting `*database.Manager` assert the contract at compile
time. `app.Migrator` is a type alias onto it. Pass via `app.WithMigration(manager)` to
run goose migrations before `BeforeStart` hooks fire.

### cron sub-second intervals

`robfig/cron/v3` truncates `@every` durations below 1 second. `Scheduler.Add` detects sub-second specs and routes them through a `time.Ticker` goroutine instead.

### idgen worker ID in production

`idgen.New()` without `WithWorkerID` auto-derives the worker ID from the last byte of the host's primary IPv4 address and emits a Warn log. This is unstable in containerized environments (IP can change, collisions possible). Always configure `idgen.worker_id` explicitly in production:

```yaml
idgen:
  worker_id: 42   # unique per deployed instance
```

### example repository

Runnable examples live in [`github.com/pemako/forge-example`](https://github.com/pemako/forge-example),
not in this repo. It holds four independent modules — `01-quickstart`,
`02-http-api`, `03-grpc-service`, `04-database` — each depending on a published
`forge` commit pseudo-version rather than a replace directive, so they build
from a fresh clone.

To test a local forge change against the examples, clone forge-example as a
sibling directory and link the two with a workspace file (gitignored there):

```bash
cd ../forge-example
go work init ./01-quickstart ./02-http-api ./03-grpc-service ./04-database
go work use ../forge
```

After a release, bump the examples with `go get github.com/pemako/forge@main && go mod tidy`
inside each module.
