package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"erpora/sdk/export"
)

// NewExportLog membuat export.Auditor sementara sampai audit writer PRD-001: INFO "ekspor data" tanpa
// nilai sel. erp.module.key ditambahkan TraceHandler dari context.
func NewExportLog(log *slog.Logger) export.Auditor { return exportLog{log: log} }

type exportLog struct{ log *slog.Logger }

func (e exportLog) RecordExport(ctx context.Context, ev export.Event) error {
	log := e.log
	if log == nil {
		log = slog.Default()
	}
	attrs := []any{
		"resource", ev.Resource, "format", string(ev.Format), "filename", ev.Filename,
		"rows", ev.Rows, "columns", ev.Columns, "classification", string(ev.Classification),
	}
	if ev.Err != nil {
		// Teks error iterator/sink modul dapat memuat nilai data (mis. isi sel); catat hanya kategori dan tipe (Codex PR#7 C6).
		attrs = append(attrs, "error_kind", errorKind(ev.Err), "error_type", rootErrorType(ev.Err))
	}
	log.InfoContext(ctx, "ekspor data", attrs...)
	return nil
}

// errorKind mengelompokkan error tanpa membuka teksnya.
func errorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "failed"
	}
}

// rootErrorType mengembalikan nama tipe error paling dalam pada rantai Unwrap (tanpa teks pesan).
func rootErrorType(err error) string {
	for {
		next := errors.Unwrap(err)
		if next == nil {
			break
		}
		err = next
	}
	return fmt.Sprintf("%T", err)
}
