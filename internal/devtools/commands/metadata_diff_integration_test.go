//go:build integration

package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"erpora/internal/devtools/metadatatools/diff"
)

// Alur ujung ke ujung pada fixture: fixture belum punya migrasi (task 14.1), sehingga semua tabel dilaporkan
// hilang; database sementara dan role dihapus, tidak ada berkas yang ditulis.
func TestMetadataDiff_EndToEndOnFixture(t *testing.T) {
	if os.Getenv(diff.EnvAdminDSN) == "" {
		t.Skip(diff.EnvAdminDSN + " tidak diset")
	}
	dir := t.TempDir()
	copyTree(t, metadataFixtureDir, dir)
	before := treeHash(t, dir)
	t.Chdir(dir)

	root := newTestRoot(t)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"metadata:diff", "fixturesales"})
	err := root.Execute()
	if exitCodeOf(err) != exitMetadataValidation {
		t.Fatalf("err = %v (stderr %s), want exit %d", err, errOut.String(), exitMetadataValidation)
	}
	if !strings.Contains(out.String(), "[table_missing] fixturesales_invoices") {
		t.Errorf("keluaran = %s", out.String())
	}
	if treeHash(t, dir) != before {
		t.Error("metadata:diff tidak boleh menulis berkas")
	}
}

// metadata:migrate pada fixture: menulis 0001 untuk fixturecore lalu fixturesales (tanpa TODO), tidak
// menerapkan apa pun, dan metadata:diff berikutnya kosong; menjalankan ulang tanpa selisih tidak membuat berkas.
func TestMetadataMigrate_EndToEndOnFixture(t *testing.T) {
	if os.Getenv(diff.EnvAdminDSN) == "" {
		t.Skip(diff.EnvAdminDSN + " tidak diset")
	}
	dir := t.TempDir()
	copyTree(t, metadataFixtureDir, dir)
	dry := t.TempDir()
	copyTree(t, metadataFixtureDir, dry)
	t.Chdir(dir)
	exec := func(args ...string) (string, error) {
		root := newTestRoot(t)
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs(args)
		err := root.Execute()
		return out.String() + errOut.String(), err
	}
	for _, mod := range []string{"fixturecore", "fixturesales"} {
		out, err := exec("metadata:migrate", mod, "initial_schema")
		if err != nil {
			t.Fatalf("migrate %s: %v\n%s", mod, err, out)
		}
		b, rerr := os.ReadFile(dir + "/modules/" + mod + "/migrations/0001_initial_schema.sql") //nolint:gosec // direktori uji
		if rerr != nil {
			t.Fatalf("%s: %v\n%s", mod, rerr, out)
		}
		if strings.Contains(string(b), "TODO(metadata:migrate)") || !strings.Contains(string(b), "CREATE TABLE") {
			t.Errorf("%s: isi migrasi tak terduga:\n%s", mod, b)
		}
		if out2, err := exec("metadata:diff", mod); err != nil || !strings.Contains(out2, "selisih kosong") {
			t.Fatalf("diff %s setelah migrate: %v\n%s", mod, err, out2)
		}
	}
	before := treeHash(t, dir)
	out, err := exec("metadata:migrate", "fixturesales", "again")
	if err != nil || !strings.Contains(out, "tidak ada selisih") {
		t.Fatalf("tanpa selisih: %v\n%s", err, out)
	}
	if treeHash(t, dir) != before {
		t.Error("tanpa selisih tidak boleh ada berkas baru")
	}
	// --dry-run tidak menulis.
	t.Chdir(dry)
	hashBefore := treeHash(t, dry)
	if out, err := exec("metadata:migrate", "--dry-run", "fixturecore", "initial_schema"); err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if treeHash(t, dry) != hashBefore {
		t.Error("--dry-run tidak boleh menulis berkas")
	}
}
