package grpcMiddleware

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryRecovery is a gRPC unary server interceptor that catches panics and
// returns a gRPC Internal status error. The panic value is formatted into the
// status message but is not propagated to the caller as a Go panic.
func UnaryRecovery(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = status.Errorf(codes.Internal, "panic: %v", fmt.Sprintf("%v", r))
		}
	}()
	return handler(ctx, req)
}
