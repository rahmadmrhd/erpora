package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Fixture spec resource-metadata (D-18, Requirement 20.5) berada di
// internal/devtools/metadatatools/testdata/repo dan TIDAK boleh menjadi bagian aplikasi atau workspace.

const metadataFixtureDir = "internal/devtools/metadatatools/testdata/repo"

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("baca %s: %v", rel, err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func mentionsFixture(s string) bool {
	return strings.Contains(s, "testdata") || strings.Contains(s, "fixturecore") || strings.Contains(s, "fixturesales")
}

// TestMetadataFixtureNotInWorkspace: go.work dan go.mod root, sdk, dan modul tidak merujuk fixture.
func TestMetadataFixtureNotInWorkspace(t *testing.T) {
	files := []string{"go.work", "go.mod", "sdk/go.mod"}
	mods, err := filepath.Glob(filepath.Join("..", "..", "modules", "*", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mods {
		rel, _ := filepath.Rel(filepath.Join("..", ".."), m)
		files = append(files, filepath.ToSlash(rel))
	}
	for _, f := range files {
		if mentionsFixture(readRepoFile(t, f)) {
			t.Errorf("%s merujuk testdata atau modul fixture; fixture tidak boleh masuk workspace/dependensi", f)
		}
	}
}

// TestMetadataFixtureNotInModulesDir: fixture tidak boleh berupa modules/<x> (archcheck dan modcheck
// meng-glob modules/*), dan direktori fixture tetap ada.
func TestMetadataFixtureNotInModulesDir(t *testing.T) {
	if st, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(metadataFixtureDir), "go.work")); err != nil || st.IsDir() {
		t.Fatalf("mini-repo fixture %s tidak ditemukan: %v", metadataFixtureDir, err)
	}
	entries, err := os.ReadDir(filepath.Join("..", "..", "modules"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "fixture") {
			t.Errorf("modules/%s: modul fixture hanya boleh di %s", e.Name(), metadataFixtureDir)
		}
	}
}

// TestMetadataFixtureNotImportedByProductionCode: tidak ada kode non-tes (di luar direktori testdata) yang
// mengimpor path testdata atau modul fixture, dan internal/app tidak menyebut modul fixture sama sekali
// (D7: hanya internal/app merakit modul; fixture tidak didaftarkan).
func TestMetadataFixtureNotImportedByProductionCode(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "testdata", "node_modules", ".git", "tmp", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if perr != nil {
			return nil // galat sintaks dilaporkan alat lain
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if mentionsFixture(path) {
				t.Errorf("%s mengimpor %q: kode non-tes tidak boleh merujuk fixture", p, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// internal/app (termasuk berkas tes) tidak boleh menyebut modul fixture.
	appDir := filepath.Join(root, "internal", "app")
	err = filepath.WalkDir(appDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		b, rerr := os.ReadFile(p) //nolint:gosec // tes arsitektur membaca sumber repo
		if rerr != nil {
			return rerr
		}
		if s := string(b); strings.Contains(s, "fixturecore") || strings.Contains(s, "fixturesales") {
			t.Errorf("%s menyebut modul fixture; fixture tidak boleh didaftarkan ke internal/app", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
