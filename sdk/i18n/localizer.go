package i18n

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

// Namespace tetap yang boleh dibaca modul mana pun selain namespace-nya sendiri (Requirement 1.2).
const (
	NamespaceUI       = "ui"
	NamespacePlatform = "platform"
)

// Resolver dipasang platform di context (WithResolver). Ia mencari sumber pesan ICU untuk satu kunci
// lengkap (tanpa awalan namespace) pada satu namespace dan bahasa, sudah memperhitungkan override dan
// fallback bahasa. Implementasi harus aman untuk banyak goroutine dan tidak boleh panik.
type Resolver interface {
	Resolve(ctx context.Context, ns, key string, loc Locale) (src string, ok bool)
}

type (
	localeKey   struct{}
	scopeKey    struct{}
	resolverKey struct{}
)

// WithLocale memasang bahasa aktif di context (dipanggil middleware platform).
func WithLocale(ctx context.Context, loc Locale) context.Context {
	return context.WithValue(ctx, localeKey{}, loc)
}

// LocaleFrom membaca bahasa aktif dari context.
func LocaleFrom(ctx context.Context) (Locale, bool) {
	l, ok := ctx.Value(localeKey{}).(Locale)
	return l, ok
}

// WithScope mengikat T ke namespace modul pemilik route, hook, atau command; diisi platform.
func WithScope(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, scopeKey{}, namespace)
}

// ScopeFrom membaca namespace scope dari context.
func ScopeFrom(ctx context.Context) (string, bool) {
	s, ok := ctx.Value(scopeKey{}).(string)
	return s, ok
}

// WithResolver memasang Resolver di context; dipanggil platform.
func WithResolver(ctx context.Context, r Resolver) context.Context {
	return context.WithValue(ctx, resolverKey{}, r)
}

// ResolverFrom membaca Resolver dari context.
func ResolverFrom(ctx context.Context) (Resolver, bool) {
	r, ok := ctx.Value(resolverKey{}).(Resolver)
	return r, ok && r != nil
}

// T menerjemahkan key untuk bahasa pada context. key relatif ("note.title") dibaca dari namespace
// scope modul; key absolut hanya boleh "ui::..." , "platform::...", atau namespace scope sendiri.
// Namespace modul lain ditolak: T mengembalikan key apa adanya dan mencatat peringatan. Bila pesan tidak
// ditemukan atau tidak dapat diformat, T mengembalikan "namespace::kunci" (Requirement 3.3). T aman
// dipakai banyak goroutine dan tidak pernah panik. Beberapa Args digabung; yang belakangan menang.
func T(ctx context.Context, key string, args ...Args) string {
	text, ok, shown, missingKey := translate(ctx, key, args)
	if missingKey {
		// Kunci tidak ada sampai ujung rantai fallback (Requirement 3.3): peringatan sekali per kunci, dan galat
		// test bila StrictMissing aktif. Lookup (opsional) sengaja tidak melaporkan.
		warnOnce("missing|"+shown, "i18n: kunci terjemahan tidak ditemukan", "key", shown)
		notifyMissing(ctx, key)
		if st := strict.Load(); st != nil {
			st.report(shown)
		}
	}
	if ok {
		return text
	}
	return shown
}

// TestReporter adalah bagian *testing.T yang dibutuhkan StrictMissing (tanpa mengimpor paket testing).
type TestReporter interface {
	Helper()
	Errorf(format string, args ...any)
	Cleanup(func())
}

// strictState membungkus reporter dengan penanda aktif di bawah mutex: sesudah cleanup test, tidak ada lagi
// pemanggilan Errorf (mis. dari goroutine server yang masih berjalan), yang akan panik bila test sudah selesai.
type strictState struct {
	mu     sync.Mutex
	active bool
	t      TestReporter
}

func (st *strictState) report(shown string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.active {
		st.t.Helper()
		st.t.Errorf("i18n: kunci terjemahan tidak ditemukan: %s", shown)
	}
}

func (st *strictState) stop() {
	st.mu.Lock()
	st.active = false
	st.mu.Unlock()
}

var strict atomic.Pointer[strictState]

// MissingObserver (opsional) diimplementasi Resolver yang ingin diberi tahu bahwa T tidak menemukan sebuah kunci
// (bukan Lookup, yang memang opsional); platform memakainya untuk metrik tanpa mengimpor telemetri ke SDK.
type MissingObserver interface {
	ObserveMissing(ctx context.Context, ns, key string, loc Locale)
}

// notifyMissing memberi tahu Resolver di ctx (bila MissingObserver) tentang kunci yang hilang.
func notifyMissing(ctx context.Context, key string) {
	res, ok := ResolverFrom(ctx)
	if !ok {
		return
	}
	mo, ok := res.(MissingObserver)
	if !ok {
		return
	}
	if ns, rel, allowed := splitKey(ctx, key); allowed {
		loc, _ := LocaleFrom(ctx)
		mo.ObserveMissing(ctx, ns, rel, loc)
	}
}

// StrictMissing menjadikan setiap kunci yang tidak ditemukan pada T (sampai ujung rantai fallback) sebagai galat
// test t selama t berjalan (Requirement 3.3: "galat di mode pengujian"). Dilepas otomatis lewat t.Cleanup. Mode ini
// global proses: pakai pada test yang tidak berjalan secara parallel dengan test lain yang sengaja memicu kunci hilang.
func StrictMissing(t TestReporter) {
	st := &strictState{t: t, active: true}
	prev := strict.Swap(st)
	t.Cleanup(func() {
		st.stop() // lebih dulu: panggilan serentak yang masih berjalan tidak lagi menyentuh t
		strict.Store(prev)
	})
}

// Lookup seperti T tetapi melaporkan apakah pesan ditemukan dan berhasil diformat; bila tidak,
// hasilnya ("", false). Dipakai platform untuk meminjam pesan opsional, mis. errors.<code>.
func Lookup(ctx context.Context, key string, args ...Args) (string, bool) {
	text, ok, _, _ := translate(ctx, key, args)
	if !ok {
		return "", false
	}
	return text, true
}

// translate mengembalikan teks terformat dan ok; bila tidak ok, shown berisi teks pengganti. missingKey true bila
// Resolver tidak menemukan kunci (bukan karena namespace ditolak, Resolver tidak ada, atau pesan rusak).
func translate(ctx context.Context, key string, args []Args) (text string, ok bool, shown string, missingKey bool) {
	shown = key
	defer func() {
		if r := recover(); r != nil {
			text, ok = "", false
			warnOnce("panic|"+key, "i18n: panic saat menerjemahkan kunci", "key", key, "panic_type", typeName(r))
		}
	}()
	ns, rel, allowed := splitKey(ctx, key)
	if !allowed {
		warnOnce("deny|"+key, "i18n: namespace tidak boleh dibaca dari scope ini", "key", key)
		return "", false, key, false
	}
	shown = ns + "::" + rel
	res, found := ResolverFrom(ctx)
	if !found {
		return "", false, shown, false
	}
	loc, _ := LocaleFrom(ctx)
	src, found := res.Resolve(ctx, ns, rel, loc)
	if !found {
		return "", false, shown, true
	}
	msg, err := cachedCompile(src)
	if err != nil {
		warnOnce("compile|"+shown, "i18n: pesan katalog tidak dapat dikompilasi", "key", shown)
		return "", false, shown, false
	}
	var out string
	var missing []string
	if mr, isReporter := msg.(MissingReporter); isReporter {
		out, missing, err = mr.FormatReport(loc, mergeArgs(args))
	} else {
		out, err = msg.Format(loc, mergeArgs(args))
	}
	if err != nil {
		warnOnce("format|"+shown, "i18n: pesan tidak dapat diformat", "key", shown, "error", err.Error())
		return "", false, shown, false
	}
	for _, name := range missing { // Requirement 2.3: argumen hilang tampil {nama} dan dicatat
		warnOnce("missingarg|"+shown+"|"+name, "i18n: argumen pesan hilang", "key", shown, "arg", name)
	}
	return out, true, shown, false
}

// splitKey memecah key menjadi namespace dan kunci relatif dan memeriksa bahwa namespace boleh dibaca
// dari scope pada ctx. Tanpa scope, key relatif tidak dapat diselesaikan (allowed=false).
func splitKey(ctx context.Context, key string) (ns, rel string, allowed bool) {
	scope, _ := ScopeFrom(ctx)
	i := strings.Index(key, "::")
	if i < 0 {
		return scope, key, scope != "" && key != ""
	}
	ns, rel = key[:i], key[i+2:]
	if ns == "" || rel == "" || strings.Contains(rel, "::") {
		return "", "", false
	}
	return ns, rel, ns == NamespaceUI || ns == NamespacePlatform || (scope != "" && ns == scope)
}

func mergeArgs(args []Args) Args {
	switch len(args) {
	case 0:
		return nil
	case 1:
		return args[0]
	}
	out := Args{}
	for _, a := range args {
		for k, v := range a {
			out[k] = v
		}
	}
	return out
}

// Cache pesan terkompilasi untuk T, dibatasi agar sumber pesan yang berubah-ubah (override) tidak
// membuat memori tumbuh tanpa batas; saat penuh seluruh isinya dibuang.
const cacheMax = 4096

type cacheEntry struct {
	msg Message
	err error
}

var compileCache = struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}{m: map[string]cacheEntry{}}

func cachedCompile(src string) (Message, error) {
	compileCache.mu.Lock()
	e, ok := compileCache.m[src]
	compileCache.mu.Unlock()
	if ok {
		return e.msg, e.err
	}
	msg, err := Compile(src, DefaultLimits())
	compileCache.mu.Lock()
	if len(compileCache.m) >= cacheMax {
		compileCache.m = map[string]cacheEntry{}
	}
	compileCache.m[src] = cacheEntry{msg: msg, err: err}
	compileCache.mu.Unlock()
	return msg, err
}

// warnOnce mencatat satu peringatan per id (dibatasi jumlahnya agar tidak menjadi kebocoran memori).
var (
	warned     sync.Map
	warnedSize atomic.Int64
)

const warnedMax = 1024

func warnOnce(id, msg string, attrs ...any) {
	if warnedSize.Load() >= warnedMax {
		return
	}
	if _, dup := warned.LoadOrStore(id, struct{}{}); dup {
		return
	}
	warnedSize.Add(1)
	slog.Warn(msg, attrs...)
}

func typeName(v any) string {
	switch v.(type) {
	case error:
		return "error"
	case string:
		return "string"
	}
	return "other"
}
