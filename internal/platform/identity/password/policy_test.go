package password

import (
	"bufio"
	"strings"
	"testing"
)

func codes(vs []Violation) []ViolationCode {
	out := make([]ViolationCode, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Code)
	}
	return out
}

func has(vs []Violation, c ViolationCode) bool {
	for _, v := range vs {
		if v.Code == c {
			return true
		}
	}
	return false
}

func TestCheckPolicyRejects(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		want ViolationCode
	}{
		{"daftar umum", "password1234", ViolationCommon},
		{"daftar umum huruf besar (normalisasi)", "PassWord1234", ViolationCommon},
		{"daftar umum Indonesia", "bismillah123", ViolationCommon},
		{"pendek", "short", ViolationTooShort},
		{"kosong", "", ViolationTooShort},
		{"11 karakter", "abcdefghijk", ViolationTooShort},
		{"1025 byte", strings.Repeat("a", 1025), ViolationTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if vs := CheckPolicy(c.pw); !has(vs, c.want) {
				t.Fatalf("CheckPolicy(%q) = %v, want memuat %q", truncate(c.pw), codes(vs), c.want)
			}
		})
	}
}

func truncate(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}

func TestCheckPolicyAccepts(t *testing.T) {
	cases := map[string]string{
		"12 karakter acak":               "kuda-hijau-77",
		"tepat 12":                       "zxkqvmwpltjb",
		"tanpa aturan komposisi (huruf)": "qzjxwvkpmbrt",
		"tanpa angka/simbol/huruf besar": "sampulbukuitu",
		"kalimat sandi":                  "tiga burung hantu minum kopi",
		"unicode 12 rune":                "密码密码密码密码密码密码",
		"tepat 1024 byte":                strings.Repeat("a1b2", 256),
		"NUL di tengah":                  "abcdef\x00ghijkl",
	}
	for name, pw := range cases {
		if vs := CheckPolicy(pw); len(vs) != 0 {
			t.Errorf("%s: ditolak %v", name, codes(vs))
		}
	}
}

func TestCheckPolicyLengthCountsRunesNotBytes(t *testing.T) {
	// 11 rune CJK = 33 byte: harus ditolak karena panjang dihitung dalam karakter.
	if vs := CheckPolicy("密码密码密码密码密码密"); !has(vs, ViolationTooShort) {
		t.Fatalf("11 rune diterima: %v", codes(vs))
	}
}

func TestCheckPolicyMaxIsBytes(t *testing.T) {
	// 342 rune x 3 byte = 1026 byte > 1024.
	if vs := CheckPolicy(strings.Repeat("密", 342)); !has(vs, ViolationTooLong) {
		t.Fatalf("1026 byte diterima: %v", codes(vs))
	}
	// Sandi sangat panjang tidak memicu pencarian daftar maupun duplikat violation.
	vs := CheckPolicy(strings.Repeat("a", 1<<20))
	if len(vs) != 1 || vs[0].Code != ViolationTooLong {
		t.Fatalf("got %v", codes(vs))
	}
}

func TestViolationCarriesLimits(t *testing.T) {
	vs := CheckPolicy("short")
	if len(vs) != 1 || vs[0].Limit != MinPasswordLength {
		t.Fatalf("got %+v", vs)
	}
	vs = CheckPolicy(strings.Repeat("a", 1025))
	if len(vs) != 1 || vs[0].Limit != MaxPasswordBytes {
		t.Fatalf("got %+v", vs)
	}
}

func TestCommonPasswordListSane(t *testing.T) {
	if n := commonPasswordCount(); n < 3000 {
		t.Fatalf("daftar sandi umum hanya %d entri, minimal 3000", n)
	}
	// Setiap entri efektif harus sudah dinormalisasi huruf kecil dan tidak mengandung spasi tepi.
	for p := range commonPasswords {
		if p != strings.ToLower(p) || p != strings.TrimSpace(p) || p == "" {
			t.Fatalf("entri tidak ternormalisasi: %q", p)
		}
	}
	for _, must := range []string{"password1234", "qwertyuiop12", "123456789012", "iloveyou1234", "sayangku1234"} {
		if _, ok := commonPasswords[must]; !ok {
			t.Errorf("daftar tidak memuat %q", must)
		}
	}
}

func TestCommonPasswordsFileFormat(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader(commonPasswordsRaw))
	headerSeen := false
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			headerSeen = true
			continue
		}
		if line != strings.TrimSpace(line) {
			t.Fatalf("spasi tepi pada baris %q", line)
		}
	}
	if !headerSeen {
		t.Fatal("berkas daftar harus memuat komentar asal/lisensi (#)")
	}
}
