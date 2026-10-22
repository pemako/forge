# Forge

Forge 是一个面向生产环境的 Go 后端服务工具包，提供开箱即用的基础设施组件和统一的工程规范。

> **模块路径**：`github.com/pemako/forge` · **Go 版本**：1.26+

---

## 目录

- [设计理念](#设计理念)
- [破坏性变更](#破坏性变更)
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

`cache.Manager` 和 `grpcclient.Registry` 实现 `core.Closer`，可直接交给
`app.WithCloser(...)`；`database.Manager` 实现 `core.Migrator`，可交给
`app.WithMigration(...)`。请实现 `core` 这一写法而不是 `app` 别名：库包不允许导入
`app`，而且只有 `core` 写法能在编译期断言。

### 函数式选项（Functional Options）

所有包构造函数遵循统一形式：

```go
func New(opts ...Option) (*T, error)

// 通用选项（绝大多数基础设施包都提供）
func WithConfig(cfg core.Config) Option  // 从配置读取参数
func WithLogger(l core.Logger) Option    // 注入日志器
```

> 例外：`metric` 没有 `WithLogger`，其选项为 `WithServiceName`、
> `WithPrometheusAddr`、`WithOTLPEndpoint`、`WithAllowNoExporters`、`WithConfig`。

---

## 破坏性变更

升级前请先阅读本节。每一项在下文对应的包说明中都有详细描述。

| 变更 | 影响 | 应对方式 |
| ---- | ---- | -------- |
| `core.Config` 新增 `IsSet(key string) bool` | 外部对 `core.Config` 的实现无法编译 | 补上该方法；它用于区分"未配置"和"配置为零值" |
| `core.Closer` 改为**逆序**执行 | `WithCloser(a, b)` 现在先关闭 `b`，而非 `a` | 这通常是修正而非回退——与 `Lifecycle` 的嵌套顺序一致。请重新检查依赖关系假设 |
| 未配置导出器时 `metric.Provider.Init` 返回 `ErrNoExporters` | `metric:` 配置块为空的服务现在会启动失败 | 配置 `metric.prometheus_addr` / `metric.otlp_endpoint`，或传入 `metric.WithAllowNoExporters()` |
| `metric.Provider.Init` 同步绑定 Prometheus 监听端口 | 端口冲突现在会以 `ErrPrometheusListen` 导致启动失败 | 以前 `/metrics` 会静默不可用——请修复端口冲突 |
| gRPC 健康服务初始状态为 `NOT_SERVING` | 不配合 `app` 使用的 `server/grpc` 永远不会被健康感知的客户端选中 | 交由 `app` 驱动，或自行调用 `grpcSrv.SetReady(true)` |
| HTTP 408/412/413/416/422/499/501/502/503/504 对应的 gRPC 码变化 | 依据 gRPC 码分支的客户端会收到新值（这些以前全部落到 `Internal`） | 检查客户端的错误码分支逻辑 |
| gRPC 指标 `status` 标签改为真实码名 | 匹配 `status="ERROR"` 的看板和告警将不再命中 | 改为匹配 `"NotFound"`、`"Unavailable"` 等 |
| `RegisterHTTPMetricHooks` / `RegisterGRPCMetricHooks` 新增 `errorsInc` 参数 | 直接调用方无法编译 | 补上该回调（库内仅 `metric` 调用） |
| `tracing.New` 无条件安装全局 W3C 传播器 | 此前断开的跨服务链路现在会连接起来 | 无需处理——这是修复。即使关闭 tracing 也要调用 `tracing.New()` |
| 未传 `WithWatch()` 时 `Config.Watch` 返回 `ErrWatchDisabled` | 原本静默无效的 `Watch` 调用现在会报错 | 补上 `config.WithWatch()`，或移除 `Watch` 调用 |
| 配置块格式错误时 `cache.New` / `database.New` 返回 `ErrInvalidConfig` | 拼错的配置块现在会报错，而不是产出零个后端 | 修正配置块，并检查返回的 error |
| `app.WithHealthCheck` 自动注册探针 | 旧的 `BeforeStart` 桥接钩子会导致每个探针被重复注册一次 | 删除该钩子 |
| 就绪状态由 `app` 驱动 | 在 `BeforeStart` 里调用 `httpSrv.SetReady(true)` 会过早对外宣告就绪 | 删除该调用，交由 `app` 处理 |

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
① 注册就绪探针           将 WithHealthCheck 的检查器注册到所有实现
                         core.ReadyCheckRegistrar 的服务器上——在任何服务器
                         启动之前完成，因此探针列表不会在处理请求时被改动
② Migrator.Up()          数据库迁移（可选）
③ Lifecycle.Init()       顺序初始化；任一失败则逆序回滚**并执行所有 Closer**
④ BeforeStart hooks      启动前钩子；顺序执行，首个错误即中止（同样回滚）
⑤ Server.Start() × N     所有服务器并发启动；随后全部标记为就绪
⑥ 阻塞等待               SIGTERM / SIGINT、App.Stop()、ctx 取消或服务器错误
⑦ 摘流                   服务器标记为**不就绪**，然后等待 WithDrainDelay
⑧ Server.Stop() × N      并发优雅停止（受 shutdownTimeout 约束）
⑨ Lifecycle.Shutdown()   逆序关闭
⑩ Closer.Close()         **逆序**释放资源
⑪ AfterStop hooks        停止后钩子
```

**生命周期契约定义在 `core` 中，而非 `app`**。`Migrator`、`Lifecycle`、`Closer`、
`ReadinessToggler`、`ReadyCheckRegistrar` 都声明在 `core/lifecycle.go`。库包不允许
导入 `app`，因此只有 `core` 这一写法才能在编译期断言契约：

```go
var _ core.Lifecycle = (*MyProvider)(nil)
```

`app` 保留了类型别名（`app.Lifecycle = core.Lifecycle`，其余同理），因此已有调用处
不受影响。

```go
// core.Lifecycle — 有初始化/关闭逻辑的组件（tracing、metric）
// Init 在服务器启动前调用；Shutdown 在服务器停止后逆序调用。
// Init[i] 失败时，Init[0..i-1] 会逆序 Shutdown，并且所有 Closer 都会被 Close。
type Lifecycle interface {
  Init(ctx context.Context) error
  Shutdown(ctx context.Context) error
}

// core.Closer — 需要释放连接的组件（cache.Manager、grpcclient.Registry）
// 按**逆序**注册顺序调用（与 Lifecycle 一致），因此一个组件可以依赖比它更早
// 注册的组件。Close 不接收 context，由 app 自行约束等待时间。
type Closer interface {
  Close() error
}

// core.Migrator — 需要在任何服务器启动前执行迁移的组件（database.Manager）
type Migrator interface {
  Up(ctx context.Context) error
}

// core.ReadinessToggler / core.ReadyCheckRegistrar — Server 可选实现的接口，
// app 通过它们驱动就绪状态、注册健康检查器。
type ReadinessToggler interface    { SetReady(ready bool) }
type ReadyCheckRegistrar interface { AddReadyCheck(name string, fn func(context.Context) error) }
// 实现者：server/http、server/grpc

// core.Metrics — 指标记录接口，与 Logger / Provider 对等
// 库包可以依赖此接口，生产环境注入真实实现，测试中注入桩实现。
// *metric.Provider 实现了它。
type Metrics interface {
  Count(ctx context.Context, name string, n int64, labels ...Field)
  Gauge(ctx context.Context, name string, value float64, labels ...Field)
  Histogram(ctx context.Context, name string, value float64, labels ...Field)
}
```

### 关停健壮性保证

| 保证 | 意义 |
| ---- | ---- |
| 信号在迁移**之前**就被接管 | 以前 `signal.Notify` 在服务器启动后才执行，迁移期间收到 SIGTERM 会走默认处置，直接把进程杀死在迁移中途 |
| **第二次** SIGTERM / SIGINT 会放弃剩余等待 | 运维人员不会遇到一个不响应信号的进程 |
| 服务器停止之后的每个阶段都有**独立**截止时间，下限 5 秒（上限为 `WithShutdownTimeout`） | 以前服务器摘流过慢会把一个已过期的 context 交给 `Lifecycle.Shutdown`，导致遥测数据永远不会被冲刷 |
| 每个 `Closer.Close()` 在独立 goroutine 中执行，并受预算约束 | `Close` 不接收 context、无法被取消；卡住的 `Close` 现在只泄漏一个 goroutine，而不再永久挂死进程 |
| 所有回调、钩子、服务器 goroutine 中的 panic 都会被恢复 | 以 `app.ErrPanic` 上报并记录堆栈；剩余的关停阶段照常执行，因此 panic 不会在仍持有资源的情况下杀死进程 |
| 启动失败会回滚 Lifecycle **并**执行 Closer | 以前 `BeforeStart` 失败会直接返回，已打开的连接池和客户端全部泄漏 |

**哨兵错误**：

| 哨兵 | 触发条件 |
| ---- | -------- |
| `app.ErrPanic` | 生命周期回调、钩子或服务器 goroutine 发生 panic |
| `app.ErrServersExited` | 在未请求关停的情况下，所有 `Server.Start` 都已返回（`Start` 必须阻塞） |

### 程序内主动关停

```go
a := app.New(app.WithServer(srv))
go func() { _ = a.RunContext(ctx) }()  // 取消 ctx 即请求优雅关停
a.Stop()                              // 或直接请求；幂等，且在 Run 之前调用也安全
```

`Run()` 等价于 `RunContext(context.Background())`。`Stop()` 立即返回，`Run` 在关停
完成后返回。取消传给 `RunContext` 的 context 所请求的关停流程与信号完全一致——关停
序列本身**不**受该 context 约束，因此取消它不会截断摘流过程。

### core.Server 契约

`Start` 必须阻塞，`Stop` 让 `Start` 解除阻塞：

```go
type Server interface {
  Start(ctx context.Context) error  // 阻塞直到 Stop 被调用
  Stop(ctx context.Context) error   // 优雅停止
}
```

实现了此接口的：`server/http`、`server/grpc`、`cron.Scheduler`、`stream.Bus.Worker()`。

如果在应用本应继续服务时，所有已注册服务器的 `Start` 都返回了，`app.Run` 会返回
`app.ErrServersExited`，而不是永久阻塞在无人提供服务的状态下。

---

## 典型接入路径

完整服务的 `main.go` 初始化顺序：

```go
// ① 配置 + 日志（注入到所有下游组件）
cfg, _ := config.Load(
  config.WithFile("config.yaml"),
  config.WithRequired("database.default.dsn"), // 缺失即启动失败，并列出所有缺失的键
)
log, _ := logger.New(logger.WithConfig(cfg))

// ② 可观测性（实现 app.Lifecycle）
// tracing.New() 同时会安装全局 W3C 链路传播器——即使没有配置导出器也要调用它，
// 否则跨服务链路会断开。
tp, _ := tracing.New(tracing.WithConfig(cfg), tracing.WithLogger(log))
// metric 没有 WithLogger 选项。若未配置任何导出器，Init 会返回 ErrNoExporters，
// 可用 metric.WithAllowNoExporters() 显式豁免。
mp, _ := metric.New(metric.WithConfig(cfg))

// ③ 数据存储（database → WithMigration，cache → WithCloser）
// 配置块格式错误时，New 返回 ErrInvalidConfig，不再静默产出零个连接/后端。
dbMgr, _ := database.New(database.WithConfig(cfg), database.WithLogger(log))
database.Register("default", dbMgr)

cacheMgr, _ := cache.New(cache.WithConfig(cfg), cache.WithLogger(log))
cache.Register("default", cacheMgr)

// ④ 基础设施客户端（可选）
locker := lock.NewWithClient(cache.RedisClient("redis"), lock.WithLogger(log))
httpclient.Register("downstream", httpclient.New(httpclient.WithBaseURL("...")))

// ⑤ HTTP 服务器
httpSrv, _ := httpserver.New(
  httpserver.WithConfig(cfg),
  httpserver.WithLogger(log),
)

// ⑥ 注册路由
httpSrv.Engine().GET("/path", ginHandler)

// ⑦ 组装并运行
app.New(
  app.WithServer(httpSrv),
  app.WithMigration(dbMgr),
  app.WithLifecycle(tp, mp),
  app.WithCloser(cacheMgr),          // 注意：Closer 按逆序执行
  app.WithLogger(log),
  // 健康检查器会在服务器启动前，自动注册到所有接受就绪探针的服务器上。
  // 不需要再写 BeforeStart 桥接钩子，也不要调用 httpSrv.SetReady(true)——
  // 就绪状态由 app 驱动。
  app.WithHealthCheck(
    dbMgr.HealthChecker("default"),   // /readyz: SELECT 1
    cacheMgr.HealthChecker("redis"),  // /readyz: PING
  ),
  app.WithDrainDelay(5 * time.Second), // 留出时间让负载均衡器观察到 /readyz 失败
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
  config.WithWatch(),              // 启用文件变更热重载
  // 声明服务缺之不能启动的键：Load 会返回 ErrMissingRequiredKey，
  // 并一次性列出**所有**缺失的键——排查一次重启即可定位，而不是每个键重启一次。
  config.WithRequired("database.default.dsn", "cache.redis.addr"),
  // 声明本服务会读取的键前缀：Load 对配置中无人声明的键打印一条 Warn，
  // 实际场景中多半是拼写错误（如 "databse.default.dsn"）。仅告警，不致命。
  config.WithKnownKeys("app", "server", "logger", "database", "cache"),
  config.WithLogger(log),          // 上述告警所使用的日志器
)

// 读取配置值
addr := cfg.GetString("server.http.addr")
timeout := cfg.GetDuration("server.http.timeout")

// 区分"未配置"和"配置为零值"——GetInt 对缺失的键和 `port: 0` 都返回 0
if cfg.IsSet("server.http.port") { /* ... */ }

// 以编程方式获取未声明的键，可自行升级为错误
if extra := cfg.UnknownKeys([]string{"app", "server"}); len(extra) > 0 { /* ... */ }

// 热重载。未传 WithWatch() 时返回 ErrWatchDisabled——
// 以前它会接受一个永远不可能被触发的回调。
if err := cfg.Watch("feature.flag", func(v any) {
  log.Info(ctx, "配置变更", logger.F("value", v))
}); err != nil { /* ... */ }
```

| 哨兵错误                       | 返回者  | 触发条件                                           |
| ------------------------------ | ------- | -------------------------------------------------- |
| `config.ErrMissingRequiredKey` | `Load`  | `WithRequired` 声明的键缺失；错误信息列出全部缺失项 |
| `config.ErrWatchDisabled`      | `Watch` | 加载时未传 `WithWatch()`                            |

> `WithKnownKeys` 刻意只告警而不报错：配置文件中合理地会存在本库完全不认识的键——
> 各部署环境的注解、由业务代码消费的配置、为未来版本预留的配置块。若希望升级为
> 错误，请自行使用 `cfg.UnknownKeys(...)`。

`WithRequired` 的键使用点号记法，且是针对合并后的配置做检查，因此仅通过环境变量
提供的键也满足要求；键存在但为空视为缺失。

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
dbMgr, err := database.New(
  database.WithConfig(cfg),
  database.WithLogger(log),
  database.WithMigrationsDir("migrations"), // goose 迁移目录
)
// 当 database.{default,read,write} 配置块无法解码时（例如 max_open_conns 写成了
// 字符串，或该块是标量而不是 map），err 包装 database.ErrInvalidConfig。
// 以前这种块会被静默跳过。配置块不存在不算错误。
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
cacheMgr, err := cache.New(
  cache.WithMemory("default"),         // 内存后端（开发/测试）
  cache.WithRedis("redis", "localhost:6379"), // Redis 后端（生产）
)
// 当 "cache" 配置块无法解码时（例如某个后端项是标量而不是 map），
// err 包装 cache.ErrInvalidConfig。以前这种情况会产出零个后端且没有任何诊断信息。
// 配置块不存在或为空不算错误。
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

// 提取 CodedError——AsType 免去 As 那套"先声明再传指针"的写法
if ce, ok := errors.AsType[*errors.CodedError](err); ok { _ = ce.Reason }

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

> **`errors.Wrap` 的 cause 链不会返回给客户端，但也不再丢失。**
> 它刻意不出现在 HTTP 响应体和 gRPC status message 中，但现在会被
> `middleware/http.ErrorHandler` 记录到当前 span 上，并由 Logger 中间件以
> `error` / `error_reason` 字段写入日志。排查 500 时请看日志行或 span，而不是响应体。

**gRPC 客户端应基于 `Reason` 分支，而不是解析 message。**
`middleware/grpc.UnaryErrorHandler` 会把 `CodedError.Reason` 和 `.Metadata` 作为
`google.rpc.ErrorInfo` detail 附带传输，domain 为 `grpcMiddleware.ErrorInfoDomain`
（即 `"forge"`）：

```go
for _, d := range status.Convert(err).Details() {
  if info, ok := d.(*errdetails.ErrorInfo); ok && info.Domain == grpcMiddleware.ErrorInfoDomain {
    switch info.Reason {
    case "USER_NOT_FOUND": // ...
    }
  }
}
```

### server/http — HTTP 服务器

```go
srv, _ := httpserver.New(
  httpserver.WithConfig(cfg),
  httpserver.WithLogger(log),
  httpserver.WithServiceName("order-svc"),
  httpserver.WithHealthChecks(dbMgr.HealthChecker("default")), // 不使用 app 时在此传入
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
| `/health` | 同 `/healthz`（兼容旧版） |

**就绪状态由 app 驱动**：`*ginServer` 实现了 `core.ReadinessToggler` 和
`core.ReadyCheckRegistrar`，因此 `app` 会在所有服务器启动完成后把它标记为就绪、在
关停开始时标记为不就绪，并在启动前注册 `app.WithHealthCheck` 的探针。

> ⚠️ **不要在 `BeforeStart` 钩子里调用 `srv.SetReady(true)`**——那会在监听端口还没
> 就绪时就对外宣告就绪，而且会与 `app` 争夺这个标志位的所有权。只有在不配合 `app`
> 单独使用服务器时才需要自己调用它。

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

> ⚠️ **即使"关闭"链路追踪，也必须调用 `tracing.New()`。** `New` 会**无条件**安装
> 全局 OTel 文本映射传播器——W3C TraceContext（`traceparent` / `tracestate`）加
> W3C Baggage——该动作位于"无导出器则提前返回"之前。以前只有存在导出器时才会安装，
> 因此入站的 `traceparent` 头被丢弃、出站请求也不会注入任何内容：跨进程链路传播
> 完全不可用。

传播与导出是两件独立的事。一个自身不导出任何数据的进程（采样率 0、没有 collector、
轻量代理、测试程序）仍然必须把入站链路上下文转发给它的下游调用，否则下游的 span 会
各自开启一条全新且断开的 trace。

```go
// 仅在需要与其他线路格式互通时才覆盖（例如对接遗留 Zipkin 网格的 B3）。
// 不传任何传播器则保留当前的全局传播器不变。
tracing.New(tracing.WithPropagators(propagation.TraceContext{}, b3.New()))
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
  tls: false

# 至少需要配置一个导出器，否则 metric.Provider.Init 返回 ErrNoExporters
# （确实要关闭指标时请传 metric.WithAllowNoExporters()）
metric:
  prometheus_addr: :9091

idgen:
  worker_id: 1 # 生产环境必须显式设置，容器中 IP 可能重复
```

> `app` 没有 `WithConfig`：`WithName`、`WithVersion`、`WithShutdownTimeout`、
> `WithDrainDelay` 等选项在代码里设置，不从 YAML 读取。若希望由配置驱动，请自行传入
> `cfg.GetString("app.name")`。

配合 `config.WithRequired(...)` 可以让缺失的配置在启动时立刻暴露，而不是在运行时
以零值的形式被发现；配合 `config.WithKnownKeys(...)` 可以发现配置文件里的拼写错误。
详见上文 [config](#config--配置加载) 一节。

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
- `tracing.New()` 无条件安装全局 W3C 传播器，因此跨服务链路可以串联；即使不配置
  导出器也要调用它

### 指标

```yaml
metric:
  prometheus_addr: :9091 # Prometheus 抓取地址
```

- HTTP 请求数、延迟、错误率自动采集（无需代码改动）
- gRPC 请求指标同样自动采集
- `metric.Count()`、`metric.Gauge()`、`metric.StartTimer()` 提供业务指标接入
  （接收 `metric.Label(...)` 选项）
- `p.Count()` / `p.Gauge()` / `p.Histogram()` 是 `core.Metrics` 的方法
  （接收 `core.Field` 标签）；库包应依赖 `core.Metrics` 接口，而不是调用包级门面，
  这样才能在测试中替换为桩实现
- `Gauge` 是**赋值**语义：连续两次 `Gauge(ctx, "q", 5)` 读回 5 而不是 10。
  需要增减请用 `GaugeAdd` / `GaugeSub`。（此前 `metric.Gauge` 和 `p.Gauge`
  的文档都写"设置"却在做累加,属于会给出错误数字的缺陷,现已修正。）
- `WithReader(sdkmetric.Reader)` 可额外注册一个 reader：测试里传
  `sdkmetric.NewManualReader()` 即可断言指标是否真的被记录。注册 reader
  也算配置了导出器，因此 `Init` 不会返回 `ErrNoExporters`。

每个服务名会安装以下指标：`*_http_requests_total`、
`*_http_request_duration_seconds`、`*_http_requests_in_flight`、
`*_http_errors_total`，以及对应的四个 `*_grpc_*` 指标。

> ⚠️ **未配置任何导出器时 `Init` 会报错。** 没有 reader 的 `MeterProvider` 会接收
> 所有测量值然后全部丢弃，于是指标配置加载失败的服务看起来很健康，实际什么都没上报。
>
> ```go
> // 要么配置 metric.prometheus_addr / metric.otlp_endpoint，要么显式豁免：
> p, _ := metric.New(metric.WithAllowNoExporters())  // 测试、CLI 构建
> ```

| 哨兵错误                      | 触发条件                                                   |
| ----------------------------- | ---------------------------------------------------------- |
| `metric.ErrNoExporters`       | 既没配 Prometheus 也没配 OTLP，且未传 `WithAllowNoExporters` |
| `metric.ErrPrometheusListen`  | `/metrics` 监听端口无法绑定                                 |

Prometheus 监听端口现在在 `Init` 内**同步**绑定，因此端口冲突会直接导致启动失败，
而不是在服务自称健康的同时让 `/metrics` 永久不可用。

**多 Provider 现在可以正常工作。** `p.Init()` 会接线 HTTP / gRPC 中间件指标，且该
接线不再是一次性的——钩子集合通过原子操作整体替换，最后一次 `Init` 生效。以前的
`sync.Once` 保护意味着第二个 Provider（另一个 app 实例，或同一测试二进制中的下一个
测试）会被静默忽略，并继续沿用第一个 Provider 的 instrument 和服务名上报。

测试中可用 `middleware/http.ResetHTTPMetricHooks()` 和
`middleware/grpc.ResetGRPCMetricHooks()` 还原为 no-op 钩子。

### 错误可观测性

`middleware/http.ErrorHandler` 除了写 JSON 响应体，还会把错误记录到当前 span、
对 5xx 设置 span 状态为 `codes.Error`、附加 `error.code` / `error.reason` 属性，并
递增 `*_http_errors_total{reason,code}`。以前失败请求只产生一个响应体，没有 span
事件也没有指标——一个 500 在链路和指标里都是不可见的。

Logger 中间件按结果选择日志级别，失败的接口无需 grep 状态码即可发现：

| 中间件 | 级别规则 |
| ------ | -------- |
| `middleware/http.Logger` | <400 INFO、4xx WARN、5xx ERROR |
| `middleware/grpc.UnaryLogger` | `OK` INFO、调用方问题类错误码 WARN、其余 ERROR |

两者都会在失败时附加 `error` 与 `error_reason` 字段；`UnaryLogger` 还会附加 `code`
字段（以前它即使在 err 为 nil 时也记录原始 error 值，且从不暴露 `Reason`）。

> ⚠️ **gRPC 指标的 `status` 标签改为真实码名**（`"NotFound"`、`"Unavailable"`、
> `"OK"`），不再是过去的 `"OK"` / `"ERROR"` 两值。匹配 `status="ERROR"` 的看板和
> 告警必须重写。

### 健康检查

```
GET /healthz  → 存活探针，始终 200
GET /readyz   → 就绪探针，所有依赖就绪才 200
GET /health   → 同 /healthz（兼容旧版）
```

接入数据库和缓存的健康检查——交给 `app`，它会在任何服务器启动之前，把检查器注册到
所有实现 `core.ReadyCheckRegistrar` 的服务器上（内置的两个服务器都实现了）：

```go
app.New(
  app.WithServer(httpSrv, grpcSrv),
  app.WithHealthCheck(
    dbMgr.HealthChecker("default"),    // 执行 SELECT 1
    cacheMgr.HealthChecker("redis"),   // 执行 PING（memory 后端恒为健康）
  ),
).Run()
```

> **旧的手动桥接写法已过时。** `app.WithHealthCheck` 以前只是存下检查器而不做任何
> 接线，因此调用方需要在 `BeforeStart` 钩子里把它们逐个复制过去。该钩子现在是多余
> 的——请删除它，否则每个探针会被重复注册一次。
>
> ```go
> // ❌ 不再需要
> app.WithBeforeStart(func(ctx context.Context) error {
>   for _, hc := range myApp.HealthCheckers() {
>     httpSrv.AddReadyCheck(hc.Name(), hc.Check)
>   }
>   return nil
> })
> ```

`App.HealthCheckers()` 仍然保留，用于把检查器接到 `app` 触及不到的地方。如果没有任何
已注册的服务器接受探针，`Run` 会打日志说明这些检查器没有落到任何地方，而不是静默丢弃。

**gRPC 健康协议**：`server/grpc` 的健康服务初始状态为 `NOT_SERVING`（以前从 `New()`
起就宣称 `SERVING`，也就是在迁移和 `Lifecycle.Init` 执行之前）。`app` 会通过
`SetReady` 翻转它，因此**不配合 `app` 使用时必须自行调用 `grpcSrv.SetReady(true)`**，
否则健康感知的负载均衡永远不会选中该实例。

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

A: Forge 自动处理。`app.Run()` 收到 `SIGTERM`/`SIGINT` 后，先把服务器标记为不就绪
并等待 `WithDrainDelay`，再按 `Server.Stop() → Lifecycle.Shutdown() → Closer.Close()
→ AfterStop` 顺序关停，默认总超时 30 秒：

```go
app.New(
  app.WithShutdownTimeout(60 * time.Second), // 自定义超时
  app.WithDrainDelay(5 * time.Second),       // 摘流等待：标记不就绪后、停止服务器前
  app.WithServer(httpSrv),
).Run()
```

要点：

- `Lifecycle.Shutdown` 和 `Closer.Close` 都按**逆序**注册顺序执行（`Closer` 以前是
  正序，会在使用某依赖的 Closer 之前就把该依赖拆掉）
- 服务器停止之后的每个阶段都有独立截止时间，下限 5 秒，因此摘流过慢不会导致遥测
  数据来不及冲刷
- **再发一次** `SIGTERM`/`SIGINT` 会放弃剩余等待立即返回
- 建议把 `WithDrainDelay + WithShutdownTimeout` 控制在 Kubernetes 的
  `terminationGracePeriodSeconds` 以内，并把摘流等待设为就绪探针
  `periodSeconds × failureThreshold` 的量级

也可以在进程内主动关停：

```go
a := app.New(app.WithServer(httpSrv))
go func() { _ = a.RunContext(ctx) }()  // 取消 ctx 即请求优雅关停
a.Stop()                              // 幂等，Run 之前调用也安全
```

**Q: 为什么我的服务启动就报 `METRIC_NO_EXPORTERS`？**

A: 这是刻意的。没有配置任何导出器时，`metric.Provider.Init` 会返回
`metric.ErrNoExporters`——因为没有 reader 的 `MeterProvider` 会把所有测量值收下再
丢弃，于是服务看起来健康，实际什么都没上报。配置 `metric.prometheus_addr` 或
`metric.otlp_endpoint`，或在确实要关闭指标时（测试、CLI 构建）传入
`metric.WithAllowNoExporters()`。

**Q: 升级后 gRPC 客户端收到的错误码变了？**

A: `middleware/grpc` 的 HTTP→gRPC 映射表从 7 项扩展到 17 项。HTTP 408、412、413、
416、422、499、501、502、503、504 以前全部落到 `codes.Internal`，现在分别映射为
`DeadlineExceeded`、`FailedPrecondition`、`ResourceExhausted`、`OutOfRange`、
`InvalidArgument`、`Canceled`、`Unimplemented`、`Unavailable`、`Unavailable`、
`DeadlineExceeded`。表中没有的 HTTP 码仍然回落到 `codes.Internal`。建议客户端改为
基于 `ErrorInfo.Reason` 分支，而不是依赖错误码。

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
