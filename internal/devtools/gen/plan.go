// Package gen adalah mesin generator developer tools (PRD-000 FR-22): rencana perubahan file (Plan), penerapan aman
// (Apply: tanpa menimpa, dry-run dengan diff), penyisipan terstruktur pada penanda, dan pembangkit make:*.
package gen

import (
	"context"
	"errors"
	"io"
)

// OpKind adalah jenis operasi file.
type OpKind int

const (
	// OpCreate membuat file baru; gagal bila file sudah ada kecuali Options.Force.
	OpCreate OpKind = iota
	// OpModify mengganti isi file yang sudah ada (Content = isi baru utuh); gagal bila file tidak ada.
	OpModify
)

// FileOp adalah satu operasi file. Path relatif terhadap root, dengan slash.
type FileOp struct {
	Path    string
	Kind    OpKind
	Content []byte
}

// PostStep adalah langkah setelah file ditulis (mis. bun install); tidak dijalankan pada dry-run.
type PostStep struct {
	Name string
	Run  func(ctx context.Context, root string) error
}

// Plan adalah seluruh perubahan satu generator.
type Plan struct {
	Ops  []FileOp
	Post []PostStep
}

// Options mengatur Apply.
type Options struct {
	Root          string
	Force, DryRun bool
	Out           io.Writer
}

var (
	// ErrExists: file target OpCreate sudah ada (gunakan --force untuk menimpa).
	ErrExists = errors.New("file sudah ada")
	// ErrMarkerNotFound: penanda generator tidak ada di file yang akan diubah.
	ErrMarkerNotFound = errors.New("penanda tidak ditemukan")
	// ErrAmbiguousMarker: penanda muncul lebih dari sekali.
	ErrAmbiguousMarker = errors.New("penanda muncul lebih dari sekali")
)
