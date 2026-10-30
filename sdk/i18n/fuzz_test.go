package i18n

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// fuzzSeeds memuat pesan fixture konformansi sebagai korpus awal.
func fuzzSeeds(f *testing.F) {
	f.Helper()
	for _, c := range loadConformance(f).Cases {
		f.Add(c.Message)
	}
	for _, s := range []string{
		"", "{", "}", "{{", "'", "''", "<", "</", "<b>", "{a, plural, other {#}}", "{a, select, other {}}",
		strings.Repeat("{a, select, other {", 8) + "x" + strings.Repeat("}}", 8),
		strings.Repeat("<b>", 8) + "x" + strings.Repeat("</b>", 8),
	} {
		f.Add(s)
	}
}

// P4 Format total dan terbatas: untuk pesan apa pun, Compile tidak panik dan menegakkan Limits;
// untuk pesan yang lolos, Format tidak panik, selesai dalam waktu terbatas, dan keluarannya tidak
// melebihi MaxFormatOutput.
func FuzzCompile(f *testing.F) {
	fuzzSeeds(f)
	lim := DefaultLimits()
	argSets := []Args{
		nil,
		{},
		{"n": 1, "a": "x", "g": "a", "x": 1.5, "0": -3},
		{"n": "abc", "a": 1e21, "g": "", "x": nil, "0": "1.0"},
		{"n": 1234567.891, "a": strings.Repeat("é", 1000), "g": "other", "x": true, "0": 0},
	}
	locales := []Locale{"id", "en", "en-GB", "ms", "", "zz", "ar", "ru"}
	f.Fuzz(func(t *testing.T, src string) {
		start := time.Now()
		m, err := Compile(src, lim)
		if err != nil {
			if _, ok := err.(*CompileError); !ok {
				t.Fatalf("Compile galat bukan *CompileError: %T %v", err, err)
			}
			return
		}
		if !utf8.ValidString(src) || utf8.RuneCountInString(src) > lim.MaxMessageLen {
			t.Fatalf("Compile menerima pesan di luar batas: len=%d", len(src))
		}
		if n := len(m.Params()); n > lim.MaxArgs+countTags(m) {
			t.Fatalf("Params %d melebihi MaxArgs", n)
		}
		for _, loc := range locales {
			for _, a := range argSets {
				out, err := m.Format(loc, a)
				if err == nil && len(out) > MaxFormatOutput {
					t.Fatalf("keluaran %d byte melebihi batas", len(out))
				}
			}
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("terlalu lama: %v untuk %q", d, src)
		}
	})
}

func countTags(m Message) int {
	n := 0
	for _, p := range m.Params() {
		if p.Kind == KindTag {
			n++
		}
	}
	return n
}

// FuzzParseCatalog memastikan pembaca katalog tidak panik dan tetap dalam batas untuk input apa pun.
func FuzzParseCatalog(f *testing.F) {
	for _, s := range []string{
		`{}`, `{"a":"x"}`, `{"a":{"b":"c"}}`, `{"a.b":{"c":"x"},"a":{"b.c":"y"}}`, `{"a":"x","a":"y"}`,
		`[]`, `{"a":1}`, `{"<b>":"x"}`, `{"k":"<script>"}`, `{"k":"{n, plural, other {#}}"}`, "\xef\xbb\xbf{}",
	} {
		f.Add(s)
	}
	lim := Limits{MaxFileBytes: 4096, MaxKeys: 50, MaxKeyLen: 40}
	f.Fuzz(func(t *testing.T, in string) {
		got, issues := ParseCatalog(strings.NewReader(in), lim)
		if len(issues) > maxIssues+1 {
			t.Fatalf("terlalu banyak issue: %d", len(issues))
		}
		if len(issues) > 0 && got != nil {
			t.Fatal("Flat harus nil bila ada Issue")
		}
		if len(got) > lim.MaxKeys {
			t.Fatalf("kunci %d melebihi batas", len(got))
		}
		for k := range got {
			if len(k) > lim.MaxKeyLen {
				t.Fatalf("kunci terlalu panjang: %q", k)
			}
		}
	})
}
