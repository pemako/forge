package httpMiddleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	ferrors "github.com/pemako/forge/errors"
	"github.com/pemako/forge/logger"
	mw "github.com/pemako/forge/middleware/http"
)

// spanStarter returns a middleware that opens a span around the rest of the
// chain, standing in for the real Tracing middleware without touching OTel
// globals.
func spanStarter(tp *sdktrace.TracerProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, span := tp.Tracer("test").Start(c.Request.Context(), "req")
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// TestErrorHandler_RecordsOnSpan pins that a failed request reaches tracing.
// ErrorHandler used to write a JSON body and nothing else, so a 500 was
// invisible in the trace backend.
func TestErrorHandler_RecordsOnSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// The span is started by an outer middleware, mirroring the real stack where
	// Tracing wraps everything and ErrorHandler is innermost. Starting it inside
	// the handler would end the span before ErrorHandler ever saw it.
	r.Use(spanStarter(tp), mw.ErrorHandler())
	r.GET("/boom", func(c *gin.Context) {
		c.Error(ferrors.Wrap(ferrors.ErrInternal, "db unreachable"))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/boom", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	spans := exp.GetSpans()
	require.Len(t, spans, 1)

	require.NotEmpty(t, spans[0].Events, "the error should be recorded as a span event")
	assert.Equal(t, "exception", spans[0].Events[0].Name)

	// 5xx must mark the span failed so it surfaces in error-rate views.
	assert.Equal(t, "Error", spans[0].Status.Code.String())

	var reason, code bool
	for _, a := range spans[0].Attributes {
		switch string(a.Key) {
		case "error.reason":
			reason = a.Value.AsString() == "INTERNAL"
		case "error.code":
			code = a.Value.AsInt64() == 500
		}
	}
	assert.True(t, reason, "error.reason attribute missing")
	assert.True(t, code, "error.code attribute missing")
}

// TestErrorHandler_4xxDoesNotFailSpan keeps client errors out of error-rate
// views — a 404 is not a server fault.
func TestErrorHandler_4xxDoesNotFailSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(spanStarter(tp), mw.ErrorHandler())
	r.GET("/missing", func(c *gin.Context) {
		c.Error(ferrors.Wrap(ferrors.ErrNotFound, "no such user"))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/missing", nil))
	require.Equal(t, http.StatusNotFound, w.Code)

	spans := exp.GetSpans()
	require.Len(t, spans, 1)
	assert.NotEqual(t, "Error", spans[0].Status.Code.String(),
		"a 4xx should not mark the span as failed")
}

// TestErrorHandler_NonCodedErrorStaysGeneric confirms an arbitrary error does
// not leak its message to the caller.
func TestErrorHandler_NonCodedErrorStaysGeneric(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw.ErrorHandler())
	r.GET("/raw", func(c *gin.Context) {
		c.Error(assert.AnError)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/raw", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), assert.AnError.Error(),
		"an unrecognised error's message must not be forwarded")
}

// TestLogger_CarriesWrapCauseAndEscalatesLevel is the regression test for the
// finding that errors.Wrap context was written and then discarded entirely: it
// is withheld from the response body on purpose, so the log is where it has to
// survive.
func TestLogger_CarriesWrapCauseAndEscalatesLevel(t *testing.T) {
	cases := []struct {
		name      string
		err       *ferrors.CodedError
		wantLevel string
		wantCode  int
	}{
		{"4xx logs at warn", ferrors.Wrap(ferrors.ErrNotFound, "no such user alice"), "warn", 404},
		{"5xx logs at error", ferrors.Wrap(ferrors.ErrInternal, "db unreachable"), "error", 500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			log, err := logger.New(logger.WithLevel("debug"), logger.WithWriter(&buf))
			require.NoError(t, err)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(mw.Logger(log), mw.ErrorHandler())
			r.GET("/u", func(c *gin.Context) { c.Error(tc.err) })

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/u", nil))
			require.Equal(t, tc.wantCode, w.Code)

			var entry map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

			assert.Equal(t, tc.wantLevel, entry["level"])
			assert.Equal(t, tc.err.Reason, entry["error_reason"])
			assert.Contains(t, entry["error"], tc.err.Error(),
				"the wrapped cause must reach the log")
			assert.NotContains(t, w.Body.String(), "alice",
				"the cause must not reach the response body")
		})
	}
}

func TestLogger_SuccessLogsAtInfoWithoutErrorFields(t *testing.T) {
	var buf bytes.Buffer
	log, err := logger.New(logger.WithLevel("debug"), logger.WithWriter(&buf))
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw.Logger(log), mw.ErrorHandler())
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/ok", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "info", entry["level"])
	assert.NotContains(t, entry, "error")
	assert.NotContains(t, entry, "error_reason")
}

// TestMetricHooks_SwapAndReset covers the atomic hook set that replaced four
// individually-assigned package vars, which the race detector flagged as being
// written while in-flight requests read them.
func TestMetricHooks_SwapAndReset(t *testing.T) {
	t.Cleanup(mw.ResetHTTPMetricHooks)

	var inFlight int
	var requests, errs int
	var lastReason, lastCode string

	mw.RegisterHTTPMetricHooks(
		func() { inFlight++ },
		func() { inFlight-- },
		func(_, _, _ string) { requests++ },
		nil, // a nil callback must be tolerated, not panic
		func(reason, code string) { errs++; lastReason = reason; lastCode = code },
	)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw.Metrics(), mw.ErrorHandler())
	r.GET("/u", func(c *gin.Context) { c.Error(ferrors.Wrap(ferrors.ErrNotFound, "nope")) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/u", nil))
	require.Equal(t, http.StatusNotFound, w.Code)

	assert.Equal(t, 0, inFlight, "in-flight must return to zero")
	assert.Equal(t, 1, requests)
	assert.Equal(t, 1, errs, "ErrorHandler should increment the error counter")
	assert.Equal(t, "NOT_FOUND", lastReason)
	assert.Equal(t, "404", lastCode)

	// After reset the hooks must be inert again, so one test cannot leak
	// instruments into the next.
	mw.ResetHTTPMetricHooks()
	before := requests
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/u", nil))
	assert.Equal(t, before, requests, "reset hooks must stop recording")
}
