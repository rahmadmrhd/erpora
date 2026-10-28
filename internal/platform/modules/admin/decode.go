package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxBodyBytes membatasi body aksi (keputusan 20).
const maxBodyBytes = 4 << 10

// actionBody adalah body opsional POST .../install.
type actionBody struct {
	WithDeps bool `json:"withDeps"`
	Enable   bool `json:"enable"`
}

// decodeAction membaca body aksi dengan decoder ketat lokal: maksimal 4 KiB, DisallowUnknownFields, satu objek.
// Body kosong diterima (semua false). Diganti httpx.DecodeJSON di M2c.
func decodeAction(w http.ResponseWriter, r *http.Request) (actionBody, error) {
	var b actionBody
	if r.Body == nil || r.Body == http.NoBody {
		return b, nil
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		if errors.Is(err, io.EOF) {
			return actionBody{}, nil
		}
		return actionBody{}, fmt.Errorf("body tidak valid: %w", err)
	}
	if dec.More() {
		return actionBody{}, errors.New("body harus satu objek JSON")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return actionBody{}, errors.New("body harus satu objek JSON")
	}
	return b, nil
}
