package mfa

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"
)

const (
	// RecoveryCodeCount adalah jumlah recovery code per set (FR-107).
	RecoveryCodeCount = 10

	recoveryBytes   = 10 // 80 bit entropi per kode
	recoveryChars   = 16 // base32 tanpa padding dari 10 byte
	recoveryGroup   = 4
	recoveryHashTag = "erpora.mfa.recovery.v1"
)

var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// generateRecoveryCodes membuat RecoveryCodeCount kode acak (crypto/rand) berformat XXXX-XXXX-XXXX-XXXX, unik dalam set.
func generateRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, RecoveryCodeCount)
	seen := make(map[string]struct{}, RecoveryCodeCount)
	for len(codes) < RecoveryCodeCount {
		var b [recoveryBytes]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, fmt.Errorf("mfa: bangkitkan recovery code: %w", err)
		}
		raw := recoveryEncoding.EncodeToString(b[:]) // 16 karakter A-Z2-7
		if _, dup := seen[raw]; dup {
			continue
		}
		seen[raw] = struct{}{}
		parts := make([]string, 0, recoveryChars/recoveryGroup)
		for i := 0; i < recoveryChars; i += recoveryGroup {
			parts = append(parts, raw[i:i+recoveryGroup])
		}
		codes = append(codes, strings.Join(parts, "-"))
	}
	return codes, nil
}

// normalizeRecoveryCode mengubah masukan pengguna ke bentuk kanonik 16 karakter huruf besar base32: huruf besar
// dan kecil, tanda hubung, dan spasi diabaikan. Selain itu (karakter non-ASCII, di luar A-Z2-7, panjang salah)
// ditolak (ok false) tanpa panik; pemanggil tetap mencatatnya sebagai kegagalan.
func normalizeRecoveryCode(in string) (string, bool) {
	if len(in) > 64 {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < len(in); i++ {
		c := in[i]
		switch {
		case c == '-' || c == ' ':
			continue
		case c >= 'a' && c <= 'z':
			c -= 'a' - 'A'
		}
		if (c < 'A' || c > 'Z') && (c < '2' || c > '7') {
			return "", false
		}
		b.WriteByte(c)
	}
	if b.Len() != recoveryChars {
		return "", false
	}
	return b.String(), true
}

// hashRecoveryCode menghitung SHA-256 atas kode kanonik, dengan pemisah domain dan userID agar kode identik milik
// dua pengguna tidak menghasilkan hash yang sama. SHA-256 cukup (bukan argon2id) karena kode berentropi 80 bit
// acak dan hash dicari lewat indeks (UNIQUE user_id, code_hash); tidak ada sandi manusia yang ditebak offline.
func hashRecoveryCode(userID, canonical string) []byte {
	h := sha256.New()
	h.Write([]byte(recoveryHashTag))
	h.Write([]byte{0})
	h.Write([]byte(userID))
	h.Write([]byte{0})
	h.Write([]byte(canonical))
	return h.Sum(nil)
}
