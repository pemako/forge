package grpcMiddleware

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const MetadataRequestID = "x-request-id"

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
