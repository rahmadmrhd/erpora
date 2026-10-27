package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func spanCtx(t *testing.T) context.Context {
	t.Helper()
	tid, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, TraceFlags: trace.FlagsSampled})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func TestWriteError_Envelope(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", nil).WithContext(spanCtx(t))
	w := httptest.NewRecorder()
	w.Header().Set(HeaderRequestID, "req-1")
	WriteError(w, r, 422, "validation_failed", "Data tidak valid.", []Detail{{Field: "title", Code: "required", Message: "wajib"}})

	if w.Code != 422 || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("status=%d ct=%q", w.Code, w.Header().Get("Content-Type"))
	}
	want := `{"error":{"code":"validation_failed","message":"Data tidak valid.","details":[{"field":"title","code":"required","message":"wajib"}],"requestId":"req-1","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}}` + "\n"
	if got := w.Body.String(); got != want {
		t.Errorf("body = %s\nwant   %s", got, want)
	}

	w2 := httptest.NewRecorder()
	WriteError(w2, httptest.NewRequest("GET", "/x", nil), 400, "bad_request", "x", nil)
	if b := w2.Body.String(); strings.Contains(b, "requestId") || strings.Contains(b, "traceId") || strings.Contains(b, "details") {
		t.Errorf("kunci opsional tidak boleh ada: %s", b)
	}
}

func TestRespondError_MapsHttpxError(t *testing.T) {
	w := httptest.NewRecorder()
	RespondError(w, httptest.NewRequest("GET", "/x", nil), fmt.Errorf("wrap: %w", NewError(409, "invalid_state", "x")))
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"code":"invalid_state"`) {
		t.Errorf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRespondError_UnknownErrorIs500Generic(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	w := httptest.NewRecorder()
	RespondError(w, httptest.NewRequest("GET", "/x", nil), errors.New("pq: secret detail"))
	if w.Code != 500 || !strings.Contains(w.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret") {
		t.Errorf("detail internal bocor: %s", w.Body.String())
	}
	if !strings.Contains(buf.String(), "level=ERROR") || !strings.Contains(buf.String(), "secret detail") {
		t.Errorf("log = %s", buf.String())
	}
}

func TestWritePage_Envelope(t *testing.T) {
	w := httptest.NewRecorder()
	WritePage(w, http.StatusOK, []int{1, 2}, Page{Page: 1, PageSize: 25, Total: 2})
	want := `{"data":[1,2],"page":{"page":1,"pageSize":25,"total":2,"hasMore":false}}` + "\n"
	if w.Body.String() != want {
		t.Errorf("body = %s", w.Body.String())
	}
}
