package database_test

import (
	"testing"

	"github.com/pemako/mk/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type User struct {
	ID   uint   `gorm:"primaryKey"`
	Name string
}

func TestNew_sqlite(t *testing.T) {
	mgr, err := database.New(
		database.WithDriver("test_sqlite1", "sqlite", "file::memory:?cache=shared&_test1"),
	)
	require.NoError(t, err)
	assert.NotNil(t, mgr)

	db := database.DB("test_sqlite1")
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
