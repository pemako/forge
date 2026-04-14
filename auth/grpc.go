package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// UnaryAuth returns a gRPC unary interceptor that validates the Bearer token
// from incoming metadata key "authorization".
// Parsed claims are stored in the context with the same key as Required,
// so FromContext works identically in gRPC handlers.
func UnaryAuth(opts ...Option) grpc.UnaryServerInterceptor {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		tokenStr, err := extractGRPCToken(ctx)
		if err != nil {
			return nil, err
		}
		claims, err := parseToken(tokenStr, o)
		if err != nil {
			o.log.Debug(ctx, "auth: invalid grpc token")
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		return handler(context.WithValue(ctx, contextKey{}, claims), req)
	}
}

// OptionalUnaryAuth returns a gRPC unary interceptor that parses the Bearer
// token if present. The request continues without claims if absent or invalid.
func OptionalUnaryAuth(opts ...Option) grpc.UnaryServerInterceptor {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if tokenStr, err := extractGRPCToken(ctx); err == nil {
			if claims, err := parseToken(tokenStr, o); err == nil {
				ctx = context.WithValue(ctx, contextKey{}, claims)
			}
		}
		return handler(ctx, req)
	}
}

func extractGRPCToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "authorization token required")
	}
	return strings.TrimPrefix(values[0], "Bearer "), nil
}
