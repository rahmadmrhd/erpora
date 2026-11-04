package authz

// maxFieldNameBytes mengikuti batas identifier PostgreSQL yang dipakai penamaan metadata (63 byte).
const maxFieldNameBytes = 63

func isLowerASCII(c byte) bool { return c >= 'a' && c <= 'z' }
func isUpperASCII(c byte) bool { return c >= 'A' && c <= 'Z' }
func isDigitASCII(c byte) bool { return c >= '0' && c <= '9' }

// CanonicalField mengubah nama field ke bentuk manifest (snake_case). DTO JSON memakai camelCase
// (costPrice) sedangkan manifest memakai snake_case (cost_price); pencocokan grup field wajib lewat
// bentuk ini. Hasil identik dengan metadatatools/naming.ToSnake untuk setiap camelCase ketat
// (address2Line menjadi address2_line, pointX menjadi point_x); nama yang sudah snake_case dikembalikan
// apa adanya.
//
// Nama yang bukan keduanya (kosong, huruf kapital berurutan seperti taxID, karakter selain huruf ASCII,
// angka, dan garis bawah, campuran camelCase dengan garis bawah, awalan bukan huruf kecil, lebih dari 63
// byte) menghasilkan "" yang tidak pernah cocok dengan field manifest mana pun. FieldAccess memperlakukan
// "" sebagai akses none (gagal tertutup, NFR-106): nama yang tak dikenal tidak boleh jatuh ke grup
// default dan bocor.
func CanonicalField(name string) string {
	n := len(name)
	if n == 0 || n > maxFieldNameBytes || !isLowerASCII(name[0]) {
		return ""
	}
	hasUpper, hasUnderscore := false, false
	for i := 1; i < n; i++ {
		c := name[i]
		switch {
		case isLowerASCII(c) || isDigitASCII(c):
		case c == '_':
			hasUnderscore = true
		case isUpperASCII(c):
			hasUpper = true
			if i+1 < n && isUpperASCII(name[i+1]) {
				return ""
			}
		default:
			return ""
		}
	}
	if !hasUpper {
		return name
	}
	if hasUnderscore {
		return ""
	}
	out := make([]byte, 0, n+4)
	for i := 0; i < n; i++ {
		c := name[i]
		if isUpperASCII(c) {
			out = append(out, '_', c+('a'-'A'))
			continue
		}
		out = append(out, c)
	}
	if len(out) > maxFieldNameBytes {
		return ""
	}
	return string(out)
}
