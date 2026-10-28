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
)

func call(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if method == http.MethodPost || method == "POST" {
		req.Header.Set("Content-Type", "application/json") // mutasi modul wajib application/json (Q32 B-9)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct{ Error struct{ Code string } }
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	return b.Error.Code
}

func TestAPI_EnableDisableRoundTrip(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	h := a.Router

	if rec := call(h, "POST", "/api/platform/modules/acme.example/install", ""); rec.Code != 200 {
		t.Fatalf("install = %d %s", rec.Code, rec.Body)
	}
	rec := call(h, "POST", "/api/platform/modules/acme.example/enable", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"enabled"`) {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, "GET", "/api/example/notes", ""); rec.Code != 200 {
		t.Fatalf("notes saat enabled = %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, "POST", "/api/platform/modules/acme.example/disable", ""); rec.Code != 200 {
		t.Fatalf("disable = %d %s", rec.Code, rec.Body)
	}
	rec = call(h, "GET", "/api/example/notes", "")
	if rec.Code != 403 || errCodeOf(t, rec) != "module_disabled" {
		t.Fatalf("notes saat disabled = %d %s", rec.Code, rec.Body)
	}
	rec = call(h, "GET", "/api/platform/modules/acme.example/history", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"actor":"api:dev"`) || !strings.Contains(rec.Body.String(), `"event":"disabled"`) {
		t.Fatalf("history = %d %s", rec.Code, rec.Body)
	}
	// Respons aksi memuat modul dan operasi; no-op menghasilkan operations [].
	rec = call(h, "POST", "/api/platform/modules/acme.example/disable", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"operations":[]`) {
		t.Fatalf("disable ulang = %d %s", rec.Code, rec.Body)
	}
	// Dependensi: addon dapat di-install tetapi tidak di-enable saat example disabled (422 dengan detail).
	if rec := call(h, "POST", "/api/platform/modules/acme.example-addon/install", ""); rec.Code != 200 {
		t.Fatalf("install addon = %d %s", rec.Code, rec.Body)
	}
	rec = call(h, "POST", "/api/platform/modules/acme.example-addon/enable", "")
	if rec.Code != 422 || errCodeOf(t, rec) != "business_rule_violated" || !strings.Contains(rec.Body.String(), "acme.example") {
		t.Fatalf("enable addon = %d %s", rec.Code, rec.Body)
	}
}

func insertForeign(t *testing.T, db *gorm.DB, other uuid.UUID, opID uuid.UUID) {
	t.Helper()
	if err := db.Exec(`INSERT INTO module_operations (id, tenant_id, module_key, operation, status, actor, error)
		VALUES (?, ?, 'acme.example', 'install', 'failed', 'cli:other', '{"code":"hook_failed","message":"x"}')`, opID, other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO module_lifecycle_events (id, tenant_id, module_key, event, actor, operation_id)
		VALUES (?, ?, 'acme.example', 'installed', 'cli:other', ?)`, uuid.New(), other, opID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO module_installations (id, tenant_id, module_key, installed_version, status)
		VALUES (?, ?, 'acme.only-other', '1.0.0', 'enabled')`, uuid.New(), other).Error; err != nil {
		t.Fatal(err)
	}
}

// CLAUDE.md / keputusan 9: baris milik tenant lain tidak pernah terlihat atau dipakai lewat API.
func TestAPI_IDOR_OtherTenantRows(t *testing.T) {
	_, a, _, db := setupAssembly(t)
	h := a.Router
	other, foreignOp := uuid.New(), uuid.New()
	insertForeign(t, db, other, foreignOp)

	rec := call(h, "GET", "/api/platform/modules/operations/"+foreignOp.String(), "")
	if rec.Code != 404 || errCodeOf(t, rec) != "not_found" {
		t.Fatalf("operasi tenant lain = %d %s", rec.Code, rec.Body)
	}
	// retry tidak memakai operasi failed milik tenant lain.
	rec = call(h, "POST", "/api/platform/modules/acme.example/retry", "")
	if rec.Code != 409 || errCodeOf(t, rec) != "invalid_state" {
		t.Fatalf("retry = %d %s", rec.Code, rec.Body)
	}
	rec = call(h, "GET", "/api/platform/modules/acme.example/history", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "cli:other") || strings.Contains(rec.Body.String(), foreignOp.String()) {
		t.Fatalf("history bocor: %d %s", rec.Code, rec.Body)
	}
	rec = call(h, "GET", "/api/platform/modules", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "acme.only-other") {
		t.Fatalf("daftar modul memuat key tenant lain: %d %s", rec.Code, rec.Body)
	}
	if rec := call(h, "GET", "/api/platform/modules/acme.only-other", ""); rec.Code != 404 {
		t.Fatalf("key hanya milik tenant lain = %d, want 404", rec.Code)
	}
	if rec := call(h, "GET", "/api/platform/modules/acme.only-other/history", ""); rec.Code != 404 {
		t.Fatalf("history key tenant lain = %d, want 404", rec.Code)
	}
}
