package grpcMiddleware

import (
	"context"

	"github.com/pemako/mk/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// httpToGRPCCode maps HTTP status codes from *errors.CodedError to the
// equivalent gRPC status codes.
var httpToGRPCCode = map[int]codes.Code{
	400: codes.InvalidArgument,
	401: codes.Unauthenticated,
	403: codes.PermissionDenied,
	404: codes.NotFound,
	409: codes.AlreadyExists,
	429: codes.ResourceExhausted,
	500: codes.Internal,
}

func toGRPCStatus(err error) *status.Status {
	var ce *errors.CodedError
	if errors.As(err, &ce) {
		code, ok := httpToGRPCCode[ce.Code]
		if !ok {
			code = codes.Internal
		}
		return status.New(code, ce.Message)
	}
	return status.New(codes.Internal, "internal server error")
}

// UnaryErrorHandler is a gRPC unary server interceptor that translates
// *errors.CodedError values returned by handlers into the appropriate gRPC
// status code. Other errors are mapped to codes.Internal. This interceptor
// should be the last in the chain (closest to the handler).
func UnaryErrorHandler(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		return nil, toGRPCStatus(err).Err()
	}
	return resp, nil
}
