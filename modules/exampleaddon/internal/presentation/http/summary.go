// Package httpapi berisi handler HTTP modul exampleaddon. Foldernya bernama
// http, tetapi package dinamai httpapi agar tidak bentrok dengan net/http.
package httpapi

import (
	"fmt"
	"net/http"

	"erpora/modules/example/contract"
	"erpora/sdk/httpx"
)

// Handlers menampung handler HTTP modul.
type Handlers struct{ notes contract.Notes }

// New membuat Handlers di atas kontrak notes modul example.
func New(notes contract.Notes) *Handlers { return &Handlers{notes: notes} }

// Summary menjawab {"data":{"noteCount":n}}; error kontrak menjadi 500 tanpa detail.
func (h *Handlers) Summary(w http.ResponseWriter, r *http.Request) {
	n, err := h.notes.Count(r.Context())
	if err != nil {
		// Detail 5xx hanya di log (api-guidelines §8); respons tetap generik.
		httpx.RespondError(w, r, fmt.Errorf("gagal membaca jumlah notes: %w", err))
		return
	}
	httpx.WriteData(w, http.StatusOK, map[string]int64{"noteCount": n})
}
