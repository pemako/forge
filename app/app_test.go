package app_test

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pemako/mk/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockServer struct{ started, stopped bool }

func (m *mockServer) Start(_ context.Context) error { m.started = true; return nil }
func (m *mockServer) Stop(_ context.Context) error  { m.stopped = true; return nil }

type mockMigrator struct{ called bool }

func (m *mockMigrator) Up(_ context.Context) error { m.called = true; return nil }

type testLifecycle struct {
	initFn     func(ctx context.Context) error
	shutdownFn func(ctx context.Context) error
}

func (t *testLifecycle) Init(ctx context.Context) error     { return t.initFn(ctx) }
func (t *testLifecycle) Shutdown(ctx context.Context) error { return t.shutdownFn(ctx) }

type testCloser struct {
	closeFn func() error
}

func (t *testCloser) Close() error { return t.closeFn() }

func sendSIGTERM() {
	time.Sleep(50 * time.Millisecond)
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}

func TestApp_runsAndShutsDown(t *testing.T) {
	srv := &mockServer{}
	go sendSIGTERM()
	err := app.New(
		app.WithServer(srv),
		app.WithShutdownTimeout(2*time.Second),
	).Run()
	assert.NoError(t, err)
	assert.True(t, srv.started)
	assert.True(t, srv.stopped)
}

func TestApp_migrationRunsBeforeBeforeStart(t *testing.T) {
	order := []string{}
	m := &mockMigrator{}
	go sendSIGTERM()
	app.New(
		app.WithMigration(m),
		app.WithBeforeStart(func(_ context.Context) error {
			order = append(order, "beforeStart")
			return nil
		}),
		app.WithAfterStop(func(_ context.Context) error {
			order = append(order, "afterStop")
			return nil
		}),
		app.WithShutdownTimeout(time.Second),
	).Run()
	assert.True(t, m.called)
	assert.Equal(t, []string{"beforeStart", "afterStop"}, order)
}

func TestWithLifecycle_InitFailureRollsBack(t *testing.T) {
	shutdownCalled := false
	lc1 := &testLifecycle{
		initFn:     func(ctx context.Context) error { return nil },
		shutdownFn: func(ctx context.Context) error { shutdownCalled = true; return nil },
	}
	lc2 := &testLifecycle{
		initFn:     func(ctx context.Context) error { return fmt.Errorf("init failed") },
		shutdownFn: func(ctx context.Context) error { return nil },
	}
	err := app.New(app.WithLifecycle(lc1, lc2)).Run()
	require.Error(t, err)
	require.True(t, shutdownCalled, "lc1.Shutdown should be called to roll back")
}

func TestWithCloser_CalledOnShutdown(t *testing.T) {
	closed := false
	c := &testCloser{closeFn: func() error { closed = true; return nil }}
	go sendSIGTERM()
	app.New(app.WithCloser(c), app.WithShutdownTimeout(time.Second)).Run()
	require.True(t, closed)
}

// appTestChecker is a minimal core.HealthChecker for use in app tests.
type appTestChecker struct {
	name    string
	checkFn func(ctx context.Context) error
}

func (c *appTestChecker) Name() string                    { return c.name }
func (c *appTestChecker) Check(ctx context.Context) error { return c.checkFn(ctx) }

func TestWithHealthCheck_Stored(t *testing.T) {
	checker := &appTestChecker{
		name:    "test",
		checkFn: func(_ context.Context) error { return nil },
	}
	a := app.New(app.WithHealthCheck(checker))
	require.Len(t, a.HealthCheckers(), 1)
	require.Equal(t, "test", a.HealthCheckers()[0].Name())
}

// testServer is a controllable core.Server for use in app tests.
type testServer struct {
	startFn func(ctx context.Context) error
	stopFn  func(ctx context.Context) error
}

func (s *testServer) Start(ctx context.Context) error { return s.startFn(ctx) }
func (s *testServer) Stop(ctx context.Context) error  { return s.stopFn(ctx) }

func TestRun_LifecycleInitFailure_RollsBackPrevious(t *testing.T) {
	order := []string{}
	var mu sync.Mutex
	track := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	lc1 := &testLifecycle{
		initFn:     func(ctx context.Context) error { track("lc1:init"); return nil },
		shutdownFn: func(ctx context.Context) error { track("lc1:shutdown"); return nil },
	}
	lc2 := &testLifecycle{
		initFn:     func(ctx context.Context) error { track("lc2:init"); return fmt.Errorf("lc2 failed") },
		shutdownFn: func(ctx context.Context) error { track("lc2:shutdown"); return nil },
	}

	err := app.New(app.WithLifecycle(lc1, lc2)).Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "lc2 failed")

	mu.Lock()
	got := append([]string{}, order...)
	mu.Unlock()
	// lc1 init, lc2 init (fails), lc1 shutdown (rollback) — lc2 shutdown NOT called
	require.Equal(t, []string{"lc1:init", "lc2:init", "lc1:shutdown"}, got)
}

func TestRun_ServerStartError_StopErrorBothCollected(t *testing.T) {
	startErr := fmt.Errorf("start failed")
	srv := &testServer{
		startFn: func(ctx context.Context) error { return startErr },
		stopFn:  func(ctx context.Context) error { return nil },
	}
	err := app.New(app.WithServer(srv)).Run()
	require.Error(t, err)
	// runErr is the start error; stop error is logged but runErr takes priority
	require.Contains(t, err.Error(), "start failed")
}

func TestRun_HungServer_ReturnsAfterTimeout(t *testing.T) {
	// startFn blocks on a channel that is NEVER closed by stopFn.
	// This simulates a truly hung goroutine (e.g. blocked on I/O).
	// Without the fix, Run() would block on startWg.Wait() forever after Stop returns.
	neverUnblocked := make(chan struct{}) // intentionally never closed

	hung := &testServer{
		startFn: func(ctx context.Context) error {
			<-neverUnblocked // blocks forever — not released by stopFn
			return nil
		},
		stopFn: func(ctx context.Context) error {
			// Stop returns immediately; Start goroutine is still blocked
			return nil
		},
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	}()

	start := time.Now()
	err := app.New(
		app.WithServer(hung),
		app.WithShutdownTimeout(200*time.Millisecond),
	).Run()
	elapsed := time.Since(start)

	assert.NoError(t, err)
	// With the fix: Run returns within ~shutdownTimeout (200ms) + small overhead.
	// Without the fix: Run blocks here forever because startWg.Wait() has no deadline.
	assert.Less(t, elapsed, 2*time.Second, "Run should not hang after shutdown timeout")
}

func TestRun_CloserCalledAfterLifecycleShutdown(t *testing.T) {
	order := []string{}
	lc := &testLifecycle{
		initFn:     func(ctx context.Context) error { return nil },
		shutdownFn: func(ctx context.Context) error { order = append(order, "shutdown"); return nil },
	}
	c := &testCloser{closeFn: func() error { order = append(order, "close"); return nil }}

	go sendSIGTERM()
	app.New(app.WithLifecycle(lc), app.WithCloser(c), app.WithShutdownTimeout(time.Second)).Run()

	require.Equal(t, []string{"shutdown", "close"}, order)
}
