package grpcMiddleware

import (
	"context"

	"github.com/pemako/mk/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

func UnaryErrorHandler(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		return nil, toGRPCStatus(err).Err()
	}
	return resp, nil
}
