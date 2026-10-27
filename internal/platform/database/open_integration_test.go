//go:build integration

package database_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/testsupport/pgtest"
)

// FR-07 guardrail 5: PrepareStmt aktif di semua mode.
func TestOpen_PrepareStmtEnabled(t *testing.T) {
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: pgtest.Start(t), DBSlowQuery: 200 * time.Millisecond}, noop.NewTracerProvider(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	if !db.PrepareStmt {
		t.Fatal("PrepareStmt harus true")
	}
}
