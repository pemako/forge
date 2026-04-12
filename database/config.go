// Package database provides GORM-backed database connection management and
// goose-based migration support for mk services. It maintains a global registry
// of named *gorm.DB connections (typically "default", "read", "write") so that
// any package can retrieve a connection without dependency injection.
//
// Use New to open connections and register them, DB to retrieve a connection
// by name, and NewMigrator to run SQL migrations.
package database

import "time"

// DSNConfig mirrors the YAML structure under the database.<name> config key.
// It is also accepted directly by WithDriver for programmatic configuration.
type DSNConfig struct {
	// Driver is the database engine: "mysql", "postgres", or "sqlite".
	Driver string `mapstructure:"driver"`
	// DSN is the data source name passed to the underlying driver.
	DSN string `mapstructure:"dsn"`
	// MaxOpenConns sets the maximum number of open database connections.
	MaxOpenConns int `mapstructure:"max_open_conns"`
	// MaxIdleConns sets the maximum number of idle connections in the pool.
	MaxIdleConns int `mapstructure:"max_idle_conns"`
	// ConnMaxLifetime sets the maximum amount of time a connection may be reused.
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	// TraceAllSQL enables the tracing plugin which emits an otel span per query.
	TraceAllSQL bool `mapstructure:"trace_all_sql"`
	// SlowThreshold is the minimum query duration before a slow-query warning is logged.
	// Defaults to 200ms.
	SlowThreshold time.Duration `mapstructure:"slow_threshold"`
}
