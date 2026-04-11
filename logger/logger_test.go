package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pemako/mk/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
