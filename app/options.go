package app

import (
	"context"
	"time"

	"github.com/pemako/mk/core"
)

// Migrator is the interface app accepts for running database migrations.
// *database.Manager implements this. Defined here to avoid import cycles.
type Migrator interface {
	Up(ctx context.Context) error
}

type options struct {
	name            string
	version         string
	servers         []core.Server
	logger          core.Logger
	migrator        Migrator
	shutdownTimeout time.Duration
	beforeStart     []func(context.Context) error
	afterStop       []func(context.Context) error
}

type Option func(*options)

func WithName(name string) Option     { return func(o *options) { o.name = name } }
func WithVersion(v string) Option     { return func(o *options) { o.version = v } }
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

func WithServer(servers ...core.Server) Option {
	return func(o *options) { o.servers = append(o.servers, servers...) }
}
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) { o.shutdownTimeout = d }
}
func WithMigration(m Migrator) Option {
	return func(o *options) { o.migrator = m }
}
func WithBeforeStart(fn func(context.Context) error) Option {
	return func(o *options) { o.beforeStart = append(o.beforeStart, fn) }
}
func WithAfterStop(fn func(context.Context) error) Option {
	return func(o *options) { o.afterStop = append(o.afterStop, fn) }
}
