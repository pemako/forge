// Package grpcclient/interceptors.go
// Unary client interceptors used by Dial: logging and retry.
// These are package-internal; callers configure behaviour through Options.
package grpcclient

import (
	"context"
	"math/rand/v2"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pemako/mk/core"
)

// shouldRetryCode reports whether the gRPC status code warrants a retry.
func shouldRetryCode(code codes.Code) bool {
	switch code {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	}
	return false
}

// grpcBackoffDelay returns the wait before attempt n (1-based) with exponential
// backoff and ±25% jitter, capped at maxDelay.
func grpcBackoffDelay(attempt int, base, maxDelay time.Duration) time.Duration {
	shift := min(attempt-1, 10)
	d := min(base*(1<<shift), maxDelay)
	jitter := time.Duration(rand.Int64N(int64(d/2+1))) - d/4
	result := d + jitter
	if result < 0 {
		return 0
	}
	if result > maxDelay {
		return maxDelay
	}
	return result
}

// unaryLoggerInterceptor logs the method, latency, and any error for each RPC.
func unaryLoggerInterceptor(log core.Logger) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		log.Info(ctx, "grpc client call",
			core.Field{Key: "method", Value: method},
			core.Field{Key: "latency", Value: time.Since(start).String()},
			core.Field{Key: "error", Value: err},
		)
		return err
	}
}

// unaryRetryInterceptor retries on transient errors using exponential backoff.
func unaryRetryInterceptor(maxRetries int, base, maxDelay time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		for attempt := range maxRetries + 1 {
			if attempt > 0 {
				delay := grpcBackoffDelay(attempt, base, maxDelay)
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}
			st, ok := status.FromError(err)
			if !ok || !shouldRetryCode(st.Code()) {
				return err
			}
			lastErr = err
		}
		return lastErr
	}
}
