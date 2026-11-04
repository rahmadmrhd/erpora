package architecture

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"erpora/internal/archcheck"
)

// d3bPathRe menangkap path berkurung backtick yang berupa paket platform atau sdk (bukan pola modul).
var d3bPathRe = regexp.MustCompile("`((?:internal/platform|sdk)/[^`]*)`")

func lineWith(t *testing.T, file, marker string) string {
	t.Helper()
	b, err := os.ReadFile("../../" + file)
	if err != nil {
		t.Fatalf("baca %s: %v", file, err)
	}
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if strings.Contains(l, marker) {
			return l
		}
	}
	t.Fatalf("%s tidak memuat baris dengan penanda %q", file, marker)
	return ""
}

// TestD3bAllowlistDocsMatchRules menjaga daftar paket GORM (D3b) di CLAUDE.md dan PRD-000 §4.2
// identik dengan archcheck.GormAllowedPaths (sumber kebenaran: internal/archcheck/rules.go).
func TestD3bAllowlistDocsMatchRules(t *testing.T) {
	want := append([]string{}, archcheck.GormAllowedPaths...)
	sort.Strings(want)

	cases := []struct{ file, marker string }{
		{"CLAUDE.md", "GORM hanya di `*/internal/infrastructure`"},
		{"AGENTS.md", "GORM hanya di `*/internal/infrastructure`"},
		{"docs/prd/PRD-000-inisialisasi-project.md", "| D3b |"},
	}
	for _, c := range cases {
		line := lineWith(t, c.file, c.marker)
		var got []string
		for _, m := range d3bPathRe.FindAllStringSubmatch(line, -1) {
			got = append(got, m[1])
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: daftar D3b berbeda dari archcheck.GormAllowedPaths\n  dokumen: %v\n  rules.go: %v", c.file, got, want)
		}
	}
}
