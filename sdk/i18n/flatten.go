package i18n

import (
	"strings"
)

// Perataan katalog (Requirement 1.5-1.7): jalur objek JSON digabung dengan '.' menjadi kunci lengkap.
// Segmen boleh memuat titik, sehingga dua jalur berbeda dapat menghasilkan kunci yang sama; tabrakan
// itu dideteksi lewat peta asal (kunci lengkap -> jalur).

// segmentProblem mengembalikan alasan segmen tidak sah, atau "" bila sah. Aturan Requirement 1.7:
// karakter [A-Za-z0-9_.-], tidak kosong, tidak diawali atau diakhiri titik, tanpa titik berurutan,
// dan tanpa "::" (pemisah namespace; sudah mustahil karena ':' tidak diizinkan).
func segmentProblem(s string) string {
	if s == "" {
		return "segmen kunci kosong"
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-'
		if !ok {
			return "segmen " + quoteForDetail(s) + " memuat karakter yang tidak diizinkan (hanya A-Z a-z 0-9 _ . -)"
		}
	}
	switch {
	case s[0] == '.' || s[len(s)-1] == '.':
		return "segmen " + quoteForDetail(s) + " diawali atau diakhiri titik"
	case strings.Contains(s, ".."):
		return "segmen " + quoteForDetail(s) + " memuat titik berurutan"
	}
	return ""
}

// pathString menulis jalur segmen sebagai /a.b/c; '/' tidak mungkin ada di segmen sah sehingga
// penulisannya tidak ambigu.
func pathString(segs []string) string {
	return "/" + strings.Join(segs, "/")
}
