package infrastructure

import (
	"net/http"

	"erpora/sdk/httpx"
)

type localDTO struct{ A string }

// Fixture: httpx.DecodeJSON di paket infrastructure selalu melanggar (1 pelanggaran DECODE).
func handle(w http.ResponseWriter, r *http.Request) {
	_, _ = httpx.DecodeJSON[localDTO](w, r)
}
