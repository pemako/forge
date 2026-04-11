package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/pemako/mk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_yaml(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "test-service", cfg.GetString("app.name"))
	assert.Equal(t, 8080, cfg.GetInt("server.http.port"))
	assert.Equal(t, 30*time.Second, cfg.GetDuration("server.http.timeout"))
}

func TestLoad_toml(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.toml"))
	require.NoError(t, err)
	assert.Equal(t, "test-service", cfg.GetString("app.name"))
}

func TestLoad_envOverride(t *testing.T) {
	os.Setenv("APP_APP_NAME", "env-service")
	defer os.Unsetenv("APP_APP_NAME")
	cfg, err := config.Load(
		config.WithFile("testdata/config.yaml"),
		config.WithEnv("APP"),
	)
	require.NoError(t, err)
	assert.Equal(t, "env-service", cfg.GetString("app.name"))
}

func TestLoad_Scan(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)
	var app struct {
		Name string `mapstructure:"name"`
	}
	require.NoError(t, cfg.Scan("app", &app))
	assert.Equal(t, "test-service", app.Name)
}
