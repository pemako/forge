package idgen

import (
	"net"
	"sync"

	"github.com/bwmarrin/snowflake"
	"github.com/pemako/mk/core"
	"github.com/pemako/mk/errors"
	"github.com/pemako/mk/logger"
)

// options holds functional configuration for a Generator.
type options struct {
	workerID int64
	log      core.Logger
}

// Option configures a Generator.
type Option func(*options)

// WithWorkerID sets the snowflake worker (node) ID, which must be in [0, 1023].
// Worker IDs should be unique per process to avoid ID collisions across instances.
// Defaults to the last byte of the primary non-loopback IP address (0–255).
func WithWorkerID(id int64) Option {
	return func(o *options) { o.workerID = id }
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
		}
	}
}

func defaultOptions() options {
	return options{
		workerID: deriveWorkerID(),
		log:      logger.Default(),
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
func New(opts ...Option) (*Generator, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
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

// --- Package-level default and named registry ---

var (
	_default  *Generator
	_initOnce sync.Once
	registry  = map[string]*Generator{}
	mu        sync.RWMutex
)

// ErrAlreadyRegistered is returned by Init when a generator with the given
// name already exists in the global registry.
var ErrAlreadyRegistered = errors.Define(409, "IDGEN_ALREADY_REGISTERED", "generator already registered")

func getDefault() *Generator {
	_initOnce.Do(func() {
		g, err := New()
		if err != nil {
			// Should not happen with derived worker ID; fall back to 0.
			g, _ = New(WithWorkerID(0))
		}
		_default = g
	})
	return _default
}

// Next generates a snowflake ID using the default package-level Generator.
// The default generator derives its worker ID from the host's primary IP
// address. Call Init to set an explicit worker ID before the first call to Next.
func Next() int64 { return getDefault().Next() }

// NextString generates a snowflake ID as a decimal string using the default generator.
func NextString() string { return getDefault().NextString() }

// Init creates a named Generator and stores it in the global registry.
// Call Init once at startup for each logical ID namespace that requires a
// distinct worker ID. Use Get to retrieve the generator later.
// Returns ErrAlreadyRegistered if a generator with that name already exists.
func Init(name string, opts ...Option) error {
	g, err := New(opts...)
	if err != nil {
		return err
	}
	mu.Lock()
	if _, ok := registry[name]; ok {
		mu.Unlock()
		return ErrAlreadyRegistered
	}
	registry[name] = g
	mu.Unlock()
	return nil
}

// Replace creates a named Generator and stores it in the global registry,
// overwriting any existing entry without error. Generator has no resources
// to close, so the old instance is simply replaced.
func Replace(name string, opts ...Option) error {
	g, err := New(opts...)
	if err != nil {
		return err
	}
	mu.Lock()
	registry[name] = g
	mu.Unlock()
	return nil
}

// Get returns the named Generator previously registered by Init.
// Returns nil if no generator with that name has been registered.
func Get(name string) *Generator {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}
