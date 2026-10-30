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
	// gate (opsional): bila tidak nil, Revisions/ListLocale menunggu sampai ditutup (meniru DB lambat/menggantung).
	revGate  chan struct{}
	loadGate chan struct{}
	// revEntered (opsional, buffer >= 1): diberi sinyal saat Revisions sudah membaca isinya dan menunggu revGate;
	// sinkronisasi deterministik pengganti Sleep.
	revEntered chan struct{}
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[tenancy.TenantID]map[[3]string]store.Override{}, revs: map[tenancy.TenantID]map[[2]string]int64{}}
}

func (f *fakeStore) Revisions(ctx context.Context, t tenancy.TenantID) ([]store.Revision, error) {
	f.mu.Lock()
	f.revCalls++
	gate := f.revGate
	// Isi dibaca SEBELUM menunggu gate: meniru query yang membaca keadaan lama lalu selesai belakangan.
	var out []store.Revision
	for k, v := range f.revs[t] {
		out = append(out, store.Revision{Namespace: k[0], Locale: k[1], Revision: v})
	}
	failErr := f.failAll
	entered := f.revEntered
	f.mu.Unlock()
	if gate != nil {
		if entered != nil {
			select {
			case entered <- struct{}{}:
			default:
			}
		}
		select {
		case <-gate:
		case <-ctx.Done(): // driver nyata berhenti menunggu saat context habis
		}
	}
	if err := ctx.Err(); err != nil { // store yang menghormati context (seperti driver nyata)
		return nil, err
	}
	if failErr != nil {
		return nil, failErr
	}
	return out, nil
}

func (f *fakeStore) ListLocale(ctx context.Context, t tenancy.TenantID, ns, loc string) ([]store.Override, error) {
	f.mu.Lock()
	f.loadCalls++
	gate := f.loadGate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
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

// R-6: DB override lambat tidak boleh mengantrekan request lain di belakang satu kunci per tenant: selama
// pemeriksaan revisi berlangsung, pemanggil lain memakai cache lama dan langsung kembali.
func TestOverrideSlowRevisionRefreshDoesNotBlockOthers(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	s := newOverrideService(t, fs, clk)
	set(t, s, tenantA, "example", "id", "a", "Override lama")
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "Override lama" {
		t.Fatalf("awal: %q", got)
	}
	clk.Advance(DefaultOverrideTTL + time.Second)
	gate := make(chan struct{})
	fs.mu.Lock()
	fs.revGate = gate
	fs.mu.Unlock()
	started := make(chan struct{})
	done := make(chan string, 1)
	go func() { // pemuat: menunggu DB lambat
		close(started)
		got, _ := resolve(s, tenantA, "example", "id", "a")
		done <- got
	}()
	<-started
	time.Sleep(50 * time.Millisecond) // beri waktu pemuat masuk ke Revisions
	for i := 0; i < 20; i++ {
		finished := make(chan string, 1)
		go func() { got, _ := resolve(s, tenantA, "example", "id", "a"); finished <- got }()
		select {
		case got := <-finished:
			if got != "Override lama" {
				t.Fatalf("pemanggil lain harus memakai cache lama: %q", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("pemanggil lain terblokir menunggu DB lambat (mutex ditahan selama query)")
		}
	}
	// katalog dan revisi juga tidak terblokir
	if _, err := s.Catalog(context.Background(), tenantA, "example", "id"); err != nil {
		t.Fatal(err)
	}
	close(gate)
	select {
	case got := <-done:
		if got != "Override lama" {
			t.Errorf("pemuat = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pemuat tidak selesai setelah DB pulih")
	}
}

// R-6: saat satu pemuat isi override berlangsung (DB lambat), pemanggil lain menunggu paling lama overrideWait lalu
// menyajikan yang ada (bawaan) alih-alih mengantre tanpa batas.
func TestOverrideSlowContentLoadBoundedWait(t *testing.T) {
	old := overrideWait
	overrideWait = 80 * time.Millisecond
	t.Cleanup(func() { overrideWait = old })
	fs, clk := newFakeStore(), newClock()
	s := newOverrideService(t, fs, clk)
	fs.mu.Lock()
	fs.rows[tenantA] = map[[3]string]store.Override{{"example", "id", "a"}: {TenantID: tenantA, Namespace: "example", Locale: "id", Key: "a", Message: "Dari DB"}}
	fs.revs[tenantA] = map[[2]string]int64{{"example", "id"}: 1}
	gate := make(chan struct{})
	fs.loadGate = gate
	fs.mu.Unlock()
	loader := make(chan string, 1)
	go func() { got, _ := resolve(s, tenantA, "example", "id", "a"); loader <- got }()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	got, _ := resolve(s, tenantA, "example", "id", "a")
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("menunggu %v, harus dibatasi overrideWait", d)
	}
	if got != "A-id" {
		t.Errorf("selama pemuatan lambat sajikan bawaan: %q", got)
	}
	close(gate)
	if v := <-loader; v != "Dari DB" {
		t.Errorf("pemuat = %q", v)
	}
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "Dari DB" {
		t.Errorf("setelah dimuat: %q", got)
	}
}

// R-6: bila query gagal, pengulangan mengikuti backoff (negative cache 30 detik), bukan tiap panggilan atau tiap TTL.
func TestOverrideStoreFailureBackoff(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	fs.failAll = errors.New("db mati")
	s := newOverrideService(t, fs, clk)
	for i := 0; i < 100; i++ {
		resolve(s, tenantA, "example", "id", "a")
		clk.Advance(100 * time.Millisecond)
	}
	if fs.revCalls != 1 {
		t.Errorf("query revisi gagal = %d dalam jendela backoff, want 1", fs.revCalls)
	}
	clk.Advance(overrideBackoff)
	resolve(s, tenantA, "example", "id", "a")
	if fs.revCalls != 2 {
		t.Errorf("setelah backoff = %d, want 2", fs.revCalls)
	}
	// kegagalan muat isi: tidak diulang tiap panggilan
	fs2, clk2 := newFakeStore(), newClock()
	fs2.revs[tenantA] = map[[2]string]int64{{"example", "id"}: 3}
	failing := &loadFailStore{fakeStore: fs2}
	s3 := newTestService(t, Options{Overrides: failing, Now: clk2.Now})
	for i := 0; i < 50; i++ {
		resolve(s3, tenantA, "example", "id", "a")
	}
	if failing.loads != 1 {
		t.Errorf("muat isi gagal = %d kali, want 1 (backoff)", failing.loads)
	}
}

// loadFailStore menggagalkan ListLocale dan menghitung percobaannya.
type loadFailStore struct {
	*fakeStore
	loads int
}

func (l *loadFailStore) ListLocale(context.Context, tenancy.TenantID, string, string) ([]store.Override, error) {
	l.loads++
	return nil, errors.New("muat gagal")
}

// F-1: request yang dibatalkan klien tidak boleh memicu backoff untuk semua pengguna (endpoint katalog/bootstrap
// publik): query bersama berjalan di luar pembatalan pemanggil, dan context.Canceled milik pemanggil bukan kegagalan DB.
func TestOverrideCanceledRequestDoesNotTriggerBackoff(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	fs, clk := newFakeStore(), newClock()
	s := newOverrideService(t, fs, clk, func(o *Options) { o.Log = log })
	set(t, s, tenantA, "example", "id", "a", "Lama")
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "Lama" {
		t.Fatalf("awal: %q", got)
	}
	// perubahan dari instance lain (langsung di store)
	fs.mu.Lock()
	fs.rows[tenantA][[3]string{"example", "id", "a"}] = store.Override{TenantID: tenantA, Namespace: "example", Locale: "id", Key: "a", Message: "Baru"}
	fs.revs[tenantA][[2]string{"example", "id"}]++
	fs.mu.Unlock()
	clk.Advance(DefaultOverrideTTL + time.Second)

	canceled, cancel := context.WithCancel(context.Background())
	cancel() // klien sudah memutus koneksi
	s.Resolve(canceled, tenantA, "example", "a", "id")
	if strings.Contains(buf.String(), WarnOverrideStoreMessage) {
		t.Errorf("pembatalan request dicatat sebagai DB tak terjangkau:\n%s", buf.String())
	}
	// pemanggil lain (request normal) tetap segar: tidak ada backoff 30 detik
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "Baru" {
		t.Errorf("setelah request dibatalkan, pemanggil lain mendapat %q, want Baru (tanpa backoff)", got)
	}
}

// F-1: pembatalan context pemanggil di TENGAH query tidak memicu backoff (query berjalan di luar pembatalan pemanggil).
// Kegagalan nyata store yang tetap memicu backoff diuji di TestOverrideStoreFailureBackoff dan test timeout DB (G-1).
func TestOverrideCallerCancelMidQueryDoesNotBackOff(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	gate := make(chan struct{})
	fs.revGate = gate
	fs.revEntered = make(chan struct{}, 1)
	s := newOverrideService(t, fs, clk)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Resolve(ctx, tenantA, "example", "a", "id"); close(done) }()
	<-fs.revEntered // query sudah berjalan dan menunggu gate
	cancel()        // pemanggil dibatalkan saat query berlangsung
	close(gate)
	<-done
	if n := fs.revCalls; n != 1 {
		t.Fatalf("revCalls = %d", n)
	}
	fs.mu.Lock()
	fs.revGate = nil
	fs.mu.Unlock()
	resolve(s, tenantA, "example", "id", "a") // masih jendela TTL (sukses) -> tidak query lagi; lalu majukan jam
	clk.Advance(DefaultOverrideTTL + time.Second)
	resolve(s, tenantA, "example", "id", "a")
	if fs.revCalls != 2 {
		t.Errorf("revCalls = %d, want 2 (tanpa backoff setelah pembatalan)", fs.revCalls)
	}
}

// F-2: hasil refresh yang dimulai SEBELUM mutasi tidak boleh menimpa revisi yang lebih baru dari afterMutation:
// revisi digabung dengan max per kunci (monoton di semua jalur).
func TestOverrideStaleRefreshDoesNotRegressRevision(t *testing.T) {
	fs, clk := newFakeStore(), newClock()
	s := newOverrideService(t, fs, clk)
	set(t, s, tenantA, "example", "id", "a", "Satu") // rev 1
	resolve(s, tenantA, "example", "id", "a")        // muat cache
	clk.Advance(DefaultOverrideTTL + time.Second)
	gate := make(chan struct{})
	fs.mu.Lock()
	fs.revGate = gate
	fs.revEntered = make(chan struct{}, 1)
	fs.mu.Unlock()
	refreshing := make(chan struct{})
	go func() { resolve(s, tenantA, "example", "id", "a"); close(refreshing) }() // refresh membaca rev 1, lalu menunggu
	<-fs.revEntered                                                              // pasti sudah membaca rev 1 dan menunggu gate (deterministik, tanpa Sleep)
	fs.mu.Lock()
	fs.revGate = nil
	fs.mu.Unlock()
	set(t, s, tenantA, "example", "id", "a", "Dua") // rev 2; afterMutation mengisi revisi 2
	close(gate)                                     // refresh lama selesai dengan hasil basi (rev 1)
	<-refreshing
	if got := s.overrideRevisions(context.Background(), tenantA)[ovKey("example", "id")]; got != 2 {
		t.Errorf("revisi cache = %d, want 2 (hasil refresh basi tidak boleh menurunkan)", got)
	}
	if got, _ := resolve(s, tenantA, "example", "id", "a"); got != "Dua" {
		t.Errorf("Resolve = %q, want Dua", got)
	}
	v, _ := s.Catalog(context.Background(), tenantA, "example", "id")
	if v.Messages["a"] != "Dua" {
		t.Errorf("Catalog = %q, want Dua", v.Messages["a"])
	}
}

// G-1: timeout DB nyata (query melewati overrideQueryTimeout) tetap memicu backoff, peringatan, dan metrik walau context
// PEMANGGIL sudah berdeadline lewat: galat query berasal dari timeout query sendiri, bukan pembatalan pemanggil.
func TestOverrideRealDBTimeoutBacksOffEvenWhenCallerHasExpiredDeadline(t *testing.T) {
	old := overrideQueryTimeout
	overrideQueryTimeout = 30 * time.Millisecond
	t.Cleanup(func() { overrideQueryTimeout = old })
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	fs, clk := newFakeStore(), newClock()
	fs.revGate = make(chan struct{}) // DB macet: tidak pernah menjawab
	s := newOverrideService(t, fs, clk, func(o *Options) { o.Log = log })
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	s.Resolve(expired, tenantA, "example", "a", "id")
	if !strings.Contains(buf.String(), WarnOverrideStoreMessage) {
		t.Errorf("timeout DB nyata tidak dicatat:\n%s", buf.String())
	}
	before := fs.revCalls
	for i := 0; i < 20; i++ { // dalam jendela backoff: tidak query lagi
		s.Resolve(expired, tenantA, "example", "a", "id")
		clk.Advance(100 * time.Millisecond)
	}
	if fs.revCalls != before {
		t.Errorf("revCalls = %d, want %d (backoff setelah timeout DB nyata)", fs.revCalls, before)
	}
}

// G-1 (varian muat isi): timeout DB nyata pada ListLocale dengan pemanggil berdeadline lewat tetap backoff.
func TestOverrideRealContentLoadTimeoutBacksOffEvenWhenCallerHasExpiredDeadline(t *testing.T) {
	old := overrideQueryTimeout
	overrideQueryTimeout = 30 * time.Millisecond
	t.Cleanup(func() { overrideQueryTimeout = old })
	fs, clk := newFakeStore(), newClock()
	fs.revs[tenantA] = map[[2]string]int64{{"example", "id"}: 3}
	fs.loadGate = make(chan struct{})
	s := newOverrideService(t, fs, clk)
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	// revisi dimuat dengan context normal agar sampai ke jalur muat isi
	s.overrideRevisions(context.Background(), tenantA)
	s.Resolve(expired, tenantA, "example", "a", "id")
	before := fs.loadCalls
	for i := 0; i < 20; i++ {
		s.Resolve(expired, tenantA, "example", "a", "id")
	}
	if fs.loadCalls != before {
		t.Errorf("loadCalls = %d, want %d (backoff setelah timeout muat isi)", fs.loadCalls, before)
	}
}
