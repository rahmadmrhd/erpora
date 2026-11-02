package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"erpora/internal/devtools/metadatatools/migrate"
)

func TestMigrationTodoFailsCheck(t *testing.T) {
	m := mod(t, "m", nil, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\n")))
	m.MigrationsFS = fstest.MapFS{
		"0001_create.sql": {Data: []byte("-- +goose Up\nCREATE TABLE m_rs (id uuid PRIMARY KEY);\n-- +goose Down\nDROP TABLE m_rs;\n")},
		"0002_rename.sql": {Data: []byte("-- +goose Up\n-- " + migrate.TodoMarker + ": ganti nama kolom lama ke kolom baru\nSELECT 1;\n-- +goose Down\nSELECT 1;\n")},
		"0003_second.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n-- " + migrate.TodoMarker + " lagi\n-- +goose Down\nSELECT 1;\n")},
		"catatan.txt":     {Data: []byte(migrate.TodoMarker)},
		"sub/0004_x.sql":  {Data: []byte(migrate.TodoMarker)},
		"0005_bersih.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 1;\n")},
	}
	rep := run(t, m)
	var got []Problem
	for _, p := range rep.Problems {
		if p.Code == CodeMigrationTodo {
			got = append(got, p)
		}
	}
	if len(got) != 2 {
		t.Fatalf("temuan = %s", dump(got))
	}
	if got[0].File != "modules/m/migrations/0002_rename.sql" || got[0].Line != 2 || got[0].Severity != SeverityError || got[0].Module != "m" ||
		got[1].File != "modules/m/migrations/0003_second.sql" || got[1].Line != 3 {
		t.Errorf("lokasi = %+v / %+v", got[0], got[1])
	}
	if !strings.Contains(got[0].Message, migrate.TodoMarker) || !strings.Contains(got[0].Message, "11.3") {
		t.Errorf("pesan = %q", got[0].Message)
	}
	if !rep.HasErrors() {
		t.Error("TODO migrasi harus menggagalkan pemeriksaan")
	}
}

func TestMigrationsWithoutTodoAreClean(t *testing.T) {
	m := mod(t, "m", nil, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\n")))
	m.MigrationsFS = fstest.MapFS{"0001_create.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 1;\n")}}
	noErrors(t, run(t, m))
	// modul tanpa migrasi tidak terpengaruh
	noErrors(t, run(t, mod(t, "m", nil, f("m/r.resource.yaml", resource("m.r", "fields:\n  a: {type: text}\n")))))
}

func TestMigrationTodoAppliesToModulesWithoutMetadata(t *testing.T) {
	m := Module{Dir: "modules/plain", Manifest: Manifest{Key: "acme.plain", Slug: "plain"},
		MigrationsFS: fstest.MapFS{"0001_x.sql": {Data: []byte("-- " + migrate.TodoMarker + "\n")}}}
	wantErr(t, run(t, m), CodeMigrationTodo, "")
}

func TestLoadWorkspaceReadsMigrationsDirectory(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("modules/alpha/module.json", `{"key":"acme.alpha","slug":"alpha"}`)
	write("modules/alpha/migrations/0001_x.sql", "-- +goose Up\nSELECT 1;\n-- "+migrate.TodoMarker+"\n")
	write("modules/beta/module.json", `{"key":"acme.beta","slug":"beta"}`)
	in, probs := LoadWorkspace(root)
	if len(probs) != 0 || len(in.Modules) != 2 {
		t.Fatalf("modul = %d, temuan = %s", len(in.Modules), dump(probs))
	}
	if in.Modules[0].MigrationsFS == nil || in.Modules[1].MigrationsFS != nil {
		t.Fatalf("MigrationsFS alpha=%v beta=%v", in.Modules[0].MigrationsFS, in.Modules[1].MigrationsFS)
	}
	rep := Run(in)
	p := wantErr(t, rep, CodeMigrationTodo, "")
	if p.File != "modules/alpha/migrations/0001_x.sql" || p.Line != 3 {
		t.Errorf("lokasi = %s:%d", p.File, p.Line)
	}
}
