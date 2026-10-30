package i18n

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"erpora/internal/platform/i18n/catalog"
	uii18n "erpora/packages/ui/i18n"
	sdki18n "erpora/sdk/i18n"
)

func parseFS(t *testing.T, fsys fs.FS, locale string) sdki18n.Flat {
	t.Helper()
	f, err := fsys.Open(locale + ".json")
	if err != nil {
		t.Fatalf("buka %s.json: %v", locale, err)
	}
	defer func() { _ = f.Close() }()
	flat, issues := sdki18n.ParseCatalog(f, sdki18n.DefaultLimits())
	if len(issues) > 0 {
		t.Fatalf("%s.json: %v", locale, issues)
	}
	return flat
}

// Katalog bawaan platform dan ui harus valid, lengkap untuk setiap bahasa registri, dan paritas argumennya
// terjaga.
func TestBuiltinCatalogsValidAndInParity(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for ns, fsys := range map[string]fs.FS{"platform": catalog.FS(), "ui": uii18n.FS()} {
		base := parseFS(t, fsys, reg.Default)
		if len(base) == 0 {
			t.Fatalf("%s: katalog bahasa default kosong", ns)
		}
		for _, code := range reg.Codes() {
			flat := parseFS(t, fsys, code)
			for k, src := range base {
				other, ok := flat[k]
				if !ok {
					t.Errorf("%s/%s: kunci %q hilang", ns, code, k)
					continue
				}
				a, _ := sdki18n.Compile(src, sdki18n.DefaultLimits())
				b, _ := sdki18n.Compile(other, sdki18n.DefaultLimits())
				if !sdki18n.SameParams(a.Params(), b.Params()) {
					t.Errorf("%s/%s: argumen kunci %q berbeda dari bahasa default", ns, code, k)
				}
			}
			for k := range flat {
				if _, ok := base[k]; !ok {
					t.Errorf("%s/%s: kunci %q berlebih", ns, code, k)
				}
			}
		}
	}
}

// Setiap kode galat katalog api-guidelines §8 punya errors.<code> di katalog platform (Requirement 4.4).
func TestPlatformCatalogCoversApiGuidelinesErrorCodes(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/architecture/api-guidelines.md")
	if err != nil {
		t.Skipf("api-guidelines tidak terbaca: %v", err)
	}
	section := string(raw)
	if i := strings.Index(section, "## 8. Error"); i >= 0 {
		section = section[i:]
	}
	if i := strings.Index(section, "## 9."); i >= 0 {
		section = section[:i]
	}
	var codes []string
	for _, line := range strings.Split(section, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || strings.TrimSpace(cells[1]) == "" || strings.TrimSpace(cells[1])[0] < '4' || strings.TrimSpace(cells[1])[0] > '5' {
			continue
		}
		for _, p := range strings.Split(cells[2], ",") {
			p = strings.Trim(strings.TrimSpace(p), "`")
			if p != "" && !strings.ContainsAny(p, " ()") {
				codes = append(codes, p)
			}
		}
	}
	if len(codes) < 20 {
		t.Fatalf("hanya %d kode terbaca dari api-guidelines; parser tabel rusak?", len(codes))
	}
	flat := parseFS(t, catalog.FS(), "id")
	for _, c := range codes {
		if _, ok := flat["errors."+c]; !ok {
			t.Errorf("errors.%s tidak ada di katalog platform", c)
		}
	}
	if !slices.Contains(codes, "internal_error") {
		t.Error("parser tidak menemukan internal_error")
	}
}
