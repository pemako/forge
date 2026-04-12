package grpcMiddleware

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryTimeout returns a gRPC unary server interceptor that enforces a maximum
// duration for each RPC. If the handler does not return within d, the context
// is cancelled and codes.DeadlineExceeded is returned to the caller.
func UnaryTimeout(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		resp, err := handler(ctx, req)
		if ctx.Err() == context.DeadlineExceeded && err == nil {
			return nil, status.Error(codes.DeadlineExceeded, "request timeout")
		}
		return resp, err
	}
}
