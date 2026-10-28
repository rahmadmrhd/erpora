//go:build integration

package lifecycle

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

type allowAll struct{}

func (allowAll) Allow(context.Context, string) bool { return true }

type tenantResolver struct{}

func (tenantResolver) Resolve(*http.Request) (tenancy.TenantID, error) { return tenantA, nil }

// B-12: AC FR-19 "health gagal saat enable -> route tidak pernah aktif" pada PostgreSQL nyata (bukan fake).
// Route modul dipasang lewat runtime.Mount di atas snapshot milik Service; request diuji sebelum enable, DI DALAM
// hook health (saat hook:enable sudah berjalan), dan sesudah operasi gagal: tidak boleh ada yang 200.
func TestService_HealthFailureOnEnableNeverActivatesRoutes(t *testing.T) {
	rec := &fxRec{}
	mod := newFx(rec, fxSpec{key: "acme.fx-mod", migrations: os.DirFS(fxMigDir + "ok")})
	e := newRealEnv(t, mod)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := httpserver.NewRouter(httpserver.Deps{
		Logger: log, Resolver: tenantResolver{},
		Connector: database.SingleTenantConnector{Default: tenantA, DB: database.TenantDB{DB: e.db}},
	})
	runtime.Mount(router, e.reg, e.snap, allowAll{}, log)
	probe := func() int {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/fx-mod/items", nil))
		return rr.Code
	}

	var mu sync.Mutex
	var seen []int
	mod.on("Health", func(context.Context, module.Env) error {
		mu.Lock()
		seen = append(seen, probe())
		mu.Unlock()
		return errors.New("tidak sehat")
	})

	if r := e.exec(t, OpInstall, "acme.fx-mod"); r.Status != "succeeded" {
		t.Fatalf("install = %+v", r)
	}
	seen = append(seen, probe())
	revBefore, _ := e.st.Revision(context.Background(), tenantA)
	r := e.exec(t, OpEnable, "acme.fx-mod")
	if revAfter, _ := e.st.Revision(context.Background(), tenantA); revAfter <= revBefore {
		t.Errorf("revision (ETag) harus naik setelah enable gagal: %d -> %d (Q32 B-4)", revBefore, revAfter)
	}
	if r.Status != "failed" || r.Error == nil || r.Error.Code != codeHealthFailed {
		t.Fatalf("enable = %+v, want failed/%s", r, codeHealthFailed)
	}
	seen = append(seen, probe())
	for i, code := range seen {
		if code == http.StatusOK {
			t.Errorf("probe #%d = 200: route tidak boleh pernah aktif (seen=%v)", i, seen)
		}
	}
	if len(seen) != 3 {
		t.Errorf("probe dalam hook health tidak terjadi: seen=%v", seen)
	}

	// Operasi tercatat failed di DB; status modul tetap installed; efek hook:enable dikompensasi hook:disable.
	if n := e.count(t, "SELECT count(*) FROM module_operations WHERE module_key = 'acme.fx-mod' AND operation = 'enable' AND status = 'failed'"); n != 1 {
		t.Errorf("operasi enable failed di DB = %d, want 1", n)
	}
	if n := e.count(t, "SELECT count(*) FROM module_installations WHERE module_key = 'acme.fx-mod' AND status = 'installed'"); n != 1 {
		t.Errorf("modul harus tetap installed: %d", n)
	}
	if e.snap.Status(tenantA, "acme.fx-mod") == runtime.StatusEnabled {
		t.Error("snapshot tidak boleh enabled")
	}
	got := rec.list()
	var enabled, disabled int
	for _, c := range got {
		switch c {
		case "acme.fx-mod:OnEnable":
			enabled++
		case "acme.fx-mod:OnDisable":
			disabled++
		}
	}
	if enabled != 1 || disabled != 1 {
		t.Errorf("hook enable/disable = %d/%d, want 1/1 (kompensasi): %v", enabled, disabled, got)
	}
}

// B-7: jejak module_migrations ditulis setelah goose commit dan tidak atomik dengannya. Bila baris jejak hilang
// (crash/DB error di antara commit dan pencatatan), retry menyelaraskannya dengan goose_module_<slug>.
func TestService_RetryReconcilesLostMigrationTrail(t *testing.T) {
	rec := &fxRec{}
	mod := newFx(rec, fxSpec{key: "acme.fx-mod", migrations: os.DirFS(fxMigDir + "ok")})
	fail := true
	mod.on("Install", func(context.Context, module.Env) error {
		if fail {
			return errors.New("gagal sementara")
		}
		return nil
	})
	e := newRealEnv(t, mod)
	first := e.exec(t, OpInstall, "acme.fx-mod")
	if first.Status != "failed" {
		t.Fatalf("install = %+v, want failed (hook:install)", first)
	}
	if n := e.count(t, "SELECT max(version_id) FROM goose_module_fx_mod"); n != 2 {
		t.Fatalf("goose harus sudah menerapkan kedua file: %d", n)
	}
	// Simulasi jejak yang hilang: goose sudah commit, pencatatan tidak pernah tertulis.
	if err := e.db.Exec("DELETE FROM module_migrations WHERE module_key = 'acme.fx-mod'").Error; err != nil {
		t.Fatal(err)
	}
	fail = false
	r, err := e.svc.Retry(context.Background(), tenantA, first.OperationID, "cli:tester")
	if err != nil || r.Status != "succeeded" {
		t.Fatalf("Retry = %+v, %v", r, err)
	}
	if n := e.count(t, "SELECT count(*) FROM module_migrations WHERE module_key = 'acme.fx-mod' AND execution_status = 'succeeded'"); n != 2 {
		t.Errorf("jejak module_migrations = %d, want 2 setelah retry", n)
	}
	// Idempoten: retry/upgrade berikutnya tidak menggandakan baris.
	if n := e.count(t, "SELECT count(DISTINCT migration_key) FROM module_migrations WHERE module_key = 'acme.fx-mod'"); n != 2 {
		t.Errorf("migration_key unik = %d, want 2", n)
	}
}
