package grpcMiddleware

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/logger"
)

// UnaryLogger returns a gRPC unary server interceptor that logs each completed
// RPC. The log entry includes the full method name, latency, and the gRPC status
// code. If log is nil, the process-wide default logger is used.
//
// The level reflects the outcome: INFO for OK, WARN for codes that indicate a
// caller problem (NotFound, InvalidArgument, ...), ERROR for server-side
// failures. The entry also carries "error" and "error_reason" so the cause chain
// attached by errors.Wrap — which is deliberately not sent to the client — is
// still recoverable.
func UnaryLogger(log core.Logger) grpc.UnaryServerInterceptor {
	if log == nil {
		log = logger.Default()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)

		code := status.Code(err)
		fields := []core.Field{
			{Key: "method", Value: info.FullMethod},
			{Key: "latency", Value: time.Since(start).String()},
			{Key: "code", Value: code.String()},
		}
		if err != nil {
			// Previously this logged the raw error value under "error" even when
			// nil, and never surfaced Reason.
			fields = append(fields, core.Field{Key: "error", Value: err.Error()})
			if ce, ok := errors.AsType[*errors.CodedError](err); ok {
				fields = append(fields, core.Field{Key: "error_reason", Value: ce.Reason})
			}
		}

		switch {
		case err == nil:
			log.Info(ctx, "grpc request", fields...)
		case isCallerFault(code):
			log.Warn(ctx, "grpc request", fields...)
		default:
			log.Error(ctx, "grpc request", fields...)
		}
		return resp, err
	}
}

// isCallerFault reports whether code describes a problem with the request
// rather than a server-side failure, so it can be logged at WARN instead of
// ERROR.
func isCallerFault(code codes.Code) bool {
	switch code {
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition,
		codes.OutOfRange, codes.Canceled:
		return true
	default:
		return false
	}
}
