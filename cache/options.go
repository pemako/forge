// Package cache provides cache client management for mk services.
// It maintains a global registry of named Cache instances so that
// any package can retrieve a client without dependency injection.
// Backends: in-memory (default) or Redis (configured via backend: redis).
//
// Use Init to initialize backends, and Client to retrieve one by name.
package cache

import "github.com/pemako/mk/core"

type backendEntry struct {
	name     string
	backend  string // "memory" (default) or "redis"
	addr     string
	password string
	db       int
}

// options holds the functional configuration for Init.
type options struct {
	backends []backendEntry
	logger   core.Logger
}

// Option is a functional option for configuring the cache.
type Option func(*options)

// WithMemory registers a named in-memory cache backend.
// This is the default backend and requires no external dependencies.
func WithMemory(name string) Option {
	return func(o *options) {
		o.backends = append(o.backends, backendEntry{name: name, backend: "memory"})
	}
}

// WithRedis registers a named Redis cache backend by host:port.
func WithRedis(name, addr string) Option {
	return func(o *options) {
		o.backends = append(o.backends, backendEntry{name: name, backend: "redis", addr: addr})
	}
}

// WithRedisAuth registers a named Redis backend with password and DB index.
func WithRedisAuth(name, addr, password string, db int) Option {
	return func(o *options) {
		o.backends = append(o.backends, backendEntry{name: name, backend: "redis", addr: addr, password: password, db: db})
	}
}

// WithConfig reads cache instances from core.Config.
// Each named instance (default, session, etc.) requires a backend field.
//
//	cache:
//	  default:
//	    backend: memory      # or redis
//	  session:
//	    backend: redis
//	    addr: "redis:6379"
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		for _, name := range []string{"default", "session"} {
			backend := cfg.GetString("cache." + name + ".backend")
			if backend == "" {
				// Skip unconfigured instances
				continue
			}
			o.backends = append(o.backends, backendEntry{
				name:     name,
				backend:  backend,
				addr:     cfg.GetString("cache." + name + ".addr"),
				password: cfg.GetString("cache." + name + ".password"),
				db:       cfg.GetInt("cache." + name + ".db"),
			})
		}
	}
}

// WithLogger sets the logger used for slow-command warnings (Redis only).
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }
