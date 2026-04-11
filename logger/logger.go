package logger

import (
	"context"
	"io"
	"os"

	"github.com/pemako/mk/core"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// F creates a core.Field shorthand.
func F(key string, value any) core.Field {
	return core.Field{Key: key, Value: value}
}

type zapLogger struct {
	z *zap.Logger
}

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

	var w zapcore.WriteSyncer
	if o.writer != nil {
		w = zapcore.AddSync(o.writer)
	} else {
		w = buildWriter(o)
	}

	c := zapcore.NewCore(enc, w, level)
	z := zap.New(c, zap.AddCaller(), zap.AddCallerSkip(1))
	return &zapLogger{z: z}, nil
}

func buildWriter(o *options) zapcore.WriteSyncer {
	var writers []zapcore.WriteSyncer
	if o.output == "stdout" || o.output == "both" || o.output == "" {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}
	if (o.output == "file" || o.output == "both") && o.filename != "" {
		lj := &lumberjack.Logger{
			Filename:   o.filename,
			MaxSize:    o.maxSizeMB,
			MaxAge:     o.maxAgeDays,
			MaxBackups: o.maxBackups,
			Compress:   false,
		}
		writers = append(writers, zapcore.AddSync(lj))
	}
	if len(writers) == 0 {
		writers = append(writers, zapcore.AddSync(io.Discard))
	}
	return zapcore.NewMultiWriteSyncer(writers...)
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

func toZapFields(ctx context.Context, fields []core.Field) []zap.Field {
	zf := extractTraceFields(ctx)
	for _, f := range fields {
		zf = append(zf, zap.Any(f.Key, f.Value))
	}
	return zf
}

func (l *zapLogger) Debug(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Debug(msg, toZapFields(ctx, fields)...)
}
func (l *zapLogger) Info(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Info(msg, toZapFields(ctx, fields)...)
}
func (l *zapLogger) Warn(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Warn(msg, toZapFields(ctx, fields)...)
}
func (l *zapLogger) Error(ctx context.Context, msg string, fields ...core.Field) {
	l.z.Error(msg, toZapFields(ctx, fields)...)
}
func (l *zapLogger) With(fields ...core.Field) core.Logger {
	zf := make([]zap.Field, len(fields))
	for i, f := range fields {
		zf[i] = zap.Any(f.Key, f.Value)
	}
	return &zapLogger{z: l.z.With(zf...)}
}

var _default core.Logger = &zapLogger{z: zap.NewNop()}

func SetDefault(log core.Logger) { _default = log }
func Default() core.Logger       { return _default }

func Debug(ctx context.Context, msg string, fields ...core.Field) { _default.Debug(ctx, msg, fields...) }
func Info(ctx context.Context, msg string, fields ...core.Field)  { _default.Info(ctx, msg, fields...) }
func Warn(ctx context.Context, msg string, fields ...core.Field)  { _default.Warn(ctx, msg, fields...) }
func Error(ctx context.Context, msg string, fields ...core.Field) { _default.Error(ctx, msg, fields...) }
