package config

import (
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type viperConfig struct {
	v        *viper.Viper
	watchers map[string][]func(any)
}

// Load initializes a new Config from the provided options.
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

	cfg := &viperConfig{v: v, watchers: make(map[string][]func(any))}

	if o.watch {
		v.WatchConfig()
		// OnConfigChange requires fsnotify.Event parameter (not any).
		v.OnConfigChange(func(_ fsnotify.Event) {
			for key, fns := range cfg.watchers {
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

func (c *viperConfig) Scan(key string, dest any) error {
	return c.v.UnmarshalKey(key, dest)
}

func (c *viperConfig) Watch(key string, fn func(any)) error {
	c.watchers[key] = append(c.watchers[key], fn)
	return nil
}
