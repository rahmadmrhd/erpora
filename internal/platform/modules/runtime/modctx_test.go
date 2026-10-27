package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"erpora/internal/platform/database"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	"erpora/sdk/authz/authztest"
	"erpora/sdk/data"
	"erpora/sdk/export"
)

func fakeConnector(t *testing.T) database.TenantConnector {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=x dbname=x"}),
		&gorm.Config{DisableAutomaticPing: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	return database.SingleTenantConnector{Default: tenantA, DB: database.TenantDB{DB: db}}
}

func TestModuleContext_CarriesTenantProviderModuleAndSystem(t *testing.T) {
	ctx, env := ModuleContext(context.Background(), tenantA, "acme.notes", fakeConnector(t), nil, "module.lifecycle:enable", Services{})
	if id, ok := tenancy.FromContext(ctx); !ok || id != tenantA {
		t.Errorf("tenant = %v, %v", id, ok)
	}
	if _, err := data.DB(ctx); err != nil {
		t.Errorf("data.DB: %v", err)
	}
	if got := observability.ModuleFromContext(ctx); got != "acme.notes" {
		t.Errorf("module = %q", got)
	}
	if r, ok := authz.SystemReason(ctx); !ok || r != "module.lifecycle:enable" {
		t.Errorf("system reason = %q, %v", r, ok)
	}
	if env.TenantID != tenantA.String() || env.Logger == nil {
		t.Errorf("env = %+v", env)
	}
}

func TestModuleContext_NoSystemReasonWhenEmpty(t *testing.T) {
	ctx, _ := ModuleContext(context.Background(), tenantA, "acme.notes", fakeConnector(t), nil, "", Services{})
	if _, ok := authz.SystemReason(ctx); ok {
		t.Error("reason kosong tidak boleh memasang hak sistem")
	}
}

// Carry-over M2a: logger ter-scope tidak menduplikasi erp.module.key lewat TraceHandler.
func TestModuleContext_LoggerSingleModuleKey(t *testing.T) {
	var buf bytes.Buffer
	base := observability.NewLogger(&buf, "json", slog.LevelDebug)
	ctx, env := ModuleContext(context.Background(), tenantA, "acme.notes", fakeConnector(t), base, "", Services{})
	env.Logger.InfoContext(ctx, "x")
	line := strings.TrimSpace(buf.String())
	if n := strings.Count(line, `"erp.module.key"`); n != 1 {
		t.Fatalf("erp.module.key muncul %d kali: %s", n, line)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil || m["erp.module.key"] != "acme.notes" {
		t.Errorf("log = %s (%v)", line, err)
	}
}

func TestModuleContext_InstallsServices(t *testing.T) {
	ctx, _ := ModuleContext(context.Background(), tenantA, "acme.notes", fakeConnector(t), nil, "",
		Services{Masker: authztest.AllowAllFields, Auditor: &recAuditor{}})
	if !authz.HasFieldMasker(ctx) || !export.HasAuditor(ctx) {
		t.Error("ModuleContext harus memasang masker dan auditor")
	}
	ctx, _ = ModuleContext(context.Background(), tenantA, "acme.notes", fakeConnector(t), nil, "", Services{})
	if authz.HasFieldMasker(ctx) || export.HasAuditor(ctx) {
		t.Error("Services kosong tidak boleh memasang apa pun")
	}
}
