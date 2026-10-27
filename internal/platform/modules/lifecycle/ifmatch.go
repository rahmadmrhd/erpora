package lifecycle

import (
	"context"
	"strconv"
	"strings"

	"erpora/internal/platform/tenancy"
)

// MatchIfMatch mengevaluasi nilai header If-Match terhadap revision tenant sebagai ETag (RFC 9110 section 13.1.1):
// "*" cocok dengan representasi yang ada; daftar entity-tag dipisah koma; perbandingan kuat, sehingga tag lemah
// (W/"...") tidak pernah cocok. Nilai kosong atau tidak terurai tidak cocok.
func MatchIfMatch(header string, rev int64) bool {
	want := `"` + strconv.FormatInt(rev, 10) + `"`
	s := strings.TrimSpace(header)
	for s != "" {
		s = strings.TrimLeft(s, " \t,")
		switch {
		case s == "":
			return false
		case s[0] == '*':
			return true
		case strings.HasPrefix(s, `W/"`):
			end := strings.IndexByte(s[3:], '"')
			if end < 0 {
				return false
			}
			s = s[3+end+1:]
		case s[0] == '"':
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				return false
			}
			if s[:end+2] == want {
				return true
			}
			s = s[end+2:]
		default:
			return false
		}
	}
	return false
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
