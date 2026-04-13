package app_test

import (
	"context"
	"fmt"
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
