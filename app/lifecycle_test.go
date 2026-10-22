package app_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/forge/app"
)

// blockingServer honours the core.Server contract and records readiness
// transitions driven by app.
type blockingServer struct {
	mu          sync.Mutex
	readyStates []bool
	readyChecks []string
	done        chan struct{}
	stopped     bool
}

func newBlockingServer() *blockingServer {
	return &blockingServer{done: make(chan struct{})}
}

func (s *blockingServer) Start(_ context.Context) error {
	<-s.done
	return nil
}

func (s *blockingServer) Stop(_ context.Context) error {
	s.mu.Lock()
	already := s.stopped
	s.stopped = true
	s.mu.Unlock()
	if !already {
		close(s.done)
	}
	return nil
}

func (s *blockingServer) SetReady(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readyStates = append(s.readyStates, ready)
}

func (s *blockingServer) AddReadyCheck(name string, _ func(context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readyChecks = append(s.readyChecks, name)
}

func (s *blockingServer) states() []bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bool(nil), s.readyStates...)
}

func (s *blockingServer) checks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.readyChecks...)
}

// runAndStop runs a in a goroutine, requests shutdown once it is up, and
// returns Run's error. Run blocks until shutdown is requested, so a test with
// no server registered has to trigger one itself.
func runAndStop(t *testing.T, a *app.App) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- a.Run() }()
	time.Sleep(50 * time.Millisecond)
	a.Stop()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after Stop")
		return nil
	}
}

// --- Programmatic shutdown (no process signal) ---

func TestStop_ShutsDownWithoutSignal(t *testing.T) {
	srv := newBlockingServer()
	a := app.New(app.WithServer(srv), app.WithShutdownTimeout(2*time.Second))

	done := make(chan error, 1)
	go func() { done <- a.Run() }()

	// Give Run time to reach the blocking select, then stop it without
	// signalling the whole process.
	time.Sleep(50 * time.Millisecond)
	a.Stop()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}

func TestStop_IsIdempotentAndSafeBeforeRun(t *testing.T) {
	a := app.New(app.WithShutdownTimeout(time.Second))
	a.Stop()
	a.Stop() // must not panic on a second close

	done := make(chan error, 1)
	go func() { done <- a.RunContext(context.Background()) }()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return when Stop was called before it started")
	}
}

func TestRunContext_CancelShutsDown(t *testing.T) {
	srv := newBlockingServer()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- app.New(
			app.WithServer(srv),
			app.WithShutdownTimeout(2*time.Second),
		).RunContext(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		// Cancelling ctx is a shutdown request, not a failure.
		assert.NoError(t, err)
		assert.True(t, srv.stopped, "server should have been stopped")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancellation")
	}
}

// TestRunContext_CancelledCtxStillDrains proves the shutdown sequence is not
// bound by the context that triggered it — otherwise Stop/Shutdown/Close would
// all receive a dead context and skip their work.
func TestRunContext_CancelledCtxStillDrains(t *testing.T) {
	var closed atomic.Bool
	c := &testCloser{closeFn: func() error { closed.Store(true); return nil }}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already dead before Run is even called

	err := app.New(app.WithCloser(c), app.WithShutdownTimeout(time.Second)).RunContext(ctx)
	// Startup is reported as aborted, but the teardown still has to happen.
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, closed.Load(), "Closer must still run when the triggering ctx is cancelled")
}

// --- Panic recovery ---

func TestPanicInLifecycleInit_RecoveredAndClosersRun(t *testing.T) {
	var closed atomic.Bool
	c := &testCloser{closeFn: func() error { closed.Store(true); return nil }}
	lc := &testLifecycle{
		initFn:     func(context.Context) error { panic("boom in init") },
		shutdownFn: func(context.Context) error { return nil },
	}

	err := app.New(
		app.WithLifecycle(lc),
		app.WithCloser(c),
		app.WithShutdownTimeout(time.Second),
	).Run()

	require.Error(t, err)
	assert.ErrorIs(t, err, app.ErrPanic)
	assert.Contains(t, err.Error(), "boom in init")
	assert.True(t, closed.Load(), "Closers must run after a panic during startup")
}

func TestPanicInBeforeStart_Recovered(t *testing.T) {
	err := app.New(
		app.WithBeforeStart(func(context.Context) error { panic("boom in hook") }),
		app.WithShutdownTimeout(time.Second),
	).Run()

	require.Error(t, err)
	assert.ErrorIs(t, err, app.ErrPanic)
}

func TestPanicInServerStart_Recovered(t *testing.T) {
	srv := &testServer{
		startFn: func(context.Context) error { panic("boom in start") },
		stopFn:  func(context.Context) error { return nil },
	}
	err := app.New(app.WithServer(srv), app.WithShutdownTimeout(time.Second)).Run()

	require.Error(t, err)
	assert.ErrorIs(t, err, app.ErrPanic)
}

func TestPanicInCloser_Recovered(t *testing.T) {
	c := &testCloser{closeFn: func() error { panic("boom in close") }}
	err := runAndStop(t, app.New(app.WithCloser(c), app.WithShutdownTimeout(time.Second)))

	require.Error(t, err)
	assert.ErrorIs(t, err, app.ErrPanic)
}

// --- Startup failure must not leak ---

func TestBeforeStartFailure_RollsBackLifecyclesAndRunsClosers(t *testing.T) {
	var order []string
	var mu sync.Mutex
	track := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	lc := &testLifecycle{
		initFn:     func(context.Context) error { track("init"); return nil },
		shutdownFn: func(context.Context) error { track("shutdown"); return nil },
	}
	c := &testCloser{closeFn: func() error { track("close"); return nil }}

	err := app.New(
		app.WithLifecycle(lc),
		app.WithCloser(c),
		app.WithBeforeStart(func(context.Context) error { return fmt.Errorf("hook failed") }),
		app.WithShutdownTimeout(time.Second),
	).Run()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook failed")

	mu.Lock()
	defer mu.Unlock()
	// Previously a BeforeStart failure returned with the Lifecycle still up and
	// the Closer never called, leaking every pool already opened.
	assert.Equal(t, []string{"init", "shutdown", "close"}, order)
}

func TestLifecycleInitFailure_RunsClosers(t *testing.T) {
	var closed atomic.Bool
	c := &testCloser{closeFn: func() error { closed.Store(true); return nil }}
	lc := &testLifecycle{
		initFn:     func(context.Context) error { return fmt.Errorf("init failed") },
		shutdownFn: func(context.Context) error { return nil },
	}

	err := app.New(
		app.WithLifecycle(lc),
		app.WithCloser(c),
		app.WithShutdownTimeout(time.Second),
	).Run()

	require.Error(t, err)
	assert.True(t, closed.Load(), "Closers must run when Lifecycle.Init fails")
}

// --- Closer ordering and boundedness ---

func TestClosers_RunInReverseRegistrationOrder(t *testing.T) {
	var order []string
	var mu sync.Mutex
	mk := func(name string) *testCloser {
		return &testCloser{closeFn: func() error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}}
	}

	err := runAndStop(t, app.New(
		app.WithCloser(mk("first"), mk("second"), mk("third")),
		app.WithShutdownTimeout(time.Second),
	))
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	// Reverse order, matching Lifecycle.Shutdown, so a Closer can depend on one
	// registered before it.
	assert.Equal(t, []string{"third", "second", "first"}, order)
}

func TestHungCloser_DoesNotHangRun(t *testing.T) {
	never := make(chan struct{}) // intentionally never closed
	hung := &testCloser{closeFn: func() error {
		<-never
		return nil
	}}
	var after atomic.Bool
	c2 := &testCloser{closeFn: func() error { after.Store(true); return nil }}

	// c2 is registered first so reverse order runs the hung closer first.
	a := app.New(
		app.WithCloser(c2, hung),
		app.WithShutdownTimeout(200*time.Millisecond),
	)
	start := time.Now()
	err := runAndStop(t, a)
	elapsed := time.Since(start)

	require.Error(t, err, "a closer that never returns should surface as an error")
	assert.Less(t, elapsed, 5*time.Second, "Run must not hang on a blocked Closer")
	assert.True(t, after.Load(), "a hung Closer must not prevent the remaining Closers")
}

// --- core.Server contract enforcement ---

func TestServersExitingEarly_IsReported(t *testing.T) {
	// Start returns nil immediately, violating core.Server's "Start must block".
	// Previously Run blocked forever with nothing serving.
	srv := &testServer{
		startFn: func(context.Context) error { return nil },
		stopFn:  func(context.Context) error { return nil },
	}

	done := make(chan error, 1)
	go func() {
		done <- app.New(app.WithServer(srv), app.WithShutdownTimeout(time.Second)).Run()
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, app.ErrServersExited)
	case <-time.After(5 * time.Second):
		t.Fatal("Run hung instead of reporting that all servers exited")
	}
}

// --- Readiness orchestration ---

func TestReadiness_MarkedReadyOnStartAndNotReadyOnShutdown(t *testing.T) {
	srv := newBlockingServer()
	a := app.New(app.WithServer(srv), app.WithShutdownTimeout(2*time.Second))

	done := make(chan error, 1)
	go func() { done <- a.Run() }()

	time.Sleep(50 * time.Millisecond)
	a.Stop()
	require.NoError(t, <-done)

	// true once servers are up, then false before the drain begins — the second
	// transition is what stops a load balancer feeding a draining instance.
	assert.Equal(t, []bool{true, false}, srv.states())
}

func TestHealthCheckers_RegisteredOnServerBeforeStart(t *testing.T) {
	srv := newBlockingServer()
	hc := &appTestChecker{name: "database:default", checkFn: func(context.Context) error { return nil }}

	a := app.New(
		app.WithServer(srv),
		app.WithHealthCheck(hc),
		app.WithShutdownTimeout(2*time.Second),
	)

	done := make(chan error, 1)
	go func() { done <- a.Run() }()
	time.Sleep(50 * time.Millisecond)
	a.Stop()
	require.NoError(t, <-done)

	// WithHealthCheck used to store checkers and wire nothing.
	assert.Equal(t, []string{"database:default"}, srv.checks())
}

func TestDrainDelay_WaitedBeforeStoppingServers(t *testing.T) {
	srv := newBlockingServer()
	a := app.New(
		app.WithServer(srv),
		app.WithDrainDelay(150*time.Millisecond),
		app.WithShutdownTimeout(2*time.Second),
	)

	done := make(chan error, 1)
	go func() { done <- a.Run() }()
	time.Sleep(50 * time.Millisecond)

	start := time.Now()
	a.Stop()
	require.NoError(t, <-done)
	assert.GreaterOrEqual(t, time.Since(start), 150*time.Millisecond,
		"Run should wait out the drain delay before stopping servers")
}
