package httpMiddleware

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/pemako/forge/errors"
)

// ErrorHandler translates gin.Context errors to JSON responses.
// *errors.CodedError -> its code + body.
// Other errors -> 500 Internal (message not forwarded).
//
// Beyond writing the response it also reports the failure to telemetry: the
// active span records the error and is marked failed for 5xx, and an
// http_errors_total counter is incremented per reason. Previously a failed
// request produced a JSON body and nothing else — no span event, no counter —
// so a 500 was invisible in tracing and metrics.
//
// The cause chain attached by errors.Wrap is deliberately kept out of the
// response body. It is recorded on the span, and the Logger middleware writes
// it to the log, so it is recoverable without being exposed to callers.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err

		ce, ok := errors.AsType[*errors.CodedError](err)
		if !ok {
			ce = errors.ErrInternal
		}

		if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
			span.RecordError(err)
			span.SetAttributes(
				attribute.Int("error.code", ce.Code),
				attribute.String("error.reason", ce.Reason),
			)
			if ce.Code >= 500 {
				span.SetStatus(codes.Error, ce.Message)
			}
		}

		httpHooks().errorsInc(ce.Reason, strconv.Itoa(ce.Code))

		c.JSON(ce.Code, ce)
	}
}
