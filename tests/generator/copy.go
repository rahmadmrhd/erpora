//go:build generator

package generator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// copyRepo menyalin root ke dst, melewati .git, node_modules, bin, tmp, .superpowers, contracts/dist, dan isi
// web/dist kecuali .gitkeep. Salinan itu repo mandiri tempat generator boleh menulis tanpa menyentuh repo asli.
func copyRepo(t *testing.T, root, dst string) {
	t.Helper()
	skipTop := map[string]bool{".git": true, "bin": true, "tmp": true, ".superpowers": true}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o750)
		}
		slash := filepath.ToSlash(rel)
		top, _, _ := strings.Cut(slash, "/")
		switch {
		case d.IsDir() && d.Name() == "node_modules":
			return fs.SkipDir
		case skipTop[top] && !strings.Contains(slash, "/") && d.IsDir():
			return fs.SkipDir
		case slash == "contracts/dist":
			return fs.SkipDir
		case strings.HasPrefix(slash, "web/dist/") && d.Name() != ".gitkeep":
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // menyalin repo sendiri untuk test
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600) //nolint:gosec // target di bawah dst milik test
	})
	if err != nil {
		t.Fatalf("copyRepo: %v", err)
	}
}

// treeChecksum menghitung SHA-256 atas (path relatif, isi) terurut, melewati node_modules.
func treeChecksum(t *testing.T, dir string) string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return fs.SkipDir
		}
		if d.Type().IsRegular() {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("treeChecksum: %v", err)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		rel, _ := filepath.Rel(dir, p)
		data, err := os.ReadFile(p) //nolint:gosec // salinan sementara
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// cmdTimeout membatasi satu perintah agar test yang menggantung tidak menghabiskan seluruh batas waktu.
const cmdTimeout = 20 * time.Minute

// runResult menjalankan perintah di dir dan mengembalikan keluaran gabungan serta exit code (-1 bila gagal dijalankan).
func runResult(t *testing.T, dir string, env []string, name string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // test memanggil tool repo (go, bun, task, erpora)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	err := cmd.Run()
	t.Logf("$ %s %s (%s)", name, strings.Join(args, " "), time.Since(start).Round(time.Second))
	if err == nil {
		return out.String(), 0
	}
	var xe *exec.ExitError
	if ok := asExit(err, &xe); ok {
		return out.String(), xe.ExitCode()
	}
	t.Fatalf("gagal menjalankan %s: %v", name, err)
	return "", -1
}

func asExit(err error, target **exec.ExitError) bool {
	xe, ok := err.(*exec.ExitError) //nolint:errorlint // exec.Cmd.Run mengembalikan *ExitError langsung
	if ok {
		*target = xe
	}
	return ok
}

// run menjalankan perintah dan gagal (t.Fatalf dengan keluaran lengkap) bila exit code bukan 0.
func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	out, code := runResult(t, dir, env, name, args...)
	if code != 0 {
		t.Fatalf("%s %s: exit %d\n%s", name, strings.Join(args, " "), code, out)
	}
	return out
}
