package grpcMiddleware

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pemako/forge/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	grpcHealth "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// errorInfoOf returns the single *errdetails.ErrorInfo attached to s, failing
// the test when it is absent.
func errorInfoOf(t *testing.T, s *status.Status) *errdetails.ErrorInfo {
	t.Helper()
	for _, d := range s.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info
		}
	}
	t.Fatalf("status %v carries no ErrorInfo detail (details=%v)", s, s.Details())
	return nil
}

// ── code mapping ─────────────────────────────────────────────────────────────

func TestToGRPCStatus_codeMapping(t *testing.T) {
	tests := []struct {
		name     string
		httpCode int
		want     codes.Code
	}{
		{"bad request", 400, codes.InvalidArgument},
		{"unauthorized", 401, codes.Unauthenticated},
		{"forbidden", 403, codes.PermissionDenied},
		{"not found", 404, codes.NotFound},
		{"request timeout", 408, codes.DeadlineExceeded},
		{"conflict", 409, codes.AlreadyExists},
		{"precondition failed", 412, codes.FailedPrecondition},
		{"payload too large", 413, codes.ResourceExhausted},
		{"range not satisfiable", 416, codes.OutOfRange},
		{"unprocessable entity", 422, codes.InvalidArgument},
		{"too many requests", 429, codes.ResourceExhausted},
		{"client closed request", 499, codes.Canceled},
		{"internal", 500, codes.Internal},
		{"not implemented", 501, codes.Unimplemented},
		{"bad gateway", 502, codes.Unavailable},
		{"service unavailable", 503, codes.Unavailable},
		{"gateway timeout", 504, codes.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := errors.Define(tt.httpCode, "SOME_REASON", "some message")
			s := toGRPCStatus(ce)
			require.NotNil(t, s)
			assert.Equal(t, tt.want, s.Code())
			assert.Equal(t, "some message", s.Message())
		})
	}
}

func TestToGRPCStatus_everyTableEntryIsReachable(t *testing.T) {
	// Guards against a table entry that the mapping code cannot reach.
	for httpCode, want := range httpToGRPCCode {
		ce := errors.Define(httpCode, "R", "m")
		assert.Equal(t, want, toGRPCStatus(ce).Code(), "http %d", httpCode)
	}
}

func TestToGRPCStatus_unmappedCodeFallsBackToInternal(t *testing.T) {
	tests := []struct {
		name     string
		httpCode int
	}{
		{"teapot", 418},
		{"locked", 423},
		{"zero", 0},
		{"nonsense", 9999},
		{"negative", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := toGRPCStatus(errors.Define(tt.httpCode, "WEIRD", "weird"))
			require.NotNil(t, s)
			assert.Equal(t, codes.Internal, s.Code())
			// The message is still the author's: only the code is defaulted.
			assert.Equal(t, "weird", s.Message())
		})
	}
}

// ── forge sentinels that previously collapsed to Internal ─────────────────────

func TestToGRPCStatus_forgeOwnSentinels(t *testing.T) {
	tests := []struct {
		name string
		err  *errors.CodedError
		want codes.Code
	}{
		// Mirrors middleware/http/timeout.go.
		{"http timeout 503", errors.Define(503, "TIMEOUT", "request timeout"), codes.Unavailable},
		// Mirrors middleware/http/body_limit.go.
		{"body limit 413", errors.Define(413, "PAYLOAD_TOO_LARGE", "request body too large"), codes.ResourceExhausted},
		{"ErrNotFound", errors.ErrNotFound, codes.NotFound},
		{"ErrTooMany", errors.ErrTooMany, codes.ResourceExhausted},
		{"ErrInternal", errors.ErrInternal, codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toGRPCStatus(tt.err).Code())
		})
	}
}

// ── context errors ───────────────────────────────────────────────────────────

func TestToGRPCStatus_contextErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"deadline exceeded", context.DeadlineExceeded, codes.DeadlineExceeded},
		{"canceled", context.Canceled, codes.Canceled},
		{"wrapped deadline exceeded", fmt.Errorf("query: %w", context.DeadlineExceeded), codes.DeadlineExceeded},
		{"wrapped canceled", fmt.Errorf("query: %w", context.Canceled), codes.Canceled},
		{"doubly wrapped canceled", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", context.Canceled)), codes.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := toGRPCStatus(tt.err)
			require.NotNil(t, s)
			assert.Equal(t, tt.want, s.Code())
		})
	}
}

func TestToGRPCStatus_realExpiredContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	<-ctx.Done()
	assert.Equal(t, codes.DeadlineExceeded, toGRPCStatus(ctx.Err()).Code())
}

func TestToGRPCStatus_codedErrorWinsOverWrappedContextError(t *testing.T) {
	// An author who classified the failure explicitly should not have that
	// classification overridden by an incidental context error in the chain.
	err := errors.Wrap(errors.ErrNotFound, "lookup: %w", context.DeadlineExceeded)
	s := toGRPCStatus(err)
	assert.Equal(t, codes.NotFound, s.Code())
	assert.Equal(t, "NOT_FOUND", errorInfoOf(t, s).GetReason())
}

// ── pass-through and fallbacks ───────────────────────────────────────────────

func TestToGRPCStatus_nilError(t *testing.T) {
	s := toGRPCStatus(nil)
	assert.Nil(t, s)
	// A nil *Status reports codes.OK, so Err() must yield a nil error. This is
	// what keeps UnaryErrorHandler from inventing a failure out of nothing.
	assert.NoError(t, s.Err())
}

func TestToGRPCStatus_existingStatusPassesThroughUnchanged(t *testing.T) {
	orig := status.New(codes.PermissionDenied, "nope")
	s := toGRPCStatus(orig.Err())
	require.NotNil(t, s)
	assert.Equal(t, codes.PermissionDenied, s.Code())
	assert.Equal(t, "nope", s.Message())
	assert.Empty(t, s.Details(), "no ErrorInfo should be synthesised for a pass-through")
}

func TestToGRPCStatus_existingStatusKeepsItsOwnDetails(t *testing.T) {
	orig, err := status.New(codes.Aborted, "txn aborted").
		WithDetails(&errdetails.ErrorInfo{Reason: "HANDLER_SET", Domain: "example.com"})
	require.NoError(t, err)

	s := toGRPCStatus(orig.Err())
	assert.Equal(t, codes.Aborted, s.Code())
	info := errorInfoOf(t, s)
	assert.Equal(t, "HANDLER_SET", info.GetReason())
	assert.Equal(t, "example.com", info.GetDomain())
}

func TestToGRPCStatus_plainErrorBecomesInternal(t *testing.T) {
	s := toGRPCStatus(fmt.Errorf("connection string is postgres://user:hunter2@db"))
	require.NotNil(t, s)
	assert.Equal(t, codes.Internal, s.Code())
	assert.Equal(t, errors.ErrInternal.Message, s.Message())
	assert.NotContains(t, s.Message(), "hunter2", "raw error text must not leak to the caller")
	// Kept consistent with middleware/http.ErrorHandler, which renders
	// errors.ErrInternal (reason INTERNAL) for the same situation.
	assert.Equal(t, "INTERNAL", errorInfoOf(t, s).GetReason())
}

// ── CodedError is not swallowed by status.FromError ──────────────────────────

func TestCodedError_hasNoGRPCStatusMethod(t *testing.T) {
	// The pass-through branch keys off status.FromError's ok flag. Document and
	// lock in why a plain *CodedError cannot take that branch.
	var err error = errors.ErrNotFound
	_, isGRPCStatus := err.(interface{ GRPCStatus() *status.Status })
	assert.False(t, isGRPCStatus, "CodedError must not implement GRPCStatus()")

	_, ok := status.FromError(err)
	assert.False(t, ok, "status.FromError must report ok=false for a CodedError")

	// And therefore it is translated, not passed through as codes.Unknown.
	assert.Equal(t, codes.NotFound, toGRPCStatus(err).Code())
}

func TestStatusFromError_nilReturnsOkTrue(t *testing.T) {
	// Behaviour of google.golang.org/grpc v1.84.0 that the nil guard in
	// toGRPCStatus exists for: ok is true and the status is nil.
	s, ok := status.FromError(nil)
	assert.True(t, ok)
	assert.Nil(t, s)
}

// ── ErrorInfo detail contents ────────────────────────────────────────────────

func TestToGRPCStatus_attachesReasonAndMetadata(t *testing.T) {
	ce := errors.Define(404, "USER_NOT_FOUND", "user not found")
	ce.Metadata = map[string]any{
		"userId":  "u-42",
		"attempt": 3,
		"cached":  true,
		"ratio":   1.5,
	}

	s := toGRPCStatus(ce)
	require.Equal(t, codes.NotFound, s.Code())

	info := errorInfoOf(t, s)
	assert.Equal(t, "USER_NOT_FOUND", info.GetReason())
	assert.Equal(t, ErrorInfoDomain, info.GetDomain())
	assert.Equal(t, map[string]string{
		"userId":  "u-42",
		"attempt": "3",
		"cached":  "true",
		"ratio":   "1.5",
	}, info.GetMetadata())
}

func TestToGRPCStatus_noMetadataStillCarriesReason(t *testing.T) {
	s := toGRPCStatus(errors.Define(409, "CONFLICT", "resource conflict"))
	info := errorInfoOf(t, s)
	assert.Equal(t, "CONFLICT", info.GetReason())
	assert.Empty(t, info.GetMetadata())
}

func TestToGRPCStatus_noDetailWhenNothingToReport(t *testing.T) {
	// An empty reason and no metadata means there is nothing programmatic to
	// convey, so no empty ErrorInfo is appended.
	s := toGRPCStatus(errors.Define(400, "", "just a message"))
	assert.Equal(t, codes.InvalidArgument, s.Code())
	assert.Empty(t, s.Details())
}

func TestToGRPCStatus_wrappedCodedErrorKeepsReasonAndHidesCause(t *testing.T) {
	err := errors.Wrap(errors.ErrNotFound, "query user id=%s: %w", "u-7", fmt.Errorf("sql: no rows"))
	s := toGRPCStatus(err)

	assert.Equal(t, codes.NotFound, s.Code())
	// Only Message crosses the wire, matching middleware/http.ErrorHandler.
	assert.Equal(t, errors.ErrNotFound.Message, s.Message())
	assert.NotContains(t, s.Message(), "sql: no rows")
	assert.Equal(t, "NOT_FOUND", errorInfoOf(t, s).GetReason())
}

// ── round trip through the interceptor over a real connection ────────────────

// startErrorHandlerServer runs an in-process gRPC server on a random port with
// UnaryErrorHandler followed by an interceptor that always fails with err.
func startErrorHandlerServer(t *testing.T, err error) grpcHealth.HealthClient {
	t.Helper()

	lis, lerr := net.Listen("tcp", ":0")
	require.NoError(t, lerr)

	failing := func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
		return nil, err
	}
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(UnaryErrorHandler, failing))
	hs := health.NewServer()
	hs.SetServingStatus("", grpcHealth.HealthCheckResponse_SERVING)
	grpcHealth.RegisterHealthServer(srv, hs)
	go srv.Serve(lis) //nolint:errcheck

	conn, cerr := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, cerr)

	t.Cleanup(func() {
		conn.Close() //nolint:errcheck
		srv.GracefulStop()
	})
	return grpcHealth.NewHealthClient(conn)
}

func TestUnaryErrorHandler_clientRecoversReasonAndMetadata(t *testing.T) {
	serverErr := errors.Define(404, "USER_NOT_FOUND", "user not found")
	serverErr.Metadata = map[string]any{"userId": "u-42", "attempt": 2}

	client := startErrorHandlerServer(t, serverErr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Check(ctx, &grpcHealth.HealthCheckRequest{})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Equal(t, "user not found", st.Message())

	// The whole point: the client can dispatch on the stable reason token and
	// read the structured metadata, neither of which survived before.
	info := errorInfoOf(t, st)
	assert.Equal(t, "USER_NOT_FOUND", info.GetReason())
	assert.Equal(t, ErrorInfoDomain, info.GetDomain())
	assert.Equal(t, map[string]string{"userId": "u-42", "attempt": "2"}, info.GetMetadata())
}

func TestUnaryErrorHandler_clientSeesMappedCodeForTimeoutSentinel(t *testing.T) {
	client := startErrorHandlerServer(t, errors.Define(503, "TIMEOUT", "request timeout"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Check(ctx, &grpcHealth.HealthCheckRequest{})
	require.Error(t, err)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Equal(t, "TIMEOUT", errorInfoOf(t, st).GetReason())
}

func TestUnaryErrorHandler_clientSeesCanceledForContextError(t *testing.T) {
	client := startErrorHandlerServer(t, fmt.Errorf("upstream: %w", context.Canceled))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Check(ctx, &grpcHealth.HealthCheckRequest{})
	require.Error(t, err)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.Canceled, st.Code())
}

func TestUnaryErrorHandler_successPassesThrough(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(UnaryErrorHandler))
	hs := health.NewServer()
	hs.SetServingStatus("", grpcHealth.HealthCheckResponse_SERVING)
	grpcHealth.RegisterHealthServer(srv, hs)
	go srv.Serve(lis) //nolint:errcheck

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close() //nolint:errcheck
		srv.GracefulStop()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := grpcHealth.NewHealthClient(conn).Check(ctx, &grpcHealth.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpcHealth.HealthCheckResponse_SERVING, resp.GetStatus())
}
