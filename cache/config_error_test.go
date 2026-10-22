package cache_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pemako/forge/cache"
	"github.com/pemako/forge/config"
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
// Scan error: `if err := cfg.Scan(...); err != nil || len(entries) == 0` used
// to treat a decode failure exactly like an absent block, yielding zero
// backends with no diagnostic.
func TestWithConfig_decodeErrorSurfaces(t *testing.T) {
	cfg := &scanErrConfig{err: assert.AnError}

	mgr, err := cache.New(cache.WithConfig(cfg))
	require.Error(t, err, "a malformed cache block must not be silently ignored")
	assert.Nil(t, mgr)
	assert.ErrorIs(t, err, cache.ErrInvalidConfig)
	assert.Contains(t, err.Error(), "cache")
	// The underlying cause must stay in the chain.
	assert.ErrorIs(t, err, assert.AnError)
}

// TestWithConfig_malformedBlockFromFile is the end-to-end form: a real config
// file whose cache block is a scalar instead of a map of backends.
func TestWithConfig_malformedBlockFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("cache: not-a-map\n"), 0o600))

	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	mgr, err := cache.New(cache.WithConfig(cfg))
	require.Error(t, err)
	assert.Nil(t, mgr)
	assert.ErrorIs(t, err, cache.ErrInvalidConfig)
}

// TestWithConfig_absentBlockIsNotAnError guards the other direction: an absent
// cache block is legitimate and must still yield an empty Manager.
func TestWithConfig_absentBlockIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "none.yaml")
	require.NoError(t, os.WriteFile(path, []byte("app:\n  name: svc\n"), 0o600))

	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	mgr, err := cache.New(cache.WithConfig(cfg))
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Nil(t, mgr.Client("default"))
}

// TestWithConfig_validBlockStillWorks confirms the happy path is unchanged.
func TestWithConfig_validBlockStillWorks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ok.yaml")
	require.NoError(t, os.WriteFile(path,
		[]byte("cache:\n  default:\n    backend: memory\n"), 0o600))

	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	mgr, err := cache.New(cache.WithConfig(cfg))
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.NotNil(t, mgr.Client("default"))
}
