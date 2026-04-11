package config

type options struct {
	file    string
	envPref string
	watch   bool
}

type Option func(*options)

// WithFile sets the config file path. Format is auto-detected (.yaml/.toml/.json).
func WithFile(path string) Option { return func(o *options) { o.file = path } }

// WithEnv enables env var overrides with the given prefix.
// APP_DB_DSN overrides db.dsn (prefix stripped, underscores become dots).
func WithEnv(prefix string) Option { return func(o *options) { o.envPref = prefix } }

// WithWatch enables hot-reload on file changes.
// Only logger.level supports live reload; other keys log a warning and are ignored.
func WithWatch() Option { return func(o *options) { o.watch = true } }
