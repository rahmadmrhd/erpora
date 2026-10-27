package gen

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// Review Focus 2: target yang sudah ada menggagalkan seluruh rencana sebelum file pertama ditulis.
func TestApply_ExistsFailsBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	write(t, root, "b.go", "package b\n")
	before := checksum(t, root)
	err := Apply(context.Background(), Plan{Ops: []FileOp{
		{Path: "a.go", Kind: OpCreate, Content: []byte("package a\n")},
		{Path: "b.go", Kind: OpCreate, Content: []byte("package new\n")},
	}}, Options{Root: root})
	if !errors.Is(err, ErrExists) || !strings.Contains(err.Error(), "b.go") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want ErrExists menyebut b.go dan --force", err)
	}
	if exists(root, "a.go") || checksum(t, root) != before {
		t.Error("tidak boleh ada file yang ditulis")
	}
}

func TestApply_ForceOverwritesCreatesOnly(t *testing.T) {
	root := t.TempDir()
	write(t, root, "b.go", "package b\n")
	write(t, root, "mod.txt", "lama\n")
	err := Apply(context.Background(), Plan{Ops: []FileOp{
		{Path: "b.go", Kind: OpCreate, Content: []byte("package new\n")},
		{Path: "mod.txt", Kind: OpModify, Content: []byte("baru\n")},
	}}, Options{Root: root, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "b.go")); string(got) != "package new\n" {
		t.Errorf("b.go = %q", got)
	}
	// OpModify pada file yang tidak ada tetap gagal walau Force.
	err = Apply(context.Background(), Plan{Ops: []FileOp{{Path: "tidak-ada.txt", Kind: OpModify, Content: []byte("x")}}}, Options{Root: root, Force: true})
	if err == nil {
		t.Error("OpModify pada file yang tidak ada harus gagal")
	}
}

func TestApply_DryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.work", "go 1.27\n\nuse (\n\t.\n)\n")
	before := checksum(t, root)
	called := false
	var out bytes.Buffer
	err := Apply(context.Background(), Plan{
		Ops: []FileOp{
			{Path: "a.go", Kind: OpCreate, Content: []byte("package a\n")},
			{Path: "go.work", Kind: OpModify, Content: []byte("go 1.27\n\nuse (\n\t.\n\t./modules/x\n)\n")},
		},
		Post: []PostStep{{Name: "bun install", Run: func(context.Context, string) error { called = true; return nil }}},
	}, Options{Root: root, DryRun: true, Out: &out})
	if err != nil {
		t.Fatal(err)
	}
	if checksum(t, root) != before {
		t.Error("dry-run mengubah isi direktori")
	}
	if called {
		t.Error("Post tidak boleh dipanggil pada dry-run")
	}
	for _, want := range []string{"create a.go", "modify go.work", "+\t./modules/x", "+package a"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("keluaran dry-run tidak memuat %q:\n%s", want, out.String())
		}
	}
}

func TestApply_PathEscapeRejected(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"../x", "a/../../x", "/abs", `a\b`} {
		err := Apply(context.Background(), Plan{Ops: []FileOp{{Path: p, Kind: OpCreate, Content: []byte("x")}}}, Options{Root: root})
		if err == nil {
			t.Errorf("path %q harus ditolak", p)
		}
	}
}

// G-6: symlink pada target atau direktori induknya ditolak sebelum menulis apa pun, sehingga generator tidak bisa
// diarahkan keluar dari root repo lewat symlink.
func TestApply_SymlinkTargetOrParentRejected(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	write(t, outside, "secret.txt", "asli")
	// Direktori: symlink, atau junction (mklink /J, tanpa hak khusus) di Windows.
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		if runtime.GOOS != "windows" {
			t.Skipf("symlink tidak didukung: %v", err)
		}
		if out, jerr := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, "linkdir"), outside).CombinedOutput(); jerr != nil {
			t.Skipf("junction tidak dapat dibuat: %v\n%s", jerr, out)
		}
	}
	write(t, root, "ok/file.txt", "x")
	cases := []struct {
		name string
		op   FileOp
	}{
		{"induk symlink (create)", FileOp{Path: "linkdir/baru.txt", Kind: OpCreate, Content: []byte("jahat")}},
		{"induk symlink bersarang", FileOp{Path: "linkdir/a/b/baru.txt", Kind: OpCreate, Content: []byte("jahat")}},
	}
	// File symlink hanya dapat dibuat bila lingkungan mengizinkan (Linux/macOS, atau Windows dengan hak symlink).
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "linkfile.txt")); err == nil {
		cases = append(cases,
			struct {
				name string
				op   FileOp
			}{"target symlink (modify)", FileOp{Path: "linkfile.txt", Kind: OpModify, Content: []byte("jahat")}},
			struct {
				name string
				op   FileOp
			}{"target symlink (create --force)", FileOp{Path: "linkfile.txt", Kind: OpCreate, Content: []byte("jahat")}},
		)
	}
	for _, c := range cases {
		err := Apply(context.Background(), Plan{Ops: []FileOp{{Path: "ok/baru.txt", Kind: OpCreate, Content: []byte("x")}, c.op}},
			Options{Root: root, Force: true})
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Errorf("%s: err = %v, want penolakan symlink", c.name, err)
		}
		if exists(root, "ok/baru.txt") {
			t.Errorf("%s: tidak boleh ada file ditulis ketika rencana ditolak", c.name)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "secret.txt")); string(b) != "asli" {
		t.Errorf("file di luar root berubah: %q", b)
	}
	if exists(outside, "baru.txt") || exists(outside, "a/b/baru.txt") {
		t.Error("file baru muncul di luar root")
	}
}

// G-7: kegagalan menulis di tengah Apply mengembalikan file yang sudah ditulis (dihapus bila baru, isi lama dipulihkan
// bila sudah ada). Kegagalan langkah lanjutan (Post) tidak di-rollback; itu didokumentasikan di generators.md.
func TestApply_WriteFailureRollsBackEarlierWrites(t *testing.T) {
	root := t.TempDir()
	write(t, root, "b.txt", "lama")
	orig := writeFileFn
	t.Cleanup(func() { writeFileFn = orig })
	writeFileFn = func(abs string, content []byte) error {
		if strings.HasSuffix(filepath.ToSlash(abs), "/c.txt") {
			return errors.New("disk penuh")
		}
		return orig(abs, content)
	}
	err := Apply(context.Background(), Plan{Ops: []FileOp{
		{Path: "a.txt", Kind: OpCreate, Content: []byte("baru")},
		{Path: "b.txt", Kind: OpModify, Content: []byte("diubah")},
		{Path: "c.txt", Kind: OpCreate, Content: []byte("gagal")},
	}}, Options{Root: root})
	if err == nil || !strings.Contains(err.Error(), "disk penuh") || !strings.Contains(err.Error(), "dikembalikan") {
		t.Fatalf("err = %v, want kegagalan tulis dengan catatan rollback", err)
	}
	if exists(root, "a.txt") {
		t.Error("a.txt (baru) harus dihapus oleh rollback")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "b.txt")); string(b) != "lama" {
		t.Errorf("b.txt = %q, want isi lama dipulihkan", b)
	}
	if exists(root, "c.txt") {
		t.Error("c.txt tidak boleh ada")
	}
}

func TestApply_WritesFilesAndRunsPostInOrder(t *testing.T) {
	root := t.TempDir()
	var order []string
	err := Apply(context.Background(), Plan{
		Ops: []FileOp{{Path: "d/e/f.txt", Kind: OpCreate, Content: []byte("isi\n")}},
		Post: []PostStep{
			{Name: "satu", Run: func(context.Context, string) error { order = append(order, "satu"); return nil }},
			{Name: "dua", Run: func(context.Context, string) error { order = append(order, "dua"); return nil }},
		},
	}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "d", "e", "f.txt")); string(got) != "isi\n" {
		t.Errorf("isi = %q", got)
	}
	if strings.Join(order, ",") != "satu,dua" {
		t.Errorf("urutan Post = %v", order)
	}
	// Tidak ada sisa file sementara.
	entries, _ := os.ReadDir(filepath.Join(root, "d", "e"))
	if len(entries) != 1 {
		t.Errorf("sisa file sementara: %v", entries)
	}
}

func TestUnifiedDiff_Basic(t *testing.T) {
	got := UnifiedDiff("x.txt", []byte("a\nb\nc\n"), []byte("a\nB\nc\nd\n"))
	for _, want := range []string{"--- a/x.txt", "+++ b/x.txt", "-b", "+B", "+d", " a", " c"} {
		if !strings.Contains(got, want) {
			t.Errorf("diff tidak memuat %q:\n%s", want, got)
		}
	}
	if got := UnifiedDiff("x.txt", []byte("sama\n"), []byte("sama\n")); strings.Contains(got, "@@") {
		t.Errorf("diff identik tidak boleh punya hunk:\n%s", got)
	}
	created := UnifiedDiff("n.txt", nil, []byte("baru\n"))
	if !strings.Contains(created, "+baru") || !strings.Contains(created, "@@ -0,0 +1,1 @@") {
		t.Errorf("diff file baru:\n%s", created)
	}
}
