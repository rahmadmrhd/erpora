package mail_test

import (
	"errors"
	"strings"
	"testing"

	"erpora/sdk/mail"
)

func validate(to, subject string) error {
	return mail.Validate(mail.Message{To: []string{to}, Subject: subject, Text: "t"})
}

// Keputusan pemilik produk (3 Nov 2026, M-3): validasi alamat ketat, hanya alamat polos ASCII.
func TestValidateRejectsStrictRecipientCases(t *testing.T) {
	cases := map[string]string{
		"domain literal IPv4":        "a@[127.0.0.1]",
		"domain literal IPv6":        "a@[IPv6:::1]",
		"domain tanpa titik":         "a@localhost",
		"domain satu huruf":          "a@x",
		"domain IPv4 telanjang":      "a@127.0.0.1",
		"domain titik akhir":         "a@example.com.",
		"domain label kosong":        "a@example..com",
		"domain awal titik":          "a@.example.com",
		"domain label diawali -":     "a@-example.com",
		"domain label diakhiri -":    "a@example-.com",
		"domain karakter ilegal":     "a@exam_ple.com",
		"domain label 64 byte":       "a@" + strings.Repeat("a", 64) + ".com",
		"non-ASCII lokal":            "ü@example.com",
		"non-ASCII domain (IDN)":     "a@exämple.com",
		"non-ASCII domain (CJK)":     "a@例え.jp",
		"U+2028 (Zl)":                "a\u2028@example.com",
		"U+2029 (Zp)":                "a\u2029@example.com",
		"U+200B (Cf)":                "a\u200b@example.com",
		"U+202E (Cf, RLO)":           "a@example.com\u202e",
		"U+FEFF (Cf)":                "\ufeffa@example.com",
		"U+00AD (Cf)":                "a@exa\u00adMple.com",
		"encoded-word lokal":         "=?utf-8?q?a?=@example.com",
		"encoded-word dengan nama":   "=?utf-8?q?Nama?= <a@example.com>",
		"lokal dikutip":              `"a b"@example.com`,
		"lokal titik ganda":          "a..b@example.com",
		"lokal diawali titik":        ".a@example.com",
		"lokal diakhiri titik":       "a.@example.com",
		"lokal 65 byte":              strings.Repeat("a", 65) + "@example.com",
		"alamat 255 byte":            strings.Repeat("a", 60) + "@" + longDomain(255-61),
		"komentar RFC 5322":          "a(komentar)@example.com",
		"route/ganda @":              "a@b@example.com",
		"nama tampilan + sudut":      "Nama <a@example.com>",
		"ruang di tengah":            "a b@example.com",
		"NUL":                        "a@example.com\x00",
		"CRLF":                       "a@example.com\r\nBcc: x@example.com",
		"tab":                        "a\t@example.com",
		"kontrol C1 (U+0085)":        "a\u0085@example.com",
		"lokal dengan kutip tunggal": "a'@example.com\x1b",
	}
	for name, to := range cases {
		if err := validate(to, "x"); !errors.Is(err, mail.ErrInvalidMessage) {
			t.Errorf("%s (%q): err = %v, want ErrInvalidMessage", name, to, err)
		}
	}
}

func TestValidateAcceptsPlainAddresses(t *testing.T) {
	ok := []string{
		"a@example.com",
		"b.c+d@example.org",
		"first_last-1@sub.example.co.id",
		"o'brien@example.com", // apostrof sah (atext)
		"a@xn--e1afmkfd.xn--p1ai",
		strings.Repeat("a", 64) + "@example.com", // tepat 64 byte
		"a@" + strings.Repeat("a", 63) + ".com",  // label tepat 63
		"a@a-b.example.com",
		"a@123.example.com",
		"a@" + longDomain(254-2), // alamat tepat 254 byte
	}
	for _, to := range ok {
		if err := validate(to, "x"); err != nil {
			t.Errorf("%q ditolak: %v", to, err)
		}
	}
}

func TestValidateRejectsDangerousSubjectCharacters(t *testing.T) {
	bad := map[string]string{
		"U+2028 (Zl)":     "a\u2028b",
		"U+2029 (Zp)":     "a\u2029b",
		"U+200B (Cf)":     "a\u200bb",
		"U+202E (Cf RLO)": "a\u202eb",
		"U+FEFF (Cf)":     "a\ufeffb",
		"NUL":             "a\x00b",
		"CR":              "a\rb",
		"LF":              "a\nb",
		"tab":             "a\tb",
		"DEL":             "a\x7fb",
		"C1":              "a\u0085b",
	}
	for name, subj := range bad {
		if err := validate("a@example.com", subj); !errors.Is(err, mail.ErrInvalidMessage) {
			t.Errorf("%s (%q): err = %v", name, subj, err)
		}
	}
	// Subjek non-ASCII biasa (teks, aksara lain, emoji) tetap sah; driver yang meng-encode header.
	for _, subj := range []string{"Reset kata sandi", "Pesan untuk Müller", "例え", "OK 👍"} {
		if err := validate("a@example.com", subj); err != nil {
			t.Errorf("subject %q ditolak: %v", subj, err)
		}
	}
}

// Panjang total alamat dibatasi 254 byte dan bagian lokal 64 byte (RFC 5321 4.5.3.1).
func TestValidateAddressLengthLimits(t *testing.T) {
	over := "a@" + longDomain(254-2+1) // 255 byte
	if len(over) != 255 {
		t.Fatalf("setup: len=%d", len(over))
	}
	if err := validate(over, "x"); !errors.Is(err, mail.ErrInvalidMessage) {
		t.Errorf("255 byte: err = %v", err)
	}
	exact := "a@" + longDomain(254-2)
	if len(exact) != 254 {
		t.Fatalf("setup: len=%d", len(exact))
	}
	if err := validate(exact, "x"); err != nil {
		t.Errorf("254 byte ditolak: %v", err)
	}
}

// longDomain membangun domain sah (label <= 63, ada titik, TLD "com") sepanjang tepat n byte.
func longDomain(n int) string {
	m := n - len(".com") // panjang bagian label di depan TLD
	var labels []string
	for m > 63 {
		labels = append(labels, strings.Repeat("a", 63))
		m -= 64 // 63 + titik pemisah
	}
	labels = append(labels, strings.Repeat("a", m))
	d := strings.Join(labels, ".") + ".com"
	if len(d) != n {
		panic("longDomain: panjang tidak tepat")
	}
	return d
}
