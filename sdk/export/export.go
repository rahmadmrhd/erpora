package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"regexp"

	"erpora/sdk/authz"
	"erpora/sdk/module"
)

// Format adalah format berkas ekspor.
type Format string

const (
	CSV  Format = "csv"
	XLSX Format = "xlsx"
)

// DefaultMaxRows adalah batas baris ekspor sinkron (api-guidelines §10 melarang proses panjang tanpa Jobs).
const DefaultMaxRows = 10000

var (
	ErrInvalidSpec             = errors.New("export: spec tidak valid")
	ErrForbiddenClassification = errors.New("export: kolom highly_confidential tidak boleh diekspor")
	ErrNoAuditor               = errors.New("export: auditor ekspor tidak terpasang di context")
	ErrNoFieldMasker           = errors.New("export: field masker tidak terpasang di context")
	ErrTooManyRows             = errors.New("export: jumlah baris melebihi batas")
	ErrNonScalarValue          = errors.New("export: nilai sel harus skalar")
)

// Column adalah satu kolom ekspor; Classification wajib diisi (kolom highly_confidential ditolak).
type Column struct {
	Key, Header    string
	Classification module.Classification
}

// Spec mendeskripsikan satu ekspor.
type Spec struct {
	Resource string // resource authz, mis. "example.note"
	Format   Format
	Filename string // nama dasar tanpa ekstensi; ekstensi dari Format
	Columns  []Column
	MaxRows  int // 0 -> DefaultMaxRows
}

// Row adalah satu baris sumber, dikunci dengan Column.Key.
type Row = map[string]any

// Event adalah catatan audit satu ekspor; tidak pernah memuat nilai sel.
type Event struct {
	Resource, Filename string
	Format             Format
	Columns            []string
	Rows               int
	Classification     module.Classification // tertinggi di antara kolom
	Err                error
}

// Auditor mencatat ekspor; diimplementasi platform (log terstruktur sebelum PRD-001).
type Auditor interface {
	RecordExport(ctx context.Context, e Event) error
}

type auditorKey struct{}

// WithAuditor memasang auditor di ctx; dipanggil platform.
func WithAuditor(ctx context.Context, a Auditor) context.Context {
	return context.WithValue(ctx, auditorKey{}, a)
}

// HasAuditor melaporkan apakah ctx membawa auditor.
func HasAuditor(ctx context.Context) bool {
	a, ok := ctx.Value(auditorKey{}).(Auditor)
	return ok && a != nil
}

var filenameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSpec, fmt.Sprintf(format, args...))
}

// Validate memeriksa spec, klasifikasi kolom, masker, dan auditor. Dipanggil Write/Serve sebelum byte pertama.
func Validate(ctx context.Context, spec Spec) error {
	if spec.Resource == "" {
		return invalid("Resource kosong")
	}
	if spec.Format != CSV && spec.Format != XLSX {
		return invalid("Format %q tidak dikenal", spec.Format)
	}
	if !filenameRE.MatchString(spec.Filename) {
		return invalid("Filename tidak valid")
	}
	if spec.MaxRows < 0 {
		return invalid("MaxRows negatif")
	}
	if len(spec.Columns) == 0 {
		return invalid("minimal satu kolom")
	}
	seen := map[string]bool{}
	var highest module.Classification
	for _, c := range spec.Columns {
		if c.Key == "" || seen[c.Key] {
			return invalid("Key kolom %q kosong atau ganda", c.Key)
		}
		seen[c.Key] = true
		if !c.Classification.Valid() {
			return invalid("kolom %q tidak punya klasifikasi valid", c.Key)
		}
		if c.Classification == module.ClassHighlyConfidential {
			return fmt.Errorf("%w: kolom %q", ErrForbiddenClassification, c.Key)
		}
		highest = module.MaxClassification(highest, c.Classification)
	}
	if !authz.HasFieldMasker(ctx) {
		return ErrNoFieldMasker
	}
	if highest.AtLeast(module.ClassConfidential) && !HasAuditor(ctx) {
		return ErrNoAuditor
	}
	return nil
}

func highestClass(cols []Column) module.Classification {
	var h module.Classification
	for _, c := range cols {
		h = module.MaxClassification(h, c.Classification)
	}
	return h
}

// sink adalah penulis format (CSV atau XLSX).
type sink interface {
	header(cells []string) error
	row(cells []string) error
	flush() error
	close() error
}

// Write menulis header dan baris ke w. Setiap baris melewati authz.MaskFields(ctx, spec.Resource, row);
// kolom yang di-mask tampil kosong. Auditor (bila terpasang) dipanggil sekali sesudah selesai, baik
// sukses maupun gagal. Mengembalikan jumlah baris yang ditulis.
func Write(ctx context.Context, w io.Writer, spec Spec, rows iter.Seq2[Row, error]) (int, error) {
	if err := Validate(ctx, spec); err != nil {
		return 0, err
	}
	count, err := write(ctx, w, spec, rows)
	if a, ok := ctx.Value(auditorKey{}).(Auditor); ok && a != nil {
		keys := make([]string, len(spec.Columns))
		for i, c := range spec.Columns {
			keys[i] = c.Key
		}
		aerr := a.RecordExport(ctx, Event{
			Resource: spec.Resource, Filename: spec.Filename, Format: spec.Format, Columns: keys,
			Rows: count, Classification: highestClass(spec.Columns), Err: err,
		})
		if err == nil && aerr != nil {
			err = fmt.Errorf("export: gagal mencatat audit: %w", aerr)
		}
	}
	return count, err
}

func write(ctx context.Context, w io.Writer, spec Spec, rows iter.Seq2[Row, error]) (count int, err error) {
	var s sink
	switch spec.Format {
	case CSV:
		s, err = newCSVSink(w)
	default:
		s, err = newXLSXSink(w)
	}
	if err != nil {
		return 0, err
	}
	headers := make([]string, len(spec.Columns))
	for i, c := range spec.Columns {
		headers[i] = c.Header
	}
	if err = s.header(headers); err != nil {
		return 0, err
	}
	// Preamble ikut terkirim sebelum baris pertama: sejak ini kegagalan dianggap terjadi di tengah stream.
	if err = s.flush(); err != nil {
		return 0, err
	}

	maxRows := spec.MaxRows
	if maxRows == 0 {
		maxRows = DefaultMaxRows
	}
	cells := make([]string, len(spec.Columns))
	for row, rerr := range rows {
		if rerr != nil {
			return count, rerr
		}
		if count >= maxRows {
			return count, ErrTooManyRows
		}
		masked, _ := authz.MaskFields(ctx, spec.Resource, row).(map[string]any)
		for i, c := range spec.Columns {
			cells[i], err = formatCell(c.Key, masked[c.Key])
			if err != nil {
				return count, err
			}
		}
		if err = s.row(cells); err != nil {
			return count, err
		}
		count++
	}
	if err = s.close(); err != nil {
		return count, err
	}
	return count, nil
}
