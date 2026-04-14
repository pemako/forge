package idgen

import (
	"context"
	"net"
	"sync"

	"github.com/bwmarrin/snowflake"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/logger"
)

// options holds functional configuration for a Generator.
type options struct {
	workerID        int64
	log             core.Logger
	derived         bool // true when workerID was derived from IP (not explicitly set)
	requireExplicit bool // if true, New() returns an error when workerID is derived
}

// Option configures a Generator.
type Option func(*options)

// WithWorkerID sets the snowflake worker (node) ID, which must be in [0, 1023].
// Worker IDs should be unique per process to avoid ID collisions across instances.
// Defaults to the last byte of the primary non-loopback IP address (0–255).
func WithWorkerID(id int64) Option {
	return func(o *options) {
		o.workerID = id
		o.derived = false
	}
}

// WithLogger sets the logger used for lifecycle messages.
// Defaults to logger.Default().
func WithLogger(l core.Logger) Option {
	return func(o *options) { o.log = l }
}

// WithConfig reads idgen settings from cfg:
//
//	idgen.worker_id — snowflake worker ID (int, 0–1023)
//
// An explicit WithWorkerID applied after WithConfig overrides the config value.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetInt("idgen.worker_id"); v != 0 {
			o.workerID = int64(v)
			o.derived = false
		}
	}
}

// WithRequireExplicitWorkerID makes New() return an error if the worker ID was
// derived from the host IP rather than set explicitly via WithWorkerID or
// WithConfig. Use this in production services to prevent silent ID collisions
// caused by multiple instances sharing the same derived ID.
//
// Place this option last so it takes effect after WithConfig:
//
//	gen, err := idgen.New(
//	    idgen.WithConfig(cfg),
//	    idgen.WithRequireExplicitWorkerID(),
//	)
func WithRequireExplicitWorkerID() Option {
	return func(o *options) { o.requireExplicit = true }
}

func defaultOptions() options {
	return options{
		workerID: deriveWorkerID(),
		log:      logger.Default(),
		derived:  true,
	}
}

// deriveWorkerID returns the last byte of the first non-loopback IPv4 address,
// giving a stable default worker ID without explicit configuration.
// Falls back to 0 if no suitable address is found.
func deriveWorkerID() int64 {
	ifaces, err := net.Interfaces()
	if err != nil {
		return 0
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP.To4()
			case *net.IPAddr:
				ip = v.IP.To4()
			}
			if ip != nil && !ip.IsLoopback() {
				return int64(ip[3])
			}
		}
	}
	return 0
}

// Generator produces monotonically increasing 64-bit snowflake IDs.
// All methods are safe for concurrent use.
type Generator struct {
	node *snowflake.Node
}

// New creates a Generator with the given options.
// Returns an error if the worker ID is out of the valid range [0, 1023].
// Returns an error if WithRequireExplicitWorkerID is set and the worker ID
// was derived from the host IP rather than set explicitly.
// Logs a Warn if the worker ID was derived from the host IP (not explicitly set)
// and WithRequireExplicitWorkerID is not set.
func New(opts ...Option) (*Generator, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	if o.derived {
		if o.requireExplicit {
			return nil, errors.Wrap(errors.ErrBadRequest,
				"idgen: worker_id must be set explicitly in production; "+
					"use WithWorkerID or set idgen.worker_id in config to avoid ID collisions across instances")
		}
		o.log.Warn(context.Background(), "idgen: worker_id derived from host IP — set WithWorkerID or idgen.worker_id to avoid collisions in multi-instance deployments")
	}
	node, err := snowflake.NewNode(o.workerID)
	if err != nil {
		return nil, err
	}
	return &Generator{node: node}, nil
}

// Next generates and returns the next unique snowflake ID as int64.
func (g *Generator) Next() int64 {
	return g.node.Generate().Int64()
}

// NextString generates and returns the next unique snowflake ID as a decimal string.
func (g *Generator) NextString() string {
	return g.node.Generate().String()
}

// --- Named registry ---

var (
	registry = map[string]*Generator{}
	mu       sync.RWMutex
)

// ErrAlreadyRegistered is returned by Register when a generator with the given
// name already exists in the global registry.
var ErrAlreadyRegistered = errors.Define(409, "IDGEN_ALREADY_REGISTERED", "generator already registered")

// Register stores g in the global registry under name.
// Returns ErrAlreadyRegistered if a generator with that name already exists.
func Register(name string, g *Generator) error {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := registry[name]; ok {
		return ErrAlreadyRegistered
	}
	registry[name] = g
	return nil
}

// Replace stores g in the global registry under name, overwriting any existing
// entry. Generator has no resources to close, so the old instance is discarded.
func Replace(name string, g *Generator) error {
	mu.Lock()
	registry[name] = g
	mu.Unlock()
	return nil
}

// CloseAll removes all registered Generators and returns nil.
// Generator holds no I/O resources; this only clears the registry.
func CloseAll() error {
	mu.Lock()
	clear(registry)
	mu.Unlock()
	return nil
}

// Get returns the named Generator previously registered by Register or Replace.
// Returns nil if no generator with that name has been registered.
func Get(name string) *Generator {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}
