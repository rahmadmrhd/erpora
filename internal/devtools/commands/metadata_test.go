package commands

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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

func TestMetadataGenerate_RegisteredInDevGroup(t *testing.T) {
	root := newTestRoot(t)
	cmd, _, err := root.Find([]string{"metadata:generate"})
	if err != nil || cmd.Name() != "metadata:generate" {
		t.Fatalf("metadata:generate tidak terdaftar: %v", err)
	}
	if cmd.GroupID != GroupDevtools || cmd.Short == "" || cmd.Long == "" {
		t.Errorf("GroupID=%q Short/Long kosong?", cmd.GroupID)
	}
	if cmd.Flags().Lookup("check") == nil {
		t.Error("flag --check tidak ada")
	}
}

func TestMetadataGenerate_ModulesWithoutMetadataAreUnaffected(t *testing.T) {
	dir := inRepo(t)
	before := treeHash(t, dir)
	for _, args := range [][]string{{"metadata:generate", "--check"}, {"metadata:generate"}} {
		out, err := runMetadata(t, args...)
		if err != nil {
			t.Fatalf("%v: err = %v\n%s", args, err, out)
		}
		if !strings.Contains(out, "0 berkas keluaran") && !strings.Contains(out, "0 berkas keluaran sinkron") {
			t.Errorf("%v: keluaran = %q", args, out)
		}
	}
	if treeHash(t, dir) != before {
		t.Error("repo tanpa metadata tidak boleh berubah")
	}
}

func TestMetadataGenerate_WriteThenCheckIsGreen(t *testing.T) {
	dir := fixtureRepo(t)
	out, err := runMetadata(t, "metadata:generate")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{"ditulis: modules/fixturesales/internal/domain/invoice_meta.gen.go", "ditulis: modules/fixturesales/openapi.yaml", "metadata:generate:"} {
		if !strings.Contains(out, want) {
			t.Errorf("keluaran tidak memuat %q:\n%s", want, out)
		}
	}
	before := treeHash(t, dir)
	out, err = runMetadata(t, "metadata:generate", "--check")
	if err != nil {
		t.Fatalf("--check harus hijau: %v\n%s", err, out)
	}
	if !strings.Contains(out, "sinkron dengan metadata") {
		t.Errorf("keluaran = %q", out)
	}
	if treeHash(t, dir) != before {
		t.Error("--check tidak boleh menulis apa pun")
	}
	// Tulis ulang tanpa perubahan: tidak ada yang ditulis.
	if out, err = runMetadata(t, "metadata:generate"); err != nil || !strings.Contains(out, "0 ditulis") {
		t.Errorf("idempoten: err=%v\n%s", err, out)
	}
}

func TestMetadataGenerate_CheckFailsOnPristineAndDriftWithoutWriting(t *testing.T) {
	dir := fixtureRepo(t)
	before := treeHash(t, dir)
	out, err := runMetadata(t, "metadata:generate", "--check")
	if exitCodeOf(err) != exitMetadataValidation || !strings.Contains(out, "basi: modules/fixturesales/openapi.yaml") {
		t.Fatalf("exit=%d err=%v\n%s", exitCodeOf(err), err, out)
	}
	if treeHash(t, dir) != before {
		t.Fatal("--check tidak boleh menulis apa pun, termasuk saat gagal")
	}
	if _, err := runMetadata(t, "metadata:generate"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "modules", "fixturesales", "internal", "domain", "tag_meta.gen.go")
	b, _ := os.ReadFile(p) //nolint:gosec // fixture salinan
	if err := os.WriteFile(p, append(b, []byte("// diubah tangan\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	before = treeHash(t, dir)
	out, err = runMetadata(t, "metadata:generate", "--check")
	if exitCodeOf(err) != exitMetadataValidation || !strings.Contains(out, "basi: modules/fixturesales/internal/domain/tag_meta.gen.go") {
		t.Fatalf("exit=%d\n%s", exitCodeOf(err), out)
	}
	if treeHash(t, dir) != before {
		t.Error("--check tidak boleh menulis apa pun")
	}
	// Mode tulis memulihkan.
	if _, err := runMetadata(t, "metadata:generate"); err != nil {
		t.Fatal(err)
	}
	if _, err := runMetadata(t, "metadata:generate", "--check"); err != nil {
		t.Fatalf("setelah dipulihkan harus hijau: %v", err)
	}
}

func TestMetadataGenerate_MissingMarkerFailsWithoutWriting(t *testing.T) {
	dir := fixtureRepo(t)
	if _, err := runMetadata(t, "metadata:generate"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "modules", "fixturesales", "internal", "presentation", "http", "invoice_dto.gen.go")
	b, _ := os.ReadFile(p) //nolint:gosec // fixture salinan
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), "// Code generated", "// ditulis tangan", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	before := treeHash(t, dir)
	for _, args := range [][]string{{"metadata:generate"}, {"metadata:generate", "--check"}} {
		if _, err := runMetadata(t, args...); exitCodeOf(err) != exitMetadataValidation {
			t.Errorf("%v: exit = %d err = %v", args, exitCodeOf(err), err)
		}
	}
	if treeHash(t, dir) != before {
		t.Error("tidak boleh menulis apa pun bila penanda hilang")
	}
}

func TestMetadataGenerate_CheckErrorsStopBeforeWriting(t *testing.T) {
	dir := fixtureRepo(t)
	line := filepath.Join(dir, "modules", "fixturesales", "metadata", "invoiceLine.resource.yaml")
	b, _ := os.ReadFile(line) //nolint:gosec // fixture salinan
	broken := strings.Replace(string(b), "type: integer\n    minimum: 1", "type: integer\n    maxLength: 5\n    minimum: 1", 1)
	if err := os.WriteFile(line, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	before := treeHash(t, dir)
	out, err := runMetadata(t, "metadata:generate")
	if exitCodeOf(err) != exitMetadataValidation || !strings.Contains(out, "field_attr_not_for_type") {
		t.Fatalf("exit=%d\n%s", exitCodeOf(err), out)
	}
	if treeHash(t, dir) != before {
		t.Error("tidak boleh menulis apa pun bila metadata:check bergalat")
	}
}

func TestMetadataGenerate_ModuleArgument(t *testing.T) {
	dir := fixtureRepo(t)
	out, err := runMetadata(t, "metadata:generate", "fixturecore")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "ditulis: modules/fixturecore/openapi.yaml") || strings.Contains(out, "fixturesales") {
		t.Errorf("hanya fixturecore yang boleh ditulis:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "modules", "fixturesales", "internal")); err == nil {
		t.Error("fixturesales tidak boleh tersentuh")
	}
	for _, bad := range []string{"nope", "../fixturesales"} {
		if _, err := runMetadata(t, "metadata:generate", bad); exitCodeOf(err) != exitMetadataValidation {
			t.Errorf("%q: exit = %d err = %v", bad, exitCodeOf(err), err)
		}
	}
}

func TestMetadataGenerate_OutsideRepoExits3(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := runMetadata(t, "metadata:generate"); exitCodeOf(err) != exitMetadataConfig {
		t.Fatalf("exit = %d err = %v", exitCodeOf(err), err)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=uji", "-c", "user.email=uji@example.invalid", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)
	cmd := exec.Command("git", full...) //nolint:gosec // argumen tetap pada test
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Requirement 12.1: menghapus field yang ada pada tag rilis terakhir modul tanpa deprecated ditolak oleh metadata:check.
func TestMetadataCheck_RemovedFieldAgainstReleaseTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git tidak tersedia")
	}
	dir := fixtureRepo(t)
	tagFile := filepath.Join(dir, "modules", "fixturesales", "metadata", "tag.resource.yaml")
	b, err := os.ReadFile(tagFile) //nolint:gosec // fixture salinan
	if err != nil {
		t.Fatal(err)
	}
	withLegacy := strings.Replace(string(b), "fields:\n", "fields:\n  legacyCode: {type: string, maxLength: 10, nullable: true}\n", 1)
	if withLegacy == string(b) {
		t.Fatal("penyuntikan field gagal")
	}
	if err := os.WriteFile(tagFile, []byte(withLegacy), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "rilis")
	gitIn(t, dir, "tag", "modules/fixturesales/v1.0.0")

	// keadaan yang sama dengan rilis: bersih.
	if out, err := runMetadata(t, "metadata:check"); err != nil {
		t.Fatalf("sama dengan rilis harus bersih: %v\n%s", err, out)
	}
	// hapus field tanpa pernah deprecated pada rilis.
	if err := os.WriteFile(tagFile, b, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runMetadata(t, "metadata:check")
	if exitCodeOf(err) != exitMetadataValidation {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "error [field_removed_without_deprecation]") || !strings.Contains(out, "modules/fixturesales/v1.0.0") ||
		!strings.Contains(out, "tag.resource.yaml:") {
		t.Errorf("keluaran:\n%s", out)
	}
}

func TestMakeModule_HasMetadataFlag(t *testing.T) {
	root := newTestRoot(t)
	cmd, _, err := root.Find([]string{"make:module"})
	if err != nil || cmd.Name() != "make:module" {
		t.Fatalf("make:module tidak terdaftar: %v", err)
	}
	f := cmd.Flags().Lookup("metadata")
	if f == nil || f.DefValue != "false" {
		t.Fatalf("flag --metadata harus ada dengan bawaan false: %+v", f)
	}
}

func TestMetadataCheck_PrintsIncludeSetRelaxationsWithReason(t *testing.T) {
	fixtureRepo(t)
	out, err := runMetadata(t, "metadata:check")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{
		`pelonggaran include set: fixturesales.invoice <- fixturesales.pricing field=[taxRate] reason="Pajak hanya wajib bila faktur kena pajak"`,
		`pelonggaran include set: fixturesales.invoice <- fixturecore.address field=[postalCode] reason="Kode pos hanya wajib bila dikirim ke pelanggan"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("keluaran tidak memuat %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "invoice.resource.yaml:") {
		t.Errorf("pelonggaran tanpa lokasi berkas:baris:\n%s", out)
	}
}

func TestMetadataCheck_PrintsIncludeSetRelaxationsOfModuleAbstracts(t *testing.T) {
	dir := fixtureRepo(t)
	abs := filepath.Join(dir, "modules", "fixturesales", "metadata", "pricingPlus.abstract.yaml")
	src := "schemaVersion: \"1\"\nkind: abstract\nkey: fixturesales.pricingPlus\ndescription: Abstrak turunan pricing dengan pelonggaran.\n" +
		"include:\n  - source: fixturesales.pricing\n    reason: Pajak khusus paket plus\n    set:\n      taxRate:\n        requiredWhen: { field: taxable, op: eq, value: true }\n"
	if err := os.WriteFile(abs, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runMetadata(t, "metadata:check") // galat lain (taxable tidak ada di abstrak) tidak menghalangi laporan
	want := `pelonggaran include set: fixturesales.pricingPlus <- fixturesales.pricing field=[taxRate] reason="Pajak khusus paket plus"`
	if !strings.Contains(out, want) {
		t.Errorf("keluaran tidak memuat pelonggaran abstrak %q:\n%s", want, out)
	}
	if !strings.Contains(out, "pricingPlus.abstract.yaml:") {
		t.Errorf("pelonggaran abstrak tanpa lokasi berkas:baris:\n%s", out)
	}
}
