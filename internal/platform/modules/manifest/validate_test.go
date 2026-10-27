package manifest

import (
	"os"
	"path/filepath"
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
