// Package httpapi berisi handler HTTP modul example. Foldernya bernama http,
// tetapi package dinamai httpapi agar tidak bentrok dengan net/http.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"erpora/modules/example/internal/application"
	"erpora/modules/example/internal/domain"
)

// NoteLister adalah use case daftar notes.
type NoteLister interface {
	ListNotes(ctx context.Context, page, pageSize int) ([]domain.Note, int64, error)
}

// Handlers menampung handler HTTP modul.
type Handlers struct{ notes NoteLister }

// New membuat Handlers.
func New(notes NoteLister) *Handlers { return &Handlers{notes: notes} }

type noteDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
}

type pageDTO struct {
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
	HasMore  bool  `json:"hasMore"`
}

// Ping menjawab {"data":{"pong":true}}.
func (h *Handlers) Ping(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"pong": true}})
}

// ListNotes menjawab daftar notes dengan pagination halaman (api-guidelines §6).
func (h *Handlers) ListNotes(w http.ResponseWriter, r *http.Request) {
	page, ok := intParam(r, "page", 1, 1, 1<<30)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "parameter page harus bilangan bulat >= 1")
		return
	}
	size, ok := intParam(r, "pageSize", application.DefaultPageSize, 1, application.MaxPageSize)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "parameter pageSize harus bilangan bulat 1 sampai 200")
		return
	}
	items, total, err := h.notes.ListNotes(r.Context(), page, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "terjadi kesalahan pada server")
		return
	}
	out := make([]noteDTO, len(items))
	for i, n := range items {
		out[i] = noteDTO{ID: n.ID, Title: n.Title, CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339)}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"page": pageDTO{Page: page, PageSize: size, Total: total, HasMore: int64(page)*int64(size) < total},
	})
}

// intParam membaca query integer; kosong = def; di luar [min,max] atau bukan angka = false.
func intParam(r *http.Request, name string, def, minV, maxV int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < minV || v > maxV {
		return 0, false
	}
	return v, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	body := map[string]any{"code": code, "message": message}
	if id := w.Header().Get("X-Request-Id"); id != "" {
		body["requestId"] = id
	}
	writeJSON(w, status, map[string]any{"error": body})
}
