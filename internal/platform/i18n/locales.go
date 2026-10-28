// Package i18n adalah runtime terjemahan platform (PRD-000 FR-25): registri bahasa, katalog per
// namespace, resolusi dengan rantai fallback, negosiasi bahasa, dan API katalog. Parser ICU, perataan
// katalog, dan penerjemah terikat context ada di erpora/sdk/i18n.
package i18n

import (
	_ "embed"

	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	"golang.org/x/text/language"
)

//go:embed locales.json
var embeddedLocales []byte

// maxFallbackDepth adalah jumlah lompatan fallback maksimal dari satu bahasa sampai akar rantainya.
const maxFallbackDepth = 4

var localeCodePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

// LocaleInfo adalah satu entri registri bahasa. Partial menandai bahasa yang sengaja belum lengkap
// (kunci hilang jatuh ke fallback tanpa peringatan; Requirement 8.4).
type LocaleInfo struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Fallback string `json:"fallback,omitempty"`
	Partial  bool   `json:"partial,omitempty"`
}

// Registry adalah registri bahasa yang didukung (Requirement 3.5). Fallback ditentukan hanya dari
// registri di repo (3.9). Nilainya immutable setelah ParseRegistry.
type Registry struct {
	Default string       `json:"default"`
	Locales []LocaleInfo `json:"locales"`

	byCode  map[string]LocaleInfo
	matcher language.Matcher // dibangun ParseRegistry; indeks cocok = indeks order
	order   []LocaleInfo     // Locales dengan bahasa default di indeks 0
}

// Load membaca registri bawaan (locales.json yang di-embed).
func Load() (*Registry, error) { return ParseRegistry(embeddedLocales) }

// ParseRegistry membaca dan memvalidasi registri: kode unik berpola ^[a-z]{2,3}(-[A-Z]{2})?$, fallback
// menunjuk kode terdaftar, tanpa siklus, kedalaman <= 4, dan bahasa default tanpa fallback.
func ParseRegistry(data []byte) (*Registry, error) {
	var r Registry
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("registri bahasa: json tidak valid: %w", err)
	}
	r.byCode = make(map[string]LocaleInfo, len(r.Locales))
	for _, l := range r.Locales {
		if !localeCodePattern.MatchString(l.Code) {
			return nil, fmt.Errorf("registri bahasa: kode %q tidak sesuai pola %s", l.Code, localeCodePattern)
		}
		if l.Name == "" {
			return nil, fmt.Errorf("registri bahasa: name kosong untuk %q", l.Code)
		}
		if _, dup := r.byCode[l.Code]; dup {
			return nil, fmt.Errorf("registri bahasa: kode %q ganda", l.Code)
		}
		r.byCode[l.Code] = l
	}
	def, ok := r.byCode[r.Default]
	if r.Default == "" || !ok {
		return nil, fmt.Errorf("registri bahasa: default %q harus berupa kode terdaftar", r.Default)
	}
	if def.Fallback != "" {
		return nil, fmt.Errorf("registri bahasa: bahasa default %q tidak boleh punya fallback", r.Default)
	}
	for _, l := range r.Locales {
		if l.Fallback != "" {
			if _, ok := r.byCode[l.Fallback]; !ok {
				return nil, fmt.Errorf("registri bahasa: fallback %q milik %q tidak terdaftar", l.Fallback, l.Code)
			}
		}
		hops, seen := 0, map[string]bool{l.Code: true}
		for cur := l; cur.Fallback != ""; {
			if seen[cur.Fallback] {
				return nil, fmt.Errorf("registri bahasa: siklus fallback pada %q", l.Code)
			}
			seen[cur.Fallback] = true
			cur = r.byCode[cur.Fallback]
			if hops++; hops > maxFallbackDepth {
				return nil, fmt.Errorf("registri bahasa: kedalaman fallback %q melebihi %d", l.Code, maxFallbackDepth)
			}
		}
	}
	tags := make([]language.Tag, len(r.Locales))
	for i, l := range r.Locales {
		tags[i] = language.Make(l.Code)
	}
	// Bahasa default harus berada di indeks 0 agar menjadi jawaban saat tidak ada kecocokan.
	di := slices.IndexFunc(r.Locales, func(l LocaleInfo) bool { return l.Code == r.Default })
	tags[0], tags[di] = tags[di], tags[0]
	r.order = slices.Clone(r.Locales)
	r.order[0], r.order[di] = r.order[di], r.order[0]
	r.matcher = language.NewMatcher(tags)
	return &r, nil
}

// Has melaporkan apakah code terdaftar.
func (r *Registry) Has(code string) bool { _, ok := r.byCode[code]; return ok }

// Info mengembalikan entri code.
func (r *Registry) Info(code string) (LocaleInfo, bool) { i, ok := r.byCode[code]; return i, ok }

// Codes mengembalikan kode bahasa menurut urutan registri.
func (r *Registry) Codes() []string {
	out := make([]string, len(r.Locales))
	for i, l := range r.Locales {
		out[i] = l.Code
	}
	return out
}

// Chain mengembalikan urutan pencarian untuk code: code, fallback-nya, dan seterusnya, selalu diakhiri
// bahasa default. Kode yang tidak terdaftar hanya menghasilkan [default]. Berhingga, tanpa duplikat.
func (r *Registry) Chain(code string) []string {
	var out []string
	for cur, ok := r.byCode[code]; ok; cur, ok = r.byCode[cur.Fallback] {
		if slices.Contains(out, cur.Code) {
			break
		}
		out = append(out, cur.Code)
		if cur.Fallback == "" {
			break
		}
	}
	if len(out) == 0 || out[len(out)-1] != r.Default {
		out = append(out, r.Default)
	}
	return out
}
