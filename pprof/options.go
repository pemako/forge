package pprof

import (
	"runtime"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/logger"
)

type options struct {
	addr          string
	log           core.Logger
	mutexFraction int // 0 = disabled; 1 = sample all; n>1 = sample 1/n
	blockRate     int // nanoseconds; 0 = disabled
}

// Option configures a pprof Server or Register call.
type Option func(*options)

func defaultOptions() options {
	return options{
		addr: "localhost:6060",
		log:  logger.Default(),
	}
}

// WithAddr sets the listening address for the standalone pprof server.
// Defaults to "localhost:6060". Only applies to New; ignored by Register.
func WithAddr(addr string) Option {
	return func(o *options) { o.addr = addr }
}

// WithLogger sets the logger used for lifecycle messages.
// Defaults to logger.Default().
func WithLogger(l core.Logger) Option {
	return func(o *options) { o.log = l }
}

// WithMutexFraction enables mutex contention profiling.
// n=1 samples every mutex event; n>1 samples 1/n events; n=0 disables (default).
// See runtime.SetMutexProfileFraction for details.
func WithMutexFraction(n int) Option {
	return func(o *options) { o.mutexFraction = n }
}

// WithBlockRate enables goroutine blocking profiling.
// ns is the threshold in nanoseconds: blocking events longer than ns are sampled.
// ns=0 disables profiling (default). See runtime.SetBlockProfileRate for details.
func WithBlockRate(ns int) Option {
	return func(o *options) { o.blockRate = ns }
}

// WithConfig reads pprof settings from cfg:
//
//	pprof.addr            — listening address (standalone mode only)
//	pprof.mutex_fraction  — mutex profiling fraction (int)
//	pprof.block_rate      — block profiling rate in nanoseconds (int)
//
// Explicit options applied after WithConfig override config values.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("pprof.addr"); v != "" {
			o.addr = v
		}
		if v := cfg.GetInt("pprof.mutex_fraction"); v != 0 {
			o.mutexFraction = v
		}
		if v := cfg.GetInt("pprof.block_rate"); v != 0 {
			o.blockRate = v
		}
	}
}

func applyRuntimeSettings(o options) {
	if o.mutexFraction > 0 {
		runtime.SetMutexProfileFraction(o.mutexFraction)
	}
	if o.blockRate > 0 {
		runtime.SetBlockProfileRate(o.blockRate)
	}
}
