package commands

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const metadataFixtureDir = "../metadatatools/testdata/repo"

// copyTree menyalin direktori (fixture) ke dst.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		b, err := os.ReadFile(p) //nolint:gosec // fixture repo
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// treeHash memberi sidik jari seluruh isi direktori (nama dan isi berkas) untuk membuktikan tidak ada penulisan.
func treeHash(t *testing.T, dir string) string {
	t.Helper()
	var entries []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			entries = append(entries, "d:"+rel)
			return nil
		}
		b, _ := os.ReadFile(p) //nolint:gosec // direktori uji
		entries = append(entries, fmt.Sprintf("f:%s:%x", rel, sha256.Sum256(b)))
		return nil
	})
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}

func runMetadata(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newTestRoot(t)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyTree(t, metadataFixtureDir, dir)
	t.Chdir(dir)
	return dir
}

func TestMetadataCheck_RegisteredInDevGroup(t *testing.T) {
	root := newTestRoot(t)
	cmd, _, err := root.Find([]string{"metadata:check"})
	if err != nil || cmd.Name() != "metadata:check" {
		t.Fatalf("metadata:check tidak terdaftar: %v", err)
	}
	if cmd.GroupID != GroupDevtools {
		t.Errorf("GroupID = %q", cmd.GroupID)
	}
	if cmd.Short == "" || cmd.Long == "" {
		t.Error("Short dan Long harus terisi")
	}
}

func TestMetadataCheck_ModulesWithoutMetadataAreUnaffected(t *testing.T) {
	dir := inRepo(t)
	before := treeHash(t, dir)
	out, err := runMetadata(t, "metadata:check")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "0 resource diperiksa, 0 galat, 0 peringatan") {
		t.Errorf("keluaran = %q", out)
	}
	if treeHash(t, dir) != before {
		t.Error("metadata:check tidak boleh menulis apa pun")
	}
}

func TestMetadataCheck_FixtureIsCleanAndReadOnly(t *testing.T) {
	dir := fixtureRepo(t)
	before := treeHash(t, dir)
	out, err := runMetadata(t, "metadata:check")
	if err != nil {
		t.Fatalf("fixture harus bersih dari galat: %v\n%s", err, out)
	}
	if !strings.Contains(out, "0 galat") || !strings.Contains(out, "resource diperiksa") {
		t.Errorf("ringkasan tidak ada: %q", out)
	}
	if !strings.Contains(out, "warning [relation_fk_no_index]") {
		t.Errorf("peringatan harus dicetak dengan berkas dan baris:\n%s", out)
	}
	if !strings.Contains(out, "modules/fixturesales/metadata/invoiceLine.resource.yaml:") {
		t.Errorf("peringatan tanpa lokasi berkas:baris:\n%s", out)
	}
	if treeHash(t, dir) != before {
		t.Error("metadata:check tidak boleh menulis apa pun")
	}
}

func TestMetadataCheck_ReportsAllProblemsWithFileAndLineAndFailsWithExit2(t *testing.T) {
	dir := fixtureRepo(t)
	inv := filepath.Join(dir, "modules", "fixturesales", "metadata", "invoice.resource.yaml")
	b, err := os.ReadFile(inv) //nolint:gosec // fixture salinan
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(b), "description: Faktur penjualan", "bogus: 1\ndescription: Faktur penjualan", 1)
	broken = strings.Replace(broken, "type: date\n    sortable: true\n    filterable: true\n    label: fixturesales.invoice.dueOn", "type: date\n    maxLength: 5\n    sortable: true\n    filterable: true\n    label: fixturesales.invoice.dueOn", 1)
	if broken == string(b) {
		t.Fatal("penyuntikan galat gagal")
	}
	if err := os.WriteFile(inv, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	before := treeHash(t, dir)
	out, err := runMetadata(t, "metadata:check")
	if err == nil || exitCodeOf(err) != exitMetadataValidation {
		t.Fatalf("err = %v (exit %d)", err, exitCodeOf(err))
	}
	if !strings.Contains(out, "invoice.resource.yaml:") || !strings.Contains(out, "error [") {
		t.Errorf("galat harus memuat berkas, baris, dan kode:\n%s", out)
	}
	if treeHash(t, dir) != before {
		t.Error("metadata:check tidak boleh menulis apa pun, termasuk saat gagal")
	}
}

func TestMetadataCheck_SemanticErrorFailsWithCodeAndLine(t *testing.T) {
	dir := fixtureRepo(t)
	line := filepath.Join(dir, "modules", "fixturesales", "metadata", "invoiceLine.resource.yaml")
	b, _ := os.ReadFile(line) //nolint:gosec // fixture salinan
	broken := strings.Replace(string(b), "type: integer\n    minimum: 1", "type: integer\n    maxLength: 5\n    minimum: 1", 1)
	if err := os.WriteFile(line, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runMetadata(t, "metadata:check")
	if exitCodeOf(err) != exitMetadataValidation {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "error [field_attr_not_for_type]") || !strings.Contains(out, "invoiceLine.resource.yaml:") {
		t.Errorf("keluaran:\n%s", out)
	}
}

func TestMetadataCheck_OutsideRepoExits3(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := runMetadata(t, "metadata:check")
	if exitCodeOf(err) != exitMetadataConfig {
		t.Fatalf("err = %v (exit %d)", err, exitCodeOf(err))
	}
}

func TestMetadataCheck_RejectsArguments(t *testing.T) {
	inRepo(t)
	if _, err := runMetadata(t, "metadata:check", "extra"); err == nil {
		t.Error("argumen tambahan harus ditolak")
	}
}
