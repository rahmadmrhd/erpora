package mfa

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestGenerateRecoveryCodes(t *testing.T) {
	codes, err := generateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != RecoveryCodeCount || RecoveryCodeCount != 10 {
		t.Fatalf("jumlah = %d, want 10", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Errorf("kode ganda: %s", c)
		}
		seen[c] = true
		n, ok := normalizeRecoveryCode(c)
		if !ok || len(n) != 16 {
			t.Errorf("kode %q tidak lolos normalisasi sendiri: %q %v", c, n, ok)
		}
		if strings.Count(c, "-") != 3 || len(c) != 19 {
			t.Errorf("format kode %q", c)
		}
	}
}

func TestNormalizeRecoveryCode(t *testing.T) {
	canon, ok := normalizeRecoveryCode("ABCD-EFGH-IJKL-MNOP")
	if !ok || canon != "ABCDEFGHIJKLMNOP" {
		t.Fatalf("kanonik = %q, %v", canon, ok)
	}
	for _, in := range []string{"abcd-efgh-ijkl-mnop", "ABCDEFGHIJKLMNOP", "abcd efgh ijkl mnop", " abcd-EFGH-ijkl-MNOP ", "AB-CD-EF-GH-IJ-KL-MN-OP"} {
		got, ok := normalizeRecoveryCode(in)
		if !ok || got != canon {
			t.Errorf("%q -> %q, %v, want %q", in, got, ok, canon)
		}
	}
	bad := []string{"", "ABCD-EFGH-IJKL-MNO", "ABCD-EFGH-IJKL-MNOPQ", "ABCD-EFGH-IJKL-MN0P" /* 0 bukan base32 */, "ABCD-EFGH-IJKL-MN1P",
		"ＡＢＣＤ-EFGH-IJKL-MNOP", "ABCD-EFGH-IJKL-MNO\x00", strings.Repeat("A", 100000), "ABCD_EFGH_IJKL_MNOP"}
	for _, in := range bad {
		if got, ok := normalizeRecoveryCode(in); ok {
			t.Errorf("%q diterima sebagai %q", in, got)
		}
	}
}

func TestHashRecoveryCodeBindsUser(t *testing.T) {
	a := hashRecoveryCode("user-a", "ABCDEFGHIJKLMNOP")
	b := hashRecoveryCode("user-b", "ABCDEFGHIJKLMNOP")
	if len(a) != sha256.Size || string(a) == string(b) {
		t.Fatal("hash harus 32 byte dan terikat pada pengguna")
	}
	if string(a) != string(hashRecoveryCode("user-a", "ABCDEFGHIJKLMNOP")) {
		t.Fatal("hash tidak deterministik")
	}
}
