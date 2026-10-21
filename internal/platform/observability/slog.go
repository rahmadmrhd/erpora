package observability

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler membungkus slog.Handler dan menambah trace_id & span_id dari span aktif di ctx.
type TraceHandler struct{ inner slog.Handler }

func (h TraceHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.inner.Enabled(ctx, l) }

func (h TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.inner.Handle(ctx, r)
}

func (h TraceHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return TraceHandler{h.inner.WithAttrs(a)}
}
func (h TraceHandler) WithGroup(n string) slog.Handler { return TraceHandler{h.inner.WithGroup(n)} }

// NewLogger membuat logger slog ("json" atau selain itu text) yang dibungkus TraceHandler.
func NewLogger(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(TraceHandler{h})
}
