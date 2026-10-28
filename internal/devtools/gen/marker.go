package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/imports"
)

// InsertAtMarker menyisipkan line (indentasi = indentasi baris penanda) tepat di atas baris yang isinya persis marker
// (setelah di-trim). line identik yang sudah ada di src membuat src tak berubah. Penanda tidak ada ->
// ErrMarkerNotFound dengan pesan "tambahkan `<line>` secara manual di <path>"; penanda ganda -> ErrAmbiguousMarker.
func InsertAtMarker(path string, src []byte, marker, line string) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	idx := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == marker {
			if idx >= 0 {
				return nil, fmt.Errorf("%w: %q di %s", ErrAmbiguousMarker, marker, path)
			}
			idx = i
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: %q tidak ada di %s; tambahkan `%s` secara manual di %s dan pasang kembali penandanya",
			ErrMarkerNotFound, marker, path, line, path)
	}
	for _, l := range lines {
		if strings.TrimSpace(l) == strings.TrimSpace(line) {
			return src, nil
		}
	}
	indent := lines[idx][:len(lines[idx])-len(strings.TrimLeft(lines[idx], " \t"))]
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:idx]...)
	out = append(out, indent+line)
	out = append(out, lines[idx:]...)
	return []byte(strings.Join(out, "\n")), nil
}

// AddGoImport menambah impor (name "" = tanpa alias) lewat astutil dan memformat hasilnya; impor yang sudah ada tak berubah.
func AddGoImport(path string, src []byte, name, importPath string) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("uraikan %s: %w", path, err)
	}
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil && p == importPath {
			return src, nil
		}
	}
	astutil.AddNamedImport(fset, f, name, importPath)
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, fmt.Errorf("format %s: %w", path, err)
	}
	return buf.Bytes(), nil
}

// FormatGo memformat dan merapikan impor (goimports in-process).
func FormatGo(path string, src []byte) ([]byte, error) {
	out, err := imports.Process(path, src, &imports.Options{Comments: true, TabIndent: true, TabWidth: 8, FormatOnly: true})
	if err != nil {
		return nil, fmt.Errorf("format %s: %w", path, err)
	}
	return out, nil
}
