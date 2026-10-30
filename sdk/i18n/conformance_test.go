package i18n

import (
	"encoding/json"
	"os"
	"testing"
)

// conformanceFile adalah isi testdata/conformance.json. Berkas yang sama dibaca test TypeScript
// (@erpora/platform-web) sehingga keluaran Go dan TS dibuktikan sama (Requirement 9; P5).
type conformanceFile struct {
	Version int               `json:"version"`
	Cases   []conformanceCase `json:"cases"`
}

type conformanceCase struct {
	Name         string         `json:"name"`
	Locale       string         `json:"locale"`
	Message      string         `json:"message"`
	Args         map[string]any `json:"args"`
	Expected     *string        `json:"expected"`
	ExpectError  bool           `json:"expectError"`
	GoOnly       bool           `json:"goOnly"`
	GoOnlyReason string         `json:"goOnlyReason"`
}

func loadConformance(t testing.TB) conformanceFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/conformance.json")
	if err != nil {
		t.Fatalf("baca fixture: %v", err)
	}
	var f conformanceFile
	// Bidang pembungkus ($comment, oracle, conventions) sengaja tidak dimodelkan di sini.
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("urai fixture: %v", err)
	}
	return f
}

// TestConformanceFixtureShape menjaga bentuk fixture agar tidak rusak diam-diam.
func TestConformanceFixtureShape(t *testing.T) {
	f := loadConformance(t)
	if f.Version != 1 {
		t.Fatalf("version = %d, want 1", f.Version)
	}
	if len(f.Cases) < 40 {
		t.Fatalf("fixture hanya %d kasus, minimal 40 (tasks.md 2.2)", len(f.Cases))
	}
	seen := map[string]bool{}
	locales := map[string]bool{}
	var goOnly int
	for _, c := range f.Cases {
		if c.Name == "" || c.Locale == "" {
			t.Errorf("kasus tanpa name/locale: %+v", c)
		}
		if seen[c.Name] {
			t.Errorf("nama kasus ganda: %q", c.Name)
		}
		seen[c.Name] = true
		locales[c.Locale] = true
		if (c.Expected != nil) == c.ExpectError {
			t.Errorf("%q: isi tepat satu dari expected atau expectError", c.Name)
		}
		if c.GoOnly {
			goOnly++
			if c.GoOnlyReason == "" {
				t.Errorf("%q: goOnly wajib punya goOnlyReason", c.Name)
			}
		}
	}
	for _, l := range []string{"id", "en"} {
		if !locales[l] {
			t.Errorf("fixture wajib mencakup locale %q (Requirement 9.2)", l)
		}
	}
	if goOnly == 0 {
		t.Error("fixture harus memuat kasus goOnly (penolakan konstruksi tak didukung dan batas ukuran)")
	}
}

// TestConformance menjalankan seluruh kasus fixture lewat Compile + Format.
func TestConformance(t *testing.T) {
	f := loadConformance(t)
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			msg, err := Compile(c.Message, DefaultLimits())
			var got string
			if err == nil {
				got, err = msg.Format(Locale(c.Locale), Args(c.Args))
			}
			if c.ExpectError {
				if err == nil {
					t.Fatalf("Compile+Format(%q) = %q, want galat", c.Message, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Compile+Format(%q) galat: %v", c.Message, err)
			}
			if c.Expected == nil {
				t.Fatalf("kasus tanpa expected")
			}
			if got != *c.Expected {
				t.Errorf("Format(%q, %v) [%s] = %q, want %q", c.Message, c.Args, c.Locale, got, *c.Expected)
			}
		})
	}
}
