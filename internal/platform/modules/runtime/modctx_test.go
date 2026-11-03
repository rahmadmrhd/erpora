package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"erpora/internal/platform/database"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	"erpora/sdk/authz/authztest"
	"erpora/sdk/data"
	"erpora/sdk/export"
	sdki18n "erpora/sdk/i18n"
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

func TestModuleContext_InstallsI18nFromServices(t *testing.T) {
	var gotT tenancy.TenantID
	var gotKey string
	svc := Services{I18n: func(ctx context.Context, tid tenancy.TenantID, key string) context.Context {
		gotT, gotKey = tid, key
		return sdki18n.WithScope(sdki18n.WithLocale(ctx, "id"), "notes")
	}}
	ctx, _ := ModuleContext(context.Background(), tenantA, "acme.notes", fakeConnector(t), nil, "", svc)
	if gotT != tenantA || gotKey != "acme.notes" {
		t.Errorf("hook dipanggil dengan %v, %q", gotT, gotKey)
	}
	if sc, _ := sdki18n.ScopeFrom(ctx); sc != "notes" {
		t.Errorf("scope = %q", sc)
	}
	if l, _ := sdki18n.LocaleFrom(ctx); l != "id" {
		t.Errorf("locale = %q", l)
	}
}

type ctxMarkerKey struct{}

// ADR-0013 7a butir 5: identity.Service/Admin hanya dipasang pada context modul core; penegakan ada di platform
// (installIdentity), bukan pada hook, sehingga hook yang salah rakit pun tidak dapat bocor ke modul lain.
func TestModuleContext_IdentityInstalledOnlyForCoreModule(t *testing.T) {
	svc := Services{Identity: func(ctx context.Context) context.Context {
		return context.WithValue(ctx, ctxMarkerKey{}, true)
	}}
	conn := fakeConnector(t)
	core, _ := ModuleContext(context.Background(), tenantA, CoreModuleKey, conn, nil, "cli:core:create-admin", svc)
	if core.Value(ctxMarkerKey{}) != true {
		t.Error("modul core harus menerima identitas")
	}
	for _, key := range []string{"acme.notes", "evil.core", "erp.core-addon", ""} {
		other, _ := ModuleContext(context.Background(), tenantA, key, conn, nil, "", svc)
		if other.Value(ctxMarkerKey{}) != nil {
			t.Errorf("modul %q tidak boleh menerima identitas", key)
		}
	}
}

func TestRouteContext_IdentityInstalledOnlyForCoreModuleRoutes(t *testing.T) {
	svc := Services{Identity: func(ctx context.Context) context.Context {
		return context.WithValue(ctx, ctxMarkerKey{}, true)
	}}
	for module, want := range map[string]bool{CoreModuleKey: true, "acme.notes": false} {
		var got bool
		h := requestLimitsMW(registry.RouteInfo{Module: module, MaxBodyBytes: 1024}, svc, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = r.Context().Value(ctxMarkerKey{}) == true
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
		if got != want {
			t.Errorf("modul %s: identitas terpasang = %v, want %v", module, got, want)
		}
	}
}
