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
