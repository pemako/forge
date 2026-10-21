package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/forge/logger"
)

func TestLogger_outputsJSON(t *testing.T) {
	var buf bytes.Buffer
	log, err := logger.New(logger.WithLevel("debug"), logger.WithWriter(&buf))
	require.NoError(t, err)

	log.Info(context.Background(), "hello", logger.F("key", "value"))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "info", entry["level"])
	assert.Equal(t, "hello", entry["msg"])
	assert.Equal(t, "value", entry["key"])
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	log, err := logger.New(logger.WithLevel("debug"), logger.WithWriter(&buf))
	require.NoError(t, err)

	child := log.With(logger.F("service", "payment"))
	child.Info(context.Background(), "charged")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "payment", entry["service"])
}

func TestSetDefault_and_Default(t *testing.T) {
	var buf bytes.Buffer
	log, _ := logger.New(logger.WithLevel("info"), logger.WithWriter(&buf))
	logger.SetDefault(log)

	assert.Equal(t, log, logger.Default())

	logger.Info(context.Background(), "global log")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "global log", entry["msg"])
}

func TestMaskFields(t *testing.T) {
	var buf bytes.Buffer
	log, err := logger.New(
		logger.WithLevel("debug"),
		logger.WithWriter(&buf),
		logger.WithMaskFields("password", "token"),
	)
	require.NoError(t, err)

	log.Info(context.Background(), "login",
		logger.F("password", "supersecret123"),
		logger.F("token", "abc"),
		logger.F("user", "alice"),
	)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.NotEqual(t, "supersecret123", entry["password"], "password should be masked")
	assert.Equal(t, "***", entry["token"], "short token should be fully masked")
	assert.Equal(t, "alice", entry["user"], "non-sensitive field should be unchanged")
}

// TestEncoding pins the wire format produced by the slog handler: the renamed
// built-in keys, a lowercase level, an ISO8601 "ts", a short "caller" pointing
// at the call site, and durations rendered as strings rather than nanoseconds.
func TestEncoding(t *testing.T) {
	var buf bytes.Buffer
	log, err := logger.New(logger.WithLevel("debug"), logger.WithWriter(&buf))
	require.NoError(t, err)

	log.Info(context.Background(), "hello", logger.F("dur", 1500*time.Millisecond))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "info", entry["level"])
	assert.Equal(t, "hello", entry["msg"])
	assert.Equal(t, "1.5s", entry["dur"], "durations must be human-readable, not nanoseconds")

	ts, ok := entry["ts"].(string)
	require.True(t, ok, "ts must be a string")
	_, perr := time.Parse("2006-01-02T15:04:05.000Z0700", ts)
	assert.NoError(t, perr, "ts must be ISO8601 with milliseconds, got %q", ts)

	caller, ok := entry["caller"].(string)
	require.True(t, ok, "caller must be present")
	assert.True(t, strings.HasPrefix(caller, "logger/logger_test.go:"),
		"caller must point at the call site, got %q", caller)
}

// TestMaskFields_With verifies that a secret bound once via With is redacted on
// every subsequent record, not just when passed at the call site.
func TestMaskFields_With(t *testing.T) {
	var buf bytes.Buffer
	log, err := logger.New(
		logger.WithLevel("debug"),
		logger.WithWriter(&buf),
		logger.WithMaskFields("token"),
	)
	require.NoError(t, err)

	log.With(logger.F("token", "supersecret123")).Info(context.Background(), "bound")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.NotEqual(t, "supersecret123", entry["token"], "With-bound secret should be masked")
}

func TestLevelSplitFiles(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app")

	log, err := logger.New(
		logger.WithLevel("debug"),
		logger.WithOutput("file"),
		logger.WithFilename(base),
	)
	require.NoError(t, err)

	ctx := context.Background()
	log.Debug(ctx, "debug msg")
	log.Info(ctx, "info msg")
	log.Warn(ctx, "warn msg")
	log.Error(ctx, "error msg")

	// Give lumberjack time to flush
	time.Sleep(20 * time.Millisecond)

	for _, level := range []string{"debug", "info", "warn", "error"} {
		path := base + "." + level + ".log"
		content, err := os.ReadFile(path)
		require.NoError(t, err, "file for level %s should exist", level)
		assert.Contains(t, string(content), level+" msg", "file should contain level message")
	}
}
