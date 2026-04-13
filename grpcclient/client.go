// Package grpcclient provides outbound gRPC connections with OTel tracing and
// per-call retry.
//
// # Instance-first usage (recommended)
//
//	conn, err := grpcclient.Dial("users:9090", grpcclient.WithRetry(3))
//	svc := pb.NewUsersClient(conn)
//
// # Registry usage (convenience for small services)
//
//	grpcclient.Register("users", conn)
//	conn := grpcclient.Get("users")
package grpcclient

import (
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pemako/mk/errors"
)

// ErrAlreadyRegistered is returned by Register when a connection with the given
// name already exists in the registry. Use Replace to overwrite an existing entry.
var ErrAlreadyRegistered = errors.Define(409, "GRPCCLIENT_ALREADY_REGISTERED", "connection already registered")

// Dial creates a new gRPC client connection to target using grpc.NewClient.
// The connection is lazy — no network activity occurs until the first RPC.
// An otelgrpc stats handler is installed for automatic trace propagation.
// Unary interceptors for logging and retry are chained in that order.
// TLS credentials are selected based on the supplied options:
//   - WithInsecure: plaintext (no TLS)
//   - WithTLS: one-way TLS verified against caCert
//   - WithMTLS: mutual TLS with client certificate
//
// The returned *grpc.ClientConn must be closed by the caller (or CloseAll).
func Dial(target string, opts ...Option) (*grpc.ClientConn, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}

	var dialOpts []grpc.DialOption

	// Tracing via stats handler (otelgrpc v0.68+ no longer exposes interceptors).
	dialOpts = append(dialOpts, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))

	// Client unary interceptor chain.
	dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(
		unaryLoggerInterceptor(o.log),
		unaryRetryInterceptor(o.retryMax, o.retryBaseDelay, o.retryMaxDelay),
	))

	// Credentials.
	switch {
	case o.insecure:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case o.tlsCert != "" && o.tlsKey != "" && o.tlsCACert != "":
		// mTLS
		creds, err := credentials.NewClientTLSFromFile(o.tlsCACert, "")
		if err != nil {
			return nil, err
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	case o.tlsCACert != "":
		// one-way TLS
		creds, err := credentials.NewClientTLSFromFile(o.tlsCACert, "")
		if err != nil {
			return nil, err
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}

	return grpc.NewClient(target, dialOpts...)
}

// --- Named registry ---

var (
	registry   = map[string]*grpc.ClientConn{}
	registryMu sync.RWMutex
)

// Register stores conn in the global registry under name.
// Returns ErrAlreadyRegistered if name is already in use. Use Replace to overwrite.
// Dial target and pass the resulting conn:
//
//	conn, err := grpcclient.Dial("users:9090", grpcclient.WithRetry(3))
//	grpcclient.Register("users", conn)
func Register(name string, conn *grpc.ClientConn) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		conn.Close() //nolint:errcheck — close the unused connection
		return ErrAlreadyRegistered
	}
	registry[name] = conn
	return nil
}

// Replace stores conn under name. If a connection previously existed under
// that name, it is closed before the new one is stored.
func Replace(name string, conn *grpc.ClientConn) error {
	registryMu.Lock()
	old := registry[name]
	registry[name] = conn
	registryMu.Unlock()
	if old != nil {
		old.Close() //nolint:errcheck
	}
	return nil
}

// Get returns the named *grpc.ClientConn previously stored by Register, or nil
// if no connection with that name has been registered.
func Get(name string) *grpc.ClientConn {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// CloseAll closes every connection in the global registry and clears it.
// Call this in an application shutdown hook (e.g. app.WithAfterStop) to drain
// in-flight RPCs before the process exits. Returns the first close error
// encountered, if any; all connections are closed regardless.
func CloseAll() error {
	registryMu.Lock()
	defer registryMu.Unlock()
	var first error
	for name, conn := range registry {
		if err := conn.Close(); err != nil && first == nil {
			first = err
		}
		delete(registry, name)
	}
	return first
}

// Registry wraps the package-level gRPC connection registry and implements
// app.Closer. Use NewRegistry() to obtain an instance and pass it to
// app.WithCloser to close all connections on shutdown.
type Registry struct{}

// NewRegistry returns a Registry that can be used with app.WithCloser.
func NewRegistry() *Registry { return &Registry{} }

// Close closes all registered connections. Implements app.Closer.
func (r *Registry) Close() error {
	return CloseAll()
}

// Init reads named gRPC connection configs and registers each one.
// Config keys follow the pattern "grpcclient.<name>.addr", ".insecure",
// ".timeout", and ".retry". Services whose addr key is empty are silently
// skipped. Returns the first dial error encountered.
//
// Example YAML:
//
//	grpcclient:
//	  user-service:
//	    addr: "user-service:9090"
//	    insecure: true
//	    timeout: 5s
//	    retry: 3
func Init(cfg interface {
	GetString(string) string
	GetBool(string) bool
	GetDuration(string) time.Duration
	GetInt(string) int
}, names ...string) error {
	for _, name := range names {
		addr := cfg.GetString("grpcclient." + name + ".addr")
		if addr == "" {
			continue
		}
		var opts []Option
		if cfg.GetBool("grpcclient." + name + ".insecure") {
			opts = append(opts, WithInsecure())
		}
		if v := cfg.GetDuration("grpcclient." + name + ".timeout"); v > 0 {
			opts = append(opts, WithTimeout(v))
		}
		if v := cfg.GetInt("grpcclient." + name + ".retry"); v > 0 {
			opts = append(opts, WithRetry(v))
		}
		if err := func() error {
			conn, err := Dial(addr, opts...)
			if err != nil {
				return err
			}
			return Register(name, conn)
		}(); err != nil {
			return err
		}
	}
	return nil
}
