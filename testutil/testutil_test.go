package testutil_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/testutil"
)

// TestFreePort verifies the returned address is a valid, connectable TCP address.
func TestFreePort(t *testing.T) {
	addr := testutil.FreePort(t)
	assert.NotEmpty(t, addr)

	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", host)
	assert.NotEqual(t, "0", port)
}

func TestFreePort_unique(t *testing.T) {
	a := testutil.FreePort(t)
	b := testutil.FreePort(t)
	assert.NotEqual(t, a, b)
}

// TestNewConfig_getString covers basic string lookup.
func TestNewConfig_getString(t *testing.T) {
	cfg := testutil.NewConfig(map[string]string{"app.name": "test-svc"})
	assert.Equal(t, "test-svc", cfg.GetString("app.name"))
	assert.Equal(t, "", cfg.GetString("missing.key"))
}

func TestNewConfig_getInt(t *testing.T) {
	cfg := testutil.NewConfig(map[string]string{
		"port":    "8080",
		"zero":    "0",
		"invalid": "abc",
	})
	assert.Equal(t, 8080, cfg.GetInt("port"))
	assert.Equal(t, 0, cfg.GetInt("zero"))
	assert.Equal(t, 0, cfg.GetInt("invalid"))
	assert.Equal(t, 0, cfg.GetInt("missing"))
}

func TestNewConfig_getBool(t *testing.T) {
	cfg := testutil.NewConfig(map[string]string{
		"a": "true", "b": "1", "c": "yes",
		"d": "false", "e": "0", "f": "no",
	})
	assert.True(t, cfg.GetBool("a"))
	assert.True(t, cfg.GetBool("b"))
	assert.True(t, cfg.GetBool("c"))
	assert.False(t, cfg.GetBool("d"))
	assert.False(t, cfg.GetBool("e"))
	assert.False(t, cfg.GetBool("f"))
}

func TestNewConfig_getFloat64(t *testing.T) {
	cfg := testutil.NewConfig(map[string]string{"ratio": "0.5", "bad": "x"})
	assert.InDelta(t, 0.5, cfg.GetFloat64("ratio"), 1e-9)
	assert.Equal(t, float64(0), cfg.GetFloat64("bad"))
}

func TestNewConfig_getDuration(t *testing.T) {
	cfg := testutil.NewConfig(map[string]string{"ttl": "30s", "bad": "nope"})
	assert.Equal(t, 30*time.Second, cfg.GetDuration("ttl"))
	assert.Equal(t, time.Duration(0), cfg.GetDuration("bad"))
}

func TestNewConfig_getStringSlice(t *testing.T) {
	cfg := testutil.NewConfig(map[string]string{
		"tags":  "a, b, c",
		"empty": "",
	})
	assert.Equal(t, []string{"a", "b", "c"}, cfg.GetStringSlice("tags"))
	assert.Nil(t, cfg.GetStringSlice("empty"))
	assert.Nil(t, cfg.GetStringSlice("missing"))
}

func TestNewConfig_implementsCoreConfig(t *testing.T) {
	cfg := testutil.NewConfig(nil)
	var _ core.Config = cfg
}

// TestNewLogger verifies the logger satisfies core.Logger and does not panic.
func TestNewLogger_implementsCoreLogger(t *testing.T) {
	log := testutil.NewLogger(t)
	var _ core.Logger = log
}

func TestNewLogger_allLevels(t *testing.T) {
	log := testutil.NewLogger(t)
	ctx := context.Background()
	// None of these should panic.
	log.Debug(ctx, "debug msg", core.Field{Key: "k", Value: "v"})
	log.Info(ctx, "info msg")
	log.Warn(ctx, "warn msg")
	log.Error(ctx, "error msg", core.Field{Key: "n", Value: 42})
}

func TestNewLogger_with(t *testing.T) {
	log := testutil.NewLogger(t)
	child := log.With(core.Field{Key: "svc", Value: "order"})
	var _ core.Logger = child
	child.Info(context.Background(), "child log")
}
