package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
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

// K5: tanpa endpoint, provider SDK tanpa exporter: tidak ada koneksi keluar, tetapi span punya ID valid.
func TestSetup_NoEndpointHasValidSpanIDsWithoutExporter(t *testing.T) {
	clearOtelEnv(t)
	p, shutdown, err := Setup(context.Background(), Options{ServiceVersion: "1", Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Enabled {
		t.Fatal("Enabled must be false")
	}
	if _, ok := p.Tracer.(noop.TracerProvider); ok {
		t.Fatal("tracer provider tidak boleh noop: ID span harus valid")
	}
	if _, ok := p.Tracer.(*sdktrace.TracerProvider); !ok {
		t.Fatalf("tracer provider is %T, want *sdktrace.TracerProvider", p.Tracer)
	}
	// Provider dibuat tanpa exporter/span processor (lihat Setup), jadi tidak ada ekspor atau koneksi keluar.
	_, span := p.Tracer.Tracer("t").Start(context.Background(), "op")
	if !span.SpanContext().TraceID().IsValid() || !span.SpanContext().SpanID().IsValid() {
		t.Fatalf("span context tidak valid: %v", span.SpanContext())
	}
	span.End()
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

// Q26 butir 3: TraceHandler menjadi sumber tunggal erp.module.key dari context, tanpa menggandakan
// atribut yang sudah disertakan pemanggil.
func TestTraceHandler_ModuleKeyFromContextNoDuplicate(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "text", slog.LevelDebug)
	ctx := WithModule(context.Background(), "acme.notes")
	log.InfoContext(ctx, "dari context")
	log.InfoContext(ctx, "eksplisit", "erp.module.key", "acme.notes")
	log.InfoContext(context.Background(), "tanpa modul")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	if strings.Count(lines[0], "erp.module.key=acme.notes") != 1 {
		t.Errorf("dari context: %s", lines[0])
	}
	if strings.Count(lines[1], "erp.module.key=") != 1 {
		t.Errorf("tidak boleh ganda: %s", lines[1])
	}
	if strings.Contains(lines[2], "erp.module.key") {
		t.Errorf("tanpa modul: %s", lines[2])
	}
}
