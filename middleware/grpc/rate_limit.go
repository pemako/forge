package grpcMiddleware

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type GRPCRateLimitConfig struct {
	Rate    float64
	Burst   int
	KeyFunc func(context.Context) string
}

func UnaryRateLimit(cfg GRPCRateLimitConfig) grpc.UnaryServerInterceptor {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(ctx context.Context) string {
			if p, ok := peer.FromContext(ctx); ok {
				return p.Addr.String()
			}
			return "unknown"
		}
	}
	var mu sync.Map
	getLimiter := func(key string) *rate.Limiter {
		v, _ := mu.LoadOrStore(key, rate.NewLimiter(rate.Limit(cfg.Rate), cfg.Burst))
		return v.(*rate.Limiter)
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !getLimiter(cfg.KeyFunc(ctx)).Allow() {
			return nil, status.Error(codes.ResourceExhausted, "too many requests")
		}
		return handler(ctx, req)
	}
}
