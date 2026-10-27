package gen

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestNextMigrationNumber(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  int
	}{
		{"kosong", nil, 1},
		{"dua file", []string{"0001_a.sql", "0002_b.sql"}, 3},
		{"loncat", []string{"0001_a.sql", "0007_b.sql"}, 8},
		{"abaikan non-migrasi", []string{"README.md", "x_1.sql", "0003_c.sql"}, 4},
	}
	for _, c := range cases {
		fsys := fstest.MapFS{}
		for _, f := range c.files {
			fsys[f] = &fstest.MapFile{Data: []byte("x")}
		}
		got, err := NextMigrationNumber(fsys)
		if err != nil || got != c.want {
			t.Errorf("%s: got %d err %v, want %d", c.name, got, err, c.want)
		}
	}
}

func TestResolveModule(t *testing.T) {
	root := fixtureRepo(t)
	for _, arg := range []string{"example", "acme.example"} {
		ref, err := ResolveModule(root, arg)
		if err != nil || ref.Dir != "example" || ref.Slug != "example" || ref.Key != "acme.example" ||
			ref.GoPath != "erpora/modules/example" || ref.Strategy != "versioned" || ref.Version != "1.0.0" || ref.HasWeb {
			t.Errorf("%s: %+v err %v", arg, ref, err)
		}
	}
	if ref, err := ResolveModule(root, "example-addon"); err != nil || ref.Dir != "exampleaddon" || ref.Strategy != "none" {
		t.Errorf("example-addon (slug): %+v err %v", ref, err)
	}
	_, err := ResolveModule(root, "nope")
	if err == nil || !strings.Contains(err.Error(), "example") || !strings.Contains(err.Error(), "exampleaddon") {
		t.Errorf("err = %v, want daftar modul", err)
	}
}

func TestMakeMigration_Golden(t *testing.T) {
	root := fixtureRepo(t)
	plan, err := MakeMigration(root, "example", "add_tags", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Ops) != 1 || plan.Ops[0].Path != "modules/example/migrations/0003_add_tags.sql" || plan.Ops[0].Kind != OpCreate {
		t.Fatalf("plan = %+v", plan.Ops)
	}
	AssertGolden(t, "testdata/golden/migration_module", plan.Ops)

	ctl, err := MakeMigration(root, "", "add_audit", "control")
	if err != nil {
		t.Fatal(err)
	}
	if len(ctl.Ops) != 1 || ctl.Ops[0].Path != "migrations/control/0003_add_audit.sql" {
		t.Fatalf("control plan = %+v", ctl.Ops)
	}
	AssertGolden(t, "testdata/golden/migration_control", ctl.Ops)
	tn, err := MakeMigration(root, "", "add_x", "tenant")
	if err != nil || tn.Ops[0].Path != "migrations/tenant/0003_add_x.sql" {
		t.Fatalf("tenant plan = %+v err %v", tn.Ops, err)
	}
}

func TestMakeMigration_Errors(t *testing.T) {
	root := fixtureRepo(t)
	if _, err := MakeMigration(root, "exampleaddon", "add_x", ""); err == nil || !strings.Contains(err.Error(), "migrations.strategy") {
		t.Errorf("strategy none: err = %v, want menyebut migrations.strategy", err)
	}
	if _, err := MakeMigration(root, "example", "AddTags", ""); err == nil {
		t.Error("nama bukan snake_case harus ditolak")
	}
	if _, err := MakeMigration(root, "example", "x", "bogus"); err == nil {
		t.Error("--platform bogus harus ditolak")
	}
	if _, err := MakeMigration(root, "nope", "add_x", ""); err == nil {
		t.Error("modul tidak ada harus error")
	}
}
