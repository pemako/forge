package grpcMiddleware

import (
	"context"
	"fmt"

	"github.com/pemako/forge/errors"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInfoDomain is the google.rpc.ErrorInfo "domain" attached to every gRPC
// status translated from an *errors.CodedError. Clients should match on the
// (domain, reason) pair so that reason tokens produced by forge services cannot
// be confused with identically named tokens from another producer.
const ErrorInfoDomain = "forge"

// httpToGRPCCode maps the HTTP status code carried by *errors.CodedError to the
// equivalent gRPC status code.
//
// The table is the inverse of the canonical google.rpc.Code to HTTP mapping
// documented in google/rpc/code.proto and implemented by grpc-gateway's
// runtime.HTTPStatusFromCode. Where the canonical mapping is many-to-one
// (HTTP 400 is produced by InvalidArgument, FailedPrecondition and OutOfRange;
// HTTP 409 by AlreadyExists and Aborted; HTTP 500 by Unknown, Internal and
// DataLoss) the most widely used gRPC code is chosen for the inverse.
//
// HTTP codes absent from this table fall back to codes.Internal.
//
// Two entries matter for codes forge produces itself: 413, from
// middleware/http.BodyLimit, matches grpc-go's own use of ResourceExhausted for
// over-sized messages; 503, from middleware/http.Timeout, is Unavailable rather
// than Internal so callers keep the retryability signal.
var httpToGRPCCode = map[int]codes.Code{
	400: codes.InvalidArgument,    // canonical
	401: codes.Unauthenticated,    // canonical
	403: codes.PermissionDenied,   // canonical
	404: codes.NotFound,           // canonical
	408: codes.DeadlineExceeded,   // Request Timeout: the request ran out of time
	409: codes.AlreadyExists,      // canonical (Aborted also maps to 409)
	412: codes.FailedPrecondition, // Precondition Failed
	413: codes.ResourceExhausted,  // Payload Too Large; see note above
	416: codes.OutOfRange,         // Range Not Satisfiable: canonical
	422: codes.InvalidArgument,    // Unprocessable Entity: semantically invalid input
	429: codes.ResourceExhausted,  // canonical
	499: codes.Canceled,           // Client Closed Request: canonical
	500: codes.Internal,           // canonical
	501: codes.Unimplemented,      // canonical
	502: codes.Unavailable,        // Bad Gateway: upstream unreachable, retryable
	503: codes.Unavailable,        // canonical; see note above
	504: codes.DeadlineExceeded,   // canonical
}

// codedErrorStatus translates ce into a gRPC status, preserving ce.Reason and
// ce.Metadata as a google.rpc.ErrorInfo detail so that clients can dispatch on
// the stable reason token instead of parsing the human-readable message.
//
// Metadata values are rendered with fmt.Sprint because ErrorInfo.Metadata is
// map[string]string; non-scalar values are therefore flattened to their default
// Go formatting. ce's cause chain is deliberately not included in the status
// message: only ce.Message is sent, matching middleware/http.ErrorHandler.
func codedErrorStatus(ce *errors.CodedError) *status.Status {
	code, ok := httpToGRPCCode[ce.Code]
	if !ok {
		code = codes.Internal
	}
	s := status.New(code, ce.Message)

	if ce.Reason == "" && len(ce.Metadata) == 0 {
		return s
	}

	info := &errdetails.ErrorInfo{
		Reason: ce.Reason,
		Domain: ErrorInfoDomain,
	}
	if len(ce.Metadata) > 0 {
		info.Metadata = make(map[string]string, len(ce.Metadata))
		for k, v := range ce.Metadata {
			info.Metadata[k] = fmt.Sprint(v)
		}
	}

	withDetails, err := s.WithDetails(info)
	if err != nil {
		// Marshalling the detail must never cost the caller the translated
		// code and message, so fall back to the bare status.
		return s
	}
	return withDetails
}

// toGRPCStatus converts err into a gRPC status. It returns nil for a nil error;
// calling Err on that nil status yields a nil error.
func toGRPCStatus(err error) *status.Status {
	if err == nil {
		return nil
	}

	// Already carries a gRPC status — pass it through unchanged so handler-set
	// codes, messages and details survive. status.FromError reports ok only for
	// a nil error (excluded above) or an error whose chain implements
	// GRPCStatus() *Status. *errors.CodedError does not implement that method,
	// so it is never swallowed here and always reaches the branch below.
	if s, ok := status.FromError(err); ok && s != nil {
		return s
	}

	var ce *errors.CodedError
	if errors.As(err, &ce) {
		return codedErrorStatus(ce)
	}

	// Context errors are neither gRPC statuses nor CodedErrors. Mapping them
	// explicitly keeps the retryability signal that codes.Internal would lose.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return status.FromContextError(err)
	}

	// Unknown error: report a generic internal failure without leaking the
	// message, mirroring middleware/http.ErrorHandler's errors.ErrInternal body.
	return codedErrorStatus(errors.ErrInternal)
}

// UnaryErrorHandler is a gRPC unary server interceptor that translates errors
// returned by handlers into gRPC statuses:
//
//   - errors already carrying a gRPC status pass through unchanged;
//   - *errors.CodedError is mapped through the canonical HTTP-to-gRPC code
//     table, with Reason and Metadata attached as a google.rpc.ErrorInfo detail
//     under the ErrorInfoDomain domain;
//   - context.DeadlineExceeded and context.Canceled become
//     codes.DeadlineExceeded and codes.Canceled;
//   - anything else becomes codes.Internal with a generic message.
//
// Clients recover the structured detail with status.Convert(err).Details() and
// a type switch on *errdetails.ErrorInfo. This interceptor should be the last
// in the chain (closest to the handler).
func UnaryErrorHandler(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)
	if err != nil {
		return nil, toGRPCStatus(err).Err()
	}
	return resp, nil
}
