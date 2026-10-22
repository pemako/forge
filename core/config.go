// Package core/config.go
package core

import "time"

// Config is the interface all configuration implementations must satisfy.
// Use config.Load to obtain a concrete implementation backed by Viper.
// Keys use dot notation (e.g., "server.http.addr"); env vars override file
// values when WithEnv is configured.
type Config interface {
	// Get returns the raw value for key, or nil if not set.
	Get(key string) any
	// GetString returns the string value for key, or "" if not set.
	GetString(key string) string
	// GetInt returns the integer value for key, or 0 if not set.
	GetInt(key string) int
	// GetBool returns the boolean value for key, or false if not set.
	GetBool(key string) bool
	// GetDuration returns the time.Duration value for key, or 0 if not set.
	GetDuration(key string) time.Duration
	// IsSet reports whether key was explicitly configured. Every Get* method
	// returns a zero value for a missing key, which makes "absent" and
	// "configured as empty" indistinguishable; use this to tell them apart.
	IsSet(key string) bool
	// GetFloat64 returns the float64 value for key, or 0 if not set.
	GetFloat64(key string) float64
	// Scan unmarshals the sub-tree at key into dest (must be a pointer to a struct).
	Scan(key string, dest any) error
	// GetStringSlice returns the string slice value for key, or nil if not set.
	GetStringSlice(key string) []string
	// Watch registers fn to be called when key changes. fn receives the new value.
	// Returns error if the watcher cannot be established (e.g., file not found).
	// Only logger.level supports live reload; DB/server keys log a warning and are ignored.
	Watch(key string, fn func(newValue any)) error
}
