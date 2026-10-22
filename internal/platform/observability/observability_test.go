package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func clearOtelEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_SERVICE_NAME"} {
		t.Setenv(k, "")
	}
}

func TestSetup_NoEndpointIsNoop(t *testing.T) {
	clearOtelEnv(t)
	p, shutdown, err := Setup(context.Background(), Options{ServiceVersion: "1", Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Enabled {
		t.Fatal("Enabled must be false")
	}
	if _, ok := p.Tracer.(noop.TracerProvider); !ok {
		t.Fatalf("tracer provider is %T, want noop", p.Tracer)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSetup_WithEndpointEnabled(t *testing.T) {
	clearOtelEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	p, shutdown, err := Setup(context.Background(), Options{ServiceVersion: "1.2.3", Environment: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_ = shutdown(ctx) // endpoint tidak ada; error flush diabaikan
	}()
	if !p.Enabled {
		t.Fatal("Enabled must be true")
	}
	got := map[string]string{}
	for _, kv := range p.Resource.Attributes() {
		got[string(kv.Key)] = kv.Value.AsString()
	}
	if got["service.name"] != "erpora" || got["service.version"] != "1.2.3" || got["deployment.environment"] != "staging" {
		t.Fatalf("resource = %v", got)
	}
}

func TestSetup_ServiceNameFromEnv(t *testing.T) {
	clearOtelEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Setenv("OTEL_SERVICE_NAME", "custom")
	p, shutdown, err := Setup(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	defer func() { _ = shutdown(ctx) }()
	for _, kv := range p.Resource.Attributes() {
		if kv.Key == "service.name" && kv.Value.AsString() != "custom" {
			t.Fatalf("service.name = %s", kv.Value.AsString())
		}
	}
}

func TestLogger_AddsTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "json", slog.LevelInfo)

	log.InfoContext(context.Background(), "no span")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec["trace_id"]; ok {
		t.Fatalf("trace_id without span: %v", rec)
	}
	if _, ok := rec["span_id"]; ok {
		t.Fatalf("span_id without span: %v", rec)
	}

	buf.Reset()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(tracetest.NewInMemoryExporter())))
	ctx, span := tp.Tracer("t").Start(context.Background(), "op")
	defer span.End()
	log.With("k", "v").WithGroup("g").InfoContext(ctx, "in span", "a", 1)
	rec = nil
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	// Setelah WithGroup, atribut record berada di dalam grup "g"; cari di kedua tempat.
	g, _ := rec["g"].(map[string]any)
	tid, sid := rec["trace_id"], rec["span_id"]
	if tid == nil && g != nil {
		tid, sid = g["trace_id"], g["span_id"]
	}
	if tid != span.SpanContext().TraceID().String() || sid != span.SpanContext().SpanID().String() {
		t.Fatalf("ids mismatch: %v", rec)
	}
}

func TestLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf, "text", slog.LevelInfo).Info("hello")
	if !bytes.Contains(buf.Bytes(), []byte("msg=hello")) {
		t.Fatalf("got %s", buf.String())
	}
}

func TestLogger_AddsRequestID(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "json", slog.LevelInfo)

	log.InfoContext(context.Background(), "tanpa id")
	if bytes.Contains(buf.Bytes(), []byte("request_id")) {
		t.Fatalf("request_id tanpa konteks: %s", buf.String())
	}
	if _, ok := RequestIDFromContext(context.Background()); ok {
		t.Fatal("ok=true tanpa request id")
	}

	buf.Reset()
	ctx := WithRequestID(context.Background(), "req-123")
	log.InfoContext(ctx, "dengan id")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["request_id"] != "req-123" {
		t.Fatalf("rec = %v", rec)
	}
}
