package stream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pemako/mk/core"
	"github.com/redis/go-redis/v9"
)

type streamWorker struct {
	bus  *Bus
	done chan struct{}
	log  core.Logger
	wg   sync.WaitGroup // tracks in-flight process() calls
}

// Start implements core.Server. It blocks until Stop is called.
func (w *streamWorker) Start(ctx context.Context) error {
	if w.bus == nil || w.bus.rdb == nil {
		return fmt.Errorf("stream: not initialized — call stream.Init before Worker.Start")
	}

	w.bus.mu.RLock()
	localSubs := make([]subscription, len(w.bus.subs))
	copy(localSubs, w.bus.subs)
	w.bus.mu.RUnlock()

	var wg sync.WaitGroup
	for _, sub := range localSubs {
		sub := sub
		// Ensure the consumer group exists; ignore BUSYGROUP error (already exists).
		err := w.bus.rdb.XGroupCreateMkStream(ctx, sub.topic, sub.group, "0").Err()
		if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
			w.log.Warn(ctx, "stream: create group",
				core.Field{Key: "topic", Value: sub.topic},
				core.Field{Key: "group", Value: sub.group},
				core.Field{Key: "err", Value: err.Error()},
			)
		}

		n := sub.concurrency
		if n < 1 {
			n = 1
		}
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				w.consume(sub)
			}()
		}
	}

	<-w.done
	wg.Wait()
	return nil
}

// Stop implements core.Server. It signals all consumer goroutines to exit
// and waits for them to finish.
func (w *streamWorker) Stop(_ context.Context) error {
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	w.wg.Wait() // wait for all in-flight process() calls to finish
	return nil
}

func (w *streamWorker) consume(sub subscription) {
	ctx := context.Background()
	for {
		select {
		case <-w.done:
			return
		default:
		}

		result, err := w.bus.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    sub.group,
			Consumer: sub.consumer,
			Streams:  []string{sub.topic, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		}
		if err != nil {
			select {
			case <-w.done:
				return
			default:
			}
			w.log.Error(ctx, "stream: read error",
				core.Field{Key: "topic", Value: sub.topic},
				core.Field{Key: "err", Value: err.Error()},
			)
			time.Sleep(time.Second)
			continue
		}

		for _, s := range result {
			for _, msg := range s.Messages {
				w.wg.Add(1)
				go func(m redis.XMessage) {
					defer w.wg.Done()
					w.process(ctx, sub, m)
				}(msg)
			}
		}
	}
}

func (w *streamWorker) process(ctx context.Context, sub subscription, xmsg redis.XMessage) {
	raw, _ := xmsg.Values["data"].(string)
	msg := Message{
		ID:      xmsg.ID,
		Topic:   sub.topic,
		Payload: []byte(raw),
	}

	delay := sub.retryDelay
	if delay == 0 {
		delay = time.Second
	}

	var lastErr error
	for attempt := range sub.retries + 1 {
		if attempt > 0 {
			time.Sleep(delay * time.Duration(attempt))
		}
		if err := sub.handler(ctx, msg); err != nil {
			lastErr = err
			w.log.Warn(ctx, "stream: handler error",
				core.Field{Key: "topic", Value: sub.topic},
				core.Field{Key: "msg_id", Value: msg.ID},
				core.Field{Key: "attempt", Value: attempt},
				core.Field{Key: "err", Value: err.Error()},
			)
			continue
		}
		w.bus.rdb.XAck(ctx, sub.topic, sub.group, xmsg.ID)
		return
	}

	// Max retries exceeded → dead-letter stream.
	w.log.Error(ctx, "stream: message moved to dead letter",
		core.Field{Key: "topic", Value: sub.topic},
		core.Field{Key: "msg_id", Value: msg.ID},
		core.Field{Key: "err", Value: lastErr.Error()},
	)
	w.bus.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: sub.topic + ":dead",
		MaxLen: 1000,
		Approx: true,
		Values: map[string]any{
			"original_id": xmsg.ID,
			"data":        raw,
			"error":       lastErr.Error(),
		},
	})
	w.bus.rdb.XAck(ctx, sub.topic, sub.group, xmsg.ID)
}
