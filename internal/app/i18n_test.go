package app

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"

	"erpora/internal/platform/i18n"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/tenancy"
	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
)

// catalogModule adalah modul uji ber-katalog.
type catalogModule struct {
	m  module.Manifest
	fs fs.FS
}

func (c catalogModule) Manifest() module.Manifest     { return c.m }
func (catalogModule) Register(module.Registrar) error { return nil }
func (c catalogModule) Catalogs() fs.FS               { return c.fs }

func newCatalogModule(t *testing.T, files map[string]string) catalogModule {
	t.Helper()
	doc := map[string]any{
		"schema_version": "1", "key": "acme.notes", "slug": "notes", "name": "Notes", "version": "1.0.0",
		"publisher":     map[string]string{"id": "acme", "name": "Acme"},
		"trust":         "builtin",
		"compatibility": map[string]string{"platform": "^0.1", "sdk": "^0.1"},
		"dependencies":  []map[string]any{{"module": registry.PlatformKey, "version": "^0.1", "optional": false}},
		"capabilities":  []string{},
		"migrations":    map[string]string{"strategy": "none"},
		"offline":       map[string]string{"mode": "online-only"},
		"i18n":          map[string]any{"locales": []string{"id", "en"}, "defaultLocale": "id"},
	}
	b, _ := json.Marshal(doc)
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return catalogModule{m: module.MustParseManifest(b), fs: m}
}

func buildTestRegistry(t *testing.T, mods ...module.Module) *registry.Registry {
	t.Helper()
	reg, err := registry.Build(registry.Options{PlatformVersion: "0.1.0", SDKVersion: "0.1.0"}, mods...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

var i18nTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

func TestNewI18nService_IncludesPlatformUIAndModuleCatalogs(t *testing.T) {
	mod := newCatalogModule(t, map[string]string{
		"id.json": `{"note": {"title": "Catatan"}}`, "en.json": `{"note": {"title": "Note"}}`,
	})
	enabled := map[string]bool{"notes": true}
	svc, err := newI18nService(slog.Default(), true, buildTestRegistry(t, mod),
		func(_ tenancy.TenantID, ns string) bool { return enabled[ns] }, nil)
	if err != nil {
		t.Fatalf("newI18nService: %v", err)
	}
	ctx := context.Background()
	for ns, wantKey := range map[string]string{"platform": "errors.not_found", "ui": "actions.save", "notes": "note.title"} {
		v, err := svc.Catalog(ctx, i18nTenant, ns, "en")
		if err != nil {
			t.Fatalf("Catalog(%s): %v", ns, err)
		}
		if _, ok := v.Messages[wantKey]; !ok {
			t.Errorf("%s: kunci %q tidak ada", ns, wantKey)
		}
	}
	enabled["notes"] = false
	if _, err := svc.Catalog(ctx, i18nTenant, "notes", "en"); !errors.Is(err, i18n.ErrNotFound) {
		t.Errorf("modul nonaktif harus ErrNotFound: %v", err)
	}
}

// Katalog modul yang rusak secara struktural menggagalkan startup dengan pesan yang menyebut berkas dan kunci.
func TestNewI18nService_RejectsStructurallyBrokenModuleCatalog(t *testing.T) {
	mod := newCatalogModule(t, map[string]string{
		"id.json": `{"note": {"title": "Halo {nama"}}`, "en.json": `{}`,
	})
	_, err := newI18nService(slog.Default(), false, buildTestRegistry(t, mod), nil, nil)
	var ce *i18n.CatalogError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *i18n.CatalogError", err)
	}
	if ce.Issues[0].File != "notes/id.json" || ce.Issues[0].Key != "note.title" {
		t.Errorf("Issue = %+v", ce.Issues[0])
	}
}

func TestNewI18nService_NoModuleCatalogsStillServesPlatformAndUI(t *testing.T) {
	svc, err := newI18nService(slog.Default(), false, buildTestRegistry(t), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	revs := svc.Revisions(context.Background(), i18nTenant)
	if revs["platform"] == "" || revs["ui"] == "" || len(revs) != 2 {
		t.Errorf("Revisions = %v", revs)
	}
}

func TestI18nHookInstallsLocaleResolverAndScope(t *testing.T) {
	mod := newCatalogModule(t, map[string]string{
		"id.json": `{"note": {"title": "Catatan"}}`, "en.json": `{"note": {"title": "Note"}}`,
	})
	reg := buildTestRegistry(t, mod)
	svc, err := newI18nService(slog.Default(), false, reg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hook := i18nHook(svc, reg)
	ctx := hook(context.Background(), i18nTenant, "acme.notes")
	if got := sdki18n.T(ctx, "note.title"); got != "Catatan" {
		t.Errorf("T di hook/command = %q, want bahasa default", got)
	}
	if got := sdki18n.T(hook(context.Background(), i18nTenant, "platform.core"), "errors.not_found"); got == "platform::errors.not_found" {
		t.Errorf("scope platform harus menemukan kunci: %q", got)
	}
}
