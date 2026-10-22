package config

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/logger"
)

type viperConfig struct {
	v *viper.Viper

	// watchersMu guards watchers. Watch appends from the caller's goroutine
	// while the fsnotify callback ranges over the same map from the watcher
	// goroutine, so every access must hold the lock.
	watchersMu sync.RWMutex
	watchers   map[string][]func(any)

	// watchEnabled records whether WithWatch was supplied; Watch rejects
	// callbacks it could never invoke.
	watchEnabled bool
}

// Load initializes a new Config from the provided options and returns a
// core.Config-compatible value. It reads the configuration file (if WithFile
// was used), binds environment variables (if WithEnv was used), and starts
// the file watcher (if WithWatch was used).
//
// Returns an error if the config file cannot be read, or an error wrapping
// ErrMissingRequiredKey naming every key declared with WithRequired that is
// absent. Keys present in the configuration but not covered by WithKnownKeys
// are logged as a warning, not treated as an error; see WithKnownKeys.
func Load(opts ...Option) (*viperConfig, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	v := viper.New()

	if o.file != "" {
		v.SetConfigFile(o.file)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	if o.envPref != "" {
		v.SetEnvPrefix(o.envPref)
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
	}

	cfg := &viperConfig{
		v:            v,
		watchers:     make(map[string][]func(any)),
		watchEnabled: o.watch,
	}

	// Fail fast on missing required keys, reporting all of them at once so a
	// misconfigured deployment needs one restart to diagnose, not one per key.
	if missing := cfg.missingKeys(o.required); len(missing) > 0 {
		return nil, errors.Wrap(ErrMissingRequiredKey,
			"missing required config keys: %s", strings.Join(missing, ", "))
	}

	// Surface undeclared (typically typo'd) keys as a warning. See WithKnownKeys
	// for why this is not an error.
	if o.declaredKnown {
		if unknown := cfg.UnknownKeys(o.knownKeys); len(unknown) > 0 {
			log := o.logger
			if log == nil {
				log = logger.Default()
			}
			log.Warn(context.Background(),
				"config contains keys that no component declared; check for typos",
				core.Field{Key: "unknown_keys", Value: unknown},
				core.Field{Key: "file", Value: o.file},
			)
		}
	}

	if o.watch {
		v.WatchConfig()
		// OnConfigChange requires fsnotify.Event parameter (not any).
		v.OnConfigChange(func(_ fsnotify.Event) {
			// Copy the callbacks under the read lock, then invoke them after
			// releasing it: a callback that calls Watch would otherwise
			// deadlock on the same non-reentrant mutex.
			cfg.watchersMu.RLock()
			snapshot := make(map[string][]func(any), len(cfg.watchers))
			for key, fns := range cfg.watchers {
				snapshot[key] = slices.Clone(fns)
			}
			cfg.watchersMu.RUnlock()

			for key, fns := range snapshot {
				val := v.Get(key)
				for _, fn := range fns {
					fn(val)
				}
			}
		})
	}

	return cfg, nil
}

func (c *viperConfig) Get(key string) any                   { return c.v.Get(key) }
func (c *viperConfig) GetString(key string) string          { return c.v.GetString(key) }
func (c *viperConfig) GetInt(key string) int                { return c.v.GetInt(key) }
func (c *viperConfig) GetBool(key string) bool              { return c.v.GetBool(key) }
func (c *viperConfig) GetDuration(key string) time.Duration { return c.v.GetDuration(key) }
func (c *viperConfig) GetFloat64(key string) float64        { return c.v.GetFloat64(key) }
func (c *viperConfig) GetStringSlice(key string) []string   { return c.v.GetStringSlice(key) }

// IsSet reports whether key has a value in the merged configuration (file,
// environment, or explicit default). It distinguishes "absent" from "present
// but zero", which the Get* methods cannot: GetInt returns 0 for both a
// missing key and `port: 0`.
//
// IsSet is part of the core.Config interface, so it is callable through that
// interface directly:
//
//	if cfg.IsSet("server.port") {
//	    ...
//	}
func (c *viperConfig) IsSet(key string) bool { return c.v.IsSet(key) }

// UnknownKeys returns the configuration keys that are not covered by any
// prefix in declared, sorted for stable output. A prefix covers itself and
// every key beneath it, so declaring "database" covers "database.default.dsn".
//
// Use it to detect typo'd or stale keys: a key nothing reads is silently
// ignored by the Get* methods, and a misspelled key reads back as a zero
// value. Load logs these as a warning when WithKnownKeys is used; call this
// directly to escalate them to an error.
//
// An empty declared slice reports every key in the configuration.
func (c *viperConfig) UnknownKeys(declared []string) []string {
	var unknown []string
	for _, key := range c.v.AllKeys() {
		if !coveredBy(key, declared) {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// missingKeys returns the subset of keys that have no value, preserving the
// order in which they were declared.
func (c *viperConfig) missingKeys(keys []string) []string {
	var missing []string
	for _, k := range keys {
		if !c.v.IsSet(k) {
			missing = append(missing, k)
		}
	}
	return missing
}

// coveredBy reports whether key equals or sits beneath any prefix in prefixes.
func coveredBy(key string, prefixes []string) bool {
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if key == p || strings.HasPrefix(key, p+".") {
			return true
		}
	}
	return false
}

func (c *viperConfig) Scan(key string, dest any) error {
	return c.v.UnmarshalKey(key, dest)
}

// Watch registers fn to be called with the new value whenever key changes on
// disk. Returns ErrWatchDisabled if the Config was loaded without WithWatch,
// because no file watcher exists and fn could never fire.
func (c *viperConfig) Watch(key string, fn func(any)) error {
	if !c.watchEnabled {
		return errors.Wrap(ErrWatchDisabled, "watch key %q", key)
	}
	c.watchersMu.Lock()
	defer c.watchersMu.Unlock()
	c.watchers[key] = append(c.watchers[key], fn)
	return nil
}
