// Package testrepo menyalin mini-repo fixture spec resource-metadata
// (internal/devtools/metadatatools/testdata/repo) ke direktori sementara untuk tes (D-18, Requirement 20.5).
//
// Fixture sengaja berada di bawah testdata: tidak masuk go.work root, tidak terpindai archcheck/modcheck pada
// root repo, dan tidak didaftarkan ke internal/app. go:embed tidak dapat menembus direktori ber-go.mod,
// sehingga pohon disalin lewat sistem berkas.
package testrepo

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// FixtureDir mengembalikan path absolut mini-repo fixture di pohon sumber.
func FixtureDir(t testing.TB) string {
	t.Helper()
	return filepath.Join(packageDir(t), "..", "..", "testdata", "repo")
}

// RepoRoot mengembalikan root repo ERPora (untuk menyalin sdk asli).
func RepoRoot(t testing.TB) string {
	t.Helper()
	return filepath.Clean(filepath.Join(packageDir(t), "..", "..", "..", "..", ".."))
}

// Copy menyalin fixture ke direktori sementara dan mengembalikan root salinan. Bila withSDK, sdk asli repo
// ikut disalin ke <root>/sdk sehingga go.work dan replace fixture berlaku apa adanya (kompilasi dan tes
// sebagai subproses). Tautan simbolik tidak disalin.
func Copy(t testing.TB, withSDK bool) string {
	t.Helper()
	dst := t.TempDir()
	copyTree(t, FixtureDir(t), dst)
	if withSDK {
		copyTree(t, filepath.Join(RepoRoot(t), "sdk"), filepath.Join(dst, "sdk"))
	}
	return dst
}

func packageDir(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testrepo: runtime.Caller gagal")
	}
	return filepath.Dir(file)
}

func copyTree(t testing.TB, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(p) //nolint:gosec // fixture tes
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600) //nolint:gosec // target di bawah direktori sementara tes (rel dari WalkDir)
	})
	if err != nil {
		t.Fatalf("testrepo: salin %s: %v", src, err)
	}
}
