package i18n

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"

	"erpora/internal/platform/tenancy"
	sdki18n "erpora/sdk/i18n"
)

var tenantA = tenancy.TenantID(uuid.MustParse("11111111-1111-7111-8111-111111111111"))

func testRegistry(t testing.TB) *Registry {
	t.Helper()
	r, err := ParseRegistry([]byte(validRegistry))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mapFS(files map[string]string) fs.FS {
	m := fstest.MapFS{}
	for name, data := range files {
		m[name] = &fstest.MapFile{Data: []byte(data)}
	}
	return m
}

// exampleSource adalah modul "example" dengan id (default), en, en-GB.
func exampleSource() Source {
	return Source{
		Namespace: "example", Module: true, Default: "id", Locales: []string{"id", "en", "en-GB"},
		FS: mapFS(map[string]string{
			"id.json":    `{"a": "A-id", "b": "B-id", "c": "C-id", "n": {"count": "{n, plural, other {# catatan}}"}}`,
			"en.json":    `{"a": "A-en", "b": "B-en", "n": {"count": "{n, plural, one {# note} other {# notes}}"}}`,
			"en-GB.json": `{"a": "A-gb"}`,
		}),
	}
}

func platformSource() Source {
	return Source{Namespace: "platform", Default: "id", FS: mapFS(map[string]string{
		"id.json": `{"errors": {"not_found": "Tidak ditemukan"}}`,
		"en.json": `{"errors": {"not_found": "Not found"}}`,
	})}
}

func uiSource() Source {
	return Source{Namespace: "ui", Default: "id", FS: mapFS(map[string]string{
		"id.json": `{"actions": {"save": "Simpan"}}`,
		"en.json": `{"actions": {"save": "Save"}}`,
	})}
}

func newTestService(t *testing.T, opts Options) *Service {
	t.Helper()
	if opts.Registry == nil {
		opts.Registry = testRegistry(t)
	}
	if opts.Sources == nil {
		opts.Sources = []Source{platformSource(), uiSource(), exampleSource()}
	}
	s, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestResolveFallbackChain(t *testing.T) {
	s := newTestService(t, Options{})
	ctx := context.Background()
	cases := []struct {
		loc, key, want string
		origin         Origin
	}{
		{"en-GB", "a", "A-gb", OriginBuiltin},  // bahasa persis
		{"en-GB", "b", "B-en", OriginFallback}, // fallback registri (en)
		{"en-GB", "c", "C-id", OriginFallback}, // sampai bahasa default
		{"en", "a", "A-en", OriginBuiltin},
		{"id", "a", "A-id", OriginBuiltin},
		{"ms", "a", "A-id", OriginFallback}, // ms terdaftar tetapi tidak punya katalog -> id
		{"zz", "a", "A-id", OriginFallback}, // bahasa tak terdaftar -> default
		{"en-GB", "zz.zz", "example::zz.zz", OriginMissing},
		{"", "a", "A-id", OriginBuiltin},
	}
	for _, c := range cases {
		got, origin := s.Resolve(ctx, tenantA, "example", c.key, sdki18n.Locale(c.loc))
		if got != c.want || origin != c.origin {
			t.Errorf("Resolve(%q, %q) = %q, %v; want %q, %v", c.loc, c.key, got, origin, c.want, c.origin)
		}
	}
}

func TestResolveUnknownNamespace(t *testing.T) {
	s := newTestService(t, Options{})
	got, origin := s.Resolve(context.Background(), tenantA, "nope", "a.b", "id")
	if got != "nope::a.b" || origin != OriginMissing {
		t.Errorf("got %q, %v", got, origin)
	}
}

func TestInactiveModuleNamespaceUnavailableButUIAndPlatformAlways(t *testing.T) {
	active := map[string]bool{"example": false}
	s := newTestService(t, Options{Active: func(_ tenancy.TenantID, ns string) bool { return active[ns] }})
	ctx := context.Background()
	if got, o := s.Resolve(ctx, tenantA, "example", "a", "id"); o != OriginMissing || got != "example::a" {
		t.Errorf("modul nonaktif: got %q, %v", got, o)
	}
	if _, err := s.Catalog(ctx, tenantA, "example", "id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Catalog modul nonaktif err = %v, want ErrNotFound", err)
	}
	for _, ns := range []string{"ui", "platform"} {
		if _, err := s.Catalog(ctx, tenantA, ns, "id"); err != nil {
			t.Errorf("Catalog(%s) harus selalu tersedia: %v", ns, err)
		}
	}
	if revs := s.Revisions(ctx, tenantA); revs["example"] != "" || revs["ui"] == "" || revs["platform"] == "" {
		t.Errorf("Revisions = %v", revs)
	}
	active["example"] = true
	if _, err := s.Catalog(ctx, tenantA, "example", "id"); err != nil {
		t.Errorf("setelah diaktifkan: %v", err)
	}
	if revs := s.Revisions(ctx, tenantA); revs["example"] == "" {
		t.Errorf("Revisions setelah aktif = %v", revs)
	}
}

func TestCatalogMergesFallbackChain(t *testing.T) {
	s := newTestService(t, Options{})
	v, err := s.Catalog(context.Background(), tenantA, "example", "en-GB")
	if err != nil {
		t.Fatal(err)
	}
	if v.Locale != "en-GB" || v.Namespace != "example" {
		t.Errorf("View = %+v", v)
	}
	want := map[string]string{
		"a": "A-gb", "b": "B-en", "c": "C-id",
		"n.count": "{n, plural, one {# note} other {# notes}}",
	}
	if len(v.Messages) != len(want) {
		t.Fatalf("Messages = %v", v.Messages)
	}
	for k, w := range want {
		if v.Messages[k] != w {
			t.Errorf("Messages[%q] = %q, want %q", k, v.Messages[k], w)
		}
	}
	if v.Revision == "" || !strings.HasPrefix(v.ETag, `W/"`) {
		t.Errorf("Revision/ETag = %q/%q", v.Revision, v.ETag)
	}
}

func TestCatalogUnknownLocaleOrNamespaceNotFound(t *testing.T) {
	s := newTestService(t, Options{})
	for _, c := range [][2]string{{"example", "zz"}, {"nope", "id"}, {"example", ""}, {"", "id"}, {"../x", "id"}} {
		if _, err := s.Catalog(context.Background(), tenantA, c[0], sdki18n.Locale(c[1])); !errors.Is(err, ErrNotFound) {
			t.Errorf("Catalog(%q, %q) err = %v, want ErrNotFound", c[0], c[1], err)
		}
	}
	// bahasa terdaftar tanpa katalog sendiri tetap tersedia (isi = fallback)
	v, err := s.Catalog(context.Background(), tenantA, "example", "ms")
	if err != nil || v.Messages["a"] != "A-id" {
		t.Errorf("Catalog(ms) = %+v, %v", v, err)
	}
}

// P7 ETag mengikuti isi: sama bila peta efektif sama, berbeda bila ada kunci atau nilai yang berbeda.
func TestETagFollowsContent(t *testing.T) {
	a := newTestService(t, Options{})
	b := newTestService(t, Options{})
	ctx := context.Background()
	va, _ := a.Catalog(ctx, tenantA, "example", "en")
	vb, _ := b.Catalog(ctx, tenantA, "example", "en")
	if va.ETag != vb.ETag || va.Revision != vb.Revision {
		t.Errorf("isi sama harus ETag/Revision sama: %q vs %q", va.ETag, vb.ETag)
	}
	if vId, _ := a.Catalog(ctx, tenantA, "example", "id"); vId.ETag == va.ETag {
		t.Error("isi berbeda harus ETag berbeda")
	}

	changed := exampleSource()
	changed.FS = mapFS(map[string]string{
		"id.json":    `{"a": "A-id", "b": "B-id", "c": "C-id-BARU", "n": {"count": "{n, plural, other {# catatan}}"}}`,
		"en.json":    `{"a": "A-en", "b": "B-en", "n": {"count": "{n, plural, one {# note} other {# notes}}"}}`,
		"en-GB.json": `{"a": "A-gb"}`,
	})
	c := newTestService(t, Options{Sources: []Source{platformSource(), uiSource(), changed}})
	vc, _ := c.Catalog(ctx, tenantA, "example", "en")
	if vc.ETag == va.ETag {
		t.Error("kunci c berubah di bahasa default; peta efektif en ikut berubah, ETag harus berbeda")
	}
	if va.Revision == vc.Revision {
		t.Error("Revision namespace harus berubah saat isi berubah")
	}
	// ui tidak berubah -> revisinya sama
	if a.Revisions(ctx, tenantA)["ui"] != c.Revisions(ctx, tenantA)["ui"] {
		t.Error("revisi ui tidak boleh berubah")
	}
}

func TestNewRejectsStructuralErrors(t *testing.T) {
	reg := testRegistry(t)
	mk := func(files map[string]string, locales ...string) []Source {
		ex := exampleSource()
		ex.Locales, ex.FS = locales, mapFS(files)
		return []Source{platformSource(), uiSource(), ex}
	}
	cases := map[string]struct {
		sources []Source
		want    []string
	}{
		"kunci ganda":               {mk(map[string]string{"id.json": `{"a": "x", "a": "y"}`}, "id"), []string{"example/id.json", "duplicate_key", `"a"`}},
		"tabrakan perataan":         {mk(map[string]string{"id.json": `{"a.b": {"c": "x"}, "a": {"b.c": "y"}}`}, "id"), []string{"example/id.json", "flatten_collision", "a.b.c"}},
		"ICU rusak":                 {mk(map[string]string{"id.json": `{"a": {"b": "Halo {n"}}`}, "id"), []string{"example/id.json", "invalid_icu", "a.b"}},
		"HTML":                      {mk(map[string]string{"id.json": `{"a": "<script>x</script>"}`}, "id"), []string{"html_not_allowed", "a"}},
		"JSON rusak di bahasa lain": {mk(map[string]string{"id.json": `{}`, "en.json": `{`}, "id", "en"), []string{"example/en.json", "invalid_json"}},
		"berkas bahasa hilang":      {mk(map[string]string{"id.json": `{}`}, "id", "en"), []string{"example/en.json", "tidak ditemukan"}},
		"bahasa tak terdaftar":      {mk(map[string]string{"id.json": `{}`, "fr.json": `{}`}, "id", "fr"), []string{`"fr"`, "registri"}},
		"default bukan anggota locales": {func() []Source {
			ex := exampleSource()
			ex.Default, ex.Locales = "en", []string{"id"}
			return []Source{platformSource(), uiSource(), ex}
		}(), []string{"default"}},
		"namespace ganda": {[]Source{platformSource(), platformSource(), uiSource()}, []string{"platform", "ganda"}},
		"namespace tidak valid": {func() []Source {
			ex := exampleSource()
			ex.Namespace = "Ex::ample"
			return []Source{platformSource(), uiSource(), ex}
		}(), []string{"namespace"}},
		"modul memakai namespace ui": {func() []Source {
			ex := exampleSource()
			ex.Namespace = "ui"
			return []Source{platformSource(), uiSource(), ex}
		}(), []string{"ui", "ganda"}},
		"platform tanpa katalog default": {[]Source{{Namespace: "platform", Default: "id", FS: mapFS(map[string]string{"en.json": `{}`})}, uiSource()}, []string{"platform", "id.json"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New(Options{Registry: reg, Sources: c.sources})
			if err == nil {
				t.Fatal("New harus galat")
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("err = %q, harus memuat %q", err, w)
				}
			}
		})
	}
}

func TestNewCollectsAllIssuesAsCatalogError(t *testing.T) {
	ex := exampleSource()
	ex.FS = mapFS(map[string]string{
		"id.json":    `{"a": "x", "a": "y"}`,
		"en.json":    `{"b": "Halo {"}`,
		"en-GB.json": `{}`,
	})
	_, err := New(Options{Registry: testRegistry(t), Sources: []Source{platformSource(), uiSource(), ex}})
	var ce *CatalogError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *CatalogError", err)
	}
	if len(ce.Issues) < 2 {
		t.Fatalf("Issues = %+v", ce.Issues)
	}
	for _, is := range ce.Issues {
		if is.File == "" || is.Locale == "" {
			t.Errorf("Issue harus memuat File dan Locale: %+v", is)
		}
	}
}

func TestParityMismatchOnlyWarns(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ex := exampleSource()
	ex.FS = mapFS(map[string]string{
		"id.json":    `{"a": "{x} {y}", "only_id": "x"}`,
		"en.json":    `{"a": "{x}", "only_en": "y"}`,
		"en-GB.json": `{}`,
	})
	if _, err := New(Options{Registry: testRegistry(t), Sources: []Source{platformSource(), uiSource(), ex}, Log: log}); err != nil {
		t.Fatalf("ketidaksesuaian antar bahasa tidak boleh menggagalkan: %v", err)
	}
	out := buf.String()
	for _, w := range []string{"example", "only_id", "only_en"} {
		if !strings.Contains(out, w) {
			t.Errorf("log peringatan harus menyebut %q:\n%s", w, out)
		}
	}
}

func TestPartialLocaleMissingKeysNotWarned(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ex := exampleSource()
	ex.Locales = []string{"id", "ms"}
	ex.FS = mapFS(map[string]string{"id.json": `{"a": "x", "b": "y"}`, "ms.json": `{"a": "x"}`})
	if _, err := New(Options{Registry: testRegistry(t), Sources: []Source{platformSource(), uiSource(), ex}, Log: log}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"b"`) || strings.Contains(buf.String(), "key=b") {
		t.Errorf("bahasa partial tidak boleh diperingatkan untuk kunci hilang:\n%s", buf.String())
	}
}

// countingHandler menghitung catatan log per pesan.
type countingHandler struct {
	mu    sync.Mutex
	count map[string]int
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.count[r.Message]++
	h.mu.Unlock()
	return nil
}
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }

func TestFallbackWarnedOncePerLocaleNsKeyInDevOnly(t *testing.T) {
	for _, dev := range []bool{true, false} {
		h := &countingHandler{count: map[string]int{}}
		s := newTestService(t, Options{Dev: dev, Log: slog.New(h)})
		ctx := context.Background()
		before := h.count[WarnFallbackMessage]
		for i := 0; i < 5; i++ {
			s.Resolve(ctx, tenantA, "example", "c", "en-GB") // fallback ke id
			s.Resolve(ctx, tenantA, "example", "a", "en-GB") // persis: tanpa peringatan
		}
		s.Resolve(ctx, tenantA, "example", "b", "en-GB") // kunci lain -> peringatan baru
		got := h.count[WarnFallbackMessage] - before
		want := 0
		if dev {
			want = 2
		}
		if got != want {
			t.Errorf("dev=%v: peringatan fallback = %d, want %d", dev, got, want)
		}
	}
}

func TestResolverAdapterWorksWithSDKT(t *testing.T) {
	s := newTestService(t, Options{})
	ctx := sdki18n.WithResolver(context.Background(), s.Resolver(tenantA))
	ctx = sdki18n.WithScope(sdki18n.WithLocale(ctx, "en"), "example")
	if got := sdki18n.T(ctx, "n.count", sdki18n.Args{"n": 1}); got != "1 note" {
		t.Errorf("T en = %q", got)
	}
	ctx = sdki18n.WithLocale(ctx, "id")
	if got := sdki18n.T(ctx, "n.count", sdki18n.Args{"n": 1500}); got != "1.500 catatan" {
		t.Errorf("T id = %q", got)
	}
	if got := sdki18n.T(ctx, "ui::actions.save"); got != "Simpan" {
		t.Errorf("T ui = %q", got)
	}
	if got := sdki18n.T(ctx, "tidak.ada"); got != "example::tidak.ada" {
		t.Errorf("T hilang = %q", got)
	}
	if got := sdki18n.T(ctx, "other::a"); got != "other::a" {
		t.Errorf("T namespace modul lain = %q", got)
	}
}

// P3 Pencarian total: Resolve selalu mengembalikan string dan tidak panik; bila tidak ada di rantai,
// hasilnya tepat namespace::kunci.
func TestPropertyResolveTotal(t *testing.T) {
	s := newTestService(t, Options{})
	locs := []string{"id", "en", "en-GB", "ms", "zz", "", "../etc/passwd", "en-US"}
	nss := []string{"example", "ui", "platform", "nope", "", "a::b"}
	keys := []string{"a", "b", "c", "zz", "", "n.count", "x::y", strings.Repeat("k", 500)}
	for _, l := range locs {
		for _, ns := range nss {
			for _, k := range keys {
				src, origin := s.Resolve(context.Background(), tenantA, ns, k, sdki18n.Locale(l))
				if origin == OriginMissing && src != ns+"::"+k {
					t.Errorf("Resolve(%q,%q,%q) missing = %q", l, ns, k, src)
				}
			}
		}
	}
}
