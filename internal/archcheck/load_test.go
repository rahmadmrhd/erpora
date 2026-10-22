package archcheck

import (
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
	for _, want := range []string{"errors", "net/http", "os/signal", "database/sql"} {
		if !strings.Contains(imports, want) {
			t.Errorf("impor %q tidak terlihat (tag/OS belum dicakup): %s", want, imports)
		}
	}
	files := strings.Join(pkg.GoFiles, ",")
	for _, want := range []string{"a.go", "b_devtools.go", "c_linux.go", "d_integration.go"} {
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
