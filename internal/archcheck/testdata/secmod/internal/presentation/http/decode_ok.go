package httpapi

import (
	"net/http"

	"erpora/sdk/httpx"
)

type createNoteDTO struct{ Title string }

// Fixture: DTO khusus endpoint (0 pelanggaran).
func decodeOK(w http.ResponseWriter, r *http.Request) {
	_, _ = httpx.DecodeJSON[createNoteDTO](w, r)
}
