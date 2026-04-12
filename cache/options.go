// Package cache provides Redis client management for mk services.
// It maintains a global registry of named *redis.Client instances so that
// any package can retrieve a client without dependency injection.
// A slow-command hook (>100ms) is attached to each client automatically.
//
// Use Init to connect and register clients, and Client to retrieve one by name.
package cache

import "github.com/pemako/mk/core"

type addrEntry struct {
	name     string
	addr     string
	password string
	db       int
}

// options holds the functional configuration for Init.
type options struct {
	addrs  []addrEntry
	logger core.Logger
}

// Option is a functional option for configuring the cache.
type Option func(*options)

// WithAddr registers a named Redis instance by its host:port address.
// name is the identifier used with Client() (e.g., "default").
// No authentication is used; use WithAddrAuth for password-protected instances.
func WithAddr(name, addr string) Option {
	return func(o *options) {
		o.addrs = append(o.addrs, addrEntry{name: name, addr: addr})
	}
}

// WithAddrAuth registers a named Redis instance with password authentication
// and a specific database index. Use db=0 for the default database.
func WithAddrAuth(name, addr, password string, db int) Option {
	return func(o *options) {
		o.addrs = append(o.addrs, addrEntry{name: name, addr: addr, password: password, db: db})
	}
}

// WithConfig reads Redis instances from core.Config. It scans
// cache.default and cache.session config keys for addr, password, and db fields.
// Only entries with a non-empty addr are registered.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		for _, name := range []string{"default", "session"} {
			addr := cfg.GetString("cache." + name + ".addr")
			if addr == "" {
				continue
			}
			o.addrs = append(o.addrs, addrEntry{
				name:     name,
				addr:     addr,
				password: cfg.GetString("cache." + name + ".password"),
				db:       cfg.GetInt("cache." + name + ".db"),
			})
		}
	}
}

// WithLogger sets the logger used for slow-command warnings.
// Defaults to logger.Default().
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }
