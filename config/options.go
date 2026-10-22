// Package config provides a Viper-backed implementation of core.Config.
// Use Load with functional options to initialize configuration from files,
// environment variables, and optional hot-reload on file changes.
//
// Fail-fast: declare the keys your service cannot start without via
// WithRequired, and Load returns ErrMissingRequiredKey naming every missing
// one instead of letting the getters hand back zero values at runtime.
// Declare the key prefixes your service understands via WithKnownKeys and
// Load logs a warning listing any key present in the file that nothing reads
// (typically a typo); use UnknownKeys for programmatic access.
package config

import "github.com/pemako/forge/core"

// options holds the functional configuration for Load.
type options struct {
	file      string
	envPref   string
	watch     bool
	required  []string
	knownKeys []string
	// declaredKnown records whether WithKnownKeys was called at all, so that
	// WithKnownKeys() with no arguments can be told apart from never calling it.
	declaredKnown bool
	logger        core.Logger
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
//
// Watch returns ErrWatchDisabled when this option was not supplied.
func WithWatch() Option { return func(o *options) { o.watch = true } }

// WithRequired declares configuration keys that must be present. Load returns
// an error wrapping ErrMissingRequiredKey that names every missing key, so a
// misconfigured deployment fails at startup rather than silently running with
// zero values.
//
// Keys use dot notation and are checked against the merged configuration, so a
// key supplied only through an environment variable (see WithEnv) satisfies the
// requirement. A key that is present but empty counts as missing.
//
//	cfg, err := config.Load(
//	    config.WithFile("config.yaml"),
//	    config.WithRequired("database.default.dsn", "cache.redis.addr"),
//	)
func WithRequired(keys ...string) Option {
	return func(o *options) { o.required = append(o.required, keys...) }
}

// WithKnownKeys declares the key prefixes this service reads. Load compares
// them against every key actually present in the configuration and logs one
// warning listing the keys that nothing declared — which in practice means a
// typo ("databse.default.dsn") that would otherwise read back as a zero value.
//
// A prefix covers itself and everything beneath it: "database" matches
// "database" and "database.default.dsn". Prefixes use dot notation.
//
// This warns rather than errors on purpose. Configuration files legitimately
// carry keys this library knows nothing about — per-deployment annotations,
// settings consumed by application code, blocks staged for a future release —
// and making an undeclared key fatal would break every existing caller. Use
// UnknownKeys if you want to escalate it to an error yourself:
//
//	cfg, _ := config.Load(config.WithFile("config.yaml"))
//	if extra := cfg.UnknownKeys([]string{"app", "server"}); len(extra) > 0 {
//	    return fmt.Errorf("unknown config keys: %v", extra)
//	}
func WithKnownKeys(prefixes ...string) Option {
	return func(o *options) {
		o.declaredKnown = true
		o.knownKeys = append(o.knownKeys, prefixes...)
	}
}

// WithLogger sets the logger used to warn about undeclared keys found by
// WithKnownKeys. Defaults to logger.Default() when not provided.
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }
