# mk Architecture Guide

## 1. Project Scope

`mk` is an instance-first Go service framework. It solves cross-cutting concerns
(lifecycle, config, logging, tracing, metrics, HTTP/gRPC servers, databases, cache,
messaging) for long-running Go services.

## 2. Package Layers and Dependency Direction

```
core/           ← interfaces only; zero deps on other mk packages
errors/         ← CodedError; zero deps on other mk packages
config/logger/  ← infrastructure leaf packages; import core/ only
tracing/metric/ ← infrastructure leaf packages; import core/ only
server/http/grpc ← import core/, errors/, middleware/*
middleware/*    ← import core/, errors/; late-wire metric via hooks
app/            ← import core/ only; consumers pass concrete types
database/cache/ ← import core/, errors/, logger/
lock/stream/    ← import core/, errors/, logger/, cache/ (for RedisClient)
auth/validate/  ← import core/, errors/, server/http (via gin)
```

**Hard rules:**
- `core/` and `errors/` MUST NOT import any other `mk` package
- `app/` MUST NOT import `database`, `cache`, `metric`, `tracing`, or any
  infrastructure package — callers pass in interface values
- New packages MUST NOT import `server/http`, `server/grpc`, `middleware/*`, or `app`

## 3. Global vs Instance

### Always instance-first
These packages have no package-level state. Always create via `New()`:
- `stream.Bus`
- `event.Bus`
- `tracing.Provider`
- `metric.Provider`

### Instance-first with named registry convenience
Create via `New()`, register explicitly via `Register(name, instance)`.
Package-level convenience functions delegate to `Get("default")`:
- `database` — `New()` → `Register("name", mgr)` → `database.DB("conn")`
- `cache` — `New()` → `Register("name", mgr)` → `cache.Client("name")`
- `httpclient`, `grpcclient`, `storage`, `idgen`

### Global facade (intentional exceptions)
These retain global helpers to break import cycles or for genuinely stateless ops:
- `logger` — `Default()` / `SetDefault()` for bootstrap convenience
- `metric` — `Count/Gauge/StartTimer` facade; needed by `middleware/*` to avoid
  import cycle. Tests and multi-provider scenarios use `metric.New()` directly.

## 4. Registry Semantics

All named registry packages follow this contract:

| Function | Behavior |
|----------|----------|
| `Register(name, instance)` | Returns `ErrAlreadyRegistered` if name exists |
| `Replace(name, instance)` | Closes old resource (if any), stores new |
| `Get(name)` | Returns nil if not found |
| `Close(name)` | Closes + removes from registry |
| `CloseAll()` | Closes all + clears registry |
| `reset()` (unexported) | Clears registry; test use only |

## 5. Lifecycle Roles

Three interfaces govern component lifecycle:

```go
// core.Server — blocking long-running component (HTTP server, gRPC server, cron, stream worker)
// Start blocks; Stop unblocks it.
type Server interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// app.Lifecycle — non-blocking resource with init/shutdown (metric provider, tracing provider)
// Init called before servers start; Shutdown called after servers stop (reverse order).
type Lifecycle interface {
    Init(ctx context.Context) error
    Shutdown(ctx context.Context) error
}

// app.Closer — resource released after shutdown (cache manager, gRPC registry)
type Closer interface {
    Close() error
}
```

`app.Run()` lifecycle order:
1. `Migration.Up()`
2. `Lifecycle.Init()` — sequential, first error aborts + reverse-shutdown already-init'd
3. `BeforeStart` hooks
4. `Server.Start()` — concurrent
5. Wait for SIGTERM/SIGINT or server error
6. `Server.Stop()` — concurrent, errors logged (not returned if runErr set)
7. `Lifecycle.Shutdown()` — reverse order
8. `Closer.Close()` — sequential
9. `AfterStop` hooks

## 6. event vs stream

| | `event` | `stream` |
|---|---|---|
| Transport | In-process only | Redis Streams (cross-instance) |
| Use when | Decoupling modules within one process | Multiple service replicas coordinate |
| Instance | `event.New()` | `stream.New(redisClient)` |

## 7. Redis Connection Sharing

`cache` is the Redis connection registry. Packages needing `*redis.Client` (lock, stream)
call `cache.RedisClient("backend-name")` or `cacheManager.RedisClient("backend-name")`.
Never open a second Redis connection to the same address.

```yaml
cache:
  default:
    backend: memory
  redis:
    backend: redis
    addr: "redis:6379"
```
```go
locker := lock.NewWithClient(cache.RedisClient("redis"))
bus, _ := stream.New(cache.RedisClient("redis"))
```

## 8. New Package Checklist

Before writing any code:
- [ ] Does it need a `core/` interface? (yes if other packages depend on it)
- [ ] Does it manage shared state? → named registry pattern
- [ ] Does it hold a blocking goroutine? → implement `core.Server`
- [ ] Does it need Redis? → get from `cache.RedisClient()`, never open new connection
- [ ] Does it have a lifecycle? → implement `app.Lifecycle` or `app.Closer`
