package i18n

import (
	"context"
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
