// Package httpapi berisi handler HTTP modul example. Foldernya bernama http,
// tetapi package dinamai httpapi agar tidak bentrok dengan net/http.
package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"erpora/modules/example/internal/application"
	"erpora/modules/example/internal/domain"
	"erpora/sdk/httpx"
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
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Ping menjawab {"data":{"pong":true}}.
func (h *Handlers) Ping(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteData(w, http.StatusOK, map[string]bool{"pong": true})
}

// ListNotes menjawab daftar notes dengan pagination halaman (api-guidelines §6).
func (h *Handlers) ListNotes(w http.ResponseWriter, r *http.Request) {
	page, ok := intParam(r, "page", 1, 1, 1<<30)
	if !ok {
		badParam(w, r, "page", "parameter page harus bilangan bulat >= 1")
		return
	}
	size, ok := intParam(r, "pageSize", application.DefaultPageSize, 1, application.MaxPageSize)
	if !ok {
		badParam(w, r, "pageSize", "parameter pageSize harus bilangan bulat 1 sampai 200")
		return
	}
	items, total, err := h.notes.ListNotes(r.Context(), page, size)
	if err != nil {
		// Detail 5xx hanya di log (api-guidelines §8); respons tetap generik.
		httpx.RespondError(w, r, fmt.Errorf("gagal membaca daftar notes: %w", err))
		return
	}
	out := make([]noteDTO, len(items))
	for i, n := range items {
		out[i] = noteDTO{
			ID: n.ID, Title: n.Title, Version: n.Version,
			CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: n.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	httpx.WritePage(w, http.StatusOK, out, httpx.Page{
		Page: page, PageSize: size, Total: total, HasMore: int64(page)*int64(size) < total,
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

func badParam(w http.ResponseWriter, r *http.Request, field, msg string) {
	httpx.WriteError(w, r, http.StatusBadRequest, "bad_request", msg,
		[]httpx.Detail{{Field: field, Code: "invalid", Message: msg}})
}
