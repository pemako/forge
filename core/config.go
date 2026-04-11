// core/config.go
package core

import "time"

// Config is the interface all configuration implementations must satisfy.
type Config interface {
	Get(key string) any
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	GetDuration(key string) time.Duration
	GetFloat64(key string) float64
	// Scan unmarshals the sub-tree at key into dest (must be a pointer to a struct).
	Scan(key string, dest any) error
	GetStringSlice(key string) []string
	// Watch registers fn to be called when key changes. fn receives the new value.
	// Returns error if the watcher cannot be established (e.g., file not found).
	// Only logger.level supports live reload; DB/server keys log a warning and are ignored.
	Watch(key string, fn func(newValue any)) error
}
