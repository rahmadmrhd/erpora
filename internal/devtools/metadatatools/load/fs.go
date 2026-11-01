package load

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// kindBySuffix memetakan akhiran nama berkas ke kind (Requirement 1.1; tipe domain memakai .type.yaml).
var kindBySuffix = []struct{ suffix, kind string }{
	{".resource.yaml", "resource"},
	{".abstract.yaml", "abstract"},
	{".type.yaml", "fieldType"},
}

// LoadFS memuat semua berkas metadata di bawah root (rekursif, urutan leksikal sehingga deterministik).
// Berkas selain .yaml/.yml diabaikan; .yaml/.yml tanpa akhiran kind yang dikenal, tautan simbolik, dan berkas
// yang kind-nya tidak cocok dengan akhiran dilaporkan. Direktori yang tidak ada bukan galat (modul tanpa
// metadata tidak terpengaruh). Hanya dokumen tanpa temuan yang dikembalikan. fs.FS bersifat hanya-baca: tidak
// ada yang ditulis.
func LoadFS(fsys fs.FS, root string) ([]*Document, []Problem) {
	return LoadFSWithLimits(fsys, root, DefaultLimits())
}

// LoadFSWithLimits seperti LoadFS dengan batas khusus.
func LoadFSWithLimits(fsys fs.FS, root string, lim Limits) ([]*Document, []Problem) {
	var docs []*Document
	var probs []Problem
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			probs = append(probs, Problem{File: p, Code: CodeRead, Message: "tidak dapat dibaca: " + err.Error()})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			probs = append(probs, Problem{File: p, Code: CodeSymlink, Message: "tautan simbolik tidak diikuti"})
			return nil
		}
		want := ""
		for _, ks := range kindBySuffix {
			if strings.HasSuffix(name, ks.suffix) {
				want = ks.kind
			}
		}
		if want == "" {
			probs = append(probs, Problem{File: p, Code: CodeFileName,
				Message: "nama berkas harus berakhiran .resource.yaml, .abstract.yaml, atau .type.yaml"})
			return nil
		}
		data, rerr := readLimited(fsys, p, lim.MaxFileBytes)
		if rerr != nil {
			probs = append(probs, Problem{File: p, Code: CodeRead, Message: "tidak dapat dibaca: " + rerr.Error()})
			return nil
		}
		doc, ps := ParseWithLimits(p, data, lim)
		if len(ps) > 0 {
			probs = append(probs, ps...)
			return nil
		}
		if doc.Kind != want {
			pos := doc.Pos("/kind")
			probs = append(probs, Problem{File: p, Line: pos.Line, Column: pos.Column, Pointer: "/kind", Code: CodeKindMismatch,
				Message: fmt.Sprintf("kind %q tidak cocok dengan akhiran nama berkas (diharapkan %q)", doc.Kind, want)})
			return nil
		}
		docs = append(docs, doc)
		return nil
	})
	if err != nil && !errors.Is(err, fs.SkipAll) {
		probs = append(probs, Problem{File: root, Code: CodeRead, Message: err.Error()})
	}
	sortProblems(probs)
	return docs, probs
}

// readLimited membaca paling banyak max+1 byte agar berkas raksasa tidak dimuat penuh ke memori.
func readLimited(fsys fs.FS, name string, max int) ([]byte, error) {
	f, err := fsys.Open(path.Clean(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(max)+1))
}
