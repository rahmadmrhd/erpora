package observability_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rahmadmrhd/erp-go/internal/platform/config"
	"github.com/rahmadmrhd/erp-go/internal/platform/observability"
)

// Finding 7: OTEL_* dari --env-file harus sampai ke Setup (sama seperti ERP_*).
func TestSetup_HonoursOTelVarsFromEnvFile(t *testing.T) {
	for _, k := range []string{"ERP_DATABASE_URL", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_SERVICE_NAME"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	p := filepath.Join(t.TempDir(), ".env")
	body := "ERP_DATABASE_URL=postgres://u:p@h/db\nOTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318\nOTEL_SERVICE_NAME=dari-file\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(p); err != nil {
		t.Fatal(err)
	}
	prov, shutdown, err := observability.Setup(context.Background(), observability.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_ = shutdown(ctx)
	}()
	if !prov.Enabled {
		t.Fatal("Enabled=false: OTEL_EXPORTER_OTLP_ENDPOINT dari env-file diabaikan")
	}
	for _, kv := range prov.Resource.Attributes() {
		if kv.Key == "service.name" && kv.Value.AsString() != "dari-file" {
			t.Fatalf("service.name = %s", kv.Value.AsString())
		}
	}
}
