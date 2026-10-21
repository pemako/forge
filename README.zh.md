# Forge

Forge 是一个面向生产环境的 Go 后端服务工具包，提供开箱即用的基础设施组件和统一的工程规范。

> **模块路径**：`github.com/pemako/forge` · **Go 版本**：1.26+

---

## 目录

- [设计理念](#设计理念)
- [快速开始](#快速开始)
- [包分层](#包分层)
- [核心概念](#核心概念)
- [典型接入路径](#典型接入路径)
- [各包使用说明](#各包使用说明)
- [配置参考](#配置参考)
- [错误处理](#错误处理)
- [可观测性](#可观测性)
- [常见问题](#常见问题)

---

## 设计理念

### 实例优先（Instance-First）

Forge 的所有包都优先使用显式实例，而非包级别的全局单例。

```go
// ✅ 推荐：显式创建实例
mgr, err := cache.New(cache.WithMemory("default"), cache.WithRedis("redis", "redis:6379"))
cache.Register("default", mgr)   // 按需注册为全局可访问
c := mgr.Client("default")       // 通过实例访问

// 便捷访问：委托给默认管理器（小型服务可用）
c := cache.Client("default")
```

### 注册表语义（Registry Semantics）

`database`、`cache`、`httpclient`、`grpcclient`、`storage`、`idgen` 等包遵循统一的注册表契约：

| 函数                       | 行为                                          |
| -------------------------- | --------------------------------------------- |
| `Register(name, instance)` | 名称重复时返回 `ErrAlreadyRegistered`，不覆盖 |
| `Replace(name, instance)`  | 关闭旧实例后存入新实例                        |
| `Get(name)`                | 不存在时返回 nil                              |
| `CloseAll() error`         | 释放所有资源并清空注册表                      |

**注意**：`Register` 接收的是已创建好的实例，而非配置选项。先用 `New()` 创建，再传给 `Register`——这让构造错误显式可见。

### 函数式选项（Functional Options）

所有包构造函数遵循统一形式：

```go
func New(opts ...Option) (*T, error)

// 通用选项（所有基础设施包都提供）
func WithConfig(cfg core.Config) Option  // 从配置读取参数
func WithLogger(l core.Logger) Option    // 注入日志器
```

---

## 快速开始

```bash
go get github.com/pemako/forge
```

**5 分钟示例**（随机端口，无外部依赖）：

```go
package main

import (
  "context"
  "net/http"
  "os"

  "github.com/gin-gonic/gin"
  "github.com/pemako/forge/app"
  "github.com/pemako/forge/config"
  "github.com/pemako/forge/errors"
  "github.com/pemako/forge/logger"
  httpserver "github.com/pemako/forge/server/http"
)

func main() {
  cfg, _ := config.Load()
  log, _ := logger.New(logger.WithConfig(cfg))

  srv, _ := httpserver.New(
    httpserver.WithConfig(cfg),
    httpserver.WithLogger(log),
    httpserver.WithServiceName("my-service"),
  )

  srv.Engine().GET("/hello/:name", func(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"message": "hello, " + c.Param("name")})
  })

  if err := app.New(app.WithServer(srv), app.WithLogger(log)).Run(); err != nil {
    log.Error(context.Background(), "退出", logger.F("err", err))
    os.Exit(1)
  }
}
```

> `server/http.New()` 已自动挂载：`RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler`，**不要重复添加**。

---

## 包分层

Forge 将 27 个包划分为四个层级，具有不同的稳定性承诺：

| 层级                    | 包                                                                                                                         | 稳定性                                             |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| **Tier 1 核心稳定层**   | `core` `errors` `app` `config` `logger`                                                                                    | 接口和行为冻结，破坏性变更需要主版本号升级         |
| **Tier 2 基础设施集成** | `server/*` `middleware/*` `database` `cache` `tracing` `metric` `httpclient` `grpcclient` `lock` `stream` `cron` `storage` | API 稳定，仅追加变更；外部依赖升级可能带来行为变化 |
| **Tier 3 观点型扩展**   | `auth` `validate` `pagination` `ratelimit` `notify` `idgen` `circuitbreaker` `pprof`                                       | 可用于生产，但有一定约束性；团队可用自己的实现替换 |
| **开发工具**            | `testutil` `event`                                                                                                         | 辅助工具，无跨服务 API 契约                        |

> **实践建议**：从核心稳定层起步，按需引入基础设施集成，Tier 3 扩展包按实际约定决定是否采用。

---

## 核心概念

### app.Run() 生命周期

```
① Migrator.Up()            数据库迁移（可选）
② Lifecycle.Init()         顺序初始化；任一失败则逆序回滚
③ BeforeStart hooks        启动前钩子；顺序执行
④ Server.Start() × N       所有服务器并发启动
⑤ 阻塞等待                  SIGTERM / SIGINT 或服务器错误
⑥ Server.Stop() × N        并发优雅停止（shutdownTimeout）
⑦ Lifecycle.Shutdown()     逆序关闭
⑧ Closer.Close()           顺序释放资源
⑨ AfterStop hooks          停止后钩子
```

**三个生命周期接口**：

```go
// app.Lifecycle — 有初始化/关闭逻辑的组件（tracing、metric）
type Lifecycle interface {
  Init(ctx context.Context) error
  Shutdown(ctx context.Context) error
}

// app.Closer — 需要释放连接的组件（cache.Manager、grpcclient.Registry）
type Closer interface {
  Close() error
}

// app.Migrator — 需要在启动前执行迁移的组件（database.Manager）
type Migrator interface {
  Up(ctx context.Context) error
}
```

### core.Server 契约

`Start` 必须阻塞，`Stop` 让 `Start` 解除阻塞：

```go
type Server interface {
  Start(ctx context.Context) error  // 阻塞直到 Stop 被调用
  Stop(ctx context.Context) error   // 优雅停止
}
```

实现了此接口的：`server/http`、`server/grpc`、`cron.Scheduler`、`stream.Bus.Worker()`。

---

## 典型接入路径

完整服务的 `main.go` 初始化顺序：

```go
// ① 配置 + 日志（注入到所有下游组件）
cfg, _ := config.Load()
log, _ := logger.New(logger.WithConfig(cfg))

// ② 可观测性（实现 app.Lifecycle）
tp, _ := tracing.New(tracing.WithConfig(cfg), tracing.WithLogger(log))
mp, _ := metric.New(metric.WithConfig(cfg), metric.WithLogger(log))

// ③ 数据存储（database → WithMigration，cache → WithCloser）
dbMgr, _ := database.New(database.WithConfig(cfg), database.WithLogger(log))
database.Register("default", dbMgr)

cacheMgr, _ := cache.New(cache.WithConfig(cfg), cache.WithLogger(log))
cache.Register("default", cacheMgr)

// ④ 基础设施客户端（可选）
locker := lock.NewWithClient(cache.RedisClient("redis"), lock.WithLogger(log))
httpclient.Register("downstream", httpclient.New(httpclient.WithBaseURL("...")))

// ⑤ HTTP 服务器（含健康检查）
httpSrv, _ := httpserver.New(
  httpserver.WithConfig(cfg),
  httpserver.WithLogger(log),
  httpserver.WithHealthChecks(
    dbMgr.HealthChecker("default"),   // /readyz: SELECT 1
    cacheMgr.HealthChecker("redis"),  // /readyz: PING
  ),
)

// ⑥ 注册路由
httpSrv.Engine().GET("/path", ginHandler)

// ⑦ 组装并运行
app.New(
  app.WithServer(httpSrv),
  app.WithMigration(dbMgr),
  app.WithLifecycle(tp, mp),
  app.WithCloser(cacheMgr),
  app.WithLogger(log),
).Run()
```

---

## 各包使用说明

### config — 配置加载

```go
// 优先级：默认值 < 配置文件 < 环境变量
cfg, err := config.Load(
  config.WithFile("config.yaml"),  // 默认搜索当前目录
  config.WithEnv("APP"),           // APP_SERVER_HTTP_ADDR 覆盖 server.http.addr
)

// 读取配置值
addr := cfg.GetString("server.http.addr")
timeout := cfg.GetDuration("server.http.timeout")

// 热重载
cfg.Watch("feature.flag", func(v any) {
  log.Info(ctx, "配置变更", logger.F("value", v))
})
```

### logger — 结构化日志

```go
log, _ := logger.New(
  logger.WithConfig(cfg),
  logger.WithLevel("info"),
)
logger.SetDefault(log) // 设为全局默认

// 使用（ctx 自动提取 trace_id/span_id）
log.Info(ctx, "用户登录", logger.F("user_id", 123))
log.Error(ctx, "查询失败", logger.F("err", err))

// 敏感字段自动脱敏（配置 mask_fields: [password, token]）
```

**配置示例**：

```yaml
logger:
  level: info # debug | info | warn | error
  output: both # console | file | both
  filename: logs/app # 按级别分文件：app.info.log, app.error.log
  mask_fields: [password, token, credit_card]
```

### database — 数据库

```go
dbMgr, _ := database.New(
  database.WithConfig(cfg),
  database.WithLogger(log),
  database.WithMigrationsDir("migrations"), // goose 迁移目录
)
database.Register("default", dbMgr)

// 获取 *gorm.DB
db := database.DB("default") // 连接名称（DSN 里配置的）

// 在 Handler 中使用
err := database.DB("default").WithContext(ctx).
  Where("email = ?", email).First(&user).Error
```

**配置示例**：

```yaml
database:
  default: # Manager 注册名
    driver: mysql # mysql | postgres | sqlite
    dsn: "user:pass@tcp(localhost:3306)/mydb?parseTime=true"
    max_open_conns: 20
    max_idle_conns: 5
    slow_threshold: 200ms
    trace_all_sql: false
```

### cache — 缓存

```go
cacheMgr, _ := cache.New(
  cache.WithMemory("default"),         // 内存后端（开发/测试）
  cache.WithRedis("redis", "localhost:6379"), // Redis 后端（生产）
)
cache.Register("default", cacheMgr)

// 使用
c := cache.Client("default")
c.Set(ctx, "key", "value", 5*time.Minute)
val, err := c.Get(ctx, "key")
if errors.Is(err, cache.ErrCacheMiss) { /* 未命中 */ }

// 获取底层 Redis 客户端（供 lock/stream 共用）
rdb := cache.RedisClient("redis")
```

> **注意**：Redis 使用 RESP2 协议（`Protocol: 2`），与 lock/stream 共享同一连接池，**不要为 lock/stream 单独创建 Redis 连接**。

### errors — 结构化错误

```go
// 定义哨兵错误（包级别）
var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "用户不存在")
var ErrEmailTaken   = errors.Define(409, "EMAIL_TAKEN",   "邮箱已被注册")

// 返回时附加上下文
return nil, errors.Wrap(ErrUserNotFound, "query user id=%d", id)

// 检查错误类型
if errors.Is(err, ErrUserNotFound) { ... }

// HTTP Handler 中：用 c.Error()，由 ErrorHandler 中间件统一转换为 JSON
func (h *Handler) Get(c *gin.Context) {
  user, err := h.svc.Get(c.Request.Context(), id)
  if err != nil {
    c.Error(err) // ❌ 不要用 c.JSON(500, ...)
    return
  }
  c.JSON(200, user)
}
```

**错误响应格式**（ErrorHandler 中间件自动生成）：

```json
{
  "code": 404,
  "reason": "USER_NOT_FOUND",
  "message": "用户不存在"
}
```

### server/http — HTTP 服务器

```go
srv, _ := httpserver.New(
  httpserver.WithConfig(cfg),
  httpserver.WithLogger(log),
  httpserver.WithServiceName("order-svc"),
  httpserver.WithHealthChecks(dbMgr.HealthChecker("default")),
)

// 注册路由（Gin 风格）
r := srv.Engine()
r.GET("/orders/:id", handler.GetOrder)
```

**内置端点**：
| 路径 | 说明 |
|------|------|
| `/healthz` | 存活检查，始终返回 200 |
| `/readyz` | 就绪检查，所有 HealthChecker 通过才返回 200 |

**自定义中间件栈**：

```go
// 完全替换默认中间件栈
srv, _ := httpserver.New(
  httpserver.WithMiddlewareStack(
    mw.RequestID(),
    myAuthMiddleware(),
    mw.Recovery(),
    mw.ErrorHandler(),
  ),
)

// 在默认栈基础上追加
stack := httpserver.DefaultMiddlewareStack("svc", log)
stack = append(stack, myMetricsMiddleware())
srv, _ := httpserver.New(httpserver.WithMiddlewareStack(stack...))

// 裸模式（不挂任何中间件）
srv, _ := httpserver.New(httpserver.WithNoMiddleware())
```

### auth — JWT 认证

```go
// 定义 Claims 结构
type UserClaims struct {
  jwt.RegisteredClaims
  UserID int64  `json:"uid"`
  Role   string `json:"role"`
}

// 生成 Token
token, err := auth.Generate(&UserClaims{
  RegisteredClaims: jwt.RegisteredClaims{
    ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
  },
  UserID: user.ID,
  Role:   "user",
}, auth.WithConfig(cfg)) // 从 auth.secret 读取密钥

// 路由保护（Gin 中间件）
protected := r.Group("/api/v1")
protected.Use(auth.Required(
  auth.WithConfig(cfg),
  auth.WithClaims(func() auth.Claims { return &UserClaims{} }),
))

// Handler 中提取 Claims
claims, ok := auth.FromContext[*UserClaims](c.Request.Context())
```

### validate — 请求校验

```go
type CreateUserReq struct {
  Name  string `json:"name"  validate:"required,min=2,max=64"`
  Email string `json:"email" validate:"required,email"`
  Age   int    `json:"age"   validate:"min=0,max=150"`
}

func (h *Handler) Create(c *gin.Context) {
  var req CreateUserReq
  if err := validate.BindJSON(c, &req); err != nil {
    c.Error(err) // 自动返回 422 + 字段级错误信息
    return
  }
  // ...
}
```

### stream — Redis Streams 消息队列

```go
// 工作队列模式（集群中只有一个实例消费每条消息）
bus, _ := stream.New(cache.RedisClient("redis"))
bus.Subscribe("order.created",
  stream.WithGroup("order-processor"),
  stream.WithRetries(3),
  stream.WithHandler(func(ctx context.Context, msg stream.Message) error {
    // 处理消息
    return nil
  }),
)

// 广播模式（每个实例都收到每条消息）
bus.Subscribe("config.reload",
  stream.WithBroadcast(),
  stream.WithHandler(reloadHandler),
)

// 注册为 core.Server，随 app 启动和停止
app.New(app.WithServer(bus.Worker()), app.WithCloser(cacheMgr)).Run()

// 发布消息
bus.Publish(ctx, "order.created", map[string]any{"order_id": 123})
```

### cron — 定时任务

```go
scheduler := cron.New(cron.WithLogger(log))

// 标准 cron 表达式
scheduler.Add("0 */6 * * *", cleanupJob, cron.WithTimeout(10*time.Minute))

// 间隔执行
scheduler.Add("@every 30s", healthCheckJob)

// 分布式锁（集群中只有一个实例执行）
scheduler.Add("@every 1m", syncJob,
  cron.WithDistributedLock(locker),
  cron.WithTimeout(55*time.Second),
)

app.New(app.WithServer(scheduler)).Run()
```

### tracing — 链路追踪

```go
tp, _ := tracing.New(
  tracing.WithConfig(cfg),       // 从 tracing.endpoint 读取 OTLP 地址
  tracing.WithLogger(log),
  tracing.WithServiceVersion("v1.2.0"),
  tracing.WithEnvironment("production"),
  // 自定义资源属性（附加到每条 trace）
  tracing.WithAttributes(
    attribute.String("region", "cn-hangzhou"),
    attribute.String("cluster", "prod-01"),
  ),
)

// 注册为 app.Lifecycle，Init 为 no-op，Shutdown 冲刷待上报的 span
app.New(app.WithLifecycle(tp)).Run()
```

**配置示例**：

```yaml
tracing:
  endpoint: "otel-collector:4317" # gRPC 端口自动选 OTLP/gRPC，其余用 HTTP
  sampler: 0.1 # 采样率，1.0 = 全采
  tls: false # 是否启用 TLS
```

### lock — 分布式锁

```go
// 共用 cache 的 Redis 连接，不单独创建连接
locker := lock.NewWithClient(cache.RedisClient("redis"), lock.WithLogger(log))

// 基础用法
acquired, err := locker.TryLock(ctx, "resource:123", 30*time.Second)
if !acquired { return errors.ErrConflict }
defer locker.Release(ctx, "resource:123")

// 等待获取锁（超时控制）
if err := locker.Lock(ctx, "payment:456", 10*time.Second); err != nil {
  return err
}
defer locker.Release(ctx, "payment:456")
```

### pagination — 分页

```go
// 从 query string 解析（?page=2&page_size=20）
p := pagination.FromQuery(c)

users, _ := repo.List(ctx, pagination.Offset(p), p.PageSize)
total, _ := repo.Count(ctx)

c.JSON(200, pagination.New(total, users, p))
```

**响应格式**：

```json
{
  "total": 100,
  "page": 2,
  "page_size": 20,
  "data": [...]
}
```

---

## 配置参考

完整配置文件示例（`config.yaml`）：

```yaml
app:
  name: my-service
  version: v1.0.0
  env: production # development | staging | production

server:
  http:
    addr: :8080
    timeout: 30s
  grpc:
    addr: :9090

database:
  default:
    driver: mysql # mysql | postgres | sqlite
    dsn: "user:pass@tcp(mysql:3306)/mydb?parseTime=true"
    max_open_conns: 20
    max_idle_conns: 5
    slow_threshold: 200ms

cache:
  default:
    backend: memory # 普通缓存用内存
  redis:
    backend: redis # lock/stream 共用此连接
    addr: "redis:6379"
    password: ""
    db: 0

logger:
  level: info
  output: both # console | file | both
  filename: logs/app
  mask_fields: [password, token]

auth:
  secret: "change-me-in-production"
  expire: 24h

tracing:
  endpoint: "otel-collector:4317"
  sampler: 1.0

metric:
  prometheus_addr: :9091

idgen:
  worker_id: 1 # 生产环境必须显式设置，容器中 IP 可能重复
```

---

## 错误处理

### 定义和使用哨兵错误

```go
// 包级别定义
var (
  ErrNotFound  = errors.Define(404, "USER_NOT_FOUND",  "用户不存在")
  ErrConflict  = errors.Define(409, "EMAIL_TAKEN",     "邮箱已被注册")
  ErrForbidden = errors.Define(403, "NO_PERMISSION",   "无操作权限")
)

// Service 层返回
func (s *UserService) Get(ctx context.Context, id uint) (*User, error) {
  var user User
  err := database.DB("default").WithContext(ctx).First(&user, id).Error
  if errors.Is(err, gorm.ErrRecordNotFound) {
    return nil, errors.Wrap(ErrNotFound, "user id=%d", id)
  }
  return &user, err
}

// Handler 层统一用 c.Error()
func (h *Handler) Get(c *gin.Context) {
  user, err := h.svc.Get(c.Request.Context(), id)
  if err != nil {
    c.Error(err)   // ErrorHandler 中间件自动转为 JSON 响应
    return
  }
  c.JSON(200, user)
}
```

### 内置哨兵错误

| 变量                     | HTTP 状态码 | Reason           |
| ------------------------ | ----------- | ---------------- |
| `errors.ErrBadRequest`   | 400         | BAD_REQUEST      |
| `errors.ErrUnauthorized` | 401         | UNAUTHORIZED     |
| `errors.ErrForbidden`    | 403         | FORBIDDEN        |
| `errors.ErrNotFound`     | 404         | NOT_FOUND        |
| `errors.ErrConflict`     | 409         | CONFLICT         |
| `errors.ErrTooMany`      | 429         | TOO_MANY_REQUEST |
| `errors.ErrInternal`     | 500         | INTERNAL         |

---

## 可观测性

Forge 的可观测性组件开箱即用，无需额外配置即可启用基础监控：

### 链路追踪

- 所有 HTTP/gRPC 请求自动注入 `trace_id` 和 `span_id`
- 日志自动从 context 中提取并附加到每条日志
- OTLP 支持 gRPC（端口 4317）和 HTTP（端口 4318），按端口自动选择

### 指标

```yaml
metric:
  prometheus_addr: :9091 # Prometheus 抓取地址
```

- HTTP 请求数、延迟、错误率自动采集（无需代码改动）
- gRPC 请求指标同样自动采集
- `metric.Count()`、`metric.Gauge()`、`metric.StartTimer()` 提供业务指标接入

### 健康检查

```
GET /healthz  → 存活探针，始终 200
GET /readyz   → 就绪探针，所有依赖就绪才 200
GET /health   → 同 /healthz（兼容旧版）
```

接入数据库和缓存的健康检查：

```go
httpserver.New(
  httpserver.WithHealthChecks(
    dbMgr.HealthChecker("default"),    // 执行 SELECT 1
    cacheMgr.HealthChecker("redis"),   // 执行 PING
  ),
)
```

---

## 常见问题

**Q: 为什么 database.DB() 返回 nil？**

A: 必须在 `database.New()` 之后调用 `database.Register()`。创建和注册是两个独立步骤：

```go
dbMgr, _ := database.New(...)
database.Register("default", dbMgr) // ← 这一步不能省
```

**Q: Redis 连接为什么要通过 cache 包获取？**

A: `cache` 是 Redis 连接的统一注册中心。`lock`、`stream`、`ratelimit` 都应通过 `cache.RedisClient("redis")` 共用同一连接池，避免多个包各自开连接：

```go
locker   := lock.NewWithClient(cache.RedisClient("redis"), ...)
streamBus, _ := stream.New(cache.RedisClient("redis"), ...)
```

**Q: 如何实现优雅停止？**

A: Forge 自动处理。`app.Run()` 接收到 `SIGTERM`/`SIGINT` 后，按 `Server.Stop() → Lifecycle.Shutdown() → Closer.Close()` 顺序优雅关闭，默认超时 30 秒：

```go
app.New(
  app.WithShutdownTimeout(60 * time.Second), // 自定义超时
  app.WithServer(httpSrv),
).Run()
```

**Q: server/http 的默认中间件栈是什么顺序？**

A: `RequestID → Tracing → Logger → Metrics → Recovery → ErrorHandler`，不要重复添加任何一个。

**Q: idgen 的 worker_id 怎么配置？**

A: 生产环境**必须**显式设置，容器环境中 IP 末位字节可能重复导致 ID 碰撞：

```yaml
idgen:
  worker_id: 42 # 每个实例唯一，范围 0-1023
```

---

## 参考文档

- [架构设计文档](./ARCHITECTURE.md) — 包依赖规则、设计模式、接口契约
- [接入手册](./docs/getting-started.md) — 四种场景的完整示例代码
