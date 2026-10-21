// Package logger provides a structured, leveled logger backed by the standard
// library's log/slog. It implements core.Logger and automatically injects
// OpenTelemetry trace_id and span_id fields from the request context.
// Log entries are emitted as JSON.
//
// Use New to create a logger, SetDefault to install it as the process-wide default,
// and the package-level Info/Debug/Warn/Error functions to log without passing a
// logger explicitly.
package logger

import (
	"io"

	"github.com/pemako/forge/core"
)

// options holds the functional configuration for New.
type options struct {
	level      string
	output     string
	filename   string
	maxSizeMB  int
	maxAgeDays int
	maxBackups int
	writer     io.Writer // test-only; nil = use output/filename
	MaskFields []string
}

// Option is a functional option for configuring the logger.
type Option func(*options)

// WithLevel sets the minimum log level. Valid values are "debug", "info",
// "warn", and "error". Defaults to "info".
func WithLevel(level string) Option { return func(o *options) { o.level = level } }

// WithOutput sets the log destination. Valid values are "stdout" (default),
// "file" (write to rotating files only), and "both" (stdout and files).
func WithOutput(output string) Option { return func(o *options) { o.output = output } }

// WithFilename sets the base path for log files when output is "file" or "both".
// Level-specific suffixes are appended (e.g., "/var/log/app.info.log").
func WithFilename(f string) Option { return func(o *options) { o.filename = f } }

// WithMaxSizeMB sets the maximum size in megabytes before a log file is rotated.
// Defaults to 100 MB.
func WithMaxSizeMB(mb int) Option { return func(o *options) { o.maxSizeMB = mb } }

// WithMaxAgeDays sets the maximum number of days to retain old log files.
// Defaults to 7 days.
func WithMaxAgeDays(days int) Option { return func(o *options) { o.maxAgeDays = days } }

// WithMaxBackups sets the maximum number of old log files to retain.
// Defaults to 10 backups.
func WithMaxBackups(n int) Option { return func(o *options) { o.maxBackups = n } }

// WithWriter redirects all log output to w. Intended for use in tests.
// When set, file output options are ignored.
func WithWriter(w io.Writer) Option { return func(o *options) { o.writer = w } }

// WithMaskFields adds field names whose string values will be partially redacted
// in log output. Matching is case-insensitive. For example, masking "password"
// turns "mypassword" into "myp***ord".
func WithMaskFields(fields ...string) Option {
	return func(o *options) { o.MaskFields = append(o.MaskFields, fields...) }
}

// WithConfig reads logger settings from core.Config using the standard keys:
// logger.level, logger.output, logger.filename, logger.max_size,
// logger.max_age, logger.max_backups, and logger.mask_fields.
// Only non-zero config values override the corresponding option.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("logger.level"); v != "" {
			o.level = v
		}
		if v := cfg.GetString("logger.output"); v != "" {
			o.output = v
		}
		if v := cfg.GetString("logger.filename"); v != "" {
			o.filename = v
		}
		if v := cfg.GetInt("logger.max_size"); v > 0 {
			o.maxSizeMB = v
		}
		if v := cfg.GetInt("logger.max_age"); v > 0 {
			o.maxAgeDays = v
		}
		if v := cfg.GetInt("logger.max_backups"); v > 0 {
			o.maxBackups = v
		}
		if v := cfg.GetStringSlice("logger.mask_fields"); len(v) > 0 {
			o.MaskFields = v
		}
	}
}
