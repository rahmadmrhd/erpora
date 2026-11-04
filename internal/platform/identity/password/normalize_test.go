package password

import (
	"strings"
	"testing"
	"time"
)

// Keputusan pemilik produk 3 Nov 2026: sandi dinormalisasi ke NFKC sebelum Hash, Verify, dan CheckPolicy.

func TestVerifyComposedAndDecomposedAreEquivalent(t *testing.T) {
	h := &Hasher{Params: testParams}
	composed := "café-sandi-panjang"    // é = U+00E9
	decomposed := "café-sandi-panjang" // e + U+0301
	if composed == decomposed {
		t.Fatal("bentuk uji harus berbeda secara byte")
	}
	phc, err := h.Hash(composed)
	if err != nil {
		t.Fatal(err)
	}
	for name, pw := range map[string]string{"komposisi": composed, "dekomposisi": decomposed} {
		ok, _, err := h.Verify(pw, phc)
		if err != nil || !ok {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}
	// Arah sebaliknya: hash dari bentuk dekomposisi diverifikasi dengan bentuk komposisi.
	phc2, err := h.Hash(decomposed)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _, err := h.Verify(composed, phc2); err != nil || !ok {
		t.Errorf("balik: ok=%v err=%v", ok, err)
	}
	if ok, _, _ := h.Verify("cafe-sandi-panjang", phc); ok {
		t.Error("e tanpa aksen tidak boleh cocok dengan é")
	}
}

func TestVerifyCompatibilityFormsFold(t *testing.T) {
	h := &Hasher{Params: testParams}
	cases := []struct{ name, stored, attempt string }{
		{"ligatur fi", "ﬁnancial-secret", "financial-secret"},
		{"angka lebar penuh", "pin１２３４５６-kuat", "pin123456-kuat"},
		{"huruf lebar penuh", "ｐａｓｓ-ｗｏｒｄ-tujuh", "pass-word-tujuh"},
		{"spasi tak-putus", "kata sandi panjang", "kata sandi panjang"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			phc, err := h.Hash(c.stored)
			if err != nil {
				t.Fatal(err)
			}
			if ok, _, err := h.Verify(c.attempt, phc); err != nil || !ok {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestNormalizationKeepsNULAndControlCharsDistinct(t *testing.T) {
	h := &Hasher{Params: testParams}
	for _, pw := range []string{"abc\x00def-ghi-jkl", "abc\x01def\x1fghi\x7f", "tab\there\nnewline\r"} {
		phc, err := h.Hash(pw)
		if err != nil {
			t.Fatal(err)
		}
		if ok, _, err := h.Verify(pw, phc); err != nil || !ok {
			t.Errorf("%q: ok=%v err=%v", pw, ok, err)
		}
		// NUL/kontrol tidak dibuang atau dipotong oleh normalisasi.
		if ok, _, _ := h.Verify(strings.NewReplacer("\x00", "", "\x01", "", "\x1f", "", "\x7f", "", "\t", "", "\n", "", "\r", "").Replace(pw), phc); ok {
			t.Errorf("%q: karakter kontrol hilang saat normalisasi", pw)
		}
	}
}

// Input mentah dibatasi 1024 byte SEBELUM normalisasi; ekspansi NFKC terburuk (U+FDFA, 3 byte menjadi 18 rune) tetap
// terbatas dan cepat.
func TestNormalizationDoesNotOpenDoS(t *testing.T) {
	h := &Hasher{Params: testParams}
	worst := strings.Repeat("ﷺ", MaxPasswordBytes/3) // 1023 byte mentah
	start := time.Now()
	phc, err := h.Hash(worst)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if ok, _, err := h.Verify(worst, phc); err != nil || !ok {
		t.Fatalf("Verify: ok=%v err=%v", ok, err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("terlalu lambat (%v)", d)
	}
	// 1025 byte mentah ditolak tanpa normalisasi, meski hasil normalisasinya pendek/panjang apa pun.
	tooLong := strings.Repeat("ﷺ", MaxPasswordBytes/3+1) // 1026 byte
	if _, err := h.Hash(tooLong); err != ErrPasswordTooLong {
		t.Errorf("Hash err = %v", err)
	}
	if ok, _, err := h.Verify(tooLong, phc); ok || err != nil {
		t.Errorf("Verify: ok=%v err=%v", ok, err)
	}
	if vs := CheckPolicy(tooLong); len(vs) != 1 || vs[0].Code != ViolationTooLong {
		t.Errorf("CheckPolicy = %v", vs)
	}
	// 1024 byte mentah yang pasca-NFKC sangat panjang tetap lolos kebijakan (batas ada pada masukan mentah).
	if vs := CheckPolicy(worst); len(vs) != 0 {
		t.Errorf("CheckPolicy(worst) = %v", codes(vs))
	}
}

func TestCheckPolicyMinLengthAfterNFKC(t *testing.T) {
	// 11 "é" dekomposisi = 22 code point mentah tetapi 11 rune pasca-NFKC: ditolak (terlalu pendek).
	if vs := CheckPolicy(strings.Repeat("é", 11)); !has(vs, ViolationTooShort) {
		t.Errorf("11 rune pasca-NFKC diterima: %v", codes(vs))
	}
	if vs := CheckPolicy(strings.Repeat("é", 12)); len(vs) != 0 {
		t.Errorf("12 rune pasca-NFKC ditolak: %v", codes(vs))
	}
	// Ligatur "ﬁ" (1 rune) menjadi 2 rune: 6 ligatur = 12 rune pasca-NFKC, lolos; 5 ligatur = 10, ditolak.
	if vs := CheckPolicy(strings.Repeat("ﬁ", 6)); len(vs) != 0 {
		t.Errorf("6 ligatur ditolak: %v", codes(vs))
	}
	if vs := CheckPolicy(strings.Repeat("ﬁ", 5)); !has(vs, ViolationTooShort) {
		t.Errorf("5 ligatur diterima: %v", codes(vs))
	}
}

func TestCheckPolicyCommonListAfterNFKCAndLowercase(t *testing.T) {
	for _, pw := range []string{
		"Ｐａｓｓword1234", // huruf lebar penuh + besar
		"PASSWORD1234", // huruf besar biasa
	} {
		if vs := CheckPolicy(pw); !has(vs, ViolationCommon) {
			t.Errorf("%q lolos daftar umum: %v", pw, codes(vs))
		}
	}
}

func TestDummyUsesSameNormalizationPath(t *testing.T) {
	d := NewDummy(testParams)
	phc := d.DummyHash()
	for _, pw := range []string{"café", "café", "ﬁnd", "１２３", "a\x00b"} {
		if ok, rehash, err := d.Verify(pw, phc); ok || rehash || err != nil {
			t.Errorf("%q: ok=%v rehash=%v err=%v", pw, ok, rehash, err)
		}
	}
}
