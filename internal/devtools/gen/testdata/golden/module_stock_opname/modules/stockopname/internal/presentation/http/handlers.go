// Package httpapi berisi handler HTTP modul stock-opname. Foldernya bernama http,
// tetapi package dinamai httpapi agar tidak bentrok dengan net/http.
package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"erpora/modules/stockopname/internal/application"
	"erpora/modules/stockopname/internal/domain"
	"erpora/sdk/httpx"
)

// ItemLister adalah use case daftar items.
type ItemLister interface {
	ListItems(ctx context.Context, page, pageSize int) ([]domain.Item, int64, error)
}

// Handlers menampung handler HTTP modul.
type Handlers struct{ items ItemLister }

// New membuat Handlers.
func New(items ItemLister) *Handlers { return &Handlers{items: items} }

type itemDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ListItems menjawab daftar items dengan pagination halaman (api-guidelines §6).
func (h *Handlers) ListItems(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := h.items.ListItems(r.Context(), page, size)
	if err != nil {
		// Detail 5xx hanya di log (api-guidelines §8); respons tetap generik.
		httpx.RespondError(w, r, fmt.Errorf("gagal membaca daftar items: %w", err))
		return
	}
	out := make([]itemDTO, len(items))
	for i, n := range items {
		out[i] = itemDTO{
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
