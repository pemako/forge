package logger

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/pemako/mk/core"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// F creates a core.Field from a key-value pair.
// Use F as a shorthand when calling Info, Debug, Warn, or Error.
func F(key string, value any) core.Field {
	return core.Field{Key: key, Value: value}
}

type zapLogger struct {
	z          *zap.Logger
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

// New creates a new core.Logger backed by zap. Configuration is applied via
// functional options; sensible defaults are used for any unset options:
// level=info, output=stdout, maxSizeMB=100, maxAgeDays=7, maxBackups=10.
// Returns an error if the level string is not a valid zapcore.Level.
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

	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(o.level)); err != nil {
		return nil, err
	}

	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		MessageKey:     "msg",
		CallerKey:      "caller",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	var cores []zapcore.Core

	// stdout core
	if o.output == "stdout" || o.output == "both" || o.output == "" {
		cores = append(cores, zapcore.NewCore(enc, zapcore.AddSync(os.Stdout), level))
	}

	// test writer override: skip file cores when writer is set
	if o.writer != nil {
		cores = append(cores, zapcore.NewCore(enc, zapcore.AddSync(o.writer), level))
	} else if (o.output == "file" || o.output == "both") && o.filename != "" {
		// Level-split file cores
		levels := []struct {
			name  string
			level zapcore.Level
			max   zapcore.Level
		}{
			{"debug", zapcore.DebugLevel, zapcore.DebugLevel},
			{"info", zapcore.InfoLevel, zapcore.InfoLevel},
			{"warn", zapcore.WarnLevel, zapcore.WarnLevel},
			{"error", zapcore.ErrorLevel, zapcore.FatalLevel},
		}
		for _, lv := range levels {
			if lv.level < level {
				continue // skip levels below configured minimum
			}
			lj := &lumberjack.Logger{
				Filename:   o.filename + "." + lv.name + ".log",
				MaxSize:    o.maxSizeMB,
				MaxAge:     o.maxAgeDays,
				MaxBackups: o.maxBackups,
				Compress:   false,
			}
			lvCopy := lv // capture for closure
			levelEnabler := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
				return l >= lvCopy.level && l <= lvCopy.max
			})
			cores = append(cores, zapcore.NewCore(enc, zapcore.AddSync(lj), levelEnabler))
		}
	}

	if len(cores) == 0 {
		cores = append(cores, zapcore.NewCore(enc, zapcore.AddSync(io.Discard), level))
	}

	z := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddCallerSkip(1))

	// Build maskFields set
	maskFields := make(map[string]struct{}, len(o.MaskFields))
	for _, f := range o.MaskFields {
		maskFields[strings.ToLower(f)] = struct{}{}
	}

	return &zapLogger{z: z, maskFields: maskFields}, nil
}

func extractTraceFields(ctx context.Context) []zap.Field {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return nil
	}
	sc := span.SpanContext()
	return []zap.Field{
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	}
}

func toZapFields(ctx context.Context, fields []core.Field, maskFields map[string]struct{}) []zap.Field {
	zf := extractTraceFields(ctx)
	for _, f := range fields {
		val := f.Value
		if _, masked := maskFields[strings.ToLower(f.Key)]; masked {
			if s, ok := val.(string); ok {
				val = maskValue(s)
			} else {
				val = "***"
			}
		}
		zf = append(zf, zap.Any(f.Key, val))
	}
	return zf
}

func (l *zapLogger) Debug(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Debug(msg, toZapFields(ctx, fields, l.maskFields)...)
}
func (l *zapLogger) Info(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Info(msg, toZapFields(ctx, fields, l.maskFields)...)
}
func (l *zapLogger) Warn(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Warn(msg, toZapFields(ctx, fields, l.maskFields)...)
}
func (l *zapLogger) Error(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Error(msg, toZapFields(ctx, fields, l.maskFields)...)
}
func (l *zapLogger) With(fields ...core.Field) core.Logger {
	zf := make([]zap.Field, len(fields))
	for i, f := range fields {
		zf[i] = zap.Any(f.Key, f.Value)
	}
	return &zapLogger{z: l.z.With(zf...), maskFields: l.maskFields}
}

// _default is the process-wide logger, initialized to a no-op.
var _default core.Logger = &zapLogger{z: zap.NewNop()}

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
