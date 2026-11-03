package mfa

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Rahasia uji RFC 6238 Appendix B (SHA-1): ASCII "12345678901234567890".
var rfcSecret = []byte("12345678901234567890")

func TestRFC6238AppendixBVectors(t *testing.T) {
	vectors := []struct {
		unix int64
		code string // 8 digit, sesuai tabel RFC 6238
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, v := range vectors {
		tm := time.Unix(v.unix, 0).UTC()
		if got := codeDigits(rfcSecret, tm, 8); got != v.code {
			t.Errorf("t=%d: 8 digit = %s, want %s", v.unix, got, v.code)
		}
		// versi 6 digit adalah 8 digit mod 10^6 (truncation dinamis yang sama)
		want6 := v.code[2:]
		if got := (TOTP{}).Code(rfcSecret, tm); got != want6 {
			t.Errorf("t=%d: 6 digit = %s, want %s", v.unix, got, want6)
		}
	}
}

func TestRFC4226HOTPVectors(t *testing.T) {
	// RFC 4226 Appendix D (6 digit), counter 0..9.
	want := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	for c, w := range want {
		if got := hotp(rfcSecret, uint64(c), 6); got != w {
			t.Errorf("counter %d = %s, want %s", c, got, w)
		}
	}
}

func TestVerifyWindowAndAntiReplay(t *testing.T) {
	now := time.Unix(1234567890, 0).UTC() // step = 41152263
	step := now.Unix() / 30
	tt := TOTP{}
	at := func(s int64) string { return tt.Code(rfcSecret, time.Unix(s*30, 0).UTC()) }

	t.Run("langkah saat ini diterima dan mengembalikan step-nya", func(t *testing.T) {
		got, ok := tt.Verify(rfcSecret, at(step), now, 0)
		if !ok || got != step {
			t.Fatalf("Verify = %d, %v, want %d, true", got, ok, step)
		}
	})
	t.Run("drift plus minus satu langkah diterima", func(t *testing.T) {
		if got, ok := tt.Verify(rfcSecret, at(step-1), now, 0); !ok || got != step-1 {
			t.Errorf("step-1: %d, %v", got, ok)
		}
		if got, ok := tt.Verify(rfcSecret, at(step+1), now, 0); !ok || got != step+1 {
			t.Errorf("step+1: %d, %v", got, ok)
		}
	})
	t.Run("drift dua langkah ditolak", func(t *testing.T) {
		if _, ok := tt.Verify(rfcSecret, at(step-2), now, 0); ok {
			t.Error("step-2 diterima")
		}
		if _, ok := tt.Verify(rfcSecret, at(step+2), now, 0); ok {
			t.Error("step+2 diterima")
		}
	})
	t.Run("kode pada langkah yang sudah dipakai ditolak (replay)", func(t *testing.T) {
		if _, ok := tt.Verify(rfcSecret, at(step), now, step); ok {
			t.Error("kode sama pada step == lastStep diterima")
		}
	})
	t.Run("kode langkah sebelumnya dalam jendela yang <= lastStep ditolak", func(t *testing.T) {
		if _, ok := tt.Verify(rfcSecret, at(step-1), now, step); ok {
			t.Error("step-1 diterima setelah step dipakai")
		}
		if _, ok := tt.Verify(rfcSecret, at(step-1), now, step-1); ok {
			t.Error("step-1 == lastStep diterima")
		}
	})
	t.Run("kode langkah berikutnya setelah lastStep tetap diterima", func(t *testing.T) {
		if got, ok := tt.Verify(rfcSecret, at(step+1), now, step); !ok || got != step+1 {
			t.Errorf("step+1 setelah lastStep=step: %d, %v", got, ok)
		}
	})
	t.Run("kode salah ditolak", func(t *testing.T) {
		good := at(step)
		bad := fmt.Sprintf("%06d", (atoi(good)+1)%1000000)
		if _, ok := tt.Verify(rfcSecret, bad, now, 0); ok {
			t.Error("kode salah diterima")
		}
	})
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestVerifyRejectsMalformedInputWithoutPanic(t *testing.T) {
	now := time.Unix(1234567890, 0).UTC()
	tt := TOTP{}
	good := tt.Code(rfcSecret, now)
	cases := map[string]string{
		"kosong":           "",
		"terlalu pendek":   good[:5],
		"terlalu panjang":  good + "0",
		"huruf":            "abcdef",
		"campur":           good[:5] + "x",
		"unicode":          "１２３４５６", // angka lebar penuh
		"nul":              "\x00\x00\x00\x00\x00\x00",
		"sangat panjang":   strings.Repeat("1", 100000),
		"minus":            "-12345",
		"plus":             "+12345",
		"spasi di tepi":    " " + good[:5],
		"hex":              "0x1234",
		"newline":          good[:5] + "\n",
		"digit non-ASCII2": "١٢٣٤٥٦", // angka Arab-Indic
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := tt.Verify(rfcSecret, code, now, 0); ok {
				t.Errorf("%q diterima", code)
			}
		})
	}
	if _, ok := tt.Verify(nil, good, now, 0); ok {
		t.Error("secret kosong diterima")
	}
	if _, ok := tt.Verify(rfcSecret, good, time.Unix(-100000, 0), 0); ok {
		t.Error("waktu sebelum epoch diterima")
	}
}

func TestVerifyAcceptsGroupedInput(t *testing.T) {
	now := time.Unix(1234567890, 0).UTC()
	tt := TOTP{}
	good := tt.Code(rfcSecret, now)
	grouped := good[:3] + " " + good[3:]
	if _, ok := tt.Verify(rfcSecret, grouped, now, 0); !ok {
		t.Error("kode berspasi tengah (tampilan authenticator) ditolak")
	}
}

func TestCodeIsZeroPadded(t *testing.T) {
	// cari waktu dengan kode berawalan nol untuk memastikan padding
	for s := int64(0); s < 5000; s++ {
		c := TOTP{}.Code(rfcSecret, time.Unix(s*30, 0).UTC())
		if len(c) != 6 {
			t.Fatalf("panjang kode %q = %d", c, len(c))
		}
		if c[0] == '0' {
			return
		}
	}
	t.Fatal("tidak menemukan kode berawalan nol dalam 5000 langkah (periksa padding)")
}
