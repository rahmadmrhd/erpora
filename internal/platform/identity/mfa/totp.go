// Package mfa menyediakan MFA TOTP (RFC 6238) dan recovery code untuk identitas lokal (PRD-001 FR-107,
// ADR-0013 §4 dan §6). TOTP ditulis sendiri dengan stdlib; anti-replay lewat last_used_step bersyarat atomik
// di store (step > last_used_step).
package mfa

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // HMAC-SHA-1 wajib untuk HOTP/TOTP (RFC 4226 §5, RFC 6238 §1.2) agar kompatibel dengan aplikasi authenticator; pengecualian disetujui pemilik produk 3 Nov 2026 (security-guidelines §3.4, ADR-0013 §6). Hanya di paket ini; tidak untuk hash sandi, tanda tangan, atau integritas data.
	"crypto/subtle"
	"encoding/binary"
	"strconv"
	"strings"
	"time"
)

const (
	// Digits adalah panjang kode TOTP (RFC 6238 default; didukung semua authenticator).
	Digits = 6
	// Period adalah panjang satu langkah waktu dalam detik (RFC 6238 default).
	Period = 30
	// Skew adalah jumlah langkah toleransi drift jam di kedua sisi (jendela plus minus satu langkah, ADR-0013 §6).
	Skew = 1

	maxCodeInputBytes = 32 // masukan lebih panjang ditolak sebelum diproses
)

// TOTP menghitung dan memverifikasi kode RFC 6238 (HMAC-SHA-1, 6 digit, 30 detik, jendela plus minus 1 langkah).
// Nilai nol siap dipakai dan tidak menyimpan keadaan.
type TOTP struct{}

// Code mengembalikan kode 6 digit untuk langkah waktu t. Waktu sebelum epoch Unix menghasilkan string kosong.
func (TOTP) Code(secret []byte, t time.Time) string {
	if t.Unix() < 0 {
		return ""
	}
	return codeDigits(secret, t, Digits)
}

// Verify memeriksa code terhadap langkah t-1, t, dan t+1 dan hanya menerima langkah yang LEBIH BESAR dari
// lastStep (anti-replay: kode yang sama pada langkah yang sama dan kode langkah sebelumnya dalam jendela yang
// sudah <= lastStep ditolak). Mengembalikan langkah yang cocok; pemanggil WAJIB menyimpannya secara atomik
// bersyarat (store.AdvanceMFAStep) dan menolak bila update itu kalah, karena Verify sendiri tidak menyimpan
// apa pun. Ketiga kandidat selalu dihitung dan dibandingkan dengan pembanding konstan-waktu. Masukan yang bukan
// tepat 6 digit ASCII (spasi diabaikan), secret kosong, atau waktu sebelum epoch ditolak tanpa panik.
func (TOTP) Verify(secret []byte, code string, t time.Time, lastStep int64) (step int64, ok bool) {
	digits, valid := cleanDigits(code)
	if !valid || len(secret) == 0 || t.Unix() < 0 {
		return 0, false
	}
	cur := t.Unix() / Period
	for s := cur + Skew; s >= cur-Skew; s-- { // menurun: bila bertabrakan, langkah tertinggi yang dipakai
		if s < 0 {
			continue
		}
		cand := hotp(secret, uint64(s), Digits)
		match := subtle.ConstantTimeCompare([]byte(cand), []byte(digits)) == 1
		if match && s > lastStep && !ok {
			step, ok = s, true
		}
	}
	return step, ok
}

// cleanDigits membuang spasi ASCII (authenticator menampilkan "123 456") lalu menuntut tepat Digits digit ASCII.
func cleanDigits(code string) (string, bool) {
	if len(code) > maxCodeInputBytes {
		return "", false
	}
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != Digits {
		return "", false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return "", false
		}
	}
	return code, true
}

// codeDigits adalah TOTP parametris atas jumlah digit (dipakai uji vektor RFC 6238 yang 8 digit).
func codeDigits(secret []byte, t time.Time, digits int) string {
	step := t.Unix() / Period
	if step < 0 {
		return ""
	}
	return hotp(secret, uint64(step), digits)
}

// hotp adalah HOTP RFC 4226 (HMAC-SHA-1 dan truncation dinamis §5.3), dengan padding nol di kiri.
func hotp(secret []byte, counter uint64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, secret) //nolint:gosec // lihat komentar impor: HMAC-SHA-1 khusus HOTP/TOTP (RFC 4226/6238)
	_, _ = mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := int(sum[len(sum)-1] & 0x0f)
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	s := strconv.FormatUint(uint64(bin%mod), 10)
	if len(s) < digits {
		s = strings.Repeat("0", digits-len(s)) + s
	}
	return s
}
