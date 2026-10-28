package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

// HeaderRequestID adalah header respons yang membawa request ID (diisi middleware platform).
const HeaderRequestID = "X-Request-Id"

// Detail adalah satu entri error.details (api-guidelines §8).
type Detail struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error adalah error HTTP terstruktur; Err (opsional) hanya untuk log dan tidak pernah dikirim.
type Error struct {
	Status  int
	Code    string
	Message string
	Details []Detail
	Err     error
}

// Error mengimplementasi error.
func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

// Unwrap mengembalikan Err.
func (e *Error) Unwrap() error { return e.Err }

// NewError membuat *Error.
func NewError(status int, code, message string, details ...Detail) *Error {
	return &Error{Status: status, Code: code, Message: message, Details: details}
}

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

// WriteError menulis {"error":{code,message,details,requestId,traceId}}. requestId diambil dari header
// respons X-Request-Id, traceId dari span context. Pesan 5xx tidak boleh memuat detail internal.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	traceID := ""
	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		traceID = sc.TraceID().String()
	}
	writeJSON(w, status, errorEnvelope{Error: errorBody{
		Code: code, Message: message, Details: details,
		RequestID: w.Header().Get(HeaderRequestID), TraceID: traceID,
	}})
}

// RespondError menulis *Error (errors.As) sesuai isinya; error lain dicatat dan dijawab 500
// internal_error generik tanpa detail.
func RespondError(w http.ResponseWriter, r *http.Request, err error) {
	var he *Error
	if errors.As(err, &he) {
		if he.Err != nil && he.Status >= 500 {
			slog.ErrorContext(r.Context(), "http error", "code", he.Code, "error", he.Err)
		}
		var details any
		if len(he.Details) > 0 {
			details = he.Details
		}
		WriteError(w, r, he.Status, he.Code, he.Message, details)
		return
	}
	slog.ErrorContext(r.Context(), "unhandled handler error", "error", err)
	WriteError(w, r, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server", nil)
}

// WriteData menulis amplop sukses {"data": v}.
func WriteData(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, struct {
		Data any `json:"data"`
	}{v})
}

// Page adalah metadata pagination halaman (api-guidelines §6).
type Page struct {
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
	HasMore  bool  `json:"hasMore"`
}

// WritePage menulis amplop daftar {"data": [...], "page": {...}}.
func WritePage(w http.ResponseWriter, status int, data any, page Page) {
	writeJSON(w, status, struct {
		Data any  `json:"data"`
		Page Page `json:"page"`
	}{data, page})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
