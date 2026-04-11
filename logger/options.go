package logger

import (
	"io"

	"github.com/pemako/mk/core"
)

type options struct {
	level      string
	output     string
	filename   string
	maxSizeMB  int
	maxAgeDays int
	maxBackups int
	writer     io.Writer // test-only; nil = use output/filename
}

type Option func(*options)

func WithLevel(level string) Option   { return func(o *options) { o.level = level } }
func WithOutput(output string) Option { return func(o *options) { o.output = output } }
func WithFilename(f string) Option    { return func(o *options) { o.filename = f } }
func WithMaxSizeMB(mb int) Option     { return func(o *options) { o.maxSizeMB = mb } }
func WithMaxAgeDays(days int) Option  { return func(o *options) { o.maxAgeDays = days } }
func WithMaxBackups(n int) Option     { return func(o *options) { o.maxBackups = n } }
func WithWriter(w io.Writer) Option   { return func(o *options) { o.writer = w } }

// WithConfig reads logger settings from core.Config.
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
	}
}
