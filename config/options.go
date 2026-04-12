// Package config provides a Viper-backed implementation of core.Config.
// Use Load with functional options to initialize configuration from files,
// environment variables, and optional hot-reload on file changes.
package config

// options holds the functional configuration for Load.
type options struct {
	file    string
	envPref string
	watch   bool
}

// Option is a functional option for configuring the config loader.
type Option func(*options)

// WithFile sets the path to the configuration file.
// The format is auto-detected from the extension (.yaml, .toml, or .json).
func WithFile(path string) Option { return func(o *options) { o.file = path } }

// WithEnv enables environment variable overrides using the given prefix.
// For example, with prefix "APP", the env var APP_DB_DSN overrides the
// key db.dsn (prefix is stripped and underscores are converted to dots).
func WithEnv(prefix string) Option { return func(o *options) { o.envPref = prefix } }

// WithWatch enables automatic hot-reload when the configuration file changes.
// Only logger.level supports live reload; changes to other keys log a warning
// and are otherwise ignored until the process restarts.
func WithWatch() Option { return func(o *options) { o.watch = true } }
