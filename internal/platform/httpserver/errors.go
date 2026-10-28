package httpserver

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"requestId,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// WriteError menulis amplop error standar. Pesan tidak boleh memuat detail internal (5xx).
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	traceID := ""
	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		traceID = sc.TraceID().String()
	}
	writeJSON(w, status, errorEnvelope{Error: errorBody{
		Code: code, Message: message, Details: details,
		RequestID: w.Header().Get(headerRequestID), TraceID: traceID,
	}})
}

// WriteData menulis amplop sukses {"data": v}.
func WriteData(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, struct {
		Data any `json:"data"`
	}{v})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// PageInfo adalah metadata pagination halaman (api-guidelines section 6).
type PageInfo struct {
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
	HasMore  bool  `json:"hasMore"`
}

// WritePage menulis amplop daftar {"data": [...], "page": {...}}.
func WritePage(w http.ResponseWriter, status int, data any, page PageInfo) {
	writeJSON(w, status, struct {
		Data any      `json:"data"`
		Page PageInfo `json:"page"`
	}{data, page})
}
