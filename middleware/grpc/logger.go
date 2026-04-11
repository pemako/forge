package grpcMiddleware

import (
	"context"
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
	"google.golang.org/grpc"
)

func UnaryLogger(log core.Logger) grpc.UnaryServerInterceptor {
	if log == nil {
		log = logger.Default()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		log.Info(ctx, "grpc request",
			core.Field{Key: "method", Value: info.FullMethod},
			core.Field{Key: "latency", Value: time.Since(start).String()},
			core.Field{Key: "error", Value: err},
		)
		return resp, err
	}
}
