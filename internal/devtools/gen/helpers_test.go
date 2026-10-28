package gen

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "regenerasi file golden")

// copyTree menyalin direktori src ke dst.
func copyTree(t *testing.T, src, dst string) {
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
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(p) //nolint:gosec // fixture test
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// fixtureRepo menyalin testdata/repo ke direktori sementara.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	copyTree(t, "testdata/repo", root)
	return root
}

// checksum menghitung SHA-256 atas seluruh path dan isi file di bawah dir (untuk bukti "tidak ada yang berubah").
func checksum(t *testing.T, dir string) string {
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
		data, err := os.ReadFile(p) //nolint:gosec // fixture test
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// AssertGolden membandingkan isi akhir setiap FileOp dengan file di dir (testdata/golden/<kasus>); flag -update
// menulis ulang dir. File golden yang tidak lagi dihasilkan dianggap basi dan menggagalkan test.
func AssertGolden(t *testing.T, dir string, ops []FileOp) {
	t.Helper()
	want := map[string]bool{}
	for _, op := range ops {
		want[op.Path] = true
		p := filepath.Join(dir, filepath.FromSlash(op.Path))
		if *update {
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, op.Content, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		golden, err := os.ReadFile(p) //nolint:gosec // fixture test
		if err != nil {
			t.Errorf("golden %s tidak ada (jalankan test dengan -update): %v", p, err)
			continue
		}
		if string(golden) != string(op.Content) {
			t.Errorf("golden %s berbeda:\n%s", p, UnifiedDiff(op.Path, golden, op.Content))
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
			t.Errorf("golden basi: %s tidak lagi dihasilkan", strings.ReplaceAll(rel, "\\", "/"))
		}
		return nil
	})
}
