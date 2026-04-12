// Package grpcclient/client.go
// Dial, named registry, and Init for the grpcclient package.
package grpcclient

import (
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

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

// Register dials target and stores the resulting connection in the global
// registry under name, making it retrievable with Get. Returns any dial error.
func Register(name, target string, opts ...Option) error {
	conn, err := Dial(target, opts...)
	if err != nil {
		return err
	}
	registryMu.Lock()
	registry[name] = conn
	registryMu.Unlock()
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
// in-flight RPCs before the process exits.
func CloseAll() {
	registryMu.Lock()
	defer registryMu.Unlock()
	for name, conn := range registry {
		conn.Close()
		delete(registry, name)
	}
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
		if err := Register(name, addr, opts...); err != nil {
			return err
		}
	}
	return nil
}
