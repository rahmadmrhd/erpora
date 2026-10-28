package i18n

import "io"

// Flat adalah katalog satu namespace dan bahasa: kunci lengkap bertitik menjadi sumber pesan.
type Flat map[string]string

// ParseCatalog membaca satu berkas katalog JSON bersarang (Requirement 1.4-1.7).
func ParseCatalog(r io.Reader, lim Limits) (Flat, []Issue) {
	return nil, []Issue{{Code: "not_implemented"}}
}
