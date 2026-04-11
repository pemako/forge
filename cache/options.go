package cache

import "github.com/pemako/mk/core"

type addrEntry struct {
	name     string
	addr     string
	password string
	db       int
}

type options struct {
	addrs  []addrEntry
	logger core.Logger
}

type Option func(*options)

// WithAddr registers a named Redis instance by host:port.
func WithAddr(name, addr string) Option {
	return func(o *options) {
		o.addrs = append(o.addrs, addrEntry{name: name, addr: addr})
	}
}

// WithAddrAuth registers a named Redis instance with password and DB index.
func WithAddrAuth(name, addr, password string, db int) Option {
	return func(o *options) {
		o.addrs = append(o.addrs, addrEntry{name: name, addr: addr, password: password, db: db})
	}
}

// WithConfig reads cache instances from core.Config.
// Scans cache.default and cache.session.
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

func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }
