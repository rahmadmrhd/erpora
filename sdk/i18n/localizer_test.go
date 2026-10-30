package i18n

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// spyResolver memuat katalog tetap dan mencatat setiap permintaan.
type spyResolver struct {
	mu   sync.Mutex
	msgs map[string]string // "ns|loc|key" -> sumber
	reqs []string          // "ns|loc|key"
}

func (s *spyResolver) Resolve(_ context.Context, ns, key string, loc Locale) (string, bool) {
	k := ns + "|" + string(loc) + "|" + key
	s.mu.Lock()
	s.reqs = append(s.reqs, k)
	s.mu.Unlock()
	v, ok := s.msgs[k]
	return v, ok
}

func newCtx(r Resolver, scope string, loc Locale) context.Context {
	ctx := WithResolver(context.Background(), r)
	if scope != "" {
		ctx = WithScope(ctx, scope)
	}
	if loc != "" {
		ctx = WithLocale(ctx, loc)
	}
	return ctx
}

func TestContextAccessors(t *testing.T) {
	ctx := context.Background()
	if _, ok := LocaleFrom(ctx); ok {
		t.Error("LocaleFrom pada ctx kosong harus false")
	}
	if _, ok := ScopeFrom(ctx); ok {
		t.Error("ScopeFrom pada ctx kosong harus false")
	}
	ctx = WithScope(WithLocale(ctx, "en-GB"), "example")
	if l, ok := LocaleFrom(ctx); !ok || l != "en-GB" {
		t.Errorf("LocaleFrom = %q, %v", l, ok)
	}
	if s, ok := ScopeFrom(ctx); !ok || s != "example" {
		t.Errorf("ScopeFrom = %q, %v", s, ok)
	}
}

func TestTRelativeKeyUsesScopeAndLocale(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{
		"example|id|note.title": "Catatan",
		"example|en|note.title": "Note",
		"example|id|note.count": "{count, plural, =0 {Kosong} other {# catatan}}",
	}}
	if got := T(newCtx(r, "example", "id"), "note.title"); got != "Catatan" {
		t.Errorf("id: got %q", got)
	}
	if got := T(newCtx(r, "example", "en"), "note.title"); got != "Note" {
		t.Errorf("en: got %q", got)
	}
	if got := T(newCtx(r, "example", "id"), "note.count", Args{"count": 1500}); got != "1.500 catatan" {
		t.Errorf("plural: got %q", got)
	}
}

func TestTAbsoluteKeys(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{
		"ui|id|actions.save":    "Simpan",
		"platform|id|errors.nf": "Tidak ditemukan",
		"example|id|note.title": "Catatan",
	}}
	ctx := newCtx(r, "example", "id")
	for key, want := range map[string]string{
		"ui::actions.save":    "Simpan",
		"platform::errors.nf": "Tidak ditemukan",
		"example::note.title": "Catatan",
	} {
		if got := T(ctx, key); got != want {
			t.Errorf("T(%q) = %q, want %q", key, got, want)
		}
	}
	// ui:: dan platform:: juga boleh tanpa scope modul
	if got := T(newCtx(r, "", "id"), "ui::actions.save"); got != "Simpan" {
		t.Errorf("tanpa scope: got %q", got)
	}
}

func TestTDeniesOtherModuleNamespace(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{"secret|id|a.b": "RAHASIA"}}
	ctx := newCtx(r, "example", "id")
	got := T(ctx, "secret::a.b")
	if got != "secret::a.b" {
		t.Errorf("got %q, want kunci apa adanya", got)
	}
	for _, q := range r.reqs {
		if strings.HasPrefix(q, "secret|") {
			t.Errorf("Resolver tidak boleh ditanya namespace modul lain: %v", r.reqs)
		}
	}
	for _, key := range []string{"::a.b", "a::b::c", "Secret::a"} {
		if got := T(ctx, key); strings.Contains(got, "RAHASIA") {
			t.Errorf("T(%q) = %q", key, got)
		}
	}
}

func TestTFallbackToNamespaceKey(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{}}
	if got := T(newCtx(r, "example", "id"), "note.title"); got != "example::note.title" {
		t.Errorf("got %q", got)
	}
	if got := T(newCtx(r, "example", "id"), "ui::x.y"); got != "ui::x.y" {
		t.Errorf("got %q", got)
	}
	// tanpa scope, kunci relatif tidak dapat diselesaikan: dikembalikan apa adanya
	if got := T(newCtx(r, "", "id"), "note.title"); got != "note.title" {
		t.Errorf("tanpa scope: got %q", got)
	}
	if len(r.reqs) != 2 {
		t.Errorf("Resolver tidak boleh ditanya tanpa scope: %v", r.reqs)
	}
	// tanpa Resolver sama sekali
	if got := T(WithScope(context.Background(), "example"), "a.b"); got != "example::a.b" {
		t.Errorf("tanpa resolver: got %q", got)
	}
	if got := T(context.Background(), "a.b"); got != "a.b" {
		t.Errorf("ctx kosong: got %q", got)
	}
}

func TestTMissingArgAndMultipleArgs(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{"m|id|k": "Halo {nama}, ada {n} item"}}
	ctx := newCtx(r, "m", "id")
	if got := T(ctx, "k"); got != "Halo {nama}, ada {n} item" {
		t.Errorf("tanpa argumen: got %q", got)
	}
	if got := T(ctx, "k", Args{"nama": "Budi"}, Args{"n": 3, "nama": "Ani"}); got != "Halo Ani, ada 3 item" {
		t.Errorf("gabungan: got %q", got)
	}
}

func TestTNeverPanics(t *testing.T) {
	// Resolver yang panik
	if got := T(newCtx(panicResolver{}, "m", "id"), "k"); got != "m::k" {
		t.Errorf("resolver panik: got %q", got)
	}
	// sumber pesan rusak dan argumen bertipe tak didukung
	r := &spyResolver{msgs: map[string]string{"m|id|bad": "Halo {x", "m|id|ok": "{v}"}}
	ctx := newCtx(r, "m", "id")
	if got := T(ctx, "bad"); got != "m::bad" {
		t.Errorf("pesan rusak: got %q", got)
	}
	if got := T(ctx, "ok", Args{"v": []int{1}}); got != "m::ok" {
		t.Errorf("argumen tak didukung: got %q", got)
	}
}

type panicResolver struct{}

func (panicResolver) Resolve(context.Context, string, string, Locale) (string, bool) {
	panic("boom")
}

func TestLookupReportsFound(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{"m|id|k": "Halo {n}"}}
	ctx := newCtx(r, "m", "id")
	if s, ok := Lookup(ctx, "k", Args{"n": 1}); !ok || s != "Halo 1" {
		t.Errorf("Lookup = %q, %v", s, ok)
	}
	if s, ok := Lookup(ctx, "tidak.ada"); ok || s != "" {
		t.Errorf("Lookup hilang = %q, %v", s, ok)
	}
}

func TestTConcurrent(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{"m|id|k": "{n, plural, other {# x}}"}}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx := newCtx(r, "m", "id")
			if got := T(ctx, "k", Args{"n": 1000}); got != "1.000 x" {
				t.Errorf("got %q", got)
			}
		}(i)
	}
	wg.Wait()
}

func TestCompileCacheIsBounded(t *testing.T) {
	r := &spyResolver{msgs: map[string]string{}}
	for i := 0; i < cacheMax*2+5; i++ {
		src := strings.Repeat("a", 1) + string(rune('A'+i%26)) + strings.Repeat("b", i%50) + string(rune(0x4e00+i))
		r.msgs["m|id|k"] = src
		_ = T(newCtx(r, "m", "id"), "k")
	}
	compileCache.mu.Lock()
	n := len(compileCache.m)
	compileCache.mu.Unlock()
	if n > cacheMax {
		t.Errorf("cache = %d entri, batas %d", n, cacheMax)
	}
}

// slogCapture memasang slog.Default ke buffer selama test dan mengembalikan pembacanya.
func slogCapture(t *testing.T) func() string {
	t.Helper()
	var mu sync.Mutex
	var sb strings.Builder
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&lockedWriter{mu: &mu, sb: &sb}, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return func() string { mu.Lock(); defer mu.Unlock(); return sb.String() }
}

type lockedWriter struct {
	mu *sync.Mutex
	sb *strings.Builder
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sb.Write(p)
}

// R-1 (Requirement 3.3): kunci yang tidak ditemukan sampai ujung rantai dicatat sebagai peringatan SATU kali per
// kunci oleh T (bukan Lookup, yang memang opsional).
func TestTMissingKeyWarnsOnceButLookupDoesNot(t *testing.T) {
	logs := slogCapture(t)
	r := &spyResolver{msgs: map[string]string{}}
	ctx := newCtx(r, "r1mod", "id")
	for i := 0; i < 3; i++ {
		if got := T(ctx, "note.titel.r1"); got != "r1mod::note.titel.r1" {
			t.Errorf("got %q", got)
		}
	}
	if n := strings.Count(logs(), "r1mod::note.titel.r1"); n != 1 {
		t.Errorf("peringatan kunci hilang dicatat %d kali, want 1:\n%s", n, logs())
	}
	if _, ok := Lookup(ctx, "opsional.r1"); ok {
		t.Fatal("Lookup kunci hilang harus false")
	}
	if strings.Contains(logs(), "opsional.r1") {
		t.Errorf("Lookup tidak boleh memperingatkan:\n%s", logs())
	}
}

// fakeReporter memenuhi TestReporter dan merekam galat serta cleanup.
type fakeReporter struct {
	errs     []string
	cleanups []func()
}

func (f *fakeReporter) Helper() {}
func (f *fakeReporter) Errorf(format string, a ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, a...))
}
func (f *fakeReporter) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }
func (f *fakeReporter) finish() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

// R-1: StrictMissing menjadikan kunci hilang pada T sebagai galat test; Lookup dan kunci yang ada tidak.
func TestStrictMissingFailsTestOnMissingKey(t *testing.T) {
	_ = slogCapture(t)
	rep := &fakeReporter{}
	StrictMissing(rep)
	r := &spyResolver{msgs: map[string]string{"r1b|id|ada": "ada"}}
	ctx := newCtx(r, "r1b", "id")
	_ = T(ctx, "ada")
	_, _ = Lookup(ctx, "opsional")
	if len(rep.errs) != 0 {
		t.Fatalf("kunci ada/Lookup tidak boleh gagal: %v", rep.errs)
	}
	_ = T(ctx, "salah.ketik.strict")
	_ = T(ctx, "salah.ketik.strict") // setiap kemunculan dilaporkan ke test (tidak dideduplikasi)
	if len(rep.errs) != 2 || !strings.Contains(rep.errs[0], "r1b::salah.ketik.strict") {
		t.Errorf("errs = %v", rep.errs)
	}
	rep.finish() // cleanup melepas mode strict
	_ = T(ctx, "salah.setelah.cleanup")
	if len(rep.errs) != 2 {
		t.Errorf("setelah cleanup tidak ada pelaporan lagi: %v", rep.errs)
	}
}

// R-3 (Requirement 2.3): argumen hilang tetap tampil {nama} dan dicatat sebagai peringatan sekali per (kunci, nama).
func TestTMissingArgumentWarnsOnce(t *testing.T) {
	logs := slogCapture(t)
	r := &spyResolver{msgs: map[string]string{"r3m|id|count": "Ada {jumlah} dan {sisa} dan {jumlah}"}}
	ctx := newCtx(r, "r3m", "id")
	for i := 0; i < 3; i++ {
		if got := T(ctx, "count", Args{"sisa": 1}); got != "Ada {jumlah} dan 1 dan {jumlah}" {
			t.Errorf("got %q", got)
		}
	}
	out := logs()
	if strings.Count(out, "jumlah") != 1 || strings.Contains(out, "sisa") {
		t.Errorf("hanya argumen yang hilang (jumlah) dan sekali:\n%s", out)
	}
}

func TestFormatReportListsMissingArguments(t *testing.T) {
	m := mustCompile(t, "{a} {n, plural, one {# {b}} other {# {c}}} {g, select, x {X} other {O}} {a}")
	mr, ok := m.(MissingReporter)
	if !ok {
		t.Fatal("Message dari Compile harus MissingReporter")
	}
	out, missing, err := mr.FormatReport("en", Args{"n": 2})
	if err != nil || out != "{a} 2 {c} {g} {a}" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if strings.Join(missing, ",") != "a,c,g" {
		t.Errorf("missing = %v, want a,c,g (urut kemunculan, tanpa duplikat)", missing)
	}
	// hanya cabang yang terpilih yang dihitung; Format biasa tetap sama
	plain, _ := m.Format("en", Args{"n": 2})
	if plain != out {
		t.Errorf("Format = %q, FormatReport = %q", plain, out)
	}
}

// observerResolver adalah Resolver yang juga MissingObserver.
type observerResolver struct {
	*spyResolver
	missing []string
}

func (o *observerResolver) ObserveMissing(_ context.Context, ns, key string, loc Locale) {
	o.missing = append(o.missing, string(loc)+"|"+ns+"|"+key)
}

// F-7: T (bukan Lookup) memberi tahu Resolver yang mengimplementasikan MissingObserver bahwa kunci tidak ditemukan,
// sehingga platform dapat menghitung metrik tanpa mengimpor telemetri ke SDK.
func TestTNotifiesMissingObserverButLookupDoesNot(t *testing.T) {
	_ = slogCapture(t)
	r := &observerResolver{spyResolver: &spyResolver{msgs: map[string]string{"f7|id|ada": "ada"}}}
	ctx := newCtx(r, "f7", "id")
	_ = T(ctx, "ada")
	_, _ = Lookup(ctx, "opsional")
	if len(r.missing) != 0 {
		t.Fatalf("kunci ada atau Lookup tidak boleh dilaporkan: %v", r.missing)
	}
	_ = T(ctx, "salah.ketik.f7")
	_ = T(ctx, "salah.ketik.f7") // setiap pemanggilan dilaporkan ke observer (metrik menghitung kejadian)
	if len(r.missing) != 2 || r.missing[0] != "id|f7|salah.ketik.f7" {
		t.Errorf("missing = %v", r.missing)
	}
}

// F-12: setelah cleanup, StrictMissing tidak lagi memanggil reporter (menghindari t.Errorf dari goroutine sesudah
// test selesai); pemanggilan serentak aman.
func TestStrictMissingStopsReportingAfterCleanupConcurrently(t *testing.T) {
	_ = slogCapture(t)
	rep := &lockedReporter{}
	StrictMissing(rep)
	r := &spyResolver{msgs: map[string]string{}}
	ctx := newCtx(r, "f12", "id")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = T(ctx, "salah.f12")
			}
		}()
	}
	rep.finish() // test "selesai" sementara goroutine masih memanggil T
	wg.Wait()
	rep.mu.Lock()
	defer rep.mu.Unlock()
	if rep.afterFinish > 0 {
		t.Errorf("reporter dipanggil %d kali SETELAH test selesai", rep.afterFinish)
	}
}

// lockedReporter menandai panggilan Errorf yang terjadi setelah finish.
type lockedReporter struct {
	mu          sync.Mutex
	cleanups    []func()
	finished    bool
	afterFinish int
}

func (l *lockedReporter) Helper() {}
func (l *lockedReporter) Errorf(string, ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.finished {
		l.afterFinish++
	}
}
func (l *lockedReporter) Cleanup(fn func()) {
	l.mu.Lock()
	l.cleanups = append(l.cleanups, fn)
	l.mu.Unlock()
}
func (l *lockedReporter) finish() {
	l.mu.Lock()
	fns := append([]func(){}, l.cleanups...)
	l.mu.Unlock()
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
	l.mu.Lock()
	l.finished = true
	l.mu.Unlock()
}
