package gen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Apply menerapkan Plan: (1) validasi seluruh Ops SEBELUM menulis apa pun (OpCreate pada file yang ada tanpa Force ->
// ErrExists yang menyebut path dan --force; OpModify pada file yang tidak ada -> error; path keluar root -> error);
// (2) DryRun mencetak "create <path>"/"modify <path>" beserta diff, tanpa menulis dan tanpa Post; (3) menulis lewat
// file sementara + rename dan membuat direktori; (4) menjalankan Post berurutan; mencetak ringkasan.
func Apply(ctx context.Context, p Plan, o Options) error {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	type step struct {
		op       FileOp
		abs      string
		existing []byte
		exists   bool
	}
	steps := make([]step, 0, len(p.Ops))
	seen := map[string]bool{}
	for _, op := range p.Ops {
		clean := path.Clean(op.Path)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "\\") {
			return fmt.Errorf("path %q keluar dari root repo", op.Path)
		}
		if seen[clean] {
			return fmt.Errorf("path %q muncul lebih dari sekali dalam rencana", op.Path)
		}
		seen[clean] = true
		abs := filepath.Join(o.Root, filepath.FromSlash(clean))
		if err := rejectSymlinks(o.Root, clean); err != nil {
			return err
		}
		existing, err := os.ReadFile(abs) //nolint:gosec // path sudah divalidasi berada di bawah root
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		switch op.Kind {
		case OpCreate:
			if exists && !o.Force {
				return fmt.Errorf("%w: %s (gunakan --force untuk menimpa)", ErrExists, clean)
			}
		case OpModify:
			if !exists {
				return fmt.Errorf("file %s tidak ada; tidak dapat diubah", clean)
			}
		}
		steps = append(steps, step{op: FileOp{Path: clean, Kind: op.Kind, Content: op.Content}, abs: abs, existing: existing, exists: exists})
	}

	if o.DryRun {
		for _, s := range steps {
			verb := "create"
			if s.op.Kind == OpModify {
				verb = "modify"
			} else if s.exists {
				verb = "overwrite"
			}
			fmt.Fprintf(out, "%s %s\n%s", verb, s.op.Path, UnifiedDiff(s.op.Path, s.existing, s.op.Content))
		}
		fmt.Fprintf(out, "dry-run: %d file, tidak ada yang ditulis\n", len(steps))
		return nil
	}

	var written []step
	for _, s := range steps {
		if s.exists && bytes.Equal(s.existing, s.op.Content) {
			continue
		}
		if err := writeFileFn(s.abs, s.op.Content); err != nil {
			// Kembalikan file yang sudah ditulis (terbalik): dihapus bila baru, isi lama dipulihkan bila sudah ada.
			restored := 0
			for i := len(written) - 1; i >= 0; i-- {
				w := written[i]
				var rerr error
				if w.exists {
					rerr = writeFileFn(w.abs, w.existing)
				} else {
					rerr = os.Remove(w.abs)
				}
				if rerr == nil {
					restored++
				}
			}
			return fmt.Errorf("tulis %s: %w (%d dari %d file yang sudah ditulis dikembalikan)", s.op.Path, err, restored, len(written))
		}
		written = append(written, s)
	}
	for _, s := range steps {
		verb := "create"
		if s.op.Kind == OpModify {
			verb = "modify"
		} else if s.exists {
			verb = "overwrite"
		}
		fmt.Fprintf(out, "%s %s\n", verb, s.op.Path)
	}
	for _, ps := range p.Post {
		fmt.Fprintf(out, "jalankan %s\n", ps.Name)
		if err := ps.Run(ctx, o.Root); err != nil {
			return fmt.Errorf("langkah %s gagal: %w", ps.Name, err)
		}
	}
	fmt.Fprintf(out, "selesai: %d file\n", len(steps))
	return nil
}

// rejectSymlinks menolak target yang itu sendiri atau salah satu direktori induknya (di bawah root) adalah symlink
// (atau titik mount/junction), agar penulisan tidak bisa diarahkan keluar dari root repo. Komponen yang belum ada
// dilewati (akan dibuat sebagai direktori biasa). Sebagai lapis kedua, ancestor terdalam yang ada harus tetap berada di
// bawah EvalSymlinks(root).
func rejectSymlinks(root, clean string) error {
	cur := root
	deepest := root
	for _, seg := range strings.Split(clean, "/") {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return err
		}
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return fmt.Errorf("%s: komponen path adalah symlink atau junction; generator menolak menulis lewat symlink (hapus atau ganti dengan direktori biasa)", clean)
		}
		deepest = cur
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realDeepest, err := filepath.EvalSymlinks(deepest)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(realRoot, realDeepest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s: path berada di luar root repo setelah symlink diselesaikan", clean)
	}
	return nil
}

// writeFileFn menulis satu file; variabel agar test dapat menyuntikkan kegagalan.
var writeFileFn = writeAtomic

// writeAtomic menulis lewat file sementara di direktori yang sama lalu rename.
func writeAtomic(abs string, content []byte) error {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".erpora-gen-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil { //nolint:gosec // file sumber repo, bukan rahasia
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, abs); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
