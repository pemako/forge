package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// Migrator wraps goose for a GORM-managed database connection.
// Obtain one with NewMigrator; Manager.Up uses this internally.
type Migrator struct {
	db  *gorm.DB
	dir string
}

// NewMigrator creates a Migrator for the given GORM database and migrations directory.
// dir should contain goose-compatible SQL migration files.
func NewMigrator(db *gorm.DB, dir string) *Migrator {
	return &Migrator{db: db, dir: dir}
}

func (m *Migrator) sqlDB() (*sql.DB, error) {
	return m.db.DB()
}

func (m *Migrator) gooseDialect() (string, error) {
	switch m.db.Dialector.Name() {
	case "mysql":
		return "mysql", nil
	case "postgres":
		return "postgres", nil
	case "sqlite":
		return "sqlite3", nil
	default:
		return "", fmt.Errorf("unsupported dialect: %s", m.db.Dialector.Name())
	}
}

// Up applies all pending migrations in the configured directory in ascending
// version order. It is safe to call multiple times; already-applied migrations
// are skipped by goose.
func (m *Migrator) Up(ctx context.Context) error {
	db, err := m.sqlDB()
	if err != nil {
		return err
	}
	dialect, err := m.gooseDialect()
	if err != nil {
		return err
	}
	if err := goose.SetDialect(dialect); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, m.dir)
}

// Down rolls back the most recent migration.
func (m *Migrator) Down(ctx context.Context) error {
	db, err := m.sqlDB()
	if err != nil {
		return err
	}
	dialect, err := m.gooseDialect()
	if err != nil {
		return err
	}
	if err := goose.SetDialect(dialect); err != nil {
		return err
	}
	return goose.DownContext(ctx, db, m.dir)
}

// UpTo applies migrations up to and including the given version number.
// Useful for controlled rollouts or testing specific migration states.
func (m *Migrator) UpTo(ctx context.Context, version int64) error {
	db, err := m.sqlDB()
	if err != nil {
		return err
	}
	dialect, err := m.gooseDialect()
	if err != nil {
		return err
	}
	if err := goose.SetDialect(dialect); err != nil {
		return err
	}
	return goose.UpToContext(ctx, db, m.dir, version)
}

// Status returns the current applied migration version as reported by goose.
// Returns 0 if no migrations have been applied yet.
func (m *Migrator) Status(ctx context.Context) (int64, error) {
	db, err := m.sqlDB()
	if err != nil {
		return 0, err
	}
	dialect, err := m.gooseDialect()
	if err != nil {
		return 0, err
	}
	if err := goose.SetDialect(dialect); err != nil {
		return 0, err
	}
	return goose.GetDBVersionContext(ctx, db)
}
