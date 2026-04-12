package cache

import (
	"context"
	"sync"
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
	"github.com/redis/go-redis/v9"
)

var (
	registry   = map[string]*redis.Client{}
	registryMu sync.RWMutex
)

// Init creates and validates all configured Redis clients, attaches a
// slow-command logging hook to each, and registers them in the global registry.
// It returns an error if any client cannot connect within 3 seconds.
// Init is typically called once during application startup via app.WithBeforeStart.
func Init(opts ...Option) error {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	if o.logger == nil {
		o.logger = logger.Default()
	}

	for _, e := range o.addrs {
		rdb := redis.NewClient(&redis.Options{
			Addr:     e.addr,
			Password: e.password,
			DB:       e.db,
		})

		// Cancel per-client immediately after Ping — do not defer in loop.
		pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := rdb.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			return err
		}

		rdb.AddHook(newLogHook(e.name, o.logger))

		registryMu.Lock()
		registry[e.name] = rdb
		registryMu.Unlock()
	}
	return nil
}

// Client returns the *redis.Client registered under name, or nil if the
// name has not been registered. Clients are registered by Init and are
// available to any package without further configuration.
func Client(name string) *redis.Client {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

type logHook struct {
	name string
	log  core.Logger
}

func newLogHook(name string, log core.Logger) *logHook {
	return &logHook{name: name, log: log}
}

func (h *logHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *logHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			h.log.Warn(ctx, "slow redis command",
				core.Field{Key: "redis", Value: h.name},
				core.Field{Key: "cmd", Value: cmd.Name()},
				core.Field{Key: "elapsed", Value: elapsed.String()},
			)
		}
		return err
	}
}

func (h *logHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
