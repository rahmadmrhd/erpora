package generate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var moduleDirRe = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// Semua galat jalur hanya menyebut jalur relatif di dalam repo dan pesan tetap: tidak pernah isi berkas atau
// tujuan symlink di luar modul (Requirement 19.5).
func unsafePath(rel, why string) error {
	return fmt.Errorf("%w: %s (%s)", ErrUnsafePath, rel, why)
}

// checkRel memvalidasi jalur keluaran: relatif, bergaris miring biasa, bersih, dan di dalam modules/<dir>/.
func checkRel(rel, dir string) error {
	if rel == "" || path.IsAbs(rel) || strings.Contains(rel, "\\") || strings.ContainsRune(rel, 0) {
		return unsafePath(rel, "bukan jalur relatif yang sah")
	}
	if path.Clean(rel) != rel {
		return unsafePath(rel, "jalur tidak bersih")
	}
	prefix := "modules/" + dir + "/"
	if !strings.HasPrefix(rel, prefix) || strings.Contains(rel, "../") || strings.HasSuffix(rel, "/") {
		return unsafePath(rel, "di luar direktori modul")
	}
	return nil
}

// guard memastikan tidak ada komponen jalur yang symlink, junction, atau berkas tidak biasa, dan hasil akhirnya
// tetap di bawah root. Komponen yang belum ada diizinkan (akan dibuat).
func guard(root, rel string) error {
	cur := root
	for _, seg := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return unsafePath(rel, "tidak dapat diperiksa")
		}
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return unsafePath(rel, "komponen jalur adalah symlink atau junction")
		}
	}
	return nil
}

// readExisting membaca berkas keluaran bila ada. Berkas harus biasa dan berada di bawah modul tanpa symlink.
func readExisting(root, rel string) ([]byte, bool, error) {
	if err := guard(root, rel); err != nil {
		return nil, false, err
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	fi, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, unsafePath(rel, "tidak dapat diperiksa")
	}
	if !fi.Mode().IsRegular() {
		return nil, false, unsafePath(rel, "bukan berkas biasa")
	}
	b, err := os.ReadFile(abs) //nolint:gosec // jalur sudah divalidasi checkRel dan guard
	if err != nil {
		return nil, false, fmt.Errorf("baca %s: %w", rel, errors.New("gagal dibaca"))
	}
	return b, true, nil
}

// isDir memeriksa direktori biasa (bukan symlink) di bawah root.
func isDir(root, rel string) (bool, error) {
	if err := guard(root, rel); err != nil {
		return false, err
	}
	fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, unsafePath(rel, "tidak dapat diperiksa")
	}
	return fi.IsDir(), nil
}

// listDir mendaftar nama berkas biasa di direktori (kosong bila tidak ada); symlink dilewati.
func listDir(root, rel string) ([]string, error) {
	ok, err := isDir(root, rel)
	if err != nil || !ok {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, unsafePath(rel, "direktori tidak dapat dibaca")
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// hasHeader melaporkan apakah isi berkas diawali kepala bangkitan.
func hasHeader(content []byte) bool {
	return strings.HasPrefix(string(content), GeneratedHeader+"\n")
}

// removeFile menghapus satu berkas bangkitan yatim (berkas biasa, tanpa symlink, di bawah modul).
func removeFile(root, rel string) error {
	if err := guard(root, rel); err != nil {
		return err
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	fi, err := os.Lstat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return unsafePath(rel, "bukan berkas biasa")
	}
	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("hapus %s: gagal", rel)
	}
	return nil
}
