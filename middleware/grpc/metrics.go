package grpcMiddleware

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

func UnaryMetrics() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		grpcInFlightInc(info.FullMethod)
		defer grpcInFlightDec(info.FullMethod)

		start := time.Now()
		resp, err := handler(ctx, req)

		grpcRequestsInc(info.FullMethod, statusCode(err))
		grpcDurationObserve(info.FullMethod, statusCode(err), time.Since(start))
		return resp, err
	}
}

func statusCode(err error) string {
	if err == nil {
		return "OK"
	}
	return "ERROR"
}

var (
	grpcInFlightInc     = func(method string) {}
	grpcInFlightDec     = func(method string) {}
	grpcRequestsInc     = func(method, status string) {}
	grpcDurationObserve = func(method, status string, d time.Duration) {}
)

// RegisterGRPCMetricHooks is called by metric.Init() to wire real counters.
func RegisterGRPCMetricHooks(
	inFlightInc, inFlightDec func(method string),
	requestsInc func(method, status string),
	durationObserve func(method, status string, d time.Duration),
) {
	grpcInFlightInc = inFlightInc
	grpcInFlightDec = inFlightDec
	grpcRequestsInc = requestsInc
	grpcDurationObserve = durationObserve
}
