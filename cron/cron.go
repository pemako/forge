// cron/cron.go
package cron

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	robfigcron "github.com/robfig/cron/v3"
)

// tickerJob represents a sub-second interval job driven by time.Ticker.
type tickerJob struct {
	stop chan struct{}
	done chan struct{}
}

// Scheduler manages named cron jobs and implements core.Server so it integrates
// with app lifecycle management via app.WithServer.
type Scheduler struct {
	c           *robfigcron.Cron
	opts        schedulerOptions
	done        chan struct{}
	entryIDs    map[string]robfigcron.EntryID
	tickerJobs  map[string]*tickerJob
	tickerJobMu sync.Mutex
}

// New creates a new Scheduler with the given options.
func New(opts ...Option) *Scheduler {
	o := defaultSchedulerOptions()
	for _, opt := range opts {
		opt(&o)
	}
	c := robfigcron.New(robfigcron.WithLocation(o.location))
	return &Scheduler{
		c:          c,
		opts:       o,
		done:       make(chan struct{}),
		entryIDs:   make(map[string]robfigcron.EntryID),
		tickerJobs: make(map[string]*tickerJob),
	}
}

// parseEveryDuration attempts to parse "@every <duration>" returning (duration, true)
// for sub-second durations, or (0, false) otherwise.
func parseEveryDuration(spec string) (time.Duration, bool) {
	spec = strings.TrimSpace(spec)
	if !strings.HasPrefix(spec, "@every ") {
		return 0, false
	}
	ds := strings.TrimSpace(strings.TrimPrefix(spec, "@every "))
	d, err := time.ParseDuration(ds)
	if err != nil || d >= time.Second {
		return 0, false
	}
	return d, true
}

// Add registers a named job with the given cron spec and per-job options.
// spec uses standard 5-field cron syntax or robfig shorthand (@every 5m, @hourly, etc.).
// Sub-second @every durations (e.g. "@every 50ms") are handled with a ticker.
func (s *Scheduler) Add(name, spec string, fn func(context.Context) error, opts ...JobOption) error {
	jo := defaultJobOptions()
	for _, opt := range opts {
		opt(&jo)
	}

	if d, ok := parseEveryDuration(spec); ok {
		return s.addTickerJob(name, d, fn, jo)
	}

	var running atomic.Int32
	id, err := s.c.AddFunc(spec, func() {
		executeJob(context.Background(), name, fn, jo, &running, s.opts.log)
	})
	if err != nil {
		return err
	}
	s.entryIDs[name] = id
	return nil
}

// addTickerJob registers a sub-second ticker-driven job.
func (s *Scheduler) addTickerJob(name string, interval time.Duration, fn func(context.Context) error, jo jobOptions) error {
	s.tickerJobMu.Lock()
	defer s.tickerJobMu.Unlock()

	if _, exists := s.tickerJobs[name]; exists {
		return fmt.Errorf("cron: job %q already registered", name)
	}

	tj := &tickerJob{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	s.tickerJobs[name] = tj

	var running atomic.Int32
	go func() {
		defer close(tj.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-tj.stop:
				return
			case <-ticker.C:
				executeJob(context.Background(), name, fn, jo, &running, s.opts.log)
			}
		}
	}()
	return nil
}

// Remove unregisters the job with the given name.
// Any currently running execution finishes normally.
func (s *Scheduler) Remove(name string) {
	if id, ok := s.entryIDs[name]; ok {
		s.c.Remove(id)
		delete(s.entryIDs, name)
		return
	}

	s.tickerJobMu.Lock()
	tj, ok := s.tickerJobs[name]
	if ok {
		close(tj.stop)
		delete(s.tickerJobs, name)
	}
	s.tickerJobMu.Unlock()

	if ok {
		<-tj.done
	}
}

// Start implements core.Server. It starts the scheduler and blocks until Stop
// is called. Run inside a goroutine — app.WithServer handles this automatically.
func (s *Scheduler) Start(_ context.Context) error {
	s.c.Start()
	<-s.done
	return nil
}

// Stop implements core.Server. It signals Start to return and waits for all
// running jobs to complete, or until ctx is cancelled (shutdown timeout).
func (s *Scheduler) Stop(ctx context.Context) error {
	// Stop ticker jobs.
	s.tickerJobMu.Lock()
	tjs := make([]*tickerJob, 0, len(s.tickerJobs))
	for _, tj := range s.tickerJobs {
		close(tj.stop)
		tjs = append(tjs, tj)
	}
	s.tickerJobs = make(map[string]*tickerJob)
	s.tickerJobMu.Unlock()

	for _, tj := range tjs {
		select {
		case <-tj.done:
		case <-ctx.Done():
		}
	}

	stopCtx := s.c.Stop()
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	select {
	case <-stopCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
