package database

import (
	"time"

	"github.com/pemako/mk/core"
)

type driverEntry struct {
	name string
	cfg  DSNConfig
}

// options holds the functional configuration for New.
type options struct {
	drivers       []driverEntry
	migrationsDir string
	logger        core.Logger
}

// Option is a functional option for configuring the database Manager.
type Option func(*options)

// WithDriver registers a named connection using explicit connection values.
// name is the connection identifier used with DB() (e.g., "default", "read").
// Defaults applied: MaxOpenConns=100, MaxIdleConns=10, SlowThreshold=200ms.
func WithDriver(name, driver, dsn string) Option {
	return func(o *options) {
		o.drivers = append(o.drivers, driverEntry{
			name: name,
			cfg: DSNConfig{
				Driver:        driver,
				DSN:           dsn,
				MaxOpenConns:  100,
				MaxIdleConns:  10,
				SlowThreshold: 200 * time.Millisecond,
			},
		})
	}
}

// WithConfig reads named database connections from core.Config. It scans
// the config keys database.default, database.read, and database.write.
// Only entries with a non-empty driver field are registered.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		for _, name := range []string{"default", "read", "write"} {
			var dc DSNConfig
			if err := cfg.Scan("database."+name, &dc); err == nil && dc.Driver != "" {
				if dc.SlowThreshold == 0 {
					dc.SlowThreshold = 200 * time.Millisecond
				}
				o.drivers = append(o.drivers, driverEntry{name: name, cfg: dc})
			}
		}
	}
}

// WithLogger sets the logger used for slow-query warnings and debug SQL output.
// Defaults to logger.Default().
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

// WithMigrationsDir sets the directory that contains goose migration files.
// When set, Manager.Up will apply all pending migrations in this directory.
func WithMigrationsDir(dir string) Option { return func(o *options) { o.migrationsDir = dir } }
