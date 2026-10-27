package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"erpora/modules/stockopname/internal/application"
	"erpora/sdk/httpx"
)

// ListStockCountsData adalah DTO satu elemen respons GET /stock-counts (api-guidelines §4).
type ListStockCountsData struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Version   int64  `json:"version"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

var errListStockCountsNotImplemented = errors.New("ListStockCounts belum disambungkan ke use case")

// listStockCountsFunc adalah titik sambung use case (page mulai 1). Ganti dengan pemanggilan use case di application/.
// Use case WAJIB membatasi hasil ke scope pemanggil (sdk/authz.Filter setelah PRD-001).
var listStockCountsFunc = func(_ context.Context, _, _ int) ([]ListStockCountsData, int64, error) {
	return nil, 0, errListStockCountsNotImplemented
}

// ListStockCounts melayani GET /stock-counts: daftar berpagination halaman (api-guidelines §6).
func ListStockCounts() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		items, total, err := listStockCountsFunc(r.Context(), page, size)
		switch {
		case err == nil:
			if items == nil {
				items = []ListStockCountsData{} // data selalu array, tidak pernah null
			}
			httpx.WritePage(w, http.StatusOK, items, httpx.Page{
				Page: page, PageSize: size, Total: total, HasMore: int64(page)*int64(size) < total,
			})
		case errors.Is(err, errListStockCountsNotImplemented):
			httpx.WriteError(w, r, http.StatusNotImplemented, "not_implemented", "Endpoint belum diimplementasi.", nil)
		default:
			httpx.RespondError(w, r, fmt.Errorf("ListStockCounts: %w", err))
		}
	})
}
