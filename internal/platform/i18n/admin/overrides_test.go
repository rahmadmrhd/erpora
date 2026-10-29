package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"erpora/internal/platform/database"
	"erpora/internal/platform/httpserver"
	"erpora/internal/platform/i18n"
	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/modules/registry"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/tenancy"
)

var tenantB = tenancy.TenantID{0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x72, 0x22, 0x82, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22}

// memStore adalah i18n.OverrideStore dalam memori yang memenuhi kontrak store (revisi hanya naik saat berubah,
// precondition di bawah "kunci", isolasi tenant).
type memStore struct {
	mu   sync.Mutex
	rows map[tenancy.TenantID]map[[3]string]store.Override
	revs map[tenancy.TenantID]map[[2]string]int64
}

func newMemStore() *memStore {
	return &memStore{rows: map[tenancy.TenantID]map[[3]string]store.Override{}, revs: map[tenancy.TenantID]map[[2]string]int64{}}
}

func (m *memStore) Revisions(_ context.Context, t tenancy.TenantID) ([]store.Revision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.Revision
	for k, v := range m.revs[t] {
		out = append(out, store.Revision{Namespace: k[0], Locale: k[1], Revision: v})
	}
	return out, nil
}

func (m *memStore) ListLocale(_ context.Context, t tenancy.TenantID, ns, loc string) ([]store.Override, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []store.Override
	for k, v := range m.rows[t] {
		if k[0] == ns && k[1] == loc {
			out = append(out, v)
		}
	}
	return out, nil
}

func (m *memStore) Get(_ context.Context, t tenancy.TenantID, ns, loc, key string) (store.Override, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.rows[t][[3]string{ns, loc, key}]; ok {
		return v, nil
	}
	return store.Override{}, store.ErrNotFound
}

func (m *memStore) List(_ context.Context, t tenancy.TenantID, f store.Filter, page, size int) ([]store.Override, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []store.Override
	for k, v := range m.rows[t] {
		if (f.Namespace == "" || f.Namespace == k[0]) && (f.Locale == "" || f.Locale == k[1]) {
			all = append(all, v)
		}
	}
	for i := range all { // urut (namespace, locale, key)
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			if a.Namespace+"\x00"+a.Locale+"\x00"+a.Key > b.Namespace+"\x00"+b.Locale+"\x00"+b.Key {
				all[i], all[j] = all[j], all[i]
			}
		}
	}
	from := min((page-1)*size, len(all))
	to := min(from+size, len(all))
	return all[from:to], int64(len(all)), nil
}

func (m *memStore) bump(t tenancy.TenantID, ns, loc string) int64 {
	if m.revs[t] == nil {
		m.revs[t] = map[[2]string]int64{}
	}
	m.revs[t][[2]string{ns, loc}]++
	return m.revs[t][[2]string{ns, loc}]
}

func (m *memStore) Upsert(_ context.Context, t tenancy.TenantID, o store.Override, pre store.Precondition) (bool, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := [3]string{o.Namespace, o.Locale, o.Key}
	var cur *store.Override
	if v, ok := m.rows[t][k]; ok {
		cur = &v
	}
	if pre != nil {
		if err := pre(cur); err != nil {
			return false, 0, err
		}
	}
	if cur != nil && cur.Message == o.Message {
		return false, m.revs[t][[2]string{o.Namespace, o.Locale}], nil
	}
	if m.rows[t] == nil {
		m.rows[t] = map[[3]string]store.Override{}
	}
	o.TenantID = t
	o.UpdatedAt = o.UpdatedAt.UTC()
	m.rows[t][k] = o
	return true, m.bump(t, o.Namespace, o.Locale), nil
}

func (m *memStore) Delete(_ context.Context, t tenancy.TenantID, ns, loc, key string, pre store.Precondition) (bool, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := [3]string{ns, loc, key}
	var cur *store.Override
	if v, ok := m.rows[t][k]; ok {
		cur = &v
	}
	if pre != nil {
		if err := pre(cur); err != nil {
			return false, 0, err
		}
	}
	if cur == nil {
		return false, m.revs[t][[2]string{ns, loc}], nil
	}
	delete(m.rows[t], k)
	return true, m.bump(t, ns, loc), nil
}

type allowFn func(ctx context.Context, permission string) bool

func (f allowFn) Allow(ctx context.Context, p string) bool { return f(ctx, p) }

var (
	allowAll = allowFn(func(context.Context, string) bool { return true })
	denyAll  = allowFn(func(context.Context, string) bool { return false })
)

// tenantByHeader memilih tenant dari header X-Tenant ("B" = tenant B) untuk uji lintas tenant.
type tenantByHeader struct{}

func (tenantByHeader) Resolve(r *http.Request) (tenancy.TenantID, error) {
	if r.Header.Get("X-Tenant") == "B" {
		return tenantB, nil
	}
	return tenantA, nil
}

type env struct {
	h   http.Handler
	svc *i18n.Service
}

// newEnv merakit Router + runtime.Mount (permission, gerbang mutasi) di atas Service ber-override memori.
func newEnv(t *testing.T, authz runtime.Authorizer, mutationsEnabled bool) *env {
	t.Helper()
	reg, err := i18n.ParseRegistry([]byte(`{"default":"id","locales":[{"code":"id","name":"Bahasa Indonesia"},{"code":"en","name":"English","fallback":"id"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := i18n.New(i18n.Options{Registry: reg, Overrides: newMemStore(), Sources: []i18n.Source{
		{Namespace: "platform", FS: mapFS(map[string]string{
			"id.json": `{"errors":{"not_found":"Tidak ditemukan"}}`, "en.json": `{"errors":{"not_found":"Not found"}}`})},
		{Namespace: "ui", FS: mapFS(map[string]string{
			"id.json": `{"actions":{"save":"Simpan"},"n":"{count, plural, other {# data}}"}`,
			"en.json": `{"actions":{"save":"Save"},"n":"{count, plural, one {# item} other {# items}}"}`})},
	}})
	if err != nil {
		t.Fatal(err)
	}
	api := New()
	api.Bind(svc)
	rg, err := registry.Build(registry.Options{
		PlatformVersion: "0.1.0", SDKVersion: "0.1.0",
		PlatformCapabilities: Capabilities,
		Platform:             api.Register,
	})
	if err != nil {
		t.Fatal(err)
	}
	router := httpserver.NewRouter(httpserver.Deps{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Resolver:  tenantByHeader{},
		Connector: database.SingleTenantConnector{Default: tenantA},
		I18n:      svc,
	})
	var opts []runtime.MountOption
	if !mutationsEnabled {
		opts = append(opts, runtime.SkipRoute(IsMutation))
	}
	runtime.Mount(router, rg, runtime.NewSnapshot(), authz, slog.New(slog.NewTextHandler(io.Discard, nil)), opts...)
	return &env{h: router, svc: svc}
}

type resp struct {
	*httptest.ResponseRecorder
}

func (e *env) do(method, target, body string, hdr ...string) resp {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	if body != "" || method == http.MethodPut || method == http.MethodDelete {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i+1] == "" {
			req.Header.Del(hdr[i])
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return resp{rec}
}

type errEnvelope struct {
	Error struct {
		Code    string
		Message string
		Details []struct{ Field, Code, Message string }
	}
}

func (r resp) errBody(t *testing.T) errEnvelope {
	t.Helper()
	var e errEnvelope
	if err := json.Unmarshal(r.Body.Bytes(), &e); err != nil {
		t.Fatalf("body bukan galat JSON: %s", r.Body)
	}
	return e
}

const putPath = "/api/platform/i18n/overrides/ui/id/actions.save"

func body(msg string) string {
	b, _ := json.Marshal(map[string]string{"message": msg})
	return string(b)
}

func TestOverrideAPI_PermissionDenied403(t *testing.T) {
	e := newEnv(t, denyAll, true)
	for name, rec := range map[string]resp{
		"GET list": e.do(http.MethodGet, "/api/platform/i18n/overrides", ""),
		"PUT":      e.do(http.MethodPut, putPath, body("x")),
		"DELETE":   e.do(http.MethodDelete, putPath, ""),
	} {
		if rec.Code != http.StatusForbidden || rec.errBody(t).Error.Code != "permission_denied" {
			t.Errorf("%s: %d %s, want 403 permission_denied", name, rec.Code, rec.Body)
		}
	}
	// katalog publik tidak butuh permission
	if rec := e.do(http.MethodGet, "/api/platform/i18n/id/ui", ""); rec.Code != http.StatusOK {
		t.Errorf("katalog publik = %d", rec.Code)
	}
}

// Mutasi mati di luar development tanpa flag: 404 SEBELUM permission dievaluasi; baca tetap tunduk permission.
func TestOverrideAPI_MutationGateReturns404BeforePermission(t *testing.T) {
	e := newEnv(t, denyAll, false)
	for name, rec := range map[string]resp{
		"PUT":    e.do(http.MethodPut, putPath, body("x")),
		"DELETE": e.do(http.MethodDelete, putPath, ""),
	} {
		if rec.Code != http.StatusNotFound || rec.errBody(t).Error.Code != "not_found" {
			t.Errorf("%s: %d %s, want 404 not_found (bukan 403)", name, rec.Code, rec.Body)
		}
	}
	if rec := e.do(http.MethodGet, "/api/platform/i18n/overrides", ""); rec.Code != http.StatusForbidden {
		t.Errorf("GET list tetap tunduk permission: %d", rec.Code)
	}
	ok := newEnv(t, allowAll, false)
	if rec := ok.do(http.MethodPut, putPath, body("Simpan!")); rec.Code != http.StatusNotFound {
		t.Errorf("gerbang mati walau permission ada: %d", rec.Code)
	}
	if rec := ok.do(http.MethodGet, "/api/platform/i18n/overrides", ""); rec.Code != http.StatusOK {
		t.Errorf("GET list = %d", rec.Code)
	}
}

func TestOverrideAPI_PutRequiresJSONContentType415(t *testing.T) {
	e := newEnv(t, allowAll, true)
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "application/json; charset=latin1", "application/jsonx"} {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			req := httptest.NewRequest(method, putPath, strings.NewReader(body("x")))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnsupportedMediaType {
				t.Errorf("%s Content-Type %q: %d, want 415", method, ct, rec.Code)
			}
		}
	}
	// charset utf-8 boleh
	req := httptest.NewRequest(http.MethodPut, putPath, strings.NewReader(body("Simpan baru")))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("charset utf-8: %d %s", rec.Code, rec.Body)
	}
}

func TestOverrideAPI_PutIsIdempotentAndReturnsETag(t *testing.T) {
	e := newEnv(t, allowAll, true)
	revBefore := e.svc.Revisions(context.Background(), tenantA)["ui"]
	first := e.do(http.MethodPut, putPath, body("Simpan sekarang"))
	if first.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", first.Code, first.Body)
	}
	etag := first.Header().Get("ETag")
	if etag == "" || strings.HasPrefix(etag, "W/") {
		t.Errorf("ETag harus kuat: %q", etag)
	}
	var env struct {
		Data OverrideView
	}
	_ = json.Unmarshal(first.Body.Bytes(), &env)
	if env.Data.Message != "Simpan sekarang" || env.Data.ETag != etag || env.Data.UpdatedBy != Actor || env.Data.Namespace != "ui" ||
		env.Data.Locale != "id" || env.Data.Key != "actions.save" {
		t.Errorf("data = %+v", env.Data)
	}
	revAfter := e.svc.Revisions(context.Background(), tenantA)["ui"]
	if revAfter == revBefore {
		t.Error("revisi harus naik setelah perubahan")
	}
	second := e.do(http.MethodPut, putPath, body("Simpan sekarang"))
	if second.Code != http.StatusOK || second.Header().Get("ETag") != etag {
		t.Errorf("PUT kedua: %d etag %q", second.Code, second.Header().Get("ETag"))
	}
	if got := e.svc.Revisions(context.Background(), tenantA)["ui"]; got != revAfter {
		t.Errorf("PUT dengan nilai sama menaikkan revisi: %s -> %s", revAfter, got)
	}
	// katalog mencerminkan override
	cat := e.do(http.MethodGet, "/api/platform/i18n/id/ui", "")
	if !strings.Contains(cat.Body.String(), "Simpan sekarang") {
		t.Errorf("katalog tidak memuat override: %s", cat.Body)
	}
}

func TestOverrideAPI_IfMatch412(t *testing.T) {
	e := newEnv(t, allowAll, true)
	// If-Match pada baris yang belum ada: 412
	if rec := e.do(http.MethodPut, putPath, body("A"), "If-Match", `"abc"`); rec.Code != http.StatusPreconditionFailed || rec.errBody(t).Error.Code != "version_mismatch" {
		t.Fatalf("baris belum ada + If-Match: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(http.MethodPut, putPath, body("A"), "If-Match", "*"); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("* pada baris belum ada: %d", rec.Code)
	}
	etag := e.do(http.MethodPut, putPath, body("Satu")).Header().Get("ETag")
	// cocok
	if rec := e.do(http.MethodPut, putPath, body("Dua"), "If-Match", etag); rec.Code != http.StatusOK {
		t.Fatalf("If-Match cocok: %d %s", rec.Code, rec.Body)
	}
	// etag lama sekarang basi
	if rec := e.do(http.MethodPut, putPath, body("Tiga"), "If-Match", etag); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("etag basi: %d", rec.Code)
	}
	cur := e.do(http.MethodGet, "/api/platform/i18n/overrides?namespace=ui", "")
	var list struct{ Data []OverrideView }
	_ = json.Unmarshal(cur.Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].Message != "Dua" {
		t.Fatalf("412 tidak boleh mengubah apa pun: %+v", list.Data)
	}
	// tag lemah tidak pernah cocok (perbandingan kuat)
	if rec := e.do(http.MethodPut, putPath, body("Empat"), "If-Match", "W/"+list.Data[0].ETag); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("tag lemah: %d", rec.Code)
	}
	// daftar dan *
	if rec := e.do(http.MethodPut, putPath, body("Lima"), "If-Match", `"x", `+list.Data[0].ETag); rec.Code != http.StatusOK {
		t.Errorf("daftar tag memuat yang cocok: %d", rec.Code)
	}
	if rec := e.do(http.MethodPut, putPath, body("Enam"), "If-Match", "*"); rec.Code != http.StatusOK {
		t.Errorf("* pada baris ada: %d", rec.Code)
	}
	// DELETE dengan If-Match basi: 412, baris tetap
	if rec := e.do(http.MethodDelete, putPath, "", "If-Match", `"basi"`); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("DELETE basi: %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, putPath, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE = %d", rec.Code)
	}
}

func TestOverrideAPI_ValidationErrors400(t *testing.T) {
	e := newEnv(t, allowAll, true)
	cases := []struct {
		name, path, msg, code, field string
	}{
		{"kunci tak ada", "/api/platform/i18n/overrides/ui/id/tidak.ada", "x", "unknown_key", "key"},
		{"namespace tak dikenal", "/api/platform/i18n/overrides/nope/id/a", "x", "unknown_key", "namespace"},
		{"bahasa tak terdaftar", "/api/platform/i18n/overrides/ui/zz/actions.save", "x", "unknown_key", "locale"},
		{"ICU rusak", putPath, "Halo {nama", "invalid_syntax", "message"},
		{"argumen berbeda", "/api/platform/i18n/overrides/ui/id/n", "tanpa argumen", "param_mismatch", "message"},
		{"HTML", putPath, "<script>alert(1)</script>", "html_not_allowed", "message"},
		{"terlalu panjang", putPath, strings.Repeat("a", 2001), "too_long", "message"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := e.do(http.MethodPut, c.path, body(c.msg))
			eb := rec.errBody(t)
			if rec.Code != http.StatusBadRequest || eb.Error.Code != "invalid_i18n_override" {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			if len(eb.Error.Details) != 1 || eb.Error.Details[0].Code != c.code || eb.Error.Details[0].Field != c.field {
				t.Errorf("details = %+v, want code %s field %s", eb.Error.Details, c.code, c.field)
			}
		})
	}
	// R-4: argumen sederhana tidak boleh menggantikan plural (jenis harus sama persis)
	if rec := e.do(http.MethodPut, "/api/platform/i18n/overrides/ui/en/n", body("{count} entries")); rec.Code != http.StatusBadRequest {
		t.Errorf("arg menggantikan plural: %d, want 400", rec.Code)
	}
	// pesan ber-argumen yang sah diterima
	if rec := e.do(http.MethodPut, "/api/platform/i18n/overrides/ui/en/n", body("{count, plural, one {# entry} other {# entries}}")); rec.Code != http.StatusOK {
		t.Errorf("override sah: %d %s", rec.Code, rec.Body)
	}
}

func TestOverrideAPI_BodyValidation(t *testing.T) {
	e := newEnv(t, allowAll, true)
	// tenantId (atau field apa pun yang tak dikenal) di body ditolak: tenant selalu dari context
	rec := e.do(http.MethodPut, putPath, `{"message":"x","tenantId":"`+tenantB.String()+`"}`)
	if rec.Code != http.StatusUnprocessableEntity || rec.errBody(t).Error.Code != "validation_failed" {
		t.Errorf("tenantId di body: %d %s", rec.Code, rec.Body)
	}
	if rec := e.do(http.MethodPut, putPath, `{}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("message hilang: %d", rec.Code)
	}
	if rec := e.do(http.MethodPut, putPath, `{"message": 5}`); rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
		t.Errorf("tipe salah: %d", rec.Code)
	}
	if rec := e.do(http.MethodPut, putPath, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("JSON rusak: %d", rec.Code)
	}
	big := body(strings.Repeat("a", 9<<10))
	if rec := e.do(http.MethodPut, putPath, big); rec.Code != http.StatusBadRequest {
		t.Errorf("body > 8 KB: %d, want 400", rec.Code)
	}
	if e.svc.Revisions(context.Background(), tenantB)["ui"] != e.svc.Revisions(context.Background(), tenantA)["ui"] {
		t.Error("tidak ada perubahan dari permintaan yang ditolak")
	}
}

// code galat tetap stabil walau message ditimpa override (Requirement 12.9): override errors.not_found
// mengubah teksnya sedangkan code tidak.
func TestOverrideAPI_ErrorCodeStableWhenMessageOverridden(t *testing.T) {
	e := newEnv(t, allowAll, true)
	rec := e.do(http.MethodGet, "/api/platform/i18n/en/tidak-ada", "", "Accept-Language", "en")
	before := rec.errBody(t)
	if rec.Code != http.StatusNotFound || before.Error.Code != "not_found" || before.Error.Message != "Not found" {
		t.Fatalf("awal: %d %+v", rec.Code, before)
	}
	if put := e.do(http.MethodPut, "/api/platform/i18n/overrides/platform/en/errors.not_found", body("Missing in action")); put.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", put.Code, put.Body)
	}
	after := e.do(http.MethodGet, "/api/platform/i18n/en/tidak-ada", "", "Accept-Language", "en").errBody(t)
	if after.Error.Code != "not_found" || after.Error.Message != "Missing in action" {
		t.Errorf("setelah override: %+v", after)
	}
	// bahasa lain tidak terpengaruh
	if idMsg := e.do(http.MethodGet, "/api/platform/i18n/id/tidak-ada", "").errBody(t).Error.Message; idMsg != "Tidak ditemukan" {
		t.Errorf("bahasa id berubah: %q", idMsg)
	}
}

// IDOR tingkat API (tenant A vs B): list, put, delete tidak pernah menembus tenant lain.
func TestOverrideAPI_TenantIsolation(t *testing.T) {
	e := newEnv(t, allowAll, true)
	if rec := e.do(http.MethodPut, putPath, body("Milik A")); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	var list struct {
		Data []OverrideView
		Page struct{ Total int64 }
	}
	rec := e.do(http.MethodGet, "/api/platform/i18n/overrides", "", "X-Tenant", "B")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Data) != 0 || list.Page.Total != 0 {
		t.Errorf("list B melihat milik A: %s", rec.Body)
	}
	if rec := e.do(http.MethodDelete, putPath, "", "X-Tenant", "B"); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE B atas milik A: %d, want 404", rec.Code)
	}
	if rec := e.do(http.MethodPut, putPath, body("x"), "X-Tenant", "B", "If-Match", rowETagOf("Milik A")); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("If-Match milik A dipakai B: %d, want 412", rec.Code)
	}
	// katalog B tidak memuat override A
	if cat := e.do(http.MethodGet, "/api/platform/i18n/id/ui", "", "X-Tenant", "B"); strings.Contains(cat.Body.String(), "Milik A") {
		t.Error("katalog B memuat override A")
	}
	// B menulis kunci yang sama; A tidak berubah
	if rec := e.do(http.MethodPut, putPath, body("Milik B"), "X-Tenant", "B"); rec.Code != http.StatusOK {
		t.Fatal(rec.Body)
	}
	rec = e.do(http.MethodGet, "/api/platform/i18n/overrides", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].Message != "Milik A" {
		t.Errorf("milik A berubah: %+v", list.Data)
	}
	if rec := e.do(http.MethodDelete, putPath, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE A = %d", rec.Code)
	}
	if rec := e.do(http.MethodDelete, putPath, ""); rec.Code != http.StatusNotFound {
		t.Errorf("DELETE kedua = %d, want 404", rec.Code)
	}
}

func rowETagOf(msg string) string { return rowETag(store.Override{Message: msg}) }

func TestOverrideAPI_ListFiltersAndPagination(t *testing.T) {
	e := newEnv(t, allowAll, true)
	puts := map[string]string{
		"/api/platform/i18n/overrides/ui/id/actions.save": "a", "/api/platform/i18n/overrides/ui/en/actions.save": "b",
		"/api/platform/i18n/overrides/platform/id/errors.not_found": "c",
	}
	for p, m := range puts {
		if rec := e.do(http.MethodPut, p, body(m)); rec.Code != http.StatusOK {
			t.Fatalf("%s: %s", p, rec.Body)
		}
	}
	get := func(q string) (rows []OverrideView, total int64, code int) {
		rec := e.do(http.MethodGet, "/api/platform/i18n/overrides"+q, "")
		var out struct {
			Data []OverrideView
			Page struct{ Total int64 }
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Data, out.Page.Total, rec.Code
	}
	if rows, total, _ := get(""); total != 3 || len(rows) != 3 || rows[0].Namespace != "platform" {
		t.Errorf("semua: %d %+v", total, rows)
	}
	if rows, total, _ := get("?namespace=ui"); total != 2 || len(rows) != 2 {
		t.Errorf("filter ns: %d", total)
	}
	if rows, total, _ := get("?namespace=ui&locale=en"); total != 1 || rows[0].Message != "b" {
		t.Errorf("filter ns+locale: %d %+v", total, rows)
	}
	if rows, total, _ := get("?pageSize=2&page=2"); total != 3 || len(rows) != 1 {
		t.Errorf("halaman 2: %d %d", total, len(rows))
	}
	for _, q := range []string{"?page=0", "?pageSize=0", "?pageSize=201", "?page=abc", "?page=9223372036854775807&pageSize=200"} {
		if _, _, code := get(q); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, code)
		}
	}
}

func TestMatchStrong(t *testing.T) {
	e := `"abc"`
	for header, want := range map[string]bool{
		`"abc"`: true, `*`: true, `"x", "abc"`: true, `"x","abc"`: true, `W/"abc"`: false, `"abcd"`: false, ``: false, `abc`: false,
		`W/"abc", "abc"`: true,
	} {
		if got := matchStrong(header, e); got != want {
			t.Errorf("matchStrong(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestIsMutation(t *testing.T) {
	mk := func(mod, method, pattern string) registry.RouteInfo {
		return registry.RouteInfo{Module: mod, Method: method, Pattern: pattern}
	}
	yes := []registry.RouteInfo{
		mk(registry.PlatformKey, "PUT", "/api/platform/i18n/overrides/{namespace}/{locale}/{key}"),
		mk(registry.PlatformKey, "DELETE", "/api/platform/i18n/overrides/{namespace}/{locale}/{key}"),
	}
	no := []registry.RouteInfo{
		mk(registry.PlatformKey, "GET", "/api/platform/i18n/overrides"),
		mk(registry.PlatformKey, "GET", "/api/platform/i18n/{locale}/{namespace}"),
		mk("acme.x", "PUT", "/api/platform/i18n/overrides/{namespace}/{locale}/{key}"),
		mk(registry.PlatformKey, "POST", "/api/platform/modules/{key}/{operation}"),
	}
	for _, r := range yes {
		if !IsMutation(r) {
			t.Errorf("%+v harus mutasi", r)
		}
	}
	for _, r := range no {
		if IsMutation(r) {
			t.Errorf("%+v bukan mutasi", r)
		}
	}
}
