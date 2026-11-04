package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"erpora/internal/devtools/gen"
	"erpora/internal/devtools/metadatatools/internal/testrepo"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
)

var update = flag.Bool("update", false, "regenerasi file golden")

// okChecker adalah pemeriksa tiruan yang meloloskan semuanya (paket check nyata disambungkan koordinator).
type okChecker struct{}

func (okChecker) Check(string, *resolve.Catalog) []load.Problem { return nil }

type failChecker struct{}

func (failChecker) Check(string, *resolve.Catalog) []load.Problem {
	return []load.Problem{{File: "modules/fixturesales/metadata/invoice.resource.yaml", Line: 3, Code: "stub", Message: "gagal"}}
}

// fixtureCatalog memuat katalog modul fixture dari root salinan sementara.
func fixtureCatalog(t *testing.T, root string) *resolve.Catalog {
	t.Helper()
	c, ps := resolve.NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	for _, m := range []struct {
		mod resolve.Module
		dir string
	}{
		{resolve.Module{Slug: "fixturecore", DefaultGroup: "general"}, "fixturecore"},
		{resolve.Module{Slug: "fixturesales", Dependencies: []string{"fixturecore"}, DefaultGroup: "general"}, "fixturesales"},
	} {
		docs, lps := load.LoadFS(os.DirFS(filepath.Join(root, "modules")), m.dir+"/metadata")
		if len(lps) != 0 {
			t.Fatal(lps)
		}
		if aps := c.AddModule(m.mod, docs...); len(aps) != 0 {
			t.Fatal(aps)
		}
	}
	return c
}

func fixtureRepo(t *testing.T) (string, *resolve.Catalog) {
	t.Helper()
	root := testrepo.Copy(t, false)
	return root, fixtureCatalog(t, root)
}

// treeSum menghitung SHA-256 atas seluruh path dan isi berkas di bawah dir (bukti "tidak ada yang berubah").
func treeSum(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	var paths []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, p)
		}
		return nil
	})
	sort.Strings(paths)
	for _, p := range paths {
		data, err := os.ReadFile(p) //nolint:gosec // fixture tes
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) //nolint:gosec // fixture tes
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func filePaths(r Result) []string {
	var out []string
	for _, f := range r.Files {
		out = append(out, f.Path)
	}
	return out
}

func fileContent(t *testing.T, r Result, p string) string {
	t.Helper()
	for _, f := range r.Files {
		if f.Path == p {
			return string(f.Content)
		}
	}
	t.Fatalf("keluaran %s tidak ada; ada: %v", p, filePaths(r))
	return ""
}

// assertGolden membandingkan keluaran dengan berkas di dir; -update menulis ulang. Golden basi menggagalkan tes.
func assertGolden(t *testing.T, dir string, files []File) {
	t.Helper()
	want := map[string]bool{}
	for _, f := range files {
		want[f.Path] = true
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if *update {
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, f.Content, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		golden, err := os.ReadFile(p) //nolint:gosec // golden tes
		if err != nil {
			t.Errorf("golden %s tidak ada (jalankan tes dengan -update): %v", p, err)
			continue
		}
		if string(golden) != string(f.Content) {
			t.Errorf("golden %s berbeda:\n%s", p, gen.UnifiedDiff(f.Path, golden, f.Content))
		}
	}
	if *update {
		return
	}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if !want[filepath.ToSlash(rel)] {
			t.Errorf("golden basi: %s tidak lagi dihasilkan", filepath.ToSlash(rel))
		}
		return nil
	})
}

func goFiles(r Result) []string {
	var out []string
	for _, f := range r.Files {
		if strings.HasSuffix(f.Path, ".go") {
			out = append(out, f.Path)
		}
	}
	return out
}

func under(p, dir string) bool { return strings.HasPrefix(path.Clean(p), dir+"/") }

func fieldNamed(name string, attrs map[string]any) *resolve.Field {
	return &resolve.Field{Name: name, Attrs: attrs}
}
