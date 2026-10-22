package config

import "github.com/pemako/forge/errors"

// Sentinel errors returned by Load and Watch. Match them with errors.Is.
var (
	// ErrMissingRequiredKey is returned by Load when one or more keys declared
	// via WithRequired are absent from the merged configuration (file plus
	// environment). The error message names every missing key, not just the
	// first one.
	ErrMissingRequiredKey = errors.Define(500, "CONFIG_MISSING_REQUIRED_KEY",
		"required configuration key is missing")

	// ErrWatchDisabled is returned by Watch when the Config was loaded without
	// WithWatch. Without a file watcher the registered callback could never
	// fire, so Watch reports the misconfiguration instead of silently
	// accepting a callback it will never call.
	ErrWatchDisabled = errors.Define(500, "CONFIG_WATCH_DISABLED",
		"config watching is not enabled; load with config.WithWatch()")
)
