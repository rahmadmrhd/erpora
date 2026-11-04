package manifest

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"erpora/sdk/module"
)

type wantProblem struct{ code, field string }

func TestValidate_Fixtures(t *testing.T) {
	cases := []struct {
		file string
		want []wantProblem
	}{
		{"valid_full.json", nil},
		{"bad_version.json", []wantProblem{{"schema_invalid", "/version"}}},
		{"bad_key.json", []wantProblem{{"schema_invalid", "/key"}}},
		{"bad_constraint.json", []wantProblem{{"invalid_constraint", "/dependencies/0/version"}}},
		{"trust_untrusted.json", []wantProblem{{"trust_not_supported", "/trust"}}},
		{"missing_slug.json", []wantProblem{{"schema_invalid", "/slug"}}},
		{"bad_dependency_module.json", []wantProblem{{"schema_invalid", "/dependencies/0/module"}}},
		{"duplicate_dependency.json", []wantProblem{{"duplicate_dependency", "/dependencies/1/module"}}},
		{"unknown_field.json", []wantProblem{{"schema_invalid", "/routes"}}},
		// B-8 (Q32): goose_module_<slug> (13 + slug) harus muat di batas identifier PostgreSQL 63 byte -> slug maksimal 50.
		{"slug_too_long.json", []wantProblem{{"schema_invalid", "/slug"}}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			m := load(t, tc.file)
			assertProblems(t, Validate(m), tc.want)
		})
	}
}

func TestValidate_RawMissing(t *testing.T) {
	got := Validate(module.Manifest{Key: "acme.x"})
	if len(got) != 1 || got[0].Code != "raw_missing" || got[0].Module != "acme.x" {
		t.Fatalf("problems = %+v", got)
	}
}

func TestValidate_ReportsAllProblems(t *testing.T) {
	raw := []byte(`{
  "schema_version": "1", "key": "acme.example", "slug": "example", "name": "X",
  "version": "1.0", "publisher": {"id": "a", "name": "A"}, "trust": "untrusted",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": [], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"}
}`)
	m, err := module.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertProblems(t, Validate(m), []wantProblem{
		{"schema_invalid", "/version"},
		{"trust_not_supported", "/trust"},
	})
}

func load(t *testing.T, name string) module.Manifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "manifests", name))
	if err != nil {
		t.Fatal(err)
	}
	m, err := module.ParseManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func assertProblems(t *testing.T, got []Problem, want []wantProblem) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("jumlah problem = %d (%+v), diharapkan %d (%+v)", len(got), got, len(want), want)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g.Code == w.code && g.Field == w.field {
				found = true
			}
		}
		if !found {
			t.Errorf("problem %+v tidak ditemukan di %+v", w, got)
		}
	}
}

// M7: struct yang berbeda dari Raw harus dilaporkan.
func TestValidate_ManifestMismatch(t *testing.T) {
	m := load(t, "valid_full.json")
	m.Slug = "Bad/Slug"
	got := Validate(m)
	if len(got) != 1 || got[0].Code != "manifest_mismatch" {
		t.Fatalf("problems = %+v", got)
	}
}

func TestValidate_FieldGroupClassification(t *testing.T) {
	assertProblems(t, Validate(load(t, "valid_full.json")), nil)
	assertProblems(t, Validate(load(t, "bad_field_group_classification.json")),
		[]wantProblem{{"schema_invalid", "/resources/0/field_groups/0/classification"}})

	for _, class := range []string{"public", "internal", "confidential", "highly_confidential"} {
		t.Run(class, func(t *testing.T) {
			raw := []byte(`{
  "schema_version": "1", "key": "acme.example", "slug": "example", "name": "X",
  "version": "1.0.0", "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": [], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"},
  "resources": [{"key": "x.note", "field_groups": [{"key": "g", "classification": "` + class + `"}]}]
}`)
			m, err := module.ParseManifest(raw)
			if err != nil {
				t.Fatal(err)
			}
			assertProblems(t, Validate(m), nil)
		})
	}
}

func TestValidate_I18nSection(t *testing.T) {
	doc := func(i18n string) module.Manifest {
		raw := []byte(`{
  "schema_version": "1", "key": "acme.example", "slug": "example", "name": "X",
  "version": "1.0.0", "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": [], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"}` + i18n + `}`)
		m, err := module.ParseManifest(raw)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if got := Validate(doc(``)); len(got) != 0 {
		t.Errorf("tanpa i18n harus valid: %+v", got)
	}
	if got := Validate(doc(`, "i18n": {"locales": ["id", "en"], "defaultLocale": "id"}`)); len(got) != 0 {
		t.Errorf("i18n sah harus valid: %+v", got)
	}
	for name, in := range map[string]string{
		"locales kosong":      `, "i18n": {"locales": [], "defaultLocale": "id"}`,
		"locales ganda":       `, "i18n": {"locales": ["id", "id"], "defaultLocale": "id"}`,
		"kode bahasa salah":   `, "i18n": {"locales": ["ID"], "defaultLocale": "ID"}`,
		"tanpa defaultLocale": `, "i18n": {"locales": ["id"]}`,
		"field tak dikenal":   `, "i18n": {"locales": ["id"], "defaultLocale": "id", "x": 1}`,
	} {
		got := Validate(doc(in))
		if len(got) == 0 || got[0].Code != CodeSchemaInvalid {
			t.Errorf("%s: problems = %+v, want schema_invalid", name, got)
		}
	}
}

func TestValidate_DeskSection(t *testing.T) {
	doc := func(desk string) module.Manifest {
		raw := []byte(`{
  "schema_version": "1", "key": "acme.example", "slug": "example", "name": "X",
  "version": "1.0.0", "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": ["example.notes.read"], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"}` + desk + `}`)
		m, err := module.ParseManifest(raw)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	item := func(extra string) string {
		return `{"id": "notes", "label": "nav.notes", "to": "/notes", "permission": "example.notes.read"` + extra + `}`
	}
	valid := map[string]string{
		"minimal":                         `, "desk": {"label": "desk.title", "icon": "file-text", "menus": []}`,
		"item":                            `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [` + item(``) + `]}`,
		"item beri ikon":                  `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [` + item(`, "icon": "file"`) + `]}`,
		"to bertingkat":                   `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a/b", "permission": "example.notes.read"}]}`,
		"dashboard":                       `, "desk": {"label": "desk.title", "icon": "file-text", "dashboard": {"label": "desk.dashboard", "icon": "gauge"}, "menus": []}`,
		"grup satu level":                 `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "g", "label": "nav.g", "children": [` + item(``) + `]}]}`,
		"settings":                        `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [], "settings": {"x": 1}}`,
		"item access authenticated (Q43)": `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a", "access": "authenticated"}]}`,
	}
	for name, in := range valid {
		if got := Validate(doc(in)); len(got) != 0 {
			t.Errorf("%s harus valid: %+v", name, got)
		}
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = `{"id": "m` + strconv.Itoa(i) + `", "label": "nav.m", "to": "/m", "permission": "example.notes.read"}`
	}
	invalid := map[string]string{
		"tanpa label":           `, "desk": {"icon": "file-text", "menus": []}`,
		"tanpa icon":            `, "desk": {"label": "desk.title", "menus": []}`,
		"tanpa menus":           `, "desk": {"label": "desk.title", "icon": "file-text"}`,
		"menus lebih dari 100":  `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [` + strings.Join(many, ",") + `]}`,
		"to bermuatan skema":    `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "https://evil.test/x", "permission": "example.notes.read"}]}`,
		"to bermuatan host":     `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "//evil.test/x", "permission": "example.notes.read"}]}`,
		"to dengan //":          `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a//b", "permission": "example.notes.read"}]}`,
		"to tanpa garis miring": `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "notes", "permission": "example.notes.read"}]}`,
		"to dengan query":       `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a?x=1", "permission": "example.notes.read"}]}`,
		"permission kosong":     `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a", "permission": ""}]}`,
		"item dengan permission dan access (Q43)": `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a", "permission": "example.notes.read", "access": "authenticated"}]}`,
		"access bukan authenticated (Q43)":        `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a", "access": "public"}]}`,
		"access kosong (Q43)":                     `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a", "access": ""}]}`,
		"item tanpa permission":                   `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "a", "label": "nav.a", "to": "/a"}]}`,
		"grup bersarang":                          `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "g", "label": "nav.g", "children": [{"id": "h", "label": "nav.h", "children": [` + item(``) + `]}]}]}`,
		"grup tanpa anak":                         `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "g", "label": "nav.g", "children": []}]}`,
		"properti tak dikenal":                    `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [], "x": 1}`,
		"properti item tak dikenal":               `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [` + item(`, "x": 1`) + `]}`,
		"label bukan kunci i18n":                  `, "desk": {"label": "Judul Desk", "icon": "file-text", "menus": []}`,
		"icon tidak sah":                          `, "desk": {"label": "desk.title", "icon": "File Text", "menus": []}`,
		"id tidak sah":                            `, "desk": {"label": "desk.title", "icon": "file-text", "menus": [{"id": "A b", "label": "nav.a", "to": "/a", "permission": "example.notes.read"}]}`,
	}
	for name, in := range invalid {
		got := Validate(doc(in))
		if len(got) == 0 || got[0].Code != CodeSchemaInvalid {
			t.Errorf("%s: problems = %+v, want schema_invalid", name, got)
		}
	}
}

// Desk yang tidak sama dengan Raw dilaporkan sebagai manifest_mismatch.
func TestValidate_ManifestMismatchDesk(t *testing.T) {
	raw := []byte(`{
  "schema_version": "1", "key": "acme.example", "slug": "example", "name": "X",
  "version": "1.0.0", "publisher": {"id": "a", "name": "A"}, "trust": "builtin",
  "compatibility": {"platform": "^0.1", "sdk": "^0.1"}, "dependencies": [],
  "capabilities": ["example.notes.read"], "migrations": {"strategy": "none"}, "offline": {"mode": "online-only"},
  "desk": {"label": "desk.title", "icon": "file-text", "menus": []}}`)
	m, err := module.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := Validate(m); len(got) != 0 {
		t.Fatalf("problems = %+v", got)
	}
	m.Desk.Menus = []module.DeskMenu{{ID: "x", Label: "nav.x", To: "/x", Permission: "example.notes.read"}}
	got := Validate(m)
	if len(got) != 1 || got[0].Code != CodeManifestMismatch {
		t.Fatalf("problems = %+v, want manifest_mismatch", got)
	}
	m2, _ := module.ParseManifest(raw)
	m2.Desk = nil
	if got := Validate(m2); len(got) != 1 || got[0].Code != CodeManifestMismatch {
		t.Fatalf("Desk nil: problems = %+v, want manifest_mismatch", got)
	}
}
