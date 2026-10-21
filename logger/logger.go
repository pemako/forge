package logger

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/pemako/forge/core"
)

// F creates a core.Field from a key-value pair.
// Use F as a shorthand when calling Info, Debug, Warn, or Error.
func F(key string, value any) core.Field {
	return core.Field{Key: key, Value: value}
}

// timeFormat matches zap's ISO8601 encoder so existing log pipelines that parse
// the "ts" field keep working.
const timeFormat = "2006-01-02T15:04:05.000Z0700"

// maxLevel is the upper bound for the error-level file, which captures
// slog.LevelError and anything above it.
const maxLevel = slog.Level(math.MaxInt32)

type slogLogger struct {
	h          slog.Handler
	maskFields map[string]struct{} // lowercase field names to mask
}

// maskValue replaces the middle portion of a string with ***.
// "mypassword" → "myp***ord"
// Short values (<=4 chars) → "***"
func maskValue(s string) string {
	r := []rune(s)
	n := len(r)
	if n == 0 {
		return "***"
	}
	if n <= 4 {
		return "***"
	}
	keep := max(n/4, 2)
	return string(r[:keep]) + "***" + string(r[n-keep:])
}

// shortCaller trims a full source path to "<dir>/<file>", matching zap's
// ShortCallerEncoder (e.g. "/home/u/app/logger/logger.go" → "logger/logger.go").
func shortCaller(file string) string {
	last := strings.LastIndexByte(file, '/')
	if last < 0 {
		return file
	}
	prev := strings.LastIndexByte(file[:last], '/')
	if prev < 0 {
		return file
	}
	return file[prev+1:]
}

// replaceAttr normalises slog's built-in attributes to the field names and
// encodings the previous zap-based implementation emitted: ts/level/msg/caller,
// lowercase level names, ISO8601 timestamps, and stringified durations.
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 {
		switch a.Key {
		case slog.TimeKey:
			return slog.String("ts", a.Value.Time().Format(timeFormat))
		case slog.LevelKey:
			lv, _ := a.Value.Any().(slog.Level)
			return slog.String("level", strings.ToLower(lv.String()))
		case slog.SourceKey:
			src, ok := a.Value.Any().(*slog.Source)
			if !ok || src.File == "" {
				return slog.Attr{}
			}
			return slog.String("caller", shortCaller(src.File)+":"+strconv.Itoa(src.Line))
		}
	}
	// slog renders time.Duration as an int64 nanosecond count; zap rendered it
	// as a human-readable string. Keep the string form.
	if d, ok := a.Value.Any().(time.Duration); ok {
		return slog.String(a.Key, d.String())
	}
	return a
}

// newJSONHandler builds a JSON handler writing to w, enabled from min upward.
func newJSONHandler(w io.Writer, min slog.Level) slog.Handler {
	return slog.NewJSONHandler(w, &slog.HandlerOptions{
		AddSource:   true,
		Level:       min,
		ReplaceAttr: replaceAttr,
	})
}

// New creates a new core.Logger backed by the standard library's log/slog.
// Configuration is applied via functional options; sensible defaults are used
// for any unset options: level=info, output=stdout, maxSizeMB=100,
// maxAgeDays=7, maxBackups=10.
// Returns an error if the level string is not a valid slog.Level.
func New(opts ...Option) (core.Logger, error) {
	o := &options{
		level:      "info",
		output:     "stdout",
		maxSizeMB:  100,
		maxAgeDays: 7,
		maxBackups: 10,
	}
	for _, opt := range opts {
		opt(o)
	}

	level := slog.LevelInfo
	if err := level.UnmarshalText([]byte(o.level)); err != nil {
		return nil, err
	}

	var handlers []slog.Handler

	// stdout handler
	if o.output == "stdout" || o.output == "both" || o.output == "" {
		handlers = append(handlers, newJSONHandler(os.Stdout, level))
	}

	// test writer override: skip file handlers when writer is set
	if o.writer != nil {
		handlers = append(handlers, newJSONHandler(o.writer, level))
	} else if (o.output == "file" || o.output == "both") && o.filename != "" {
		// Level-split file handlers
		levels := []struct {
			name string
			min  slog.Level
			max  slog.Level
		}{
			{"debug", slog.LevelDebug, slog.LevelDebug},
			{"info", slog.LevelInfo, slog.LevelInfo},
			{"warn", slog.LevelWarn, slog.LevelWarn},
			{"error", slog.LevelError, maxLevel},
		}
		for _, lv := range levels {
			if lv.min < level {
				continue // skip levels below configured minimum
			}
			lj := &lumberjack.Logger{
				Filename:   o.filename + "." + lv.name + ".log",
				MaxSize:    o.maxSizeMB,
				MaxAge:     o.maxAgeDays,
				MaxBackups: o.maxBackups,
				Compress:   false,
			}
			handlers = append(handlers, &rangeHandler{
				inner: newJSONHandler(lj, lv.min),
				min:   lv.min,
				max:   lv.max,
			})
		}
	}

	if len(handlers) == 0 {
		handlers = append(handlers, newJSONHandler(io.Discard, level))
	}

	// Build maskFields set
	maskFields := make(map[string]struct{}, len(o.MaskFields))
	for _, f := range o.MaskFields {
		maskFields[strings.ToLower(f)] = struct{}{}
	}

	return &slogLogger{h: &fanoutHandler{handlers: handlers}, maskFields: maskFields}, nil
}

// traceAttrs returns trace_id and span_id attributes when ctx carries a
// recording OpenTelemetry span, and nil otherwise.
func traceAttrs(ctx context.Context) []slog.Attr {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}
	sc := span.SpanContext()
	return []slog.Attr{
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	}
}

// toAttrs converts core.Fields to slog attributes, redacting any field whose
// name appears in maskFields.
func toAttrs(fields []core.Field, maskFields map[string]struct{}) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(fields))
	for _, f := range fields {
		val := f.Value
		if _, masked := maskFields[strings.ToLower(f.Key)]; masked {
			if s, ok := val.(string); ok {
				val = maskValue(s)
			} else {
				val = "***"
			}
		}
		attrs = append(attrs, slog.Any(f.Key, val))
	}
	return attrs
}

// log emits one record. callerSkip frames are skipped when resolving the
// "caller" attribute so it points at the application call site rather than
// at this package.
func (l *slogLogger) log(ctx context.Context, level slog.Level, msg string, fields []core.Field) {
	if !l.h.Enabled(ctx, level) {
		return
	}
	// Skip runtime.Callers, this method, and the exported Debug/Info/Warn/Error
	// wrapper so the caller attribute points at application code.
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.AddAttrs(traceAttrs(ctx)...)
	r.AddAttrs(toAttrs(fields, l.maskFields)...)
	_ = l.h.Handle(ctx, r)
}

func (l *slogLogger) Debug(ctx context.Context, msg string, fields ...core.Field) {
	l.log(ctx, slog.LevelDebug, msg, fields)
}
func (l *slogLogger) Info(ctx context.Context, msg string, fields ...core.Field) {
	l.log(ctx, slog.LevelInfo, msg, fields)
}
func (l *slogLogger) Warn(ctx context.Context, msg string, fields ...core.Field) {
	l.log(ctx, slog.LevelWarn, msg, fields)
}
func (l *slogLogger) Error(ctx context.Context, msg string, fields ...core.Field) {
	l.log(ctx, slog.LevelError, msg, fields)
}

// With returns a child logger that attaches fields to every subsequent record.
// Masked field names are redacted here too, so a secret bound once via With is
// never written in clear text.
func (l *slogLogger) With(fields ...core.Field) core.Logger {
	return &slogLogger{
		h:          l.h.WithAttrs(toAttrs(fields, l.maskFields)),
		maskFields: l.maskFields,
	}
}

// discardHandler drops every record; it backs the no-op default logger.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }

// _default is the process-wide logger, initialized to a no-op.
var _default core.Logger = &slogLogger{h: discardHandler{}}

// SetDefault sets the package-level default logger used as a fallback when no
// logger is explicitly injected. Call this once at application startup in main()
// after creating the logger with New.
//
// In application and infrastructure packages, prefer constructor injection over
// calling Default() at call sites:
//
//	// Preferred
//	func New(opts ...Option) *T {
//	    o := defaultOptions() // uses logger.Default() as fallback
//	}
//
//	// Avoid in business logic
//	logger.Default().Info(ctx, "something")
func SetDefault(log core.Logger) { _default = log }

// Default returns the package-level default logger set by SetDefault.
// It serves as a fallback for packages that have not been given an explicit
// logger via constructor injection. Do not use as a general-purpose global
// logger in business logic; prefer constructor injection.
func Default() core.Logger { return _default }

// Debug logs a message at DEBUG level using the default logger.
func Debug(ctx context.Context, msg string, fields ...core.Field) {
	_default.Debug(ctx, msg, fields...)
}

// Info logs a message at INFO level using the default logger.
func Info(ctx context.Context, msg string, fields ...core.Field) {
	_default.Info(ctx, msg, fields...)
}

// Warn logs a message at WARN level using the default logger.
func Warn(ctx context.Context, msg string, fields ...core.Field) {
	_default.Warn(ctx, msg, fields...)
}

// Error logs a message at ERROR level using the default logger.
func Error(ctx context.Context, msg string, fields ...core.Field) {
	_default.Error(ctx, msg, fields...)
}
