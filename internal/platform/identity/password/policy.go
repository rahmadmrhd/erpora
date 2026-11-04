package password

import (
	_ "embed" // //go:embed untuk daftar sandi umum
	"strings"
	"unicode/utf8"
)

//go:generate go run gencommon.go

// MinPasswordLength adalah panjang minimum sandi dalam karakter (rune), PRD-001 FR-101.
const MinPasswordLength = 12

// ViolationCode mengidentifikasi pelanggaran kebijakan sandi. Kode stabil untuk pemetaan ke pesan i18n.
type ViolationCode string

const (
	ViolationTooShort ViolationCode = "too_short" // kurang dari MinPasswordLength karakter
	ViolationTooLong  ViolationCode = "too_long"  // lebih dari MaxPasswordBytes byte
	ViolationCommon   ViolationCode = "common"    // ada di daftar sandi umum
)

// Violation adalah satu pelanggaran kebijakan. Limit memuat batas yang dilanggar (panjang minimum untuk too_short,
// byte maksimum untuk too_long, 0 untuk common) agar pesan dapat menyebut angkanya. Tidak pernah memuat sandi.
type Violation struct {
	Code  ViolationCode
	Limit int
}

//go:embed commonpasswords.txt
var commonPasswordsRaw string

// commonPasswords adalah himpunan sandi umum dalam memori (huruf kecil).
var commonPasswords = loadCommonPasswords(commonPasswordsRaw)

func loadCommonPasswords(raw string) map[string]struct{} {
	set := make(map[string]struct{}, strings.Count(raw, "\n"))
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = struct{}{}
	}
	return set
}

func commonPasswordCount() int { return len(commonPasswords) }

// CheckPolicy memeriksa sandi baru terhadap kebijakan FR-101: panjang minimum 12 karakter (rune pasca-NFKC), maksimum
// 1024 byte (masukan mentah), dan tidak ada di daftar sandi umum (NFKC lalu huruf kecil). Tanpa aturan komposisi. Hasil kosong
// berarti sandi memenuhi kebijakan. Sandi lebih dari 1024 byte hanya menghasilkan too_long (tidak diperiksa lebih jauh).
func CheckPolicy(plain string) []Violation {
	if len(plain) > MaxPasswordBytes {
		return []Violation{{Code: ViolationTooLong, Limit: MaxPasswordBytes}}
	}
	// Batas byte diperiksa pada masukan MENTAH di atas (sebelum normalisasi, mencegah alokasi besar). Panjang minimum
	// dihitung dalam rune pasca-NFKC (yang dikirim ke hash adalah bentuk ini) dan daftar umum dicari pada NFKC + huruf kecil.
	normalized := normalize(plain)
	if utf8.RuneCountInString(normalized) < MinPasswordLength {
		return []Violation{{Code: ViolationTooShort, Limit: MinPasswordLength}}
	}
	if _, common := commonPasswords[strings.ToLower(normalized)]; common {
		return []Violation{{Code: ViolationCommon}}
	}
	return nil
}
