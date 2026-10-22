package database

import (
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/errors"
)

// ErrInvalidConfig is returned by New when a "database.*" configuration block
// cannot be decoded (e.g. max_open_conns is a string, or the block is a scalar
// instead of a map). Previously such a block was skipped with no diagnostic.
var ErrInvalidConfig = errors.Define(500, "DATABASE_INVALID_CONFIG", "invalid database configuration")

type driverEntry struct {
	name string
	cfg  DSNConfig
}

// options holds the functional configuration for New.
type options struct {
	drivers       []driverEntry
	migrationsDir string
	logger        core.Logger
	// err holds the first error produced by an Option. Options cannot return
	// errors, so WithConfig stashes decode failures here and New surfaces them.
	err error
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
//
// A malformed block is reported by New as ErrInvalidConfig rather than being
// silently skipped. An absent block is not an error.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		for _, name := range []string{"default", "read", "write"} {
			key := "database." + name
			var dc DSNConfig
			if err := cfg.Scan(key, &dc); err != nil {
				if o.err == nil {
					o.err = errors.Wrap(ErrInvalidConfig, "scan %q config block: %w", key, err)
				}
				return
			}
			if dc.Driver == "" {
				continue
			}
			if dc.SlowThreshold == 0 {
				dc.SlowThreshold = 200 * time.Millisecond
			}
			o.drivers = append(o.drivers, driverEntry{name: name, cfg: dc})
		}
	}
}

// WithLogger sets the logger used for slow-query warnings and debug SQL output.
// Defaults to logger.Default().
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

// WithMigrationsDir sets the directory that contains goose migration files.
// When set, Manager.Up will apply all pending migrations in this directory.
func WithMigrationsDir(dir string) Option { return func(o *options) { o.migrationsDir = dir } }
