package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"erpora/internal/platform/database"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/tenancy"
	sdki18n "erpora/sdk/i18n"
)

func i18nFS(m map[string]string) fs.FS {
	out := fstest.MapFS{}
	for k, v := range m {
		out[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return out
}

func testI18n(t *testing.T) *i18n.Service {
	t.Helper()
	reg, err := i18n.ParseRegistry([]byte(`{"default":"id","locales":[{"code":"id","name":"ID"},{"code":"en","name":"EN","fallback":"id"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s, err := i18n.New(i18n.Options{Registry: reg, Sources: []i18n.Source{
		{Namespace: "platform", FS: i18nFS(map[string]string{
			"id.json": `{"errors":{"x":"galat id","internal_error":"kesalahan server","tenant_unresolved":"tenant tidak jelas","unauthenticated":"belum masuk"}}`,
			"en.json": `{"errors":{"x":"error en","internal_error":"server error","tenant_unresolved":"tenant unknown","unauthenticated":"not signed in"}}`})},
		{Namespace: "ui", FS: i18nFS(map[string]string{"id.json": `{"hi":"Halo"}`, "en.json": `{"hi":"Hello"}`})},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newI18nRouter(t *testing.T) *Router {
	t.Helper()
	return NewRouter(Deps{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:   tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector:  database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Assets:     fstest.MapFS{"index.html": {Data: []byte("x")}},
		AppVersion: "1",
		I18n:       testI18n(t),
	})
}

// Middleware locale memasang bahasa hasil negosiasi Accept-Language dan Resolver ke context request;
// hasilnya selalu anggota registri (Requirement 3.1, 3.4, 4.3).
func TestLocaleContext_NegotiatesAndInstallsResolver(t *testing.T) {
	r := newI18nRouter(t)
	var gotLocale sdki18n.Locale
	var gotText string
	r.Handle("GET /t", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotLocale, _ = sdki18n.LocaleFrom(req.Context())
		gotText = sdki18n.T(req.Context(), "ui::hi")
	}))
	cases := map[string][2]string{
		"":               {"id", "Halo"},
		"en":             {"en", "Hello"},
		"en-US,en;q=0.9": {"en", "Hello"},
		"fr-FR":          {"id", "Halo"},
		"garbage;;;,,,":  {"id", "Halo"},
		"id":             {"id", "Halo"},
	}
	for header, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		if header != "" {
			req.Header.Set("Accept-Language", header)
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
		if string(gotLocale) != want[0] || gotText != want[1] {
			t.Errorf("Accept-Language %q: locale=%q text=%q, want %v", header, gotLocale, gotText, want)
		}
	}
}

func TestLocaleContext_NoI18nServiceLeavesContextUntouched(t *testing.T) {
	r, _ := newTestRouter(t, nil)
	var has bool
	r.Handle("GET /t", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, has = sdki18n.LocaleFrom(req.Context())
	}))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/t", nil))
	if has {
		t.Error("tanpa Deps.I18n, context tidak boleh membawa locale")
	}
}

// Locale dipasang SETELAH tenantContext: resolver di context terikat tenant request.
func TestLocaleContext_ResolverBoundToTenant(t *testing.T) {
	r := newI18nRouter(t)
	var tid tenancy.TenantID
	var ok bool
	r.Handle("GET /t", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tid, ok = tenancy.FromContext(req.Context())
	}))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/t", nil))
	if !ok || tid != defaultTenant {
		t.Errorf("tenant = %v, %v", tid, ok)
	}
}

// R-11 (Requirement 4.4): galat yang ditulis SEBELUM rantai dalam (recover 500, tenant_unresolved, Authenticate)
// juga mengikuti Accept-Language; jalur sebelum tenant dikenal memakai katalog bawaan.
type errBody struct {
	Error struct{ Code, Message string }
}

func errOf(t *testing.T, rec *httptest.ResponseRecorder) errBody {
	t.Helper()
	var e errBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body bukan galat JSON: %s", rec.Body)
	}
	return e
}

func doLang(r http.Handler, target, lang string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestLocalizedErrors_RecoverPanic500(t *testing.T) {
	r := newI18nRouter(t)
	r.Handle("GET /boom", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("rahasia") }))
	for lang, want := range map[string]string{"en": "server error", "id": "kesalahan server", "": "kesalahan server"} {
		rec := doLang(r, "/boom", lang)
		e := errOf(t, rec)
		if rec.Code != http.StatusInternalServerError || e.Error.Code != "internal_error" || e.Error.Message != want {
			t.Errorf("lang %q: %d %+v, want 500 internal_error %q", lang, rec.Code, e.Error, want)
		}
		if strings.Contains(rec.Body.String(), "rahasia") {
			t.Error("nilai panic tidak boleh bocor")
		}
	}
}

func TestLocalizedErrors_TenantUnresolved(t *testing.T) {
	r := NewRouter(Deps{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:   failingResolver{},
		Connector:  database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Assets:     fstest.MapFS{"index.html": {Data: []byte("x")}},
		AppVersion: "1",
		I18n:       testI18n(t),
	})
	r.Handle("GET /t", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	e := errOf(t, doLang(r, "/t", "en"))
	if e.Error.Code != "tenant_unresolved" || e.Error.Message != "tenant unknown" {
		t.Errorf("error = %+v", e.Error)
	}
	if e := errOf(t, doLang(r, "/t", "id")); e.Error.Message != "tenant tidak jelas" {
		t.Errorf("id = %+v", e.Error)
	}
}

type failingResolver struct{}

func (failingResolver) Resolve(*http.Request) (tenancy.TenantID, error) {
	return tenancy.TenantID{}, errors.New("tidak ada tenant")
}

func TestLocalizedErrors_AuthenticateRejection(t *testing.T) {
	r := NewRouter(Deps{
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:   tenancy.SingleTenantResolver{ID: defaultTenant},
		Connector:  database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
		Assets:     fstest.MapFS{"index.html": {Data: []byte("x")}},
		AppVersion: "1",
		I18n:       testI18n(t),
		Authenticate: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				WriteError(w, req, http.StatusUnauthorized, "unauthenticated", "asli", nil)
			})
		},
	})
	r.Handle("GET /secure", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := doLang(r, "/secure", "en")
	if e := errOf(t, rec); rec.Code != http.StatusUnauthorized || e.Error.Message != "not signed in" {
		t.Errorf("%d %+v", rec.Code, e.Error)
	}
}

// ovStore adalah i18n.OverrideStore tetap: override HANYA untuk defaultTenant (ui/id hi dan platform/id tenant_unresolved).
type ovStore struct{}

func (ovStore) Revisions(_ context.Context, t tenancy.TenantID) ([]store.Revision, error) {
	if t != defaultTenant {
		return nil, nil
	}
	return []store.Revision{{Namespace: "ui", Locale: "id", Revision: 1}, {Namespace: "platform", Locale: "id", Revision: 1}}, nil
}

func (ovStore) ListLocale(_ context.Context, t tenancy.TenantID, ns, loc string) ([]store.Override, error) {
	if t != defaultTenant || loc != "id" {
		return nil, nil
	}
	switch ns {
	case "ui":
		return []store.Override{{Namespace: "ui", Locale: "id", Key: "hi", Message: "Halo OVERRIDE"}}, nil
	case "platform":
		return []store.Override{{Namespace: "platform", Locale: "id", Key: "errors.tenant_unresolved", Message: "OVERRIDE tenant"}}, nil
	}
	return nil, nil
}

func (ovStore) Get(context.Context, tenancy.TenantID, string, string, string) (store.Override, error) {
	return store.Override{}, store.ErrNotFound
}

func (ovStore) List(context.Context, tenancy.TenantID, store.Filter, int, int) ([]store.Override, int64, error) {
	return nil, 0, nil
}

func (ovStore) Upsert(context.Context, tenancy.TenantID, store.Override, store.Precondition) (bool, int64, error) {
	return false, 0, errors.New("tidak dipakai")
}

func (ovStore) Delete(context.Context, tenancy.TenantID, string, string, string, store.Precondition) (bool, int64, error) {
	return false, 0, errors.New("tidak dipakai")
}

func testI18nWithOverrides(t *testing.T) *i18n.Service {
	t.Helper()
	reg, err := i18n.ParseRegistry([]byte(`{"default":"id","locales":[{"code":"id","name":"ID"},{"code":"en","name":"EN","fallback":"id"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s, err := i18n.New(i18n.Options{Registry: reg, Overrides: ovStore{}, Sources: []i18n.Source{
		{Namespace: "platform", FS: i18nFS(map[string]string{
			"id.json": `{"errors":{"tenant_unresolved":"tenant tidak jelas"}}`, "en.json": `{"errors":{"tenant_unresolved":"tenant unknown"}}`})},
		{Namespace: "ui", FS: i18nFS(map[string]string{"id.json": `{"hi":"Halo"}`, "en.json": `{"hi":"Hello"}`})},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// F-5: sesudah tenant dikenal, handler memperoleh Resolver TERIKAT TENANT (override tenant berlaku); galat sebelum
// tenant dikenal memakai Resolver bawaan (override tidak pernah dibaca). Test gagal bila localeContext berhenti
// mengganti Resolver bawaan dengan yang terikat tenant, atau bila Resolver bawaan ikut membaca override.
func TestLocaleResolverBindsTenantOverridesAfterTenantKnown(t *testing.T) {
	deps := func(res tenancy.Resolver) Deps {
		return Deps{
			Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			Resolver:   res,
			Connector:  database.SingleTenantConnector{Default: defaultTenant, DB: database.TenantDB{DB: dryRunDB(t)}},
			Assets:     fstest.MapFS{"index.html": {Data: []byte("x")}},
			AppVersion: "1",
			I18n:       testI18nWithOverrides(t),
		}
	}
	// (1) tenant dikenal: T di handler memakai override milik defaultTenant
	r := NewRouter(deps(tenancy.SingleTenantResolver{ID: defaultTenant}))
	var got string
	r.Handle("GET /t", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { got = sdki18n.T(req.Context(), "ui::hi") }))
	doLang(r, "/t", "id")
	if got != "Halo OVERRIDE" {
		t.Errorf("handler memperoleh %q, want override tenant (Resolver terikat tenant)", got)
	}
	// tenant lain tidak melihat override
	other := tenancy.TenantID{0x77}
	r2 := NewRouter(deps(tenancy.SingleTenantResolver{ID: other}))
	var got2 string
	r2.Handle("GET /t", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { got2 = sdki18n.T(req.Context(), "ui::hi") }))
	doLang(r2, "/t", "id")
	if got2 != "Halo" {
		t.Errorf("tenant lain memperoleh %q, want bawaan", got2)
	}
	// (2) tenant tidak dapat ditentukan: galat memakai teks BAWAAN walau ada override untuk defaultTenant
	r3 := NewRouter(deps(failingResolver{}))
	r3.Handle("GET /t", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if e := errOf(t, doLang(r3, "/t", "id")); e.Error.Code != "tenant_unresolved" || e.Error.Message != "tenant tidak jelas" {
		t.Errorf("galat pra-tenant = %+v, want teks bawaan (bukan override)", e.Error)
	}
}
