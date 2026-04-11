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
)

// Manager holds initialized connections and satisfies app.Migrator.
type Manager struct {
	opts options
}

var (
	registry   = map[string]*gorm.DB{}
	registryMu sync.RWMutex
)

// New opens all configured connections, validates each with Ping, and registers them.
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
		registry[d.name] = db
		registryMu.Unlock()
	}

	return &Manager{opts: o}, nil
}

// DB returns the *gorm.DB for the named connection, or nil if not registered.
func DB(name string) *gorm.DB {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// Up runs pending migrations. Satisfies app.Migrator interface.
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
