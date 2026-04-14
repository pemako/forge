package cache

import "github.com/pemako/forge/core"

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

// WithConfig reads all named cache instances from core.Config.
// Every key under "cache" that has a non-empty "backend" field is registered.
//
//	cache:
//	  default:
//	    backend: memory
//	  redis:
//	    backend: redis
//	    addr: "redis:6379"
//	  session:
//	    backend: redis
//	    addr: "redis-session:6379"
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		// Scan the entire cache sub-tree into a map of backend configs.
		var entries map[string]struct {
			Backend  string `mapstructure:"backend"`
			Addr     string `mapstructure:"addr"`
			Password string `mapstructure:"password"`
			DB       int    `mapstructure:"db"`
		}
		if err := cfg.Scan("cache", &entries); err != nil || len(entries) == 0 {
			return
		}
		for name, e := range entries {
			if e.Backend == "" {
				continue
			}
			o.backends = append(o.backends, backendEntry{
				name:     name,
				backend:  e.Backend,
				addr:     e.Addr,
				password: e.Password,
				db:       e.DB,
			})
		}
	}
}

// WithLogger sets the logger used for slow-command warnings (Redis only).
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }
