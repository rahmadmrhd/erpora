package desks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
)

const (
	maxConfigBytes = 8 << 10
	maxConfigIDs   = 100
)

var configIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// Config adalah isi kolom config timpaan Desk: id menu yang disembunyikan dan urutan menu tingkat atas. Dashboard
// tidak pernah terpengaruh karena disisipkan setelah merge (R12.5: tidak dapat disembunyikan atau dipindah).
type Config struct {
	Hidden []string `json:"hidden,omitempty"`
	Order  []string `json:"order,omitempty"`
}

// ParseConfig memvalidasi dan mengurai config timpaan secara ketat: satu objek JSON (tanpa data setelahnya),
// hanya properti "hidden" dan "order" (tanpa kunci ganda), masing-masing array paling banyak 100 id yang cocok
// ^[a-z][a-z0-9-]{0,39}$, ukuran paling banyak 8 KiB. Config tidak tepercaya: baris rusak ditolak, bukan dipakai.
func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	if len(data) > maxConfigBytes {
		return cfg, fmt.Errorf("desks: config melebihi %d byte", maxConfigBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return cfg, errors.New("desks: config harus berupa objek JSON")
	}
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return Config{}, fmt.Errorf("desks: config rusak: %w", err)
		}
		key, _ := keyTok.(string)
		if seen[key] {
			return Config{}, fmt.Errorf("desks: config memuat kunci ganda %q", key)
		}
		seen[key] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return Config{}, fmt.Errorf("desks: config rusak: %w", err)
		}
		list, err := parseIDs(key, raw)
		if err != nil {
			return Config{}, err
		}
		if key == "hidden" {
			cfg.Hidden = list
		} else {
			cfg.Order = list
		}
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return Config{}, errors.New("desks: config rusak: objek tidak tertutup")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("desks: config memuat data setelah objek")
	}
	return cfg, nil
}

func parseIDs(key string, raw json.RawMessage) ([]string, error) {
	if key != "hidden" && key != "order" {
		return nil, fmt.Errorf("desks: config memuat properti tak dikenal %q", key)
	}
	if len(raw) == 0 || raw[0] != '[' {
		return nil, fmt.Errorf("desks: config.%s harus berupa array", key)
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("desks: config.%s harus array string: %w", key, err)
	}
	if len(list) > maxConfigIDs {
		return nil, fmt.Errorf("desks: config.%s melebihi %d id", key, maxConfigIDs)
	}
	for _, id := range list {
		if !configIDPattern.MatchString(id) {
			return nil, fmt.Errorf("desks: config.%s memuat id tidak sah %q", key, id)
		}
	}
	return list, nil
}

// apply menerapkan timpaan pada menu yang sudah disaring izin: id di Hidden (item, grup, atau anak grup) dibuang
// dan grup yang menjadi kosong dibuang; Order mengurutkan menu tingkat atas (id tidak dikenal atau ganda
// diabaikan, sisanya mengikuti urutan semula).
func (c Config) apply(menus []Menu) []Menu {
	hidden := make(map[string]bool, len(c.Hidden))
	for _, id := range c.Hidden {
		hidden[id] = true
	}
	var kept []Menu
	for _, m := range menus {
		if hidden[m.ID] {
			continue
		}
		if m.Type == TypeGroup {
			var kids []Menu
			for _, k := range m.Children {
				if !hidden[k.ID] {
					kids = append(kids, k)
				}
			}
			if len(kids) == 0 {
				continue
			}
			m.Children = kids
		}
		kept = append(kept, m)
	}
	if len(c.Order) == 0 {
		return kept
	}
	used := make([]bool, len(kept))
	out := make([]Menu, 0, len(kept))
	for _, id := range c.Order {
		if i := slices.IndexFunc(kept, func(m Menu) bool { return m.ID == id }); i >= 0 && !used[i] {
			used[i] = true
			out = append(out, kept[i])
		}
	}
	for i, m := range kept {
		if !used[i] {
			out = append(out, m)
		}
	}
	return out
}
