// Package lock/renewer.go
// Background TTL renewal for held locks.
package lock

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/pemako/mk/core"
)

// renewLua atomically extends the TTL only if the token still matches.
// Returns 1 on success, 0 if the key is gone or owned by another instance.
const renewLua = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
  return 0
end`

// autoRenewer periodically renews the lock TTL.
// If renewal fails (token mismatch = lock lost), it cancels lockCtx so
// the TryLock callback is notified via ctx.Done().
type autoRenewer struct {
	client     *redis.Client
	key        string
	token      string
	ttl        time.Duration
	log        core.Logger
	stop       chan struct{}
	cancelLock context.CancelFunc
}

// newAutoRenewer creates a renewer and returns it along with a derived context
// that will be cancelled if the lock is lost.
func newAutoRenewer(client *redis.Client, key, token string, ttl time.Duration, log core.Logger) (*autoRenewer, context.Context) {
	lockCtx, cancel := context.WithCancel(context.Background())
	return &autoRenewer{
		client:     client,
		key:        key,
		token:      token,
		ttl:        ttl,
		log:        log,
		stop:       make(chan struct{}),
		cancelLock: cancel,
	}, lockCtx
}

// start begins the background renewal goroutine.
func (r *autoRenewer) start() {
	interval := r.ttl / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-ticker.C:
				result, err := r.client.Eval(
					context.Background(),
					renewLua,
					[]string{r.key},
					r.token,
					r.ttl.Milliseconds(),
				).Int()
				if err != nil || result == 0 {
					r.log.Warn(context.Background(), "lock renewal failed — lock lost",
						core.Field{Key: "key", Value: r.key},
						core.Field{Key: "err", Value: err},
					)
					r.cancelLock()
					return
				}
				r.log.Debug(context.Background(), "lock renewed",
					core.Field{Key: "key", Value: r.key},
				)
			}
		}
	}()
}

// stopRenew halts the renewal goroutine.
func (r *autoRenewer) stopRenew() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}
