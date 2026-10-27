package observability

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler membungkus slog.Handler dan menambah trace_id & span_id dari span aktif di ctx.
type TraceHandler struct {
	inner slog.Handler
	// moduleBound true bila erp.module.key sudah di-bind lewat Logger.With (tidak ditambahkan lagi dari context).
	moduleBound bool
}

func (h TraceHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.inner.Enabled(ctx, l) }

func (h TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	// Sumber tunggal atribut modul (Q26 butir 3): erp.module.key dari context, kecuali pemanggil
	// sudah menyertakannya sendiri (mis. access log yang membacanya dari meta request).
	if m := ModuleFromContext(ctx); m != "" && !h.moduleBound && !hasAttr(r, moduleKeyAttr) {
		r.AddAttrs(slog.String(moduleKeyAttr, m))
	}
	if id, ok := RequestIDFromContext(ctx); ok {
		r.AddAttrs(slog.String("request_id", id))
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.inner.Handle(ctx, r)
}

const moduleKeyAttr = "erp.module.key"

func hasAttr(r slog.Record, key string) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = true
		}
		return !found
	})
	return found
}

func (h TraceHandler) WithAttrs(a []slog.Attr) slog.Handler {
	bound := h.moduleBound
	for _, at := range a {
		if at.Key == moduleKeyAttr {
			bound = true
		}
	}
	return TraceHandler{inner: h.inner.WithAttrs(a), moduleBound: bound}
}
func (h TraceHandler) WithGroup(n string) slog.Handler {
	return TraceHandler{inner: h.inner.WithGroup(n), moduleBound: h.moduleBound}
}

// NewLogger membuat logger slog ("json" atau selain itu text) yang dibungkus TraceHandler.
func NewLogger(w io.Writer, format string, level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(TraceHandler{inner: h})
}
