package i18n

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/tenancy"
	sdki18n "erpora/sdk/i18n"
)

var tenantB = tenancy.TenantID{0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x72, 0x22, 0x82, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22}

// fakeStore adalah OverrideStore dalam memori (dipakai bersama beberapa Service untuk meniru banyak instance).
type fakeStore struct {
	mu        sync.Mutex
	rows      map[tenancy.TenantID]map[[3]string]store.Override
	revs      map[tenancy.TenantID]map[[2]string]int64
	revCalls  int
	loadCalls int
	failAll   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[tenancy.TenantID]map[[3]string]store.Override{}, revs: map[tenancy.TenantID]map[[2]string]int64{}}
}

func (f *fakeStore) Revisions(_ context.Context, t tenancy.TenantID) ([]store.Revision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revCalls++
	if f.failAll != nil {
		return nil, f.failAll
	}
	var out []store.Revision
	for k, v := range f.revs[t] {
		out = append(out, store.Revision{Namespace: k[0], Locale: k[1], Revision: v})
	}
	return out, nil
}

func (f *fakeStore) ListLocale(_ context.Context, t tenancy.TenantID, ns, loc string) ([]store.Override, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loadCalls++
	if f.failAll != nil {
		return nil, f.failAll
	}
	var out []store.Override
	for k, v := range f.rows[t] {
		if k[0] == ns && k[1] == loc {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeStore) Get(_ context.Context, t tenancy.TenantID, ns, loc, key string) (store.Override, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.rows[t][[3]string{ns, loc, key}]; ok {
		return v, nil
	}
	return store.Override{}, store.ErrNotFound
}

func (f *fakeStore) List(_ context.Context, t tenancy.TenantID, flt store.Filter, _, _ int) ([]store.Override, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Override
	for k, v := range f.rows[t] {
		if (flt.Namespace == "" || flt.Namespace == k[0]) && (flt.Locale == "" || flt.Locale == k[1]) {
			out = append(out, v)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeStore) bump(t tenancy.TenantID, ns, loc string) int64 {
	if f.revs[t] == nil {
		f.revs[t] = map[[2]string]int64{}
	}
	f.revs[t][[2]string{ns, loc}]++
	return f.revs[t][[2]string{ns, loc}]
}

func (f *fakeStore) Upsert(_ context.Context, t tenancy.TenantID, o store.Override, pre store.Precondition) (bool, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return false, 0, f.failAll
	}
	k := [3]string{o.Namespace, o.Locale, o.Key}
	var cur *store.Override
	if v, ok := f.rows[t][k]; ok {
		cur = &v
	}
	if pre != nil {
		if err := pre(cur); err != nil {
			return false, 0, err
		}
	}
	if cur != nil && cur.Message == o.Message {
		return false, f.revs[t][[2]string{o.Namespace, o.Locale}], nil
	}
	if f.rows[t] == nil {
		f.rows[t] = map[[3]string]store.Override{}
	}
	o.TenantID = t
	f.rows[t][k] = o
	return true, f.bump(t, o.Namespace, o.Locale), nil
}

func (f *fakeStore) Delete(_ context.Context, t tenancy.TenantID, ns, loc, key string, pre store.Precondition) (bool, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAll != nil {
		return false, 0, f.failAll
	}
	k := [3]string{ns, loc, key}
	var cur *store.Override
	if v, ok := f.rows[t][k]; ok {
		cur = &v
	}
	if pre != nil {
		if err := pre(cur); err != nil {
			return false, 0, err
		}
	}
	if cur == nil {
		return false, f.revs[t][[2]string{ns, loc}], nil
	}
	delete(f.rows[t], k)
	return true, f.bump(t, ns, loc), nil
}

// fakeClock adalah jam uji yang dapat dimajukan.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newOverrideService(t *testing.T, fs *fakeStore, clk *fakeClock, extra ...func(*Options)) *Service {
	t.Helper()
	opts := Options{Overrides: fs, Now: clk.Now}
	for _, f := range extra {
		f(&opts)
	}
	return newTestService(t, opts)
}

func newClock() *fakeClock { return &fakeClock{t: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)} }

func set(t *testing.T, s *Service, tn tenancy.TenantID, ns, loc, key, msg string) bool {
	t.Helper()
	changed, err := s.SetOverride(context.Background(), tn, Override{Namespace: ns, Locale: loc, Key: key, Message: msg}, "api:dev", nil)
	if err != nil {
		t.Fatalf("SetOverride(%s/%s/%s): %v", ns, loc, key, err)
	}
	return changed
}

func resolve(s *Service, tn tenancy.TenantID, ns, loc, key string) (string, Origin) {
	return s.Resolve(context.Background(), tn, ns, key, sdki18n.Locale(loc))
}

func overrideErrCode(t *testing.T, err error) string {
	t.Helper()
	var oe *OverrideError
	if !errors.As(err, &oe) {
		t.Fatalf("err = %v, want *OverrideError", err)
	}
	return oe.Code
}

func TestOverrideBeatsBuiltinOnSameLocale(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	set(t, s, tenantA, "example", "id", "a", "A-baru")
	if got, o := resolve(s, tenantA, "example", "id", "a"); got != "A-baru" || o != OriginOverride {
		t.Errorf("Resolve = %q, %v", got, o)
	}
	v, _ := s.Catalog(context.Background(), tenantA, "example", "id")
	if v.Messages["a"] != "A-baru" || v.Messages["b"] != "B-id" {
		t.Errorf("Catalog = %v", v.Messages)
	}
}

// Urutan fallback (Requirement 3.2): pada setiap bahasa dalam rantai, override dahulu lalu katalog bawaan.
func TestOverrideOrderingAcrossFallbackChain(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	// en-GB punya a bawaan sendiri; override pada en (bahasa fallback) tidak mengalahkannya
	set(t, s, tenantA, "example", "en", "a", "A-en-override")
	if got, _ := resolve(s, tenantA, "example", "en-GB", "a"); got != "A-gb" {
		t.Errorf("bawaan bahasa persis harus menang atas override fallback: %q", got)
	}
	// b tidak ada di en-GB: override en menang atas bawaan en
	set(t, s, tenantA, "example", "en", "b", "B-en-override")
	if got, o := resolve(s, tenantA, "example", "en-GB", "b"); got != "B-en-override" || o != OriginFallback {
		t.Errorf("b = %q, %v", got, o)
	}
	// override pada bahasa persis mengalahkan semuanya
	set(t, s, tenantA, "example", "en-GB", "b", "B-gb-override")
	if got, o := resolve(s, tenantA, "example", "en-GB", "b"); got != "B-gb-override" || o != OriginOverride {
		t.Errorf("b = %q, %v", got, o)
	}
	// override pada bahasa default menjadi fallback terakhir
	set(t, s, tenantA, "example", "id", "c", "C-id-override")
	if got, _ := resolve(s, tenantA, "example", "en-GB", "c"); got != "C-id-override" {
		t.Errorf("c = %q", got)
	}
	v, _ := s.Catalog(context.Background(), tenantA, "example", "en-GB")
	if v.Messages["a"] != "A-gb" || v.Messages["b"] != "B-gb-override" || v.Messages["c"] != "C-id-override" {
		t.Errorf("Catalog en-GB = %v", v.Messages)
	}
}

// P8: SetOverride dua kali dengan nilai sama tidak menaikkan revisi kedua kali; DeleteOverride pada kunci
// yang tidak ada tidak menaikkan revisi.
func TestOverrideIdempotentRevision(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	ctx := context.Background()
	before := s.Revisions(ctx, tenantA)["example"]
	if !set(t, s, tenantA, "example", "id", "a", "X") {
		t.Fatal("perubahan pertama harus changed")
	}
	r1 := s.Revisions(ctx, tenantA)["example"]
	if r1 == before {
		t.Error("revisi namespace harus naik setelah perubahan")
	}
	if set(t, s, tenantA, "example", "id", "a", "X") {
		t.Error("nilai sama tidak boleh changed")
	}
	if r2 := s.Revisions(ctx, tenantA)["example"]; r2 != r1 {
		t.Errorf("revisi kedua kali naik: %s -> %s", r1, r2)
	}
	existed, err := s.DeleteOverride(ctx, tenantA, "example", "id", "tidak.ada", "api:dev", nil)
	if err != nil || existed {
		t.Fatalf("Delete kunci tak ada: %v %v", existed, err)
	}
	if r3 := s.Revisions(ctx, tenantA)["example"]; r3 != r1 {
		t.Errorf("Delete tak ada menaikkan revisi: %s -> %s", r1, r3)
	}
	existed, err = s.DeleteOverride(ctx, tenantA, "example", "id", "a", "api:dev", nil)
	if err != nil || !existed {
		t.Fatalf("Delete ada: %v %v", existed, err)
	}
	if got, o := resolve(s, tenantA, "example", "id", "a"); got != "A-id" || o != OriginBuiltin {
		t.Errorf("setelah hapus kembali ke bawaan: %q, %v", got, o)
	}
	if r4 := s.Revisions(ctx, tenantA)["example"]; r4 == r1 {
		t.Error("revisi harus naik setelah hapus yang berefek")
	}
}

func TestOverrideValidation(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	ctx := context.Background()
	long := strings.Repeat("a", 2001)
	cases := []struct {
		name string
		o    Override
		code string
	}{
		{"kunci tak ada di bawaan default", Override{Namespace: "example", Locale: "id", Key: "tidak.ada", Message: "x"}, CodeUnknownKey},
		{"namespace tak dikenal", Override{Namespace: "nope", Locale: "id", Key: "a", Message: "x"}, CodeUnknownKey},
		{"bahasa tak terdaftar", Override{Namespace: "example", Locale: "zz", Key: "a", Message: "x"}, CodeUnknownKey},
		{"ICU tidak valid", Override{Namespace: "example", Locale: "id", Key: "a", Message: "Halo {nama"}, CodeInvalidSyntax},
		{"selectordinal tak didukung", Override{Namespace: "example", Locale: "id", Key: "a", Message: "{n, selectordinal, other {#}}"}, CodeInvalidSyntax},
		{"pesan kosong", Override{Namespace: "example", Locale: "id", Key: "a", Message: ""}, CodeInvalidSyntax},
		{"argumen berbeda (ekstra)", Override{Namespace: "example", Locale: "id", Key: "n.count", Message: "{n, plural, other {# x}} {baru}"}, CodeParamMismatch},
		{"argumen berbeda (kurang)", Override{Namespace: "example", Locale: "id", Key: "n.count", Message: "tanpa argumen"}, CodeParamMismatch},
		{"argumen ada pada pesan tanpa argumen", Override{Namespace: "example", Locale: "id", Key: "a", Message: "Halo {nama}"}, CodeParamMismatch},
		{"HTML", Override{Namespace: "example", Locale: "id", Key: "a", Message: "<script>alert(1)</script>"}, CodeHTMLNotAllowed},
		{"HTML bentuk lain", Override{Namespace: "example", Locale: "id", Key: "a", Message: "a < b"}, CodeHTMLNotAllowed},
		{"terlalu panjang", Override{Namespace: "example", Locale: "id", Key: "a", Message: long}, CodeTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changed, err := s.SetOverride(ctx, tenantA, c.o, "api:dev", nil)
			if changed || overrideErrCode(t, err) != c.code {
				t.Errorf("changed=%v err=%v, want code %s", changed, err, c.code)
			}
		})
	}
	// tidak ada yang tersimpan dari kasus gagal
	if got, o := resolve(s, tenantA, "example", "id", "a"); got != "A-id" || o != OriginBuiltin {
		t.Errorf("override ditolak tidak boleh berlaku: %q %v", got, o)
	}
	// yang sah: parameter sama (urutan dan teks lain bebas), tag rich sederhana
	for _, msg := range []string{"{n, plural, one {# catatan satu} other {# catatan lain}}", "{n, plural, other {Ada # buah}}"} {
		if !set(t, s, tenantA, "example", "id", "n.count", msg) && msg == "" {
			t.Error("harus tersimpan")
		}
	}
	// R-4: untuk override, jenis argumen harus SAMA PERSIS: arg dan plural TIDAK saling menggantikan
	_, err := s.SetOverride(ctx, tenantA, Override{Namespace: "example", Locale: "id", Key: "n.count", Message: "{n} catatan"}, "api:dev", nil)
	if overrideErrCode(t, err) != CodeParamMismatch {
		t.Errorf("plural -> arg harus param_mismatch: %v", err)
	}
}

// R-4: override {name} (argumen sederhana, tipe string|number) tidak boleh menjadi plural (angka): "Budi" menjadi NaN.
func TestOverrideStrictParamKinds(t *testing.T) {
	ex := exampleSource()
	ex.FS = mapFS(map[string]string{
		"id.json": `{"greet": "Halo {name}", "cnt": "{n, plural, other {# x}}", "g": "{s, select, a {A} other {O}}", "rich": "<b>{name}</b>"}`,
		"en.json": `{}`, "en-GB.json": `{}`,
	})
	s := newTestService(t, Options{Overrides: newFakeStore(), Now: newClock().Now, Sources: []Source{platformSource(), uiSource(), ex}})
	ctx := context.Background()
	reject := map[string][2]string{
		"arg -> plural":    {"greet", "{name, plural, other {# klien}}"},
		"arg -> select":    {"greet", "{name, select, a {A} other {O}}"},
		"plural -> arg":    {"cnt", "{n} item"},
		"select -> arg":    {"g", "{s}"},
		"plural -> select": {"cnt", "{n, select, a {A} other {O}}"},
		"arg -> tag":       {"rich", "<name>x</name>"},
	}
	for name, c := range reject {
		_, err := s.SetOverride(ctx, tenantA, Override{Namespace: "example", Locale: "id", Key: c[0], Message: c[1]}, "api:dev", nil)
		if overrideErrCode(t, err) != CodeParamMismatch {
			t.Errorf("%s: err = %v, want param_mismatch", name, err)
		}
	}
	accept := map[string][2]string{
		"arg -> arg":       {"greet", "Selamat datang, {name}"},
		"plural -> plural": {"cnt", "{n, plural, one {# satu} other {# banyak}}"},
		"select -> select": {"g", "{s, select, a {Aa} b {Bb} other {Oo}}"},
		"tag -> tag":       {"rich", "<b>Halo {name}</b>!"},
	}
	for name, c := range accept {
		if _, err := s.SetOverride(ctx, tenantA, Override{Namespace: "example", Locale: "id", Key: c[0], Message: c[1]}, "api:dev", nil); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// P10: override yang diterima selalu punya Params() sama dengan pesan bawaan bahasa default pada kunci itu.
func TestPropertyAcceptedOverrideKeepsParams(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	ctx := context.Background()
	base, _ := sdki18n.Compile("{n, plural, other {# catatan}}", sdki18n.DefaultLimits())
	candidates := []string{"x", "{n}", "{n} {n}", "{n, plural, one {a} other {b}}", "{m}", "{n}{m}", "<b>{n}</b>", "{n, select, a {x} other {y}}", "", "{"}
	for _, msg := range candidates {
		_, err := s.SetOverride(ctx, tenantA, Override{Namespace: "example", Locale: "id", Key: "n.count", Message: msg}, "api:dev", nil)
		if err != nil {
			continue
		}
		got, _ := resolve(s, tenantA, "example", "id", "n.count")
		m, cerr := sdki18n.Compile(got, sdki18n.DefaultLimits())
		if cerr != nil || !sdki18n.SameParams(base.Params(), m.Params()) {
			t.Errorf("override %q diterima tetapi Params berbeda", msg)
		}
	}
}

// P6 Isolasi tenant: override tenant A tidak pernah muncul di Resolve/Catalog tenant B, dan ETag B tidak
// bergantung pada data A.
func TestOverrideTenantIsolation(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	ctx := context.Background()
	vBefore, _ := s.Catalog(ctx, tenantB, "example", "id")
	set(t, s, tenantA, "example", "id", "a", "Milik A")
	if got, o := resolve(s, tenantB, "example", "id", "a"); got != "A-id" || o != OriginBuiltin {
		t.Errorf("Resolve B = %q %v", got, o)
	}
	vB, _ := s.Catalog(ctx, tenantB, "example", "id")
	if vB.Messages["a"] != "A-id" || vB.ETag != vBefore.ETag || vB.Revision != vBefore.Revision {
		t.Errorf("Catalog B berubah oleh A: %v etag %s -> %s", vB.Messages["a"], vBefore.ETag, vB.ETag)
	}
	vA, _ := s.Catalog(ctx, tenantA, "example", "id")
	if vA.Messages["a"] != "Milik A" || vA.ETag == vB.ETag {
		t.Errorf("Catalog A = %v", vA.Messages["a"])
	}
	if s.Revisions(ctx, tenantB)["example"] != vBefore.Revision {
		t.Error("revisi B berubah oleh A")
	}
	// B memasang kunci yang sama dengan nilai lain: terpisah
	set(t, s, tenantB, "example", "id", "a", "Milik B")
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "Milik A" {
		t.Errorf("A tertimpa B: %q", got)
	}
}

func TestOverrideRevisionChangesETagAndViewRevision(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	ctx := context.Background()
	v0, _ := s.Catalog(ctx, tenantA, "example", "id")
	set(t, s, tenantA, "example", "id", "a", "Baru")
	v1, _ := s.Catalog(ctx, tenantA, "example", "id")
	if v1.ETag == v0.ETag || v1.Revision == v0.Revision {
		t.Errorf("ETag/Revision harus berubah: %s/%s -> %s/%s", v0.ETag, v0.Revision, v1.ETag, v1.Revision)
	}
	if s.Revisions(ctx, tenantA)["example"] != v1.Revision {
		t.Error("Revisions harus sama dengan View.Revision")
	}
	// hapus override mengembalikan isi efektif awal (ETag kembali sama dengan isi semula)
	if _, err := s.DeleteOverride(ctx, tenantA, "example", "id", "a", "api:dev", nil); err != nil {
		t.Fatal(err)
	}
	v2, _ := s.Catalog(ctx, tenantA, "example", "id")
	if v2.ETag != v0.ETag {
		t.Errorf("isi kembali ke bawaan, ETag harus sama dengan semula: %s vs %s", v2.ETag, v0.ETag)
	}
	if v2.Revision == v0.Revision {
		t.Error("Revision namespace tetap berbeda (jejak perubahan), tidak kembali ke nilai awal")
	}
}

// Konsistensi antar-instance bersifat eventual (<= TTL): instance kedua melihat perubahan setelah TTL lewat,
// dan pada instance yang sama mutasi langsung berlaku.
func TestOverrideCacheFreshAfterMutationAndAfterTTL(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	a := newOverrideService(t, fs, clk)
	b := newOverrideService(t, fs, clk)
	if got, _ := resolve(b, tenantA, "example", "id", "a"); got != "A-id" {
		t.Fatalf("awal: %q", got)
	}
	set(t, a, tenantA, "example", "id", "a", "Dari instance A")
	if got, _ := resolve(a, tenantA, "example", "id", "a"); got != "Dari instance A" {
		t.Errorf("instance yang sama harus langsung segar: %q", got)
	}
	if got, _ := resolve(b, tenantA, "example", "id", "a"); got != "A-id" {
		t.Errorf("instance lain masih memakai cache dalam TTL: %q", got)
	}
	clk.Advance(DefaultOverrideTTL - time.Second)
	if got, _ := resolve(b, tenantA, "example", "id", "a"); got != "A-id" {
		t.Errorf("sebelum TTL berakhir cache masih berlaku: %q", got)
	}
	clk.Advance(2 * time.Second)
	if got, _ := resolve(b, tenantA, "example", "id", "a"); got != "Dari instance A" {
		t.Errorf("setelah TTL, instance lain harus segar: %q", got)
	}
}

func TestOverrideRevisionCheckIsThrottledByTTL(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	s := newOverrideService(t, fs, clk)
	for i := 0; i < 50; i++ {
		resolve(s, tenantA, "example", "id", "a")
	}
	if fs.revCalls != 1 {
		t.Errorf("query revisi = %d dalam satu jendela TTL, want 1", fs.revCalls)
	}
	clk.Advance(DefaultOverrideTTL + time.Second)
	resolve(s, tenantA, "example", "id", "a")
	if fs.revCalls != 2 {
		t.Errorf("query revisi setelah TTL = %d, want 2", fs.revCalls)
	}
	if fs.loadCalls != 0 {
		t.Errorf("tanpa override tidak ada muat isi: %d", fs.loadCalls)
	}
}

// DB override tidak terjangkau: bawaan tetap disajikan dengan peringatan; mutasi gagal.
func TestOverrideStoreDownServesBuiltin(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := newOverrideService(t, fs, clk, func(o *Options) { o.Log = log })
	set(t, s, tenantA, "example", "id", "a", "Override")
	clk.Advance(DefaultOverrideTTL + time.Second)
	fs.mu.Lock()
	fs.failAll = errors.New("koneksi putus")
	fs.mu.Unlock()
	if got, o := resolve(s, tenantA, "example", "id", "a"); got == "" || o == OriginMissing {
		t.Errorf("harus tetap menyajikan sesuatu: %q %v", got, o)
	}
	if v, err := s.Catalog(context.Background(), tenantA, "example", "id"); err != nil || v.Messages["b"] != "B-id" {
		t.Errorf("Catalog saat DB mati: %v %v", v.Messages, err)
	}
	if !strings.Contains(buf.String(), WarnOverrideStoreMessage) {
		t.Errorf("peringatan DB tak terjangkau tidak tercatat:\n%s", buf.String())
	}
	if _, err := s.SetOverride(context.Background(), tenantA, Override{Namespace: "example", Locale: "id", Key: "b", Message: "x"}, "api:dev", nil); err == nil {
		t.Error("mutasi saat DB mati harus gagal")
	}
	// DB mati dari awal (tanpa cache): bawaan murni
	fs2 := newFakeStore()
	fs2.failAll = errors.New("mati")
	s2 := newOverrideService(t, fs2, newClock(), func(o *Options) { o.Log = log })
	if got, o := resolve(s2, tenantA, "example", "id", "b"); got != "B-id" || o != OriginBuiltin {
		t.Errorf("bawaan murni: %q %v", got, o)
	}
}

func TestOverrideWithoutStoreIsUnavailable(t *testing.T) {
	s := newTestService(t, Options{})
	_, err := s.SetOverride(context.Background(), tenantA, Override{Namespace: "example", Locale: "id", Key: "a", Message: "x"}, "api:dev", nil)
	if !errors.Is(err, ErrOverridesUnavailable) {
		t.Errorf("err = %v, want ErrOverridesUnavailable", err)
	}
	if _, err := s.DeleteOverride(context.Background(), tenantA, "example", "id", "a", "api:dev", nil); !errors.Is(err, ErrOverridesUnavailable) {
		t.Errorf("Delete err = %v", err)
	}
	if got, o := resolve(s, tenantA, "example", "id", "a"); got != "A-id" || o != OriginBuiltin {
		t.Errorf("tanpa store tetap melayani bawaan: %q %v", got, o)
	}
}

// Audit: setiap perubahan override dicatat sebagai log terstruktur i18n_override_changed.
func TestOverrideChangeIsLogged(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	s := newOverrideService(t, newFakeStore(), newClock(), func(o *Options) { o.Log = log })
	set(t, s, tenantA, "example", "id", "a", "Rahasia isi pesan")
	if _, err := s.DeleteOverride(context.Background(), tenantA, "example", "id", "a", "cli:budi", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, w := range []string{"i18n_override_changed", "namespace=example", "locale=id", "key=a", "actor=api:dev", "actor=cli:budi", "action=set", "action=delete", "tenant=" + tenantA.String()} {
		if !strings.Contains(out, w) {
			t.Errorf("log tidak memuat %q:\n%s", w, out)
		}
	}
	if strings.Contains(out, "Rahasia isi pesan") {
		t.Error("isi pesan tidak boleh dicatat di log")
	}
	// perubahan yang tidak berefek tidak dicatat
	buf.Reset()
	set(t, s, tenantA, "example", "id", "b", "sama")
	set(t, s, tenantA, "example", "id", "b", "sama")
	if strings.Count(buf.String(), "i18n_override_changed") != 1 {
		t.Errorf("hanya perubahan yang berefek dicatat:\n%s", buf.String())
	}
}

func TestOverridePreconditionPassesThroughAndAborts(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	set(t, s, tenantA, "example", "id", "a", "satu")
	stale := errors.New("basi")
	_, err := s.SetOverride(context.Background(), tenantA, Override{Namespace: "example", Locale: "id", Key: "a", Message: "dua"}, "api:dev",
		func(cur *store.Override) error {
			if cur == nil || cur.Message != "satu" {
				t.Errorf("precondition melihat %+v", cur)
			}
			return stale
		})
	if !errors.Is(err, stale) {
		t.Errorf("err = %v", err)
	}
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "satu" {
		t.Errorf("precondition gagal tidak boleh mengubah: %q", got)
	}
	if _, err := s.DeleteOverride(context.Background(), tenantA, "example", "id", "a", "api:dev", func(*store.Override) error { return stale }); !errors.Is(err, stale) {
		t.Errorf("Delete err = %v", err)
	}
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "satu" {
		t.Errorf("Delete dibatalkan: %q", got)
	}
}

func TestGetAndListOverrides(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	set(t, s, tenantA, "example", "id", "a", "satu")
	set(t, s, tenantA, "ui", "id", "actions.save", "Simpan!")
	ctx := context.Background()
	got, err := s.GetOverride(ctx, tenantA, "example", "id", "a")
	if err != nil || got.Message != "satu" {
		t.Errorf("Get = %+v %v", got, err)
	}
	if _, err := s.GetOverride(ctx, tenantB, "example", "id", "a"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get lintas tenant err = %v", err)
	}
	list, total, err := s.ListOverrides(ctx, tenantA, store.Filter{Namespace: "example"}, 1, 10)
	if err != nil || total != 1 || len(list) != 1 {
		t.Errorf("List = %v %d %v", list, total, err)
	}
	if list, total, _ := s.ListOverrides(ctx, tenantB, store.Filter{}, 1, 10); total != 0 || len(list) != 0 {
		t.Errorf("List B = %v", list)
	}
}

// Override hanya untuk kunci yang ada di katalog bawaan BAHASA DEFAULT, bukan sembarang bahasa.
func TestOverrideKeyOnlyInNonDefaultLocaleIsUnknown(t *testing.T) {
	ex := exampleSource()
	ex.FS = mapFS(map[string]string{"id.json": `{"a": "A-id"}`, "en.json": `{"a": "A-en", "only_en": "E"}`, "en-GB.json": `{}`})
	s := newTestService(t, Options{Overrides: newFakeStore(), Now: newClock().Now, Sources: []Source{platformSource(), uiSource(), ex}})
	_, err := s.SetOverride(context.Background(), tenantA, Override{Namespace: "example", Locale: "en", Key: "only_en", Message: "x"}, "api:dev", nil)
	if overrideErrCode(t, err) != CodeUnknownKey {
		t.Errorf("err = %v", err)
	}
}

// R-8: View.Revision selalu sama dengan Revisions()[ns] (kontrak: bootstrap.i18n.revisions[ns]) walau override
// dipasang pada bahasa LAIN dari yang sedang dilayani dan View-nya sudah ter-cache.
func TestViewRevisionConsistentWithRevisionsAcrossLocales(t *testing.T) {
	s := newOverrideService(t, newFakeStore(), newClock())
	ctx := context.Background()
	v0, _ := s.Catalog(ctx, tenantA, "example", "id") // di-cache
	set(t, s, tenantA, "example", "en", "a", "Override en")
	v1, _ := s.Catalog(ctx, tenantA, "example", "id")
	want := s.Revisions(ctx, tenantA)["example"]
	if v1.Revision != want {
		t.Errorf("View.Revision = %s, Revisions = %s; harus sama", v1.Revision, want)
	}
	if v1.Revision == v0.Revision {
		t.Error("revisi namespace harus berubah walau override ada di bahasa lain")
	}
	// isi katalog id sendiri tidak berubah, jadi ETag-nya tetap
	if v1.ETag != v0.ETag {
		t.Errorf("ETag id berubah padahal isi id sama: %s -> %s", v0.ETag, v1.ETag)
	}
}

// R-7a: afterMutation monoton: mutasi yang selesai terbalik tidak menurunkan revisi di cache.
func TestAfterMutationIsMonotonic(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	s := newOverrideService(t, fs, clk)
	set(t, s, tenantA, "example", "id", "a", "satu")
	_ = s.overrideRevisions(context.Background(), tenantA) // muat cache
	s.afterMutation(tenantA, "example", "id", 9)
	s.afterMutation(tenantA, "example", "id", 5) // selesai terbalik
	if got := s.overrideRevisions(context.Background(), tenantA)[ovKey("example", "id")]; got != 9 {
		t.Errorf("revisi cache = %d, want 9 (tidak boleh turun)", got)
	}
}
