package grpcserver

import "github.com/pemako/mk/core"

type options struct {
	addr       string
	logger     core.Logger
	reflection bool
	env        string
	tlsCert    string
	tlsKey     string
}

type Option func(*options)

func WithAddr(addr string) Option     { return func(o *options) { o.addr = addr } }
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }
func WithReflection(on bool) Option   { return func(o *options) { o.reflection = on } }
func WithTLS(cert, key string) Option {
	return func(o *options) { o.tlsCert = cert; o.tlsKey = key }
}

// WithConfig reads server.grpc.addr, app.env from core.Config.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("server.grpc.addr"); v != "" {
			o.addr = v
		}
		if v := cfg.GetString("app.env"); v != "" {
			o.env = v
		}
	}
}
