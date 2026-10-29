//go:build integration

package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/i18n/store"
)

// callJSON mengirim request dengan Content-Type application/json (PUT/DELETE override wajib; 415 bila tidak).
func callJSON(h http.Handler, method, target, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const overrideURL = "/api/platform/i18n/overrides/ui/id/actions.save"

// Alur lengkap (Requirement 5.7, 12.6): PUT -> revisi naik -> katalog berubah (ETag baru, ETag lama -> 200) ->
// bootstrap berubah -> DELETE -> kembali ke bawaan.
func TestI18nOverrideFlow_PutChangesCatalogAndBootstrapThenDeleteRestores(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	h := a.Router
	cat0, etag0 := catalogOf(t, h, "/api/platform/i18n/id/ui")
	boot0, bootETag0 := i18nBootstrap(t, h)
	if cat0.Data.Messages["actions.save"] != "Simpan" {
		t.Fatalf("bawaan = %q", cat0.Data.Messages["actions.save"])
	}

	put := callJSON(h, http.MethodPut, overrideURL, `{"message":"Simpan sekarang"}`, nil)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", put.Code, put.Body)
	}
	cat1, etag1 := catalogOf(t, h, "/api/platform/i18n/id/ui")
	if cat1.Data.Messages["actions.save"] != "Simpan sekarang" || etag1 == etag0 || cat1.Data.Revision == cat0.Data.Revision {
		t.Errorf("katalog setelah PUT: %q etag %s->%s revisi %s->%s", cat1.Data.Messages["actions.save"], etag0, etag1, cat0.Data.Revision, cat1.Data.Revision)
	}
	// ETag lama tidak lagi 304; ETag baru 304
	if rec := get(h, "/api/platform/i18n/id/ui", map[string]string{"If-None-Match": etag0}); rec.Code != http.StatusOK {
		t.Errorf("ETag lama = %d, want 200", rec.Code)
	}
	if rec := get(h, "/api/platform/i18n/id/ui", map[string]string{"If-None-Match": etag1}); rec.Code != http.StatusNotModified {
		t.Errorf("ETag baru = %d, want 304", rec.Code)
	}
	boot1, bootETag1 := i18nBootstrap(t, h)
	if boot1.Data.I18n.Revisions["ui"] == boot0.Data.I18n.Revisions["ui"] || bootETag1 == bootETag0 {
		t.Errorf("bootstrap tidak berubah: revisi %s->%s etag %s->%s", boot0.Data.I18n.Revisions["ui"], boot1.Data.I18n.Revisions["ui"], bootETag0, bootETag1)
	}
	if boot1.Data.I18n.Revisions["platform"] != boot0.Data.I18n.Revisions["platform"] {
		t.Error("revisi namespace lain tidak boleh berubah")
	}
	// bahasa lain tidak terpengaruh
	if en, _ := catalogOf(t, h, "/api/platform/i18n/en/ui"); en.Data.Messages["actions.save"] != "Save" {
		t.Errorf("en = %q", en.Data.Messages["actions.save"])
	}

	// idempoten: PUT dengan nilai sama tidak menaikkan revisi
	callJSON(h, http.MethodPut, overrideURL, `{"message":"Simpan sekarang"}`, nil)
	if boot2, _ := i18nBootstrap(t, h); boot2.Data.I18n.Revisions["ui"] != boot1.Data.I18n.Revisions["ui"] {
		t.Error("PUT sama menaikkan revisi")
	}

	if del := callJSON(h, http.MethodDelete, overrideURL, "", nil); del.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d %s", del.Code, del.Body)
	}
	cat2, etag2 := catalogOf(t, h, "/api/platform/i18n/id/ui")
	if cat2.Data.Messages["actions.save"] != "Simpan" || etag2 != etag0 {
		t.Errorf("setelah DELETE: %q etag %s, want bawaan dan %s", cat2.Data.Messages["actions.save"], etag2, etag0)
	}
	if rec := callJSON(h, http.MethodDelete, overrideURL, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE kedua = %d, want 404", rec.Code)
	}
}

// error.message mengikuti Accept-Language dan tertimpa override, sementara code tetap (Requirement 4.4, 12.9).
func TestI18nOverrideFlow_ErrorMessageOverriddenCodeStable(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	h := a.Router
	type envelope struct {
		Error struct{ Code, Message string }
	}
	probe := func() envelope {
		rec := get(h, "/api/platform/modules/acme.tidak-ada", map[string]string{"Accept-Language": "en"})
		var e envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d %s", rec.Code, rec.Body)
		}
		return e
	}
	before := probe()
	if before.Error.Code != "not_found" || before.Error.Message == "" {
		t.Fatalf("awal = %+v", before)
	}
	if put := callJSON(h, http.MethodPut, "/api/platform/i18n/overrides/platform/en/errors.not_found", `{"message":"Nothing here"}`, nil); put.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", put.Code, put.Body)
	}
	after := probe()
	if after.Error.Code != "not_found" || after.Error.Message != "Nothing here" {
		t.Errorf("setelah override = %+v", after)
	}
	if callJSON(h, http.MethodDelete, "/api/platform/i18n/overrides/platform/en/errors.not_found", "", nil).Code != http.StatusNoContent {
		t.Fatal("DELETE gagal")
	}
	if got := probe(); got != before {
		t.Errorf("setelah DELETE = %+v, want %+v", got, before)
	}
}

// Validasi lewat DB nyata dan list/If-Match.
func TestI18nOverrideAPI_ValidationListAndIfMatch(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	h := a.Router
	if rec := callJSON(h, http.MethodPut, "/api/platform/i18n/overrides/ui/id/tidak.ada", `{"message":"x"}`, nil); rec.Code != http.StatusBadRequest || errCodeOf(t, rec) != "invalid_i18n_override" {
		t.Errorf("kunci tak ada: %d %s", rec.Code, rec.Body)
	}
	put := callJSON(h, http.MethodPut, overrideURL, `{"message":"Satu"}`, nil)
	etag := put.Header().Get("ETag")
	if etag == "" {
		t.Fatal("PUT tanpa ETag")
	}
	if rec := callJSON(h, http.MethodPut, overrideURL, `{"message":"Dua"}`, map[string]string{"If-Match": `"basi"`}); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("If-Match basi = %d", rec.Code)
	}
	if rec := callJSON(h, http.MethodPut, overrideURL, `{"message":"Dua"}`, map[string]string{"If-Match": etag}); rec.Code != http.StatusOK {
		t.Errorf("If-Match cocok = %d %s", rec.Code, rec.Body)
	}
	rec := get(h, "/api/platform/i18n/overrides?namespace=ui", nil)
	var list struct {
		Data []struct{ Key, Message, UpdatedBy string }
		Page struct{ Total int64 }
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || list.Page.Total != 1 || list.Data[0].Message != "Dua" || list.Data[0].UpdatedBy != "api:dev" {
		t.Errorf("list = %d %s", rec.Code, rec.Body)
	}
	if rec := callJSON(h, http.MethodPut, overrideURL, `{"message":"x","tenantId":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"}`, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("tenantId di body = %d, want 422", rec.Code)
	}
}

// Gerbang mutasi (FR-20.2) end-to-end: production tanpa flag -> 404 sebelum permission; dengan flag -> permission.
func TestI18nOverrideAPI_MutationGate(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*config.Config)
		wantPut    int
		wantList   int
		wantDelete int
	}{
		{"production tanpa flag, stub allow", func(c *config.Config) { c.Env = config.EnvProduction }, 404, 200, 404},
		{"production tanpa flag, stub deny", func(c *config.Config) { c.Env = config.EnvProduction; c.AuthzStub = "deny" }, 404, 403, 404},
		{"production dengan flag, stub deny", func(c *config.Config) {
			c.Env, c.AuthzStub, c.ModulesAdminAPI = config.EnvProduction, "deny", true
		}, 403, 403, 403},
		{"production dengan flag, stub allow", func(c *config.Config) {
			c.Env, c.AuthzStub, c.ModulesAdminAPI = config.EnvProduction, "allow", true
		}, 200, 200, 204},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, a, _, _ := setupAssemblyWith(t, tc.mutate)
			if rec := callJSON(a.Router, http.MethodPut, overrideURL, `{"message":"x"}`, nil); rec.Code != tc.wantPut {
				t.Errorf("PUT = %d, want %d: %s", rec.Code, tc.wantPut, rec.Body)
			}
			if rec := get(a.Router, "/api/platform/i18n/overrides", nil); rec.Code != tc.wantList {
				t.Errorf("GET list = %d, want %d", rec.Code, tc.wantList)
			}
			if rec := callJSON(a.Router, http.MethodDelete, overrideURL, "", nil); rec.Code != tc.wantDelete {
				t.Errorf("DELETE = %d, want %d", rec.Code, tc.wantDelete)
			}
		})
	}
}

// CLI dan API berbagi Service: override lewat Service (jalur `erpora i18n override`) langsung terlihat di katalog.
func TestI18nOverrideFlow_ServiceWriteVisibleThroughAPI(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	changed, err := a.I18n.SetOverride(t.Context(), a.Tenant, i18n.Override{Namespace: "ui", Locale: "id", Key: "actions.save", Message: "Simpan via CLI"}, "cli:test", nil)
	if err != nil || !changed {
		t.Fatalf("SetOverride: %v %v", changed, err)
	}
	if cat, _ := catalogOf(t, a.Router, "/api/platform/i18n/id/ui"); cat.Data.Messages["actions.save"] != "Simpan via CLI" {
		t.Errorf("katalog = %q", cat.Data.Messages["actions.save"])
	}
	row, err := a.I18n.GetOverride(t.Context(), a.Tenant, "ui", "id", "actions.save")
	if err != nil || row.UpdatedBy != "cli:test" {
		t.Errorf("GetOverride = %+v %v", row, err)
	}
	if existed, err := a.I18n.DeleteOverride(t.Context(), a.Tenant, "ui", "id", "actions.save", "cli:test", nil); err != nil || !existed {
		t.Errorf("DeleteOverride: %v %v", existed, err)
	}
	if _, err := a.I18n.GetOverride(t.Context(), a.Tenant, "ui", "id", "actions.save"); err != store.ErrNotFound {
		t.Errorf("setelah hapus err = %v", err)
	}
}
