package grpcMiddleware

import (
	"context"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryMetrics returns a gRPC unary server interceptor that records
// grpc_requests_total, grpc_request_duration_seconds, grpc_requests_in_flight,
// and grpc_errors_total for each RPC. The hooks are no-ops until metric.Init
// wires real instruments by calling RegisterGRPCMetricHooks.
func UnaryMetrics() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Load the hook set once per RPC so a concurrent rewire cannot observe a
		// half-updated set.
		h := grpcHooks()
		h.inFlightInc(info.FullMethod)
		defer h.inFlightDec(info.FullMethod)

		start := time.Now()
		resp, err := handler(ctx, req)

		code := statusCode(err)
		h.requestsInc(info.FullMethod, code)
		h.durationObserve(info.FullMethod, code, time.Since(start))
		if err != nil {
			h.errorsInc(info.FullMethod, code)
		}
		return resp, err
	}
}

// statusCode returns the canonical gRPC code name for err, e.g. "OK",
// "NotFound", "Unavailable".
//
// It previously collapsed every failure to the single label "ERROR", which made
// it impossible to tell a NotFound from an Internal on a dashboard even though
// the code was right there on the error.
func statusCode(err error) string {
	return status.Code(err).String()
}

// grpcMetricHooks is the full set of metric callbacks used by the gRPC
// middleware, swapped as one immutable value so the RPC path never reads a
// partially-wired set.
type grpcMetricHooks struct {
	inFlightInc     func(method string)
	inFlightDec     func(method string)
	requestsInc     func(method, status string)
	durationObserve func(method, status string, d time.Duration)
	errorsInc       func(method, status string)
}

var noopGRPCHooks = &grpcMetricHooks{
	inFlightInc:     func(string) {},
	inFlightDec:     func(string) {},
	requestsInc:     func(string, string) {},
	durationObserve: func(string, string, time.Duration) {},
	errorsInc:       func(string, string) {},
}

var grpcHooksHolder atomic.Pointer[grpcMetricHooks]

func grpcHooks() *grpcMetricHooks {
	if h := grpcHooksHolder.Load(); h != nil {
		return h
	}
	return noopGRPCHooks
}

// RegisterGRPCMetricHooks is called by metric.Init to wire real instruments.
// Any nil callback is replaced with a no-op. Calling it more than once replaces
// the previous set; the most recent call wins. It is safe to call while RPCs are
// being served.
func RegisterGRPCMetricHooks(
	inFlightInc, inFlightDec func(method string),
	requestsInc func(method, status string),
	durationObserve func(method, status string, d time.Duration),
	errorsInc func(method, status string),
) {
	h := &grpcMetricHooks{
		inFlightInc:     inFlightInc,
		inFlightDec:     inFlightDec,
		requestsInc:     requestsInc,
		durationObserve: durationObserve,
		errorsInc:       errorsInc,
	}
	if h.inFlightInc == nil {
		h.inFlightInc = noopGRPCHooks.inFlightInc
	}
	if h.inFlightDec == nil {
		h.inFlightDec = noopGRPCHooks.inFlightDec
	}
	if h.requestsInc == nil {
		h.requestsInc = noopGRPCHooks.requestsInc
	}
	if h.durationObserve == nil {
		h.durationObserve = noopGRPCHooks.durationObserve
	}
	if h.errorsInc == nil {
		h.errorsInc = noopGRPCHooks.errorsInc
	}
	grpcHooksHolder.Store(h)
}

// ResetGRPCMetricHooks restores the no-op hook set, so a test can unwire
// instruments installed by metric.Init.
func ResetGRPCMetricHooks() { grpcHooksHolder.Store(noopGRPCHooks) }
