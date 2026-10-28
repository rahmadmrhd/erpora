package i18n

// Tabel karakter yang dipakai parser ICU. Definisinya mengikuti @formatjs/icu-messageformat-parser
// (rujukan paritas, design.md D-2) agar pesan yang sama dikenali sama di Go dan TypeScript.

// isPatternWhiteSpace melaporkan Unicode Pattern_White_Space.
func isPatternWhiteSpace(r rune) bool {
	switch {
	case r >= 0x09 && r <= 0x0d:
		return true
	case r == 0x20, r == 0x85, r == 0x200e, r == 0x200f, r == 0x2028, r == 0x2029:
		return true
	}
	return false
}

// isPatternSyntax melaporkan Unicode Pattern_Syntax. Penanda ini memutus pengenal (nama argumen,
// tipe argumen, kunci opsi): `{a-b}` bukan nama sah.
func isPatternSyntax(r rune) bool {
	switch {
	case r >= 0x21 && r <= 0x2f, r >= 0x3a && r <= 0x40, r >= 0x5b && r <= 0x5e, r == 0x60, r >= 0x7b && r <= 0x7e:
		return true
	case r >= 0xa1 && r <= 0xa7, r == 0xa9, r == 0xab, r == 0xac, r == 0xae, r == 0xb0, r == 0xb1, r == 0xb6,
		r == 0xbb, r == 0xbf, r == 0xd7, r == 0xf7:
		return true
	case r >= 0x2010 && r <= 0x2027, r >= 0x2030 && r <= 0x203e, r >= 0x2041 && r <= 0x2053,
		r >= 0x2055 && r <= 0x205e, r >= 0x2190 && r <= 0x245f, r >= 0x2500 && r <= 0x2775,
		r >= 0x2794 && r <= 0x2bff, r >= 0x2e00 && r <= 0x2e7f, r >= 0x3001 && r <= 0x3003,
		r >= 0x3008 && r <= 0x3020, r == 0x3030, r == 0xfd3e, r == 0xfd3f, r == 0xfe45, r == 0xfe46:
		return true
	}
	return false
}

// isIdentRune melaporkan apakah r boleh menjadi bagian pengenal.
func isIdentRune(r rune) bool { return !isPatternWhiteSpace(r) && !isPatternSyntax(r) }

// isAlpha melaporkan huruf ASCII; huruf pertama nama tag.
func isAlpha(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

// isTagNameRune melaporkan karakter yang boleh muncul di nama tag setelah huruf pertama.
func isTagNameRune(c rune) bool {
	switch {
	case c == '-', c == '.', c >= '0' && c <= '9', c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z',
		c == 0xb7, c >= 0xc0 && c <= 0xd6, c >= 0xd8 && c <= 0xf6, c >= 0xf8 && c <= 0x37d,
		c >= 0x37f && c <= 0x1fff, c >= 0x200c && c <= 0x200d, c >= 0x203f && c <= 0x2040,
		c >= 0x2070 && c <= 0x218f, c >= 0x2c00 && c <= 0x2fef, c >= 0x3001 && c <= 0xd7ff,
		c >= 0xf900 && c <= 0xfdcf, c >= 0xfdf0 && c <= 0xfffd, c >= 0x10000 && c <= 0xeffff:
		return true
	}
	return false
}
