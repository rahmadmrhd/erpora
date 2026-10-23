// Package httpapi berisi handler HTTP modul exampleaddon. Foldernya bernama
// http, tetapi package dinamai httpapi agar tidak bentrok dengan net/http.
package httpapi

import (
	"encoding/json"
	"net/http"

	"erpora/modules/example/contract"
)

// Handlers menampung handler HTTP modul.
type Handlers struct{ notes contract.Notes }

// New membuat Handlers di atas kontrak notes modul example.
func New(notes contract.Notes) *Handlers { return &Handlers{notes: notes} }

// Summary menjawab {"data":{"noteCount":n}}; error kontrak menjadi 500 tanpa detail.
func (h *Handlers) Summary(w http.ResponseWriter, r *http.Request) {
	n, err := h.notes.Count(r.Context())
	if err != nil {
		body := map[string]any{"code": "internal_error", "message": "terjadi kesalahan pada server"}
		if id := w.Header().Get("X-Request-Id"); id != "" {
			body["requestId"] = id
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": body})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]int64{"noteCount": n}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
