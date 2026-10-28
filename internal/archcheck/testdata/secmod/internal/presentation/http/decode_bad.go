package httpapi

import (
	"encoding/json"
	"net/http"

	"erpora/modules/secfixture/internal/infrastructure"
	"erpora/sdk/httpx"
)

type wrapDTO struct{ N infrastructure.NoteModel }

// Fixture: tepat 4 pelanggaran DECODE di sini (+1 di handler_bad.go).
func decodeBad(w http.ResponseWriter, r *http.Request, b []byte) {
	_, _ = httpx.DecodeJSON[infrastructure.NoteModel](w, r) // 1
	var m infrastructure.NoteModel
	_ = json.Unmarshal(b, &m)                                         // 2
	_ = json.NewDecoder(r.Body).Decode(&[]infrastructure.NoteModel{}) // 3
	_, _ = httpx.DecodeJSON[wrapDTO](w, r)                            // 4
}
