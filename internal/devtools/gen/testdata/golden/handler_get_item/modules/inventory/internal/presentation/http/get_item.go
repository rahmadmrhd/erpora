package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"erpora/modules/inventory/internal/domain"
	"erpora/sdk/httpx"
)

// GetItemData adalah DTO respons GET /items/{id} (api-guidelines §4: camelCase, waktu ISO 8601 UTC).
type GetItemData struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

var errGetItemNotImplemented = errors.New("GetItem belum disambungkan ke use case")

// getItemFunc adalah titik sambung use case. Ganti dengan pemanggilan use case di application/ (mis. lewat Handlers).
// Use case WAJIB membatasi pencarian ke scope pemanggil dan mengembalikan domain.ErrNotFound untuk record yang tidak ada
// ATAU di luar scope (404, bukan 403; PRD-001 FR-119). Setelah sdk/authz tersedia (PRD-001), muat record lewat
// authz.Check/authz.Filter.
var getItemFunc = defaultGetItem

// defaultGetItem adalah titik sambung bawaan kerangka: 501 sampai use case disambungkan. Test IDOR hasil generate
// gagal bila getItemFunc sudah bukan fungsi ini (lihat TestGetItem_IDOR_*).
func defaultGetItem(_ context.Context, _ string) (GetItemData, error) {
	return GetItemData{}, errGetItemNotImplemented
}

// GetItem melayani GET /items/{id}: memuat satu record menurut ID.
func GetItem() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := uuid.Parse(id); err != nil { // ID rusak = tidak ada; jangan bedakan dari record di luar scope
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data tidak ditemukan.", nil)
			return
		}
		out, err := getItemFunc(r.Context(), id)
		switch {
		case err == nil:
			httpx.WriteData(w, http.StatusOK, out)
		case errors.Is(err, domain.ErrNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data tidak ditemukan.", nil)
		case errors.Is(err, errGetItemNotImplemented):
			httpx.WriteError(w, r, http.StatusNotImplemented, "not_implemented", "Endpoint belum diimplementasi.", nil)
		default:
			// Detail 5xx hanya di log (api-guidelines §8); respons tetap generik.
			httpx.RespondError(w, r, fmt.Errorf("GetItem: %w", err))
		}
	})
}
