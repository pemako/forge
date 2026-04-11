package database

import (
	"time"

	"github.com/pemako/mk/core"
)

type driverEntry struct {
	name string
	cfg  DSNConfig
}

type options struct {
	drivers       []driverEntry
	migrationsDir string
	logger        core.Logger
}

type Option func(*options)

// WithDriver registers a named connection by explicit values.
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

// WithConfig reads named connections from core.Config.
// Scans database.default, database.read, database.write.
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

func WithLogger(l core.Logger) Option     { return func(o *options) { o.logger = l } }
func WithMigrationsDir(dir string) Option { return func(o *options) { o.migrationsDir = dir } }
