package lifecycle

import (
	"context"
	"strconv"
	"strings"

	"erpora/internal/platform/tenancy"
)

// MatchIfMatch mengevaluasi nilai header If-Match terhadap revision tenant sebagai ETag (RFC 9110 section 13.1.1):
// If-Match = "*" / #entity-tag. "*" sah hanya bila seluruh header persis "*" (tidak boleh dicampur dengan tag);
// selain itu daftar entity-tag dipisah koma, tiap tag harus diikuti pemisah atau akhir nilai. Perbandingan kuat,
// sehingga tag lemah (W/"...") tidak pernah cocok. Nilai kosong atau tidak terurai tidak cocok.
func MatchIfMatch(header string, rev int64) bool {
	want := `"` + strconv.FormatInt(rev, 10) + `"`
	s := strings.Trim(header, " \t")
	if s == "*" {
		return true
	}
	matched := false
	for s != "" {
		s = strings.TrimLeft(s, " \t,")
		if s == "" {
			break
		}
		weak := strings.HasPrefix(s, `W/"`)
		start := 0
		if weak {
			start = 2
		}
		if s[start] != '"' {
			return false
		}
		end := strings.IndexByte(s[start+1:], '"')
		if end < 0 {
			return false
		}
		tag := s[start : start+end+2]
		s = s[start+end+2:]
		// Setelah entity-tag wajib ada pemisah (OWS lalu koma) atau akhir nilai.
		rest := strings.TrimLeft(s, " \t")
		if rest != "" && rest[0] != ',' {
			return false
		}
		if !weak && tag == want {
			matched = true
		}
	}
	return matched
}

// checkIfMatch memeriksa prasyarat If-Match terhadap revision tenant. Dipanggil di dalam advisory lock.
func (s *Service) checkIfMatch(ctx context.Context, t tenancy.TenantID, ifMatch string) error {
	if ifMatch == "" {
		return nil
	}
	rev, err := s.d.Store.Revision(ctx, t)
	if err != nil {
		return err
	}
	if !MatchIfMatch(ifMatch, rev) {
		return &Error{Code: CodeVersionMismatch, Revision: rev, Message: "If-Match tidak cocok dengan revision status modul saat ini"}
	}
	return nil
}
