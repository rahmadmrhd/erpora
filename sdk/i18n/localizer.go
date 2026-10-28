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
	text, ok, shown := translate(ctx, key, args)
	if ok {
		return text
	}
	return shown
}

// Lookup seperti T tetapi melaporkan apakah pesan ditemukan dan berhasil diformat; bila tidak,
// hasilnya ("", false). Dipakai platform untuk meminjam pesan opsional, mis. errors.<code>.
func Lookup(ctx context.Context, key string, args ...Args) (string, bool) {
	text, ok, _ := translate(ctx, key, args)
	if !ok {
		return "", false
	}
	return text, true
}

// translate mengembalikan teks terformat dan ok; bila tidak ok, shown berisi teks pengganti.
func translate(ctx context.Context, key string, args []Args) (text string, ok bool, shown string) {
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
		return "", false, key
	}
	shown = ns + "::" + rel
	res, found := ResolverFrom(ctx)
	if !found {
		return "", false, shown
	}
	loc, _ := LocaleFrom(ctx)
	src, found := res.Resolve(ctx, ns, rel, loc)
	if !found {
		return "", false, shown
	}
	msg, err := cachedCompile(src)
	if err != nil {
		warnOnce("compile|"+shown, "i18n: pesan katalog tidak dapat dikompilasi", "key", shown)
		return "", false, shown
	}
	out, err := msg.Format(loc, mergeArgs(args))
	if err != nil {
		warnOnce("format|"+shown, "i18n: pesan tidak dapat diformat", "key", shown, "error", err.Error())
		return "", false, shown
	}
	return out, true, shown
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
