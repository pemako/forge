package logger

import (
	"context"
	"errors"
	"log/slog"
)

// rangeHandler restricts inner to a closed level range [min, max].
//
// slog.HandlerOptions only supports a minimum level, so level-split file output
// (one file per level) needs an explicit upper bound as well.
type rangeHandler struct {
	inner slog.Handler
	min   slog.Level
	max   slog.Level
}

func (h *rangeHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.min && l <= h.max
}

func (h *rangeHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}

func (h *rangeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &rangeHandler{inner: h.inner.WithAttrs(attrs), min: h.min, max: h.max}
}

func (h *rangeHandler) WithGroup(name string) slog.Handler {
	return &rangeHandler{inner: h.inner.WithGroup(name), min: h.min, max: h.max}
}

// fanoutHandler dispatches every record to each sub-handler that accepts its
// level, mirroring zapcore.NewTee. Handler errors are joined so one failing
// destination does not suppress the others.
type fanoutHandler struct {
	handlers []slog.Handler
}

func (h *fanoutHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for _, sub := range h.handlers {
		if sub.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, sub := range h.handlers {
		if !sub.Enabled(ctx, r.Level) {
			continue
		}
		// Clone because a handler may retain or mutate the record's attrs.
		if err := sub.Handle(ctx, r.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	subs := make([]slog.Handler, len(h.handlers))
	for i, sub := range h.handlers {
		subs[i] = sub.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: subs}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	subs := make([]slog.Handler, len(h.handlers))
	for i, sub := range h.handlers {
		subs[i] = sub.WithGroup(name)
	}
	return &fanoutHandler{handlers: subs}
}
