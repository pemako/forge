package database_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pemako/forge/config"
	"github.com/pemako/forge/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scanErrConfig is a core.Config whose Scan always fails, standing in for a
// malformed configuration block.
type scanErrConfig struct {
	err error
}

func (m *scanErrConfig) Get(string) any                   { return nil }
func (m *scanErrConfig) GetString(string) string          { return "" }
func (m *scanErrConfig) GetInt(string) int                { return 0 }
func (m *scanErrConfig) GetBool(string) bool              { return false }
func (m *scanErrConfig) GetFloat64(string) float64        { return 0 }
func (m *scanErrConfig) IsSet(string) bool                { return false }
func (m *scanErrConfig) GetDuration(string) time.Duration { return 0 }
func (m *scanErrConfig) GetStringSlice(string) []string   { return nil }
func (m *scanErrConfig) Watch(string, func(any)) error    { return nil }
func (m *scanErrConfig) Scan(string, any) error           { return m.err }

// TestWithConfig_decodeErrorSurfaces is the regression test for the swallowed
// Scan error: WithConfig used to drop it on the floor (err == nil && ...), so a
// malformed database block produced a Manager with zero connections and no
// diagnostic at all.
func TestWithConfig_decodeErrorSurfaces(t *testing.T) {
	cfg := &scanErrConfig{err: assert.AnError}

	mgr, err := database.New(database.WithConfig(cfg))
	require.Error(t, err, "a malformed database block must not be silently skipped")
	assert.Nil(t, mgr)
	assert.ErrorIs(t, err, database.ErrInvalidConfig)
	// The failing key must be named so the operator knows where to look.
	assert.Contains(t, err.Error(), "database.default")
	// The underlying cause must stay in the chain.
	assert.ErrorIs(t, err, assert.AnError)
}

// TestWithConfig_malformedBlockFromFile is the end-to-end form: a real config
// file whose database.default is a scalar instead of a map.
func TestWithConfig_malformedBlockFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path,
		[]byte("database:\n  default: not-a-map\n"), 0o600))

	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	mgr, err := database.New(database.WithConfig(cfg))
	require.Error(t, err)
	assert.Nil(t, mgr)
	assert.ErrorIs(t, err, database.ErrInvalidConfig)
}

// TestWithConfig_absentBlockIsNotAnError guards the other direction: an absent
// database block is legitimate and must still yield an empty Manager.
func TestWithConfig_absentBlockIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "none.yaml")
	require.NoError(t, os.WriteFile(path, []byte("app:\n  name: svc\n"), 0o600))

	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	mgr, err := database.New(database.WithConfig(cfg))
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Nil(t, mgr.DB("default"))
}
