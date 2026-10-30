//go:build integration

package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"erpora/sdk/module"
)

type catalogResp struct {
	Data struct {
		Locale, Namespace, Revision string
		Messages                    map[string]string
	}
}

func catalogOf(t *testing.T, h http.Handler, target string) (catalogResp, string) {
	t.Helper()
	rec := get(h, target, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body)
	}
	var c catalogResp
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	return c, rec.Header().Get("ETag")
}

type i18nBoot struct {
	Data struct {
		I18n *struct {
			DefaultLocale string
			Locales       []struct{ Code, Name string }
			Revisions     map[string]string
		}
	}
}

func i18nBootstrap(t *testing.T, h http.Handler) (i18nBoot, string) {
	t.Helper()
	rec := get(h, "/api/bootstrap", nil)
	var b i18nBoot
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b, rec.Header().Get("ETag")
}

// Katalog platform dan ui disajikan lewat rakitan penuh: bahasa id/en, ETag/304, 404 seragam.
func TestI18nAPI_PlatformAndUICatalogs(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	h := a.Router

	id, etagID := catalogOf(t, h, "/api/platform/i18n/id/ui")
	en, etagEN := catalogOf(t, h, "/api/platform/i18n/en/ui")
	if id.Data.Locale != "id" || id.Data.Namespace != "ui" || id.Data.Messages["actions.save"] != "Simpan" {
		t.Errorf("id = %+v", id.Data)
	}
	if en.Data.Messages["actions.save"] != "Save" || etagID == etagEN {
		t.Errorf("en = %+v etag id/en = %s/%s", en.Data.Messages["actions.save"], etagID, etagEN)
	}
	if rec := get(h, "/api/platform/i18n/id/ui", map[string]string{"If-None-Match": etagID}); rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match = %d, want 304", rec.Code)
	}
	if p, _ := catalogOf(t, h, "/api/platform/i18n/en/platform"); p.Data.Messages["errors.not_found"] == "" {
		t.Error("katalog platform harus memuat errors.not_found")
	}
	var first string
	for _, target := range []string{"/api/platform/i18n/zz/ui", "/api/platform/i18n/id/tidak-ada", "/api/platform/i18n/id/example"} {
		rec := get(h, target, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", target, rec.Code)
		}
		var env struct {
			Error struct{ Code, Message string }
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		if first == "" {
			first = env.Error.Message
		}
		if env.Error.Code != "not_found" || env.Error.Message != first {
			t.Errorf("%s: error = %+v", target, env.Error)
		}
	}
}

// Bootstrap memuat bahasa dan revisi katalog namespace aktif saja; ETag bootstrap mengikuti revisi.
func TestI18nAPI_BootstrapCarriesI18n(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	b, etag := i18nBootstrap(t, a.Router)
	if b.Data.I18n == nil || b.Data.I18n.DefaultLocale != "id" || len(b.Data.I18n.Locales) < 2 {
		t.Fatalf("i18n = %+v", b.Data.I18n)
	}
	if b.Data.I18n.Revisions["ui"] == "" || b.Data.I18n.Revisions["platform"] == "" {
		t.Errorf("revisions = %v", b.Data.I18n.Revisions)
	}
	for ns := range b.Data.I18n.Revisions {
		if ns != "ui" && ns != "platform" {
			t.Errorf("modul tanpa katalog / nonaktif tidak boleh punya revisi: %q", ns)
		}
	}
	ui, _ := catalogOf(t, a.Router, "/api/platform/i18n/id/ui")
	if ui.Data.Revision != b.Data.I18n.Revisions["ui"] {
		t.Errorf("revisi katalog %q != bootstrap %q", ui.Data.Revision, b.Data.I18n.Revisions["ui"])
	}
	if rec := get(a.Router, "/api/bootstrap", map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Errorf("bootstrap If-None-Match = %d", rec.Code)
	}
}

// Namespace modul hanya tersedia saat modulnya enabled, dan bootstrap hanya memuat revisi namespace aktif.
func TestI18nAPI_ModuleNamespaceFollowsLifecycle(t *testing.T) {
	e := newRefEnv(t)
	_, a := e.assemble(func() []module.Module {
		return []module.Module{newCatalogModule(t, map[string]string{
			"id.json": `{"note": {"title": "Catatan"}}`, "en.json": `{"note": {"title": "Note"}}`,
		})}
	})
	h := a.Router
	const target = "/api/platform/i18n/en/notes"
	if rec := get(h, target, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("modul belum terpasang: %d, want 404", rec.Code)
	}
	if b, _ := i18nBootstrap(t, h); b.Data.I18n.Revisions["notes"] != "" {
		t.Errorf("revisi modul nonaktif muncul: %v", b.Data.I18n.Revisions)
	}
	_, etagBefore := i18nBootstrap(t, h)

	for _, op := range []string{"install", "enable"} {
		if rec := call(h, "POST", "/api/platform/modules/acme.notes/"+op, ""); rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", op, rec.Code, rec.Body)
		}
	}
	c, _ := catalogOf(t, h, target)
	if c.Data.Messages["note.title"] != "Note" {
		t.Errorf("messages = %v", c.Data.Messages)
	}
	b, etagAfter := i18nBootstrap(t, h)
	if b.Data.I18n.Revisions["notes"] != c.Data.Revision || etagAfter == etagBefore {
		t.Errorf("revisions = %v, katalog %q, etag %s -> %s", b.Data.I18n.Revisions, c.Data.Revision, etagBefore, etagAfter)
	}

	if rec := call(h, "POST", "/api/platform/modules/acme.notes/disable", ""); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, target, nil); rec.Code != http.StatusNotFound {
		t.Errorf("modul nonaktif: %d, want 404", rec.Code)
	}
}

// message galat mengikuti Accept-Language lewat errors.<code> di katalog platform; code tetap (Requirement 4.4).
func TestI18nAPI_ErrorMessageFollowsAcceptLanguage(t *testing.T) {
	_, a, _, _ := setupAssembly(t)
	type envelope struct {
		Error struct{ Code, Message string }
	}
	do := func(lang string) envelope {
		hdr := map[string]string{}
		if lang != "" {
			hdr["Accept-Language"] = lang
		}
		rec := get(a.Router, "/api/platform/modules/acme.tidak-ada", hdr)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d %s", rec.Code, rec.Body)
		}
		var env envelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return env
	}
	id, en := do(""), do("en-US,en;q=0.9")
	if id.Error.Code != "not_found" || en.Error.Code != "not_found" {
		t.Fatalf("code harus tetap not_found: %+v %+v", id, en)
	}
	if id.Error.Message == en.Error.Message || !strings.Contains(en.Error.Message, "not found") {
		t.Errorf("message id=%q en=%q", id.Error.Message, en.Error.Message)
	}
	if fr := do("fr-FR"); fr.Error.Message != id.Error.Message {
		t.Errorf("bahasa tak terdaftar jatuh ke default: %q vs %q", fr.Error.Message, id.Error.Message)
	}
}
