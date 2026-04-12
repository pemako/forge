# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.1.0] - 2026-04-12

### Added

**Foundation**
- `core` — interface definitions: `Logger`, `Config`, `Server`, `Provider`
- `errors` — typed `CodedError` with HTTP status code, machine-readable reason, and cause chaining; sentinel errors (`ErrBadRequest`, `ErrNotFound`, `ErrInternal`, etc.)
- `config` — multi-format configuration (YAML/TOML/JSON), environment variable override, hot-reload via `Watch`
- `logger` — structured JSON logging (zap + lumberjack); automatic `trace_id`/`span_id` injection from OpenTelemetry context; per-level log file splitting; configurable sensitive field masking
- `app` — application lifecycle manager: database migrations → `BeforeStart` hooks → concurrent server start → OS signal handling → graceful shutdown → `AfterStop` hooks

**Serving**
- `server/http` — Gin-based HTTP server with default middleware stack (RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler); `/healthz`, `/readyz`, `/health` endpoints; `WithConfig` option
- `server/grpc` — gRPC server with interceptor chain matching HTTP stack; health check service; optional reflection and TLS/mTLS
- `middleware/http` — default stack + optional middleware: CORS, Timeout, BodyLimit, RateLimit (token bucket), IPFilter (CIDR whitelist/blacklist)
- `middleware/grpc` — matching interceptor set for gRPC (Timeout, RateLimit, IPFilter)

**Data**
- `database` — gorm wrapper supporting MySQL, PostgreSQL, SQLite; multi-datasource named registry; SQL tracing plugin (otel child spans + slow query logging); goose migration runner (`Up`/`Down`/`UpTo`/`Status`)
- `cache` — unified `Cache` interface; in-memory backend (default, no external deps, TTL support); Redis backend (optional); named registry pattern

**Observability**
- `tracing` — OpenTelemetry OTLP HTTP provider; no-op fallback when endpoint is not configured; implements `core.Provider`
- `metric` — dual-channel export: Prometheus `/metrics` endpoint + OTLP push; automatic HTTP/gRPC metrics (requests total, duration, in-flight); business helpers: `Count`, `Gauge`, `GaugeAdd`, `GaugeSub`, `StartTimer`, `Duration`

**Clients & utilities**
- `httpclient` — outbound HTTP client with otelhttp tracing, exponential-backoff retry (configurable base delay and cap), JSON marshal/unmarshal helpers, named registry
- `grpcclient` — gRPC connection manager using `grpc.NewClient`; otelgrpc stats handler; per-call retry interceptor; TLS/mTLS support; named registry with `CloseAll()`
- `lock` — Redis distributed lock; atomic acquire/release via Lua scripts; token-based ownership (prevents foreign lock release); optional auto-renewal with token verification; wait-with-timeout polling
- `cron` — cron job scheduler implementing `core.Server`; per-job retry with exponential backoff, execution timeout, skip-if-running guard, panic recovery; sub-second intervals via `time.Ticker`

**Tooling**
- `example/` — runnable demo service demonstrating all packages (HTTP + gRPC servers, SQLite database, in-memory cache, distributed lock, external HTTP client, cron scheduler, tracing, metrics)
- `CLAUDE.md` — Claude Code guidance for this repository
- `Makefile` — common development commands (`test`, `test-race`, `lint`, `vet`, `fmt`, `tidy`)
- `.golangci.yml` — golangci-lint configuration
- `.github/workflows/ci.yml` — GitHub Actions CI (test + lint + example build)

[Unreleased]: https://github.com/pemako/mk/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/pemako/mk/releases/tag/v0.1.0
