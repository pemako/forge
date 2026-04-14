# Getting Started with mk

This guide gives you four task-oriented recipes. Each one shows **only the packages you need** for that scenario and explicitly lists what you can ignore.

For a full package reference see [README.md](../README.md).
For architecture decisions see [ARCHITECTURE.md](../ARCHITECTURE.md).

---

## Recipe 1: Minimal HTTP service

**You need:** `config`, `logger`, `server/http`, `app`, `errors`

**You can ignore for now:** tracing, metric, database, cache, auth, validate, pagination, notify, stream, cron, storage, circuitbreaker, idgen, pprof, ratelimit

```go
package main

import (
    "context"
    "net/http"
    "os"

    "github.com/gin-gonic/gin"
    "github.com/pemako/mk/app"
    "github.com/pemako/mk/config"
    "github.com/pemako/mk/errors"
    "github.com/pemako/mk/logger"
    httpserver "github.com/pemako/mk/server/http"
)

func main() {
    cfg, _ := config.Load()
    log, _ := logger.New(logger.WithConfig(cfg))

    srv, _ := httpserver.New(
        httpserver.WithConfig(cfg),
        httpserver.WithLogger(log),
        httpserver.WithServiceName("my-service"),
    )

    srv.Engine().GET("/users/:id", getUser)

    if err := app.New(app.WithServer(srv)).Run(); err != nil {
        log.Error(context.Background(), "run failed", logger.F("err", err))
        os.Exit(1)
    }
}

func getUser(c *gin.Context) {
    id := c.Param("id")
    if id == "" {
        c.Error(errors.Wrap(errors.ErrBadRequest, "id is required"))
        return
    }
    c.JSON(http.StatusOK, gin.H{"id": id})
}
```

**Key things to know:**

- `server/http.New()` **automatically mounts** RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler. Do not add them again via `Engine().Use()`.
- Return errors from handlers via `c.Error(err)`, not `c.JSON(500, ...)`. The pre-mounted `ErrorHandler` converts `*errors.CodedError` to the right HTTP status and JSON body automatically.
- `app.New(...).Run()` blocks until SIGTERM/SIGINT, then shuts down gracefully.

---

## Recipe 2: HTTP service with database and Redis cache

**You need:** everything in Recipe 1, plus `database`, `cache`

**You can ignore for now:** tracing, metric, auth, validate, notify, stream, cron, storage, circuitbreaker, idgen, pprof, ratelimit

```go
package main

import (
    "context"
    "os"

    "github.com/pemako/mk/app"
    "github.com/pemako/mk/cache"
    "github.com/pemako/mk/config"
    "github.com/pemako/mk/database"
    "github.com/pemako/mk/logger"
    httpserver "github.com/pemako/mk/server/http"
)

func main() {
    cfg, _ := config.Load()
    log, _ := logger.New(logger.WithConfig(cfg))

    // Database — *Manager implements app.Migrator (runs goose migrations at startup).
    dbMgr, _ := database.New(database.WithConfig(cfg))
    database.Register("default", dbMgr)

    // Cache — Manager implements app.Closer (released on shutdown).
    cacheMgr, _ := cache.New(cache.WithConfig(cfg))
    cache.Register("default", cacheMgr)

    srv, _ := httpserver.New(
        httpserver.WithConfig(cfg),
        httpserver.WithLogger(log),
        // Wire health checks so /readyz reflects real dependency status.
        httpserver.WithHealthChecks(
            dbMgr.HealthChecker("default"),
            cacheMgr.HealthChecker("redis"),
        ),
    )

    // Register routes — use database.DB("main") and cache.Client("default") in handlers.
    registerRoutes(srv.Engine())

    if err := app.New(
        app.WithMigration(dbMgr),   // runs migrations before servers start
        app.WithServer(srv),
        app.WithCloser(cacheMgr),   // closes Redis pool on shutdown
    ).Run(); err != nil {
        log.Error(context.Background(), "run failed", logger.F("err", err))
        os.Exit(1)
    }
}
```

**config.yaml additions:**

```yaml
database:
  default:
    driver: mysql
    dsn: "user:pass@tcp(localhost:3306)/mydb?parseTime=true"
    migrations_dir: ./migrations

cache:
  default:
    backend: memory       # in-process (dev/test)
  redis:
    backend: redis
    addr: "localhost:6379"
```

**Key things to know:**

- `database.Register("default", mgr)` makes `database.DB("connName")` available globally. Always create with `New()` first, then register — never pass options to `Register`.
- `app.WithMigration(dbMgr)` runs goose migrations **before** BeforeStart hooks and server startup. No migrations run if not set.
- `cacheMgr.HealthChecker("redis")` probes `PING`; `dbMgr.HealthChecker("default")` probes `SELECT 1`. Both feed `/readyz` automatically once wired.
- Cache TTL semantics differ: Redis is exact; memory backend uses a lazy sweep. Do not rely on sub-second TTL with the memory backend. See [ARCHITECTURE.md §13](./ARCHITECTURE.md) for details.

---

## Recipe 3: Pure worker service (no HTTP)

**Scenario:** background jobs (cron), stream consumers, or a mix. No inbound HTTP traffic — or only a minimal health endpoint.

**You need:** `app`, `config`, `logger`, and one or more of: `cron`, `stream`, `cache` (for Redis)

**You can ignore:** server/http, server/grpc, database (unless the job needs it), auth, validate, pagination, storage, notify

### Option A: cron scheduler

```go
package main

import (
    "context"
    "os"
    "time"

    "github.com/pemako/mk/app"
    "github.com/pemako/mk/config"
    "github.com/pemako/mk/cron"
    "github.com/pemako/mk/logger"
)

func main() {
    cfg, _ := config.Load()
    log, _ := logger.New(logger.WithConfig(cfg))

    scheduler := cron.New(cron.WithLogger(log))

    scheduler.Add("@every 1m", func(ctx context.Context) error {
        log.Info(ctx, "tick")
        return nil
    }, cron.WithTimeout(30*time.Second))

    // cron.Scheduler implements core.Server — Start blocks, Stop drains.
    if err := app.New(app.WithServer(scheduler)).Run(); err != nil {
        log.Error(context.Background(), "run failed", logger.F("err", err))
        os.Exit(1)
    }
}
```

### Option B: Redis Streams consumer

```go
package main

import (
    "context"
    "os"

    "github.com/pemako/mk/app"
    "github.com/pemako/mk/cache"
    "github.com/pemako/mk/config"
    "github.com/pemako/mk/logger"
    "github.com/pemako/mk/stream"
)

func main() {
    cfg, _ := config.Load()
    log, _ := logger.New(logger.WithConfig(cfg))

    cacheMgr, _ := cache.New(cache.WithConfig(cfg))
    cache.Register("default", cacheMgr)

    bus, _ := stream.New(cache.RedisClient("redis"))

    bus.Subscribe("orders",
        stream.WithGroup("order-processor"),
        stream.WithHandler(func(ctx context.Context, msg stream.Message) error {
            log.Info(ctx, "received order", logger.F("id", msg.ID))
            return nil
        }),
    )

    // bus.Worker() implements core.Server.
    if err := app.New(
        app.WithServer(bus.Worker()),
        app.WithCloser(cacheMgr),
    ).Run(); err != nil {
        log.Error(context.Background(), "run failed", logger.F("err", err))
        os.Exit(1)
    }
}
```

### Option C: worker + minimal health endpoint

```go
srv, _ := httpserver.New(httpserver.WithAddr(":8081"), httpserver.WithLogger(log))
// No business routes — only /healthz and /readyz are registered automatically.

app.New(
    app.WithServer(srv),
    app.WithServer(scheduler),   // two servers run concurrently
).Run()
```

**Key things to know:**

- Any type that implements `core.Server` (`Start` blocks, `Stop` unblocks) can be passed to `app.WithServer`. `cron.Scheduler` and `stream.Bus.Worker()` both implement it.
- Multiple servers are started **concurrently**. Order within `Start` is not guaranteed.
- `stream` has two dispatch modes: `WithGroup` (work queue — one consumer per message) and `WithBroadcast` (fan-out — every subscriber receives every message).
- For distributed cron (only one instance runs a job per tick), add `cron.WithDistributedLock(locker)`.

---

## Recipe 4: What to ignore until you need it

Not every package is needed from day one. Here is a pragmatic reading order:

### Start with these (Recipe 1 level)

| Package | Why you need it |
|---------|----------------|
| `core` | Interfaces — you'll reference them but rarely import directly |
| `errors` | Structured errors for all handlers |
| `config` | Config loading — one call, then pass `cfg` to everything |
| `logger` | Structured logging — one instance, inject everywhere |
| `server/http` | Your HTTP server |
| `app` | Process lifecycle — the entry point |

### Add when you connect to external systems

| Package | Add when... |
|---------|------------|
| `database` | You need SQL (MySQL / Postgres / SQLite) |
| `cache` | You need in-memory or Redis cache |
| `httpclient` | You call other HTTP services |
| `grpcclient` | You call gRPC services |
| `lock` | You need distributed locking (requires Redis) |
| `stream` | You need cross-instance pub/sub (requires Redis) |
| `storage` | You need object storage (S3 / OSS / COS) |

### Add for observability

| Package | Add when... |
|---------|------------|
| `tracing` | You want distributed traces (OTLP) |
| `metric` | You want Prometheus / OTLP metrics |

Both implement `app.Lifecycle` and are no-ops when their endpoint is not configured — safe to wire early.

### Add for specific features (Opinionated Extensions)

These packages are production-ready but opinionated. Skip them if you have your own equivalent.

| Package | Skip if... |
|---------|-----------|
| `auth` | You use a different JWT library or an API gateway handles auth |
| `validate` | You use a different validation library |
| `pagination` | Your API uses cursor-based or different pagination |
| `ratelimit` | You rate-limit at the gateway layer |
| `circuitbreaker` | You use a different circuit breaker |
| `notify` | You have your own notification service |
| `idgen` | You use UUIDs or a different ID scheme |
| `pprof` | You expose pprof through another mechanism |
| `cron` | You use an external job scheduler |

### Concepts you can learn as you go

These concepts appear in the docs but you do not need to understand them on day one:

| Concept | When it matters |
|---------|----------------|
| `app.Lifecycle` vs `app.Closer` | When you add tracing/metric (they implement Lifecycle) or cache/grpcclient (they implement Closer) |
| Metric late wiring | Only relevant if you are adding a new middleware — the metric-middleware import cycle is already solved internally |
| Registry semantics (`Register` vs `Replace`) | When you need hot-reload or dynamic reconfiguration of a named backend |
| `DefaultMiddlewareStack` | When you need a custom middleware stack on server/http |
| `core.HealthChecker` wiring | When you add database or cache and want them reflected in `/readyz` |
