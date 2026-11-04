//go:build integration

package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
	"erpora/internal/platform/desks"
	"erpora/internal/platform/modules/lifecycle"
)

type desksBody struct {
	Data struct {
		Desks    []desks.Desk `json:"desks"`
		Revision string       `json:"revision"`
	} `json:"data"`
}

func getDesks(t *testing.T, h http.Handler, hdr ...string) (*httptest.ResponseRecorder, desksBody) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/platform/desks", nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var b desksBody
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatalf("body %q: %v", rec.Body, err)
		}
	}
	return rec, b
}

func deskIDs(b desksBody) []string {
	var out []string
	for _, d := range b.Data.Desks {
		out = append(out, d.ID)
	}
	return out
}

func enable(t *testing.T, a *assembly, key string) {
	t.Helper()
	ctx := t.Context()
	if res, err := a.Lifecycle.Execute(ctx, lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpInstall, Key: key, Enable: true, Actor: "cli:test"}); err != nil || len(res) == 0 {
		t.Fatalf("install+enable %s: %+v, %v", key, res, err)
	}
}

func insertDeskOverride(t *testing.T, db *gorm.DB, tenant uuid.UUID, desk, config string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO desk_overrides (tenant_id, desk_id, scope, subject, config, updated_by)
		VALUES (?, ?, 'site', '', ?::jsonb, 'test')`, tenant, desk, config).Error; err != nil {
		t.Fatal(err)
	}
}

// Endpoint penuh terhadap modul referensi: modul tidak enabled tidak membentuk Desk, enable memunculkannya tanpa
// restart, disable menghilangkannya; ETag berubah mengikuti isi dan 304 dilayani untuk ETag yang sama.
func TestDesks_FollowsModuleLifecycle(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	h := a.authed(t)

	rec, b := getDesks(t, h)
	if rec.Code != 200 {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	if got := deskIDs(b); len(got) != 1 || got[0] != "platform" {
		t.Fatalf("awal: Desk = %v, want [platform]", got)
	}
	if rec.Header().Get("Cache-Control") != "private, no-cache" || !strings.Contains(strings.ToLower(strings.Join(rec.Header().Values("Vary"), ",")), "authorization") {
		t.Errorf("header cache = %v", rec.Header())
	}
	etag0 := rec.Header().Get("ETag")
	if rec304, _ := getDesks(t, h, "If-None-Match", etag0); rec304.Code != http.StatusNotModified {
		t.Errorf("If-None-Match sama = %d, want 304", rec304.Code)
	}

	enable(t, a, "acme.example")
	rec, b = getDesks(t, h)
	if got := deskIDs(b); len(got) != 2 || got[0] != "platform" || got[1] != "example" {
		t.Fatalf("setelah enable: Desk = %v", got)
	}
	ex := b.Data.Desks[1]
	if ex.Source != desks.SourceModule || ex.ModuleKey != "acme.example" || ex.Label.NS != "example" || ex.Label.Key != "desk.title" {
		t.Errorf("desk example = %+v", ex)
	}
	if len(ex.Menus) != 2 || ex.Menus[0].ID != "dashboard" || ex.Menus[0].To != "/example" || ex.Menus[1].To != "/example/notes" {
		t.Errorf("menu example = %+v", ex.Menus)
	}
	if rec.Header().Get("ETag") == etag0 {
		t.Error("ETag harus berubah setelah modul diaktifkan")
	}
	if recOld, _ := getDesks(t, h, "If-None-Match", etag0); recOld.Code != http.StatusOK {
		t.Errorf("ETag lama = %d, want 200", recOld.Code)
	}

	enable(t, a, "acme.example-addon")
	if _, b = getDesks(t, h); len(b.Data.Desks) != 3 || b.Data.Desks[2].ID != "example-addon" ||
		b.Data.Desks[2].Menus[1].To != "/example-addon/summary" {
		t.Fatalf("setelah enable addon: %+v", deskIDs(b))
	}

	if _, err := a.Lifecycle.Execute(t.Context(), lifecycle.Request{Tenant: a.Tenant, Op: lifecycle.OpDisable, Key: "acme.example-addon", Actor: "cli:test"}); err != nil {
		t.Fatal(err)
	}
	if _, b = getDesks(t, h); len(b.Data.Desks) != 2 {
		t.Fatalf("setelah disable addon: %v", deskIDs(b))
	}
}

// Timpaan situs milik tenant dipakai; baris tenant lain tidak pernah memengaruhi respons (isolasi tenant).
func TestDesks_SiteOverridesAppliedAndTenantIsolated(t *testing.T) {
	_, a, _, db := setupAssembly(t)
	h := a.authed(t)
	enable(t, a, "acme.example")

	insertDeskOverride(t, db, uuid.New(), "example", `{"hidden":["notes"]}`) // tenant lain
	if _, b := getDesks(t, h); len(b.Data.Desks) != 2 {
		t.Fatalf("timpaan tenant lain memengaruhi respons: %v", deskIDs(b))
	}

	insertDeskOverride(t, db, uuid.UUID(a.Tenant), "example", `{"hidden":["notes"]}`)
	if _, b := getDesks(t, h); len(b.Data.Desks) != 1 || b.Data.Desks[0].ID != "platform" {
		t.Fatalf("timpaan tenant ini tidak diterapkan: %v", deskIDs(b))
	}

	// config rusak dilewati, bukan menggagalkan daftar
	if err := db.Exec(`UPDATE desk_overrides SET config = '{"hidden":"notes"}'::jsonb WHERE tenant_id = ?`, uuid.UUID(a.Tenant)).Error; err != nil {
		t.Fatal(err)
	}
	if rec, b := getDesks(t, h); rec.Code != 200 || len(b.Data.Desks) != 2 {
		t.Fatalf("config rusak: status=%d desks=%v", rec.Code, deskIDs(b))
	}
}

// Permission: AuthzStub deny menolak endpoint (403) tanpa membocorkan daftar.
func TestDesks_DeniedWithoutPermission(t *testing.T) {
	_, a, _, _ := setupAssemblyWith(t, func(c *config.Config) { c.AuthzStub = "deny" })
	rec, _ := getDesks(t, a.authed(t))
	if rec.Code != http.StatusForbidden || errCodeOf(t, rec) != "permission_denied" {
		t.Fatalf("GET = %d %s, want 403 permission_denied", rec.Code, rec.Body)
	}
}
