package app_test

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/pemako/mk/app"
	"github.com/stretchr/testify/assert"
)

type mockServer struct{ started, stopped bool }

func (m *mockServer) Start(_ context.Context) error { m.started = true; return nil }
func (m *mockServer) Stop(_ context.Context) error  { m.stopped = true; return nil }

type mockMigrator struct{ called bool }

func (m *mockMigrator) Up(_ context.Context) error { m.called = true; return nil }

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
