//go:build integration

package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	tnoop "go.opentelemetry.io/otel/trace/noop"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/database"
	"erpora/internal/platform/migrate"
	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/observability"
	"erpora/internal/platform/testsupport/pgtest"
)

func testProviders() observability.Providers {
	return observability.Providers{Tracer: tnoop.NewTracerProvider(), Meter: noop.NewMeterProvider()}
}

func testLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func openTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := database.Open(context.Background(),
		config.Config{DatabaseURL: dsn, DBSlowQuery: 200 * time.Millisecond}, tnoop.NewTracerProvider(), testLog())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	return db
}

func get(h http.Handler, target string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type bootData struct {
	Data struct {
		Modules []struct {
			Key, Status, BasePath string
		}
		Revision int64
	}
}

func bootstrapOf(t *testing.T, h http.Handler) (bootData, string) {
	t.Helper()
	rec := get(h, "/api/bootstrap", nil)
	if rec.Code != 200 {
		t.Fatalf("bootstrap = %d %s", rec.Code, rec.Body)
	}
	var b bootData
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b, rec.Header().Get("ETag")
}

func statusOf(b bootData, key string) string {
	for _, m := range b.Data.Modules {
		if m.Key == key {
			return m.Status
		}
	}
	return ""
}

func setupAssembly(t *testing.T) (context.Context, *assembly, string, *gorm.DB) {
	t.Helper()
	dsn := pgtest.Start(t)
	db := openTestDB(t, dsn)
	cfg := config.Config{DatabaseURL: dsn, Env: config.EnvDevelopment, AuthzStub: "allow", DBSlowQuery: 200 * time.Millisecond}
	if err := migrate.Up(context.Background(), db, cfg.Env); err != nil {
		t.Fatal(err)
	}
	reg, err := BuildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a, err := assemble(ctx, cfg, testLog(), db, testProviders(), reg)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, a, dsn, db
}

func TestAssemble_BootstrapReflectsLifecycle(t *testing.T) {
	ctx, a, _, _ := setupAssembly(t)
	b, etag0 := bootstrapOf(t, a.Router)
	if b.Data.Revision != 0 || statusOf(b, "acme.example") != "available" || statusOf(b, "acme.example-addon") != "available" {
		t.Fatalf("bootstrap awal = %+v", b)
	}
	for _, m := range b.Data.Modules {
		if m.Key == "platform.core" {
			t.Error("platform.core tidak boleh ada di bootstrap")
		}
	}
	if b.Data.Modules[0].BasePath != "/example" {
		t.Errorf("basePath = %q", b.Data.Modules[0].BasePath)
	}

	res, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpInstall, Key: "acme.example", Enable: true, Actor: "cli:test"})
	if err != nil || len(res) != 2 || res[1].Status != "succeeded" {
		t.Fatalf("Execute = %+v, %v", res, err)
	}
	b, etag1 := bootstrapOf(t, a.Router)
	if statusOf(b, "acme.example") != "enabled" || b.Data.Revision == 0 || etag1 == etag0 {
		t.Fatalf("bootstrap = %+v etag %s -> %s", b, etag0, etag1)
	}
	if rec := get(a.Router, "/api/example/ping", nil); rec.Code != 200 {
		t.Fatalf("ping = %d %s", rec.Code, rec.Body)
	}
	if rec := get(a.Router, "/api/bootstrap", map[string]string{"If-None-Match": etag1}); rec.Code != 304 {
		t.Errorf("If-None-Match = %d", rec.Code)
	}
}

// Perubahan oleh proses lain (CLI) terlihat lewat polling revision tanpa restart.
func TestAssemble_CrossProcessChangeVisibleViaWatch(t *testing.T) {
	ctx, a, dsn, _ := setupAssembly(t)
	if _, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpInstall, Key: "acme.example", Enable: true, Actor: "cli:a"}); err != nil {
		t.Fatal(err)
	}
	if rec := get(a.Router, "/api/example/ping", nil); rec.Code != 200 {
		t.Fatalf("ping = %d", rec.Code)
	}

	db2 := openTestDB(t, dsn)
	reg, _ := BuildRegistry()
	other := buildLifecycle(reg, db2, a.Tenant, nil, nil, testProviders(), testLog())
	res, err := other.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpDisable, Key: "acme.example", Actor: "cli:b"})
	if err != nil || len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("disable = %+v, %v", res, err)
	}

	deadline := time.Now().Add(3*lifecycle.DefaultPollInterval + 500*time.Millisecond)
	for {
		rec := get(a.Router, "/api/example/ping", nil)
		if rec.Code == http.StatusForbidden {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ping masih %d setelah %v", rec.Code, 3*lifecycle.DefaultPollInterval)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestMigrateUp_EnsureRequiredNoRequiredModules(t *testing.T) {
	ctx, a, _, db := setupAssembly(t)
	res, err := a.Lifecycle.EnsureRequired(ctx, a.Tenant, "system:migrate")
	if err != nil || len(res) != 0 {
		t.Fatalf("EnsureRequired = %+v, %v", res, err)
	}
	var n int64
	if err := db.Raw("SELECT count(*) FROM module_installations").Scan(&n).Error; err != nil || n != 0 {
		t.Errorf("module_installations = %d (%v)", n, err)
	}
}
