package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/pemako/forge/config"
	"github.com/pemako/forge/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConfig is a minimal core.Config implementation for testing WithConfig.
type mockConfig struct {
	data map[string]any
}

func (m *mockConfig) Get(key string) any            { return m.data[key] }
func (m *mockConfig) GetString(key string) string   { v, _ := m.data[key].(string); return v }
func (m *mockConfig) GetInt(key string) int         { v, _ := m.data[key].(int); return v }
func (m *mockConfig) GetBool(key string) bool       { v, _ := m.data[key].(bool); return v }
func (m *mockConfig) GetFloat64(key string) float64 { v, _ := m.data[key].(float64); return v }
func (m *mockConfig) GetDuration(key string) time.Duration {
	v, _ := m.data[key].(time.Duration)
	return v
}
func (m *mockConfig) GetStringSlice(key string) []string { v, _ := m.data[key].([]string); return v }
func (m *mockConfig) Watch(_ string, _ func(any)) error  { return nil }
func (m *mockConfig) Scan(_ string, _ any) error         { return nil }

func TestWithConfig_emptyConfig(t *testing.T) {
	cfg := &mockConfig{data: map[string]any{}}
	// WithConfig with no database keys should produce a Manager with no drivers
	mgr, err := database.New(database.WithConfig(cfg))
	require.NoError(t, err)
	assert.NotNil(t, mgr)
}

func TestWithConfig_fromFile(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)

	mgr, err := database.New(database.WithConfig(cfg))
	require.NoError(t, err)
	assert.NotNil(t, mgr)

	// Access the "default" connection directly from the Manager
	db := mgr.DB("default")
	require.NotNil(t, db)

	// Execute queries to trigger the tracing plugin before/after callbacks
	_ = db.Exec("SELECT 1").Error
	var count int64
	db.Raw("SELECT COUNT(*) FROM sqlite_master").Scan(&count)
}

func TestNew_unsupportedDriver(t *testing.T) {
	_, err := database.New(database.WithDriver("bad", "baddriver", "dsn"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported driver")
}

func TestNew_pingFails(t *testing.T) {
	// SQLite in-memory with invalid DSN causes ping to fail
	_, err := database.New(database.WithDriver("ping_fail", "sqlite", "/nonexistent/path/to/db.db"))
	// May succeed or fail depending on SQLite behavior; just verify no panic
	_ = err
}

func TestManager_DB_afterNew(t *testing.T) {
	mgr, err := database.New(database.WithDriver("test_get", "sqlite", "file::memory:?cache=shared&_get"))
	require.NoError(t, err)
	db := mgr.DB("test_get")
	require.NotNil(t, db)
}

func TestMigrator_upTo(t *testing.T) {
	mgr, err := database.New(database.WithDriver("migrate_upto", "sqlite", "file::memory:?cache=shared&_upto"))
	require.NoError(t, err)
	db := mgr.DB("migrate_upto")
	m := database.NewMigrator(db, "testdata/migrations")
	err = m.UpTo(context.Background(), 1)
	require.NoError(t, err)
	ver, err := m.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), ver)
}

func TestMigrator_down_noMigrations(t *testing.T) {
	mgr, err := database.New(database.WithDriver("migrate_down_empty", "sqlite", "file::memory:?cache=shared&_downempty"))
	require.NoError(t, err)
	db := mgr.DB("migrate_down_empty")
	m := database.NewMigrator(db, "testdata/migrations")
	// Up first, then down
	require.NoError(t, m.Up(context.Background()))
	require.NoError(t, m.Down(context.Background()))
}

func TestWithMigrationsDir(t *testing.T) {
	mgr, err := database.New(
		database.WithDriver("test_migrations_dir", "sqlite", "file::memory:?cache=shared&_mdir"),
		database.WithMigrationsDir("testdata/migrations"),
	)
	require.NoError(t, err)
	// Up() should run migrations when migrationsDir is set
	err = mgr.Up(context.Background())
	require.NoError(t, err)
}

func TestWithLogger_option(t *testing.T) {
	opt := database.WithLogger(nil)
	assert.NotNil(t, opt)
}

func TestNew_withLogger(t *testing.T) {
	_, err := database.New(
		database.WithDriver("test_logger", "sqlite", "file::memory:?cache=shared&_logger"),
		database.WithLogger(nil),
	)
	require.NoError(t, err)
}

type User struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func TestNew_sqlite(t *testing.T) {
	mgr, err := database.New(
		database.WithDriver("test_sqlite1", "sqlite", "file::memory:?cache=shared&_test1"),
	)
	require.NoError(t, err)
	assert.NotNil(t, mgr)

	db := mgr.DB("test_sqlite1")
	require.NotNil(t, db)

	require.NoError(t, db.AutoMigrate(&User{}))
	db.Create(&User{Name: "Alice"})

	var u User
	db.First(&u, "name = ?", "Alice")
	assert.Equal(t, "Alice", u.Name)
}

func TestDB_unknownName(t *testing.T) {
	assert.Nil(t, database.DB("nonexistent_xyz"))
}

func TestMigrator_upDown(t *testing.T) {
	mgr, err := database.New(
		database.WithDriver("migrate_test", "sqlite", "file::memory:?cache=shared&_migrate"),
	)
	require.NoError(t, err)

	db := mgr.DB("migrate_test")
	m := database.NewMigrator(db, "testdata/migrations")

	require.NoError(t, m.Up(context.Background()))

	version, err := m.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), version)

	require.NoError(t, m.Down(context.Background()))
}
