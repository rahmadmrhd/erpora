package desks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	"erpora/sdk/module"
)

// recorder memenuhi module.Registrar dan mencatat route yang didaftarkan.
type recorder struct {
	handlers map[string]http.Handler
	perms    map[string]string
	public   map[string]bool
	classes  map[string]module.Classification
}

func newRecorder() *recorder {
	return &recorder{handlers: map[string]http.Handler{}, perms: map[string]string{}, public: map[string]bool{}, classes: map[string]module.Classification{}}
}

func (r *recorder) Handle(method, path, permission string, h http.Handler, opts ...module.RouteOption) {
	k := method + " " + path
	r.handlers[k], r.perms[k] = h, permission
	r.classes[k] = module.ResolveRouteOptions(opts...).Classification
}

func (r *recorder) Public(method, path string, h http.Handler, _ ...module.RouteOption) {
	k := method + " " + path
	r.handlers[k], r.public[k] = h, true
}
func (r *recorder) Command(module.Command)    {}
func (r *recorder) Scope(authz.ScopeProvider) {}

type fakeBackend struct {
	view View
	err  error
}

func (f fakeBackend) List(context.Context) (View, error) { return f.view, f.err }

func sampleView() View {
	return View{
		Desks: []Desk{{
			ID: "example", Source: SourceModule, ModuleKey: "acme.example",
			Label: MessageRef{NS: "example", Key: "desk.title"}, Icon: "file-text",
			Menus: []Menu{
				{Type: TypeItem, ID: "dashboard", Label: MessageRef{NS: "platform", Key: "desk.dashboard"}, To: "/example", Icon: "layout-dashboard", Locked: true, OwnerDesk: "example"},
				{Type: TypeItem, ID: "notes", Label: MessageRef{NS: "example", Key: "nav.notes"}, To: "/example/notes", OwnerDesk: "example"},
			},
		}},
		Revision: "0123456789abcdef0123456789abcdef", ETag: `"0123456789abcdef0123456789abcdef"`,
	}
}

func newMux(t *testing.T, b Backend, withTenant bool) (*http.ServeMux, *recorder) {
	t.Helper()
	api := New()
	if b != nil {
		api.Bind(b)
	}
	rec := newRecorder()
	if err := api.Register(rec); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for pat, h := range rec.handlers {
		h := h
		mux.Handle(pat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if withTenant {
				r = r.WithContext(tenancy.WithTenant(r.Context(), tenantA))
			}
			h.ServeHTTP(w, r)
		}))
	}
	return mux, rec
}

func doGet(mux http.Handler, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/desks", nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestAPI_RegisterGetDesksWithPermissionNotPublic(t *testing.T) {
	_, rec := newMux(t, fakeBackend{view: sampleView()}, true)
	if len(rec.handlers) != 1 {
		t.Fatalf("route terdaftar = %v, want hanya GET /desks", rec.handlers)
	}
	if rec.perms["GET /desks"] != "platform.desks.read" || rec.public["GET /desks"] {
		t.Errorf("GET /desks: permission=%q public=%v", rec.perms["GET /desks"], rec.public["GET /desks"])
	}
	if rec.classes["GET /desks"] != module.ClassInternal {
		t.Errorf("klasifikasi = %q, want internal", rec.classes["GET /desks"])
	}
}

func TestCapabilities(t *testing.T) {
	want := []string{"platform.desks.read", "platform.desks.update"}
	if len(Capabilities) != 2 || Capabilities[0] != want[0] || Capabilities[1] != want[1] {
		t.Errorf("Capabilities = %v, want %v", Capabilities, want)
	}
}

func TestAPI_OKEnvelopeAndHeaders(t *testing.T) {
	mux, _ := newMux(t, fakeBackend{view: sampleView()}, true)
	w := doGet(mux)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body)
	}
	if got := w.Header().Get("ETag"); got != sampleView().ETag {
		t.Errorf("ETag = %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-cache" {
		t.Errorf("Cache-Control = %q", got)
	}
	vary := strings.ToLower(strings.Join(w.Header().Values("Vary"), ","))
	if !strings.Contains(vary, "cookie") || !strings.Contains(vary, "authorization") {
		t.Errorf("Vary = %q, want memuat Cookie dan Authorization", vary)
	}
	var body struct {
		Data struct {
			Desks    []Desk `json:"desks"`
			Revision string `json:"revision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Revision != sampleView().Revision || len(body.Data.Desks) != 1 || body.Data.Desks[0].ID != "example" ||
		body.Data.Desks[0].Menus[1].To != "/example/notes" {
		t.Errorf("body = %s", w.Body)
	}
}

func TestAPI_EmptyDesksEncodedAsEmptyArray(t *testing.T) {
	mux, _ := newMux(t, fakeBackend{view: View{Revision: "r", ETag: `"r"`}}, true)
	w := doGet(mux)
	if !strings.Contains(w.Body.String(), `"desks":[]`) {
		t.Errorf("body = %s, want desks berupa array kosong (bukan null)", w.Body)
	}
}

func TestAPI_NotModifiedKeepsCacheHeaders(t *testing.T) {
	mux, _ := newMux(t, fakeBackend{view: sampleView()}, true)
	for _, inm := range []string{sampleView().ETag, "W/" + sampleView().ETag, "*"} {
		w := doGet(mux, "If-None-Match", inm)
		if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
			t.Errorf("If-None-Match %q: status=%d body=%q", inm, w.Code, w.Body)
		}
		if w.Header().Get("ETag") != sampleView().ETag || w.Header().Get("Cache-Control") != "private, no-cache" ||
			!strings.Contains(strings.ToLower(strings.Join(w.Header().Values("Vary"), ",")), "cookie") {
			t.Errorf("304 harus tetap membawa ETag/Cache-Control/Vary: %v", w.Header())
		}
	}
	if w := doGet(mux, "If-None-Match", `"lain"`); w.Code != http.StatusOK {
		t.Errorf("ETag berbeda harus 200, dapat %d", w.Code)
	}
}

func TestAPI_NotReadyBeforeBind(t *testing.T) {
	mux, _ := newMux(t, nil, true)
	w := doGet(mux)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"not_ready"`) {
		t.Errorf("status=%d body=%s, want 503 not_ready", w.Code, w.Body)
	}
}

func TestAPI_ServiceErrors(t *testing.T) {
	mux, _ := newMux(t, fakeBackend{err: errors.New("db mati")}, true)
	w := doGet(mux)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "db mati") {
		t.Errorf("status=%d body=%s, want 500 tanpa detail internal", w.Code, w.Body)
	}
	mux, _ = newMux(t, fakeBackend{err: ErrNoTenant}, true)
	if w := doGet(mux); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "tenant_unresolved") {
		t.Errorf("ErrNoTenant: status=%d body=%s, want 400 tenant_unresolved", w.Code, w.Body)
	}
}

func TestAPI_NoTenantInContext(t *testing.T) {
	mux, _ := newMux(t, fakeBackend{view: sampleView()}, false)
	if w := doGet(mux); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "tenant_unresolved") {
		t.Errorf("status=%d body=%s", w.Code, w.Body)
	}
}

// Endpoint tidak menerima id apa pun (tidak ada titik masuk IDOR) dan hanya GET.
func TestAPI_OnlyGet(t *testing.T) {
	mux, _ := newMux(t, fakeBackend{view: sampleView()}, true)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(m, "/desks", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", m, w.Code)
		}
	}
}

// Service nyata di belakang API: dua prinsipal dengan izin berbeda tidak saling melihat respons dan ETag mereka
// berbeda, sehingga If-None-Match milik A tidak menghasilkan 304 untuk B.
func TestAPI_WithRealServiceNoCrossPrincipalNotModified(t *testing.T) {
	reg := buildRegistry(t, newModule(t, "alpha", deskWith(item("alpha", "a", "read"), item("alpha", "b", "admin"))))
	svc := newService(reg, statuses{"acme.alpha": runtime.StatusEnabled}, nil)
	mux, _ := newMux(t, svc, false)

	call := func(perms []string, hdr ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/desks", nil)
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		req = req.WithContext(ctxFor(tenantA, perms...))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	wa := call([]string{"alpha.items.read", "alpha.items.admin"})
	wb := call([]string{"alpha.items.read"}, "If-None-Match", wa.Header().Get("ETag"))
	if wa.Code != http.StatusOK || wb.Code != http.StatusOK {
		t.Fatalf("A=%d B=%d, want 200 keduanya (B tidak boleh menerima 304 dari ETag A)", wa.Code, wb.Code)
	}
	if strings.Contains(wb.Body.String(), `"id":"b"`) || !strings.Contains(wa.Body.String(), `"id":"b"`) {
		t.Errorf("A=%s\nB=%s", wa.Body, wb.Body)
	}
	if again := call([]string{"alpha.items.read"}, "If-None-Match", wb.Header().Get("ETag")); again.Code != http.StatusNotModified {
		t.Errorf("prinsipal sama dengan ETag sendiri harus 304, dapat %d", again.Code)
	}
}
