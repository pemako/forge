package database

import (
	"context"
	"fmt"
	"sync"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/pemako/mk/errors"
)

// Manager holds the configuration for all registered database connections and
// satisfies the app.Migrator interface. Obtain one with New.
type Manager struct {
	opts options
}

var (
	registry   = map[string]*gorm.DB{}
	registryMu sync.RWMutex
)

// ErrAlreadyRegistered is returned by New when a connection with the given
// name already exists in the global registry.
var ErrAlreadyRegistered = errors.Define(409, "DATABASE_ALREADY_REGISTERED", "database connection already registered")

// New opens all configured database connections, validates each with a Ping,
// applies connection pool settings, optionally installs the tracing plugin,
// and registers each connection in the global registry. It returns an error if
// any connection cannot be opened or does not respond to Ping.
func New(opts ...Option) (*Manager, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	for _, d := range o.drivers {
		db, err := openDB(d.cfg)
		if err != nil {
			return nil, fmt.Errorf("open db %q: %w", d.name, err)
		}

		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("get sql.DB for %q: %w", d.name, err)
		}

		if err := sqlDB.Ping(); err != nil {
			return nil, fmt.Errorf("ping db %q: %w", d.name, err)
		}

		if d.cfg.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(d.cfg.MaxOpenConns)
		}
		if d.cfg.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(d.cfg.MaxIdleConns)
		}
		if d.cfg.ConnMaxLifetime > 0 {
			sqlDB.SetConnMaxLifetime(d.cfg.ConnMaxLifetime)
		}

		if d.cfg.TraceAllSQL {
			db.Use(newTracingPlugin(d.name, o.logger, d.cfg.SlowThreshold))
		}

		registryMu.Lock()
		if _, ok := registry[d.name]; ok {
			registryMu.Unlock()
			sqlDB, _ := db.DB()
			sqlDB.Close() //nolint:errcheck
			return nil, ErrAlreadyRegistered
		}
		registry[d.name] = db
		registryMu.Unlock()
	}

	return &Manager{opts: o}, nil
}

// Register is a convenience function that creates a single named database
// connection and registers it in the global registry.
// It is equivalent to New(WithDriver(name, driver, dsn), opts...).
//
// Example:
//
//	database.Register("default", "sqlite", "file::memory:", database.WithLogger(log))
//	db := database.DB("default")
func Register(name, driver, dsn string, opts ...Option) error {
	_, err := New(append([]Option{WithDriver(name, driver, dsn)}, opts...)...)
	return err
}

// DB returns the *gorm.DB for the named connection from the global registry,
// or nil if the name has not been registered. Connections are registered
// by New and are available to any package without further configuration.
func DB(name string) *gorm.DB {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// Up applies all pending migrations for every registered database connection.
// It is a no-op when WithMigrationsDir was not configured.
// Up satisfies the app.Migrator interface and is called by app.Run before
// BeforeStart hooks and server startup.
func (m *Manager) Up(ctx context.Context) error {
	if m.opts.migrationsDir == "" {
		return nil
	}
	for _, d := range m.opts.drivers {
		db := DB(d.name)
		if db == nil {
			continue
		}
		if err := NewMigrator(db, m.opts.migrationsDir).Up(ctx); err != nil {
			return fmt.Errorf("database: migrate %q: %w", d.name, err)
		}
	}
	return nil
}

func openDB(cfg DSNConfig) (*gorm.DB, error) {
	gcfg := &gorm.Config{Logger: gormlogger.Discard}
	switch cfg.Driver {
	case "mysql":
		return gorm.Open(mysql.Open(cfg.DSN), gcfg)
	case "postgres":
		return gorm.Open(postgres.Open(cfg.DSN), gcfg)
	case "sqlite":
		return gorm.Open(sqlite.Open(cfg.DSN), gcfg)
	default:
		return nil, fmt.Errorf("unsupported driver: %q (mysql/postgres/sqlite)", cfg.Driver)
	}
}
