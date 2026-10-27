package audit

import (
	"context"
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
		attrs = append(attrs, "error", ev.Err)
	}
	log.InfoContext(ctx, "ekspor data", attrs...)
	return nil
}
