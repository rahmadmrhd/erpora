package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

// Codex PR#7 C6: teks error iterator/sink modul dapat memuat data; log hanya memuat kategori dan tipe error.
func TestExportLog_DoesNotLogRawErrorText(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(&buf, "json", slog.LevelDebug)
	secret := "NIK 3171234567890001 gaji 25000000"
	err := NewExportLog(log).RecordExport(context.Background(), export.Event{
		Resource: "x.note", Format: export.CSV, Filename: "notes", Columns: []string{"id"}, Rows: 0,
		Classification: "confidential", Err: fmt.Errorf("baris 4: %w", errors.New(secret)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "3171234567890001") || strings.Contains(buf.String(), "25000000") {
		t.Fatalf("log memuat teks error mentah: %s", buf.String())
	}
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec["error_kind"] != "failed" || rec["error_type"] != "*errors.errorString" {
		t.Errorf("record = %v", rec)
	}
	if _, has := rec["error"]; has {
		t.Errorf("field error mentah tidak boleh ada: %v", rec)
	}
}

func TestExportLog_ErrorKindCanceled(t *testing.T) {
	if errorKind(fmt.Errorf("x: %w", context.Canceled)) != "canceled" || errorKind(context.DeadlineExceeded) != "deadline_exceeded" {
		t.Error("errorKind")
	}
}
