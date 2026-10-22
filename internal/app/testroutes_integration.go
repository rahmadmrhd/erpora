//go:build integration

package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rahmadmrhd/erp-go/internal/platform/httpserver"
)

// registerTestRoutes menambah route lambat HANYA pada biner yang dibangun dengan tag `integration`
// (dipakai tests/integration untuk menguji graceful shutdown dengan request yang benar-benar sedang berjalan).
// Biner release/devtools biasa tidak memuat route ini.
func registerTestRoutes(r *httpserver.Router) {
	r.Handle("GET /__test/slow", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ms, _ := strconv.Atoi(req.URL.Query().Get("ms"))
		if ms <= 0 || ms > 20000 {
			ms = 1000
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
		httpserver.WriteData(w, http.StatusOK, map[string]string{"status": "slow-done"})
	}), httpserver.WithoutTenant())
}
