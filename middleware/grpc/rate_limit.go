package grpcMiddleware

import (
	"context"

	"github.com/pemako/forge/ratelimit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// GRPCRateLimitConfig configures the token-bucket rate limiter interceptor.
type GRPCRateLimitConfig struct {
	// Rate is the number of requests per second allowed for each key.
	Rate float64
	// Burst is the maximum burst size above the sustained rate.
	Burst int
	// KeyFunc extracts the rate limit key from the RPC context.
	// Defaults to the peer address when nil.
	KeyFunc func(context.Context) string
}

// UnaryRateLimit returns a gRPC unary server interceptor that enforces a per-key
// token-bucket rate limit. A separate limiter is maintained for each unique key
// returned by KeyFunc. RPCs that exceed the limit receive codes.ResourceExhausted.
func UnaryRateLimit(cfg GRPCRateLimitConfig) grpc.UnaryServerInterceptor {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(ctx context.Context) string {
			if p, ok := peer.FromContext(ctx); ok {
				return p.Addr.String()
			}
			return "unknown"
		}
	}
	lim := ratelimit.New(ratelimit.WithRate(cfg.Rate), ratelimit.WithBurst(cfg.Burst))
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !lim.Allow(ctx, cfg.KeyFunc(ctx)) {
			return nil, status.Error(codes.ResourceExhausted, "too many requests")
		}
		return handler(ctx, req)
	}
}
