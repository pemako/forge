// Package database provides a GORM-based connection manager for MySQL, PostgreSQL,
// and SQLite, with goose migrations and optional SQL tracing.
//
// # Instance-first usage (recommended)
//
//	mgr, err := database.New(database.WithConfig(cfg))
//	database.Register("default", mgr)    // optional global access
//	db := mgr.DB("default")              // instance access
//
// # Convenience access (small services)
//
//	db := database.DB("default")         // delegates to Get("default").DB("default")
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

// Manager holds all database connections created by New and satisfies the
// app.Migrator interface. Obtain one with New, then register it globally with
// Register if package-level access via Get / DB is needed.
type Manager struct {
	opts        options
	connections map[string]*gorm.DB
}

var (
	registry   = map[string]*Manager{}
	registryMu sync.RWMutex
)

// ErrAlreadyRegistered is returned by Register when a Manager with the given
// name already exists in the global registry.
var ErrAlreadyRegistered = errors.Define(409, "DATABASE_ALREADY_REGISTERED", "database connection already registered")

// New opens all configured database connections, validates each with a Ping,
// applies connection pool settings, and optionally installs the tracing plugin.
// Connections are stored on the returned Manager; nothing is written to the
// global registry. Use Register to make the Manager accessible by name.
func New(opts ...Option) (*Manager, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	m := &Manager{
		opts:        o,
		connections: make(map[string]*gorm.DB, len(o.drivers)),
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

		m.connections[d.name] = db
	}
	return m, nil
}

// DB returns the *gorm.DB for the named connection held by this Manager,
// or nil if no connection with that name was opened.
func (m *Manager) DB(name string) *gorm.DB {
	return m.connections[name]
}

// Close closes all database connections held by this Manager and removes them
// from the internal map. The first close error encountered is returned; other
// connections are still closed.
func (m *Manager) Close() error {
	var first error
	for name, db := range m.connections {
		sqlDB, err := db.DB()
		if err != nil {
			continue
		}
		if err := sqlDB.Close(); err != nil && first == nil {
			first = err
		}
		delete(m.connections, name)
	}
	return first
}

// Register stores m in the global registry under name. Returns
// ErrAlreadyRegistered if a Manager is already registered under that name.
func Register(name string, m *Manager) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		return ErrAlreadyRegistered
	}
	registry[name] = m
	return nil
}

// Replace stores m in the global registry under name, replacing any previous
// Manager. If a previous Manager existed it is closed before the function
// returns. Replace never returns an error.
func Replace(name string, m *Manager) error {
	registryMu.Lock()
	old := registry[name]
	registry[name] = m
	registryMu.Unlock()
	if old != nil {
		old.Close() //nolint:errcheck
	}
	return nil
}

// CloseAll closes all database connections held by every registered Manager
// and clears the registry. It collects and returns the first non-nil error
// encountered while closing.
func CloseAll() error {
	registryMu.Lock()
	var first error
	for _, m := range registry {
		if err := m.Close(); err != nil && first == nil {
			first = err
		}
	}
	clear(registry)
	registryMu.Unlock()
	return first
}

// Get returns the Manager registered under name, or nil.
func Get(name string) *Manager {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// DB is a convenience function that delegates to Get("default").DB(connName).
// It returns nil when no Manager is registered as "default" or when the named
// connection does not exist on that Manager.
func DB(connName string) *gorm.DB {
	registryMu.RLock()
	m := registry["default"]
	registryMu.RUnlock()
	if m == nil {
		return nil
	}
	return m.DB(connName)
}

// Up applies all pending migrations for every database connection held by this
// Manager. It is a no-op when WithMigrationsDir was not configured.
// Up satisfies the app.Migrator interface and is called by app.Run before
// BeforeStart hooks and server startup.
func (m *Manager) Up(ctx context.Context) error {
	if m.opts.migrationsDir == "" {
		return nil
	}
	for _, d := range m.opts.drivers {
		db := m.DB(d.name)
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
