package commands

import (
	"io"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/diff"
)

func runMigrate(t *testing.T, args ...string) error {
	t.Helper()
	root := newTestRoot(t)
	root.SetArgs(append([]string{"metadata:migrate"}, args...))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	return root.Execute()
}

func TestMetadataMigrate_RegisteredInDevRoot(t *testing.T) {
	for _, c := range newTestRoot(t).Commands() {
		if c.Name() == "metadata:migrate" {
			if c.GroupID != GroupDevtools || c.Flags().Lookup("dry-run") == nil {
				t.Errorf("GroupID=%q dry-run=%v", c.GroupID, c.Flags().Lookup("dry-run"))
			}
			return
		}
	}
	t.Fatal("metadata:migrate tidak terdaftar")
}

func TestMetadataMigrate_ArgumentRules(t *testing.T) {
	inRepo(t)
	for _, args := range [][]string{{}, {"onlymodule"}, {"a", "b", "c"}} {
		if err := runMigrate(t, args...); err == nil || exitCodeOf(err) == 0 {
			t.Errorf("args %v harus ditolak, err = %v", args, err)
		}
	}
}

func TestMetadataMigrate_RequiresAdminDSN(t *testing.T) {
	inRepo(t)
	t.Setenv(diff.EnvAdminDSN, "")
	t.Setenv("ERP_DATABASE_URL", "postgres://x:y@127.0.0.1:1/prod?sslmode=disable")
	err := runMigrate(t, "x", "nama")
	if exitCodeOf(err) != exitMetadataConfig || !strings.Contains(err.Error(), diff.EnvAdminDSN) {
		t.Fatalf("err = %v", err)
	}
}

func TestMetadataMigrate_UnknownModuleAndBadName(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, metadataFixtureDir, dir)
	t.Chdir(dir)
	t.Setenv(diff.EnvAdminDSN, "postgres://x:y@127.0.0.1:1/postgres?sslmode=disable")
	if err := runMigrate(t, "tidakada", "nama"); exitCodeOf(err) != exitMetadataValidation || !strings.Contains(err.Error(), "tidakada") {
		t.Errorf("modul tak ada: %v", err)
	}
	if err := runMigrate(t, "fixturecore", "Bad-Name"); exitCodeOf(err) != exitMetadataValidation {
		t.Errorf("nama tak sah: %v", err)
	}
}
