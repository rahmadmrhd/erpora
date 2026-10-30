package admin

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"

	"erpora/internal/platform/i18n"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/module"
)

var tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

func mapFS(m map[string]string) fs.FS {
	out := fstest.MapFS{}
	for k, v := range m {
		out[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return out
}

// recorder memenuhi module.Registrar dan mencatat handler yang didaftarkan.
type recorder struct {
	routes map[string]http.Handler
	public map[string]bool
}

func (r *recorder) Handle(method, path, _ string, h http.Handler, _ ...module.RouteOption) {
	r.routes[method+" "+path] = h
}
func (r *recorder) Public(method, path string, h http.Handler, _ ...module.RouteOption) {
	r.routes[method+" "+path] = h
	r.public[method+" "+path] = true
}
func (r *recorder) Command(module.Command) {}

func newMux(t *testing.T, active func(tenancy.TenantID, string) bool, bind bool) (*http.ServeMux, *API) {
	t.Helper()
	reg, err := i18n.ParseRegistry([]byte(`{"default":"id","locales":[{"code":"id","name":"Bahasa Indonesia"},{"code":"en","name":"English","fallback":"id"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := i18n.New(i18n.Options{Registry: reg, Active: active, Sources: []i18n.Source{
		{Namespace: "platform", FS: mapFS(map[string]string{"id.json": `{"errors":{"x":"galat"}}`, "en.json": `{"errors":{"x":"error"}}`})},
		{Namespace: "ui", FS: mapFS(map[string]string{"id.json": `{"hi":"Halo","only_id":"x"}`, "en.json": `{"hi":"Hello"}`})},
		{Namespace: "example", Module: true, Default: "id", Locales: []string{"id"}, FS: mapFS(map[string]string{"id.json": `{"a":"A"}`})},
	}})
	if err != nil {
		t.Fatal(err)
	}
	api := New()
	if bind {
		api.Bind(svc)
	}
	rec := &recorder{routes: map[string]http.Handler{}, public: map[string]bool{}}
	if err := api.Register(rec); err != nil {
		t.Fatal(err)
	}
	if !rec.public["GET /i18n/{locale}/{namespace}"] {
		t.Fatalf("route katalog harus publik; terdaftar: %v", rec.routes)
	}
	mux := http.NewServeMux()
	for pat, h := range rec.routes {
		mux.Handle(pat, withTenant(h))
	}
	return mux, api
}

func withTenant(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(tenancy.WithTenant(r.Context(), tenantA)))
	})
}

func get(mux http.Handler, target string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

type catalogBody struct {
	Data struct {
		Locale    string            `json:"locale"`
		Namespace string            `json:"namespace"`
		Revision  string            `json:"revision"`
		Messages  map[string]string `json:"messages"`
	} `json:"data"`
}

func TestGetCatalogServesMergedMessagesWithETag(t *testing.T) {
	mux, _ := newMux(t, nil, true)
	rec := get(mux, "/i18n/en/ui")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var b catalogBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.Data.Locale != "en" || b.Data.Namespace != "ui" || b.Data.Revision == "" {
		t.Errorf("data = %+v", b.Data)
	}
	// en tidak punya only_id: fallback ke id sudah digabung
	if b.Data.Messages["hi"] != "Hello" || b.Data.Messages["only_id"] != "x" {
		t.Errorf("messages = %v", b.Data.Messages)
	}
	if rec.Header().Get("ETag") == "" || !strings.HasPrefix(rec.Header().Get("ETag"), `W/"`) {
		t.Errorf("ETag = %q", rec.Header().Get("ETag"))
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-cache" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestGetCatalogIfNoneMatch304(t *testing.T) {
	mux, _ := newMux(t, nil, true)
	first := get(mux, "/i18n/id/ui")
	etag := first.Header().Get("ETag")
	rec := get(mux, "/i18n/id/ui", "If-None-Match", etag)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("status = %d body = %q, want 304 tanpa body", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "private, no-cache" || rec.Header().Get("ETag") != etag {
		t.Errorf("304 harus membawa ETag dan Cache-Control: %v", rec.Header())
	}
	if rec := get(mux, "/i18n/id/ui", "If-None-Match", `W/"lain"`); rec.Code != http.StatusOK {
		t.Errorf("ETag berbeda harus 200, got %d", rec.Code)
	}
	// bahasa lain, isi lain, ETag lain
	if en := get(mux, "/i18n/en/ui"); en.Header().Get("ETag") == etag {
		t.Error("ETag bahasa berbeda tidak boleh sama")
	}
}

// Bahasa atau namespace tak dikenal, dan modul nonaktif, dijawab 404 identik tanpa membedakan penyebab.
func TestGetCatalogUnknownAndInactiveAreIdentical404(t *testing.T) {
	active := map[string]bool{"example": false}
	mux, _ := newMux(t, func(_ tenancy.TenantID, ns string) bool { return active[ns] }, true)
	var bodies []string
	for _, target := range []string{
		"/i18n/zz/ui", "/i18n/id/nope", "/i18n/id/example", "/i18n/..%2F..%2Fetc/ui", "/i18n/id/UI", "/i18n/ID/ui",
		"/i18n/" + strings.Repeat("x", 100) + "/ui", "/i18n/id/" + strings.Repeat("x", 100),
	} {
		rec := get(mux, target)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", target, rec.Code)
		}
		var env struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		if env.Error.Code != "not_found" {
			t.Errorf("%s: code = %q", target, env.Error.Code)
		}
		bodies = append(bodies, env.Error.Message)
	}
	for _, b := range bodies {
		if b != bodies[0] {
			t.Errorf("pesan 404 harus identik untuk semua penyebab: %q vs %q", b, bodies[0])
		}
	}
	active["example"] = true
	if rec := get(mux, "/i18n/id/example"); rec.Code != http.StatusOK {
		t.Errorf("modul aktif: status = %d", rec.Code)
	}
}

func TestGetCatalogNotReadyBeforeBind(t *testing.T) {
	mux, _ := newMux(t, nil, false)
	rec := get(mux, "/i18n/id/ui")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "not_ready") {
		t.Errorf("status = %d body = %s", rec.Code, rec.Body)
	}
}

func TestGetCatalogWithoutTenantIs400(t *testing.T) {
	_, api := newMux(t, nil, true)
	rec := &recorder{routes: map[string]http.Handler{}, public: map[string]bool{}}
	_ = api.Register(rec)
	mux := http.NewServeMux()
	for pat, h := range rec.routes {
		mux.Handle(pat, h) // tanpa tenant di context
	}
	if r := get(mux, "/i18n/id/ui"); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "tenant_unresolved") {
		t.Errorf("status = %d: %s", r.Code, r.Body)
	}
}
