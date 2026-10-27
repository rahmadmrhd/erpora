package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"erpora/internal/platform/observability"
	"erpora/sdk/export"
)

func TestExportLog_RecordsEventWithoutValues(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(&buf, "json", slog.LevelDebug)
	ctx := observability.WithModule(context.Background(), "acme.x")
	err := NewExportLog(log).RecordExport(ctx, export.Event{
		Resource: "x.note", Format: export.CSV, Filename: "notes", Columns: []string{"id"}, Rows: 3,
		Classification: "confidential",
	})
	if err != nil {
		t.Fatal(err)
	}
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	if rec["msg"] != "ekspor data" || rec["rows"] != float64(3) || rec["classification"] != "confidential" ||
		rec["erp.module.key"] != "acme.x" || rec["resource"] != "x.note" {
		t.Errorf("record = %v", rec)
	}
}
