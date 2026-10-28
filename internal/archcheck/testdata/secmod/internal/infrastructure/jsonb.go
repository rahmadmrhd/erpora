package infrastructure

import "encoding/json"

// Payload jsonb milik adapter: json.Unmarshal di dalam paket infrastructure sah (0 pelanggaran).
func loadModel(raw []byte) (NoteModel, error) {
	var m NoteModel
	err := json.Unmarshal(raw, &m)
	return m, err
}
