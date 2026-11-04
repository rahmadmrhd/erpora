package authz

import (
	"errors"
	"sort"
	"strings"
)

var (
	// ErrNotFound: record tidak ada atau di luar scope pemanggil (HTTP 404 not_found; api-guidelines §8).
	// Sengaja tidak membedakan "tidak ada" dari "di luar scope" agar keberadaan record tidak terungkap.
	ErrNotFound = errors.New("authz: not found")
	// ErrForbidden: pemanggil tidak punya permission aksi (HTTP 403 permission_denied).
	ErrForbidden = errors.New("authz: forbidden")
	// ErrUnauthenticated: tidak ada principal dan tidak ada hak sistem di context (HTTP 401 unauthenticated).
	ErrUnauthenticated = errors.New("authz: unauthenticated")
	// ErrUnsupportedRecord: Check menerima nilai yang bukan Record maupun map[string]any. Kesalahan
	// pemrograman, bukan kegagalan otorisasi: ditolak (gagal tertutup) dan tidak dipetakan ke 4xx oleh
	// httpx.WriteAuthzError sehingga handler menjawab 500.
	ErrUnsupportedRecord = errors.New("authz: tipe record tidak didukung (wajib authz.Record atau map[string]any)")
)

// FieldForbiddenError: akses ke field terlarang (HTTP 403 field_forbidden): menulis field tanpa akses
// write, atau memfilter/mengurutkan/mengelompokkan/memilih field berakses none (FR-112). Fields memuat
// nama field sebagaimana diberikan pemanggil (mis. camelCase JSON), tanpa duplikat, terurut.
type FieldForbiddenError struct {
	Resource string
	Fields   []string
}

// Error mengimplementasi error.
func (e *FieldForbiddenError) Error() string {
	return "authz: field_forbidden: " + e.Resource + ": " + strings.Join(e.Fields, ", ")
}

// newFieldForbidden membuat galat dengan Fields dideduplikasi dan diurutkan.
func newFieldForbidden(resource string, fields []string) *FieldForbiddenError {
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if _, dup := seen[f]; dup {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	sort.Strings(out)
	return &FieldForbiddenError{Resource: resource, Fields: out}
}
