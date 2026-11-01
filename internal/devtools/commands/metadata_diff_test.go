package commands

import (
	"io"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/diff"
)

func TestMetadataDiff_RegisteredInDevRoot(t *testing.T) {
	root := newTestRoot(t)
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "metadata:diff" {
			found = true
			if c.GroupID != GroupDevtools {
				t.Errorf("GroupID = %q", c.GroupID)
			}
		}
	}
	if !found {
		t.Fatal("metadata:diff tidak terdaftar")
	}
}

func TestMetadataDiff_RequiresAdminDSN(t *testing.T) {
	inRepo(t)
	t.Setenv(diff.EnvAdminDSN, "")
	root := newTestRoot(t)
	root.SetArgs([]string{"metadata:diff"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.Execute()
	if exitCodeOf(err) != exitMetadataConfig || !strings.Contains(err.Error(), diff.EnvAdminDSN) {
		t.Fatalf("err = %v, want exit %d menyebut %s", err, exitMetadataConfig, diff.EnvAdminDSN)
	}
}

func TestMetadataDiff_NeverUsesApplicationDSN(t *testing.T) {
	inRepo(t)
	t.Setenv(diff.EnvAdminDSN, "")
	t.Setenv("ERP_DATABASE_URL", "postgres://x:y@127.0.0.1:1/prod?sslmode=disable")
	root := newTestRoot(t)
	root.SetArgs([]string{"metadata:diff"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if exitCodeOf(root.Execute()) != exitMetadataConfig {
		t.Fatal("tanpa DSN admin harus berhenti; DSN aplikasi tidak boleh dipakai")
	}
}

func TestMetadataDiff_PlanOrdersDependenciesFirst(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, metadataFixtureDir, dir)
	plan, err := planMetadataDiff(dir, "fixturesales", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, s := range plan.sources {
		slugs = append(slugs, s.Slug)
	}
	if strings.Join(slugs, ",") != "fixturecore,fixturesales" {
		t.Errorf("urutan sumber = %v", slugs)
	}
	for _, tb := range plan.model.Tables {
		if tb.Module != "fixturesales" {
			t.Errorf("model hanya memuat modul sasaran, dapat %s (%s)", tb.Name, tb.Module)
		}
	}
	if plan.model.Table("fixturesales_invoices") == nil {
		t.Error("model tanpa fixturesales_invoices")
	}
	if len(plan.prefixes) != 1 || plan.prefixes[0] != "fixturesales_" {
		t.Errorf("prefixes = %v", plan.prefixes)
	}

	all, err := planMetadataDiff(dir, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.sources) != 2 || all.model.Table("fixturecore_customers") == nil || all.model.Table("fixturesales_invoices") == nil {
		t.Errorf("semua modul: sources=%d", len(all.sources))
	}
}

func TestMetadataDiff_UnknownModule(t *testing.T) {
	dir := t.TempDir()
	copyTree(t, metadataFixtureDir, dir)
	_, err := planMetadataDiff(dir, "tidakada", io.Discard)
	if exitCodeOf(err) != exitMetadataValidation || !strings.Contains(err.Error(), "tidakada") {
		t.Fatalf("err = %v", err)
	}
}
