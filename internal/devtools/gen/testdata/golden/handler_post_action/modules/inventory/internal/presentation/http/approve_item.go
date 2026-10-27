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

// ApproveItemRequest adalah body opsional POST /items/{id}/approve. httpx.DecodeJSON menolak field tak dikenal dan tipe salah (422).
type ApproveItemRequest struct {
	Reason string `json:"reason"`
}

// ApproveItemData adalah DTO respons (api-guidelines §4).
type ApproveItemData struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

var errApproveItemNotImplemented = errors.New("ApproveItem belum disambungkan ke use case")

// approveItemFunc adalah titik sambung use case; ifMatch adalah nilai header If-Match (kosong = tidak dikirim). Ganti dengan
// pemanggilan use case di application/. Use case WAJIB membatasi record ke scope pemanggil (domain.ErrNotFound bila di
// luar scope), memeriksa ifMatch terhadap Version (412 version_mismatch lewat *httpx.Error), dan menolak transisi yang tidak
// sah (409 invalid_state lewat *httpx.Error).
var approveItemFunc = func(_ context.Context, _, _ string, _ ApproveItemRequest) (ApproveItemData, error) {
	return ApproveItemData{}, errApproveItemNotImplemented
}

// ApproveItem melayani POST /items/{id}/approve: aksi pada satu record (api-guidelines §2: POST .../{id}/<aksi>).
func ApproveItem() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := uuid.Parse(id); err != nil { // ID rusak = tidak ada; jangan bedakan dari record di luar scope
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data tidak ditemukan.", nil)
			return
		}
		req, err := httpx.DecodeJSON[ApproveItemRequest](w, r, httpx.AllowEmptyBody())
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}
		out, err := approveItemFunc(r.Context(), id, r.Header.Get("If-Match"), req)
		switch {
		case err == nil:
			httpx.WriteData(w, http.StatusOK, out)
		case errors.Is(err, domain.ErrNotFound):
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "Data tidak ditemukan.", nil)
		case errors.Is(err, errApproveItemNotImplemented):
			httpx.WriteError(w, r, http.StatusNotImplemented, "not_implemented", "Endpoint belum diimplementasi.", nil)
		default:
			httpx.RespondError(w, r, fmt.Errorf("ApproveItem: %w", err))
		}
	})
}
