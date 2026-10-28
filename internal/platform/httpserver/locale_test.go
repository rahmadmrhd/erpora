package httpserver

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"erpora/internal/platform/database"
	"erpora/internal/platform/i18n"
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
		{Namespace: "platform", FS: i18nFS(map[string]string{"id.json": `{"errors":{"x":"galat id"}}`, "en.json": `{"errors":{"x":"error en"}}`})},
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
