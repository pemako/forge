package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

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
