package archcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Load harus melihat file ber-build-tag (devtools, integration) dan khusus-OS, bukan hanya build default host.
func TestLoad_SeesTaggedAndOSSpecificFiles(t *testing.T) {
	t.Setenv("GOWORK", "off") // fixture adalah module mandiri di luar go.work
	repo, err := Load("testdata/tagged")
	if err != nil {
		t.Fatal(err)
	}
	var pkg *Package
	for i := range repo.Packages {
		if repo.Packages[i].ImportPath == "example.test/tagged" {
			pkg = &repo.Packages[i]
		}
	}
	if pkg == nil {
		t.Fatalf("paket fixture tidak ditemukan: %+v", repo.Packages)
	}
	imports := strings.Join(pkg.Imports, ",")
	for _, want := range []string{"errors", "net/http", "os/signal", "database/sql", "os/exec", "net/netip", "hash/crc32", "encoding/csv"} {
		if !strings.Contains(imports, want) {
			t.Errorf("impor %q tidak terlihat (tag/OS belum dicakup): %s", want, imports)
		}
	}
	files := strings.Join(pkg.GoFiles, ",")
	for _, want := range []string{"a.go", "b_devtools.go", "c_linux.go", "d_integration.go", "e_devtools_only.go", "f_integration_only.go", "g_windows_nodev.go", "h_plain_linux.go"} {
		if !strings.Contains(files, want) {
			t.Errorf("file %q tidak terlihat: %s", want, files)
		}
	}
	seen := map[string]int{}
	for _, f := range pkg.GoFiles {
		seen[f]++
		if seen[f] > 1 {
			t.Errorf("file duplikat %s", f)
		}
	}
}

// Finding 6: path module dengan spasi (umum di Windows) tidak boleh terpecah.
func TestLoad_ModuleDirWithSpaces(t *testing.T) {
	t.Setenv("GOWORK", "off")
	dir := filepath.Join(t.TempDir(), "dengan spasi", "mod")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/spaced\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package spaced\n\nimport \"errors\"\n\nvar E = errors.New(\"x\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.Mods) != 1 || repo.Mods[0].Path != "example.test/spaced" {
		t.Fatalf("mods = %+v", repo.Mods)
	}
	if len(repo.Packages) != 1 || repo.Packages[0].ImportPath != "example.test/spaced" {
		t.Fatalf("packages = %+v", repo.Packages)
	}
}

// B7: tanpa workspace (GOWORK=off) Load menandai Work.Enabled=false dan Check gagal dengan pesan jelas,
// bukan diam-diam memeriksa root saja.
func TestLoad_WorkspaceDisabledIsReported(t *testing.T) {
	t.Setenv("GOWORK", "off")
	repo, err := Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	if repo.Work == nil || repo.Work.Enabled {
		t.Fatalf("Work = %+v, want Enabled=false", repo.Work)
	}
	var found bool
	for _, v := range Check(repo) {
		if v.Rule == "GOWORK" && strings.Contains(v.Detail, "workspace tidak aktif") {
			found = true
		}
	}
	if !found {
		t.Fatal("Check tidak melaporkan workspace nonaktif")
	}
}

// B7: pada repo nyata, setiap go.mod hasil glob tercantum di use go.work.
func TestLoad_RealWorkspaceListsAllModules(t *testing.T) {
	repo, err := Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	if repo.Work == nil || !repo.Work.Enabled || len(repo.Work.ModDirs) == 0 {
		t.Fatalf("Work = %+v", repo.Work)
	}
	for _, d := range repo.Work.ModDirs {
		if !contains(repo.Work.Use, d) {
			t.Errorf("%s tidak ada di use: %v", d, repo.Work.Use)
		}
	}
}

// web/package.json di root adalah app shell (bukan frontend modul): tidak boleh diperiksa sebagai F2.
func TestLoad_WebPackagesOnlyForModules(t *testing.T) {
	repo, err := Load("../..")
	if err != nil {
		t.Fatal(err)
	}
	for path := range repo.Web {
		if !strings.HasPrefix(path, modulesBase) {
			t.Errorf("Web memuat %s yang bukan modul", path)
		}
	}
}
