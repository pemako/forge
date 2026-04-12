package grpcMiddleware

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataRequestID is the gRPC metadata key used to propagate request identifiers.
const MetadataRequestID = "x-request-id"

// UnaryRequestID is a gRPC unary server interceptor that ensures every RPC has
// a unique request identifier. If the incoming metadata already contains an
// x-request-id key, that value is reused; otherwise a new UUID is generated.
// The identifier is stored in the incoming context metadata for downstream use.
func UnaryRequestID(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	ids := md.Get(MetadataRequestID)
	id := ""
	if len(ids) > 0 {
		id = ids[0]
	}
	if id == "" {
		id = uuid.NewString()
	}
	ctx = metadata.NewIncomingContext(ctx, metadata.Join(md, metadata.Pairs(MetadataRequestID, id)))
	return handler(ctx, req)
}
