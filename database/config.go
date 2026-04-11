package database

import "time"

// DSNConfig mirrors the YAML structure under the database.<name> key.
type DSNConfig struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	TraceAllSQL     bool          `mapstructure:"trace_all_sql"`
	SlowThreshold   time.Duration `mapstructure:"slow_threshold"`
}
