package i18n

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"erpora/internal/platform/tenancy"
	sdki18n "erpora/sdk/i18n"
)

// Namespace tetap platform; namespace modul = slug modul.
const (
	NamespaceUI       = sdki18n.NamespaceUI
	NamespacePlatform = sdki18n.NamespacePlatform
)

// WarnFallbackMessage dan WarnParityMessage adalah pesan log peringatan Service (dipakai test).
const (
	WarnFallbackMessage = "i18n: pesan jatuh ke bahasa fallback"
	WarnParityMessage   = "i18n: katalog tidak sejajar dengan bahasa default"
)

var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ErrNotFound berarti bahasa atau namespace tidak dikenal, atau modul pemilik namespace tidak aktif;
// penyebabnya sengaja tidak dibedakan (Requirement 5.2, 10.2).
var ErrNotFound = errors.New("i18n: bahasa atau namespace tidak ditemukan")

// Origin menyatakan dari mana sebuah pesan berasal.
type Origin int

// Nilai Origin.
const (
	// OriginMissing: pesan tidak ada di rantai fallback; Resolve mengembalikan "namespace::kunci".
	OriginMissing Origin = iota
	// OriginBuiltin: katalog bawaan pada bahasa yang diminta persis.
	OriginBuiltin
	// OriginFallback: bahasa fallback (registri atau default), bawaan maupun override.
	OriginFallback
	// OriginOverride: override tenant pada bahasa yang diminta persis.
	OriginOverride
)

// Source adalah satu katalog namespace yang dibawa ke Service.
type Source struct {
	// Namespace: "platform", "ui", atau slug modul.
	Namespace string
	// Module true bagi namespace modul: tersedia hanya selama Options.Active melaporkan modulnya aktif.
	// ui dan platform selalu aktif.
	Module bool
	// FS berisi berkas "<locale>.json" di akar.
	FS fs.FS
	// Locales adalah bahasa yang dideklarasikan (setiap berkasnya wajib ada). nil = semua bahasa
	// registri yang berkasnya ada (ui dan platform).
	Locales []string
	// Default adalah bahasa dasar kunci namespace ini (berkasnya wajib ada); kosong = bahasa default registri.
	Default string
}

// Options mengonfigurasi New.
type Options struct {
	Registry *Registry
	Sources  []Source
	// Active melaporkan apakah modul ber-namespace ns aktif untuk tenant t; nil = semua aktif.
	Active func(t tenancy.TenantID, ns string) bool
	// Dev true mencatat peringatan fallback sekali per (bahasa, namespace, kunci) (Requirement 3.7).
	Dev bool
	// Log default slog.Default().
	Log *slog.Logger
	// Limits default sdki18n.DefaultLimits().
	Limits sdki18n.Limits
	// Overrides (opsional) menyimpan override istilah per tenant; nil = tanpa override (Set/Delete menolak).
	Overrides OverrideStore
	// OverrideTTL adalah selang pemeriksaan revisi override ke DB; default DefaultOverrideTTL.
	OverrideTTL time.Duration
	// Now adalah jam; default time.Now (untuk test).
	Now func() time.Time
}

// CatalogError adalah galat struktural katalog saat New; Service tidak dibuat (Requirement 8.1).
type CatalogError struct{ Issues []sdki18n.Issue }

// Error menulis satu baris per Issue dengan berkas dan kunci.
func (e *CatalogError) Error() string {
	var sb strings.Builder
	sb.WriteString("i18n: katalog tidak valid:")
	for _, is := range e.Issues {
		fmt.Fprintf(&sb, "\n  %s [%s]", is.File, is.Code)
		if is.Key != "" {
			fmt.Fprintf(&sb, " %q", is.Key)
		}
		if is.Detail != "" {
			sb.WriteString(": " + is.Detail)
		}
	}
	return sb.String()
}

// View adalah katalog efektif satu namespace dan bahasa dengan fallback sudah digabung. Isinya read-only
// dan dibagikan antar pemanggil: jangan diubah.
type View struct {
	Locale    string
	Namespace string
	// Revision adalah revisi namespace (sama untuk semua bahasa; dipakai bootstrap).
	Revision string
	// ETag berupa W/"<sha256 16 byte>" atas isi Messages, identik bila isi sama.
	ETag     string
	Messages map[string]string
}

type nsData struct {
	name     string
	module   bool
	def      string
	flats    map[string]sdki18n.Flat // bahasa -> kunci lengkap -> pesan
	revision string
}

type snapshot struct {
	reg *Registry
	ns  map[string]*nsData
}

// Service menyajikan katalog bawaan (dan kelak override) dengan rantai fallback. Snapshot katalog
// immutable dan ditukar atomik; aman dipakai banyak goroutine.
type Service struct {
	snap   atomic.Pointer[snapshot]
	active func(tenancy.TenantID, string) bool
	dev    bool
	log    *slog.Logger

	viewsMu sync.Mutex
	views   map[string]*View // "tenant\x00ns\x00locale\x00tanda-revisi" -> View (dibatasi)

	ovs           OverrideStore
	ttl           time.Duration
	now           func() time.Time
	ovMu          sync.Mutex
	ov            map[tenancy.TenantID]*tenantOv
	lastWarnStore atomic.Int64

	warned     sync.Map
	warnedSize atomic.Int64
}

// New membangun Service: membaca dan memvalidasi semua katalog dengan sdki18n.ParseCatalog (definisi
// "valid" yang sama dengan i18n:check). Galat struktural mengembalikan *CatalogError (startup ditolak);
// ketidaksejajaran antar bahasa hanya dicatat sebagai peringatan.
func New(opts Options) (*Service, error) {
	if opts.Registry == nil {
		return nil, errors.New("i18n: Registry wajib diisi")
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	lim := opts.Limits
	snap := &snapshot{reg: opts.Registry, ns: map[string]*nsData{}}
	var issues []sdki18n.Issue
	var problems []string
	for _, src := range opts.Sources {
		nd, srcIssues, err := loadSource(opts.Registry, src, lim)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if _, dup := snap.ns[src.Namespace]; dup {
			problems = append(problems, fmt.Sprintf("namespace %q ganda", src.Namespace))
			continue
		}
		issues = append(issues, srcIssues...)
		if len(srcIssues) == 0 {
			snap.ns[src.Namespace] = nd
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("i18n: konfigurasi katalog tidak valid: %s", strings.Join(problems, "; "))
	}
	if len(issues) > 0 {
		return nil, &CatalogError{Issues: issues}
	}
	for _, name := range sortedKeys(snap.ns) {
		warnParity(log, opts.Registry, snap.ns[name])
	}
	s := &Service{active: opts.Active, dev: opts.Dev, log: log, views: map[string]*View{},
		ovs: opts.Overrides, ttl: opts.OverrideTTL, now: opts.Now, ov: map[tenancy.TenantID]*tenantOv{}}
	if s.ttl <= 0 {
		s.ttl = DefaultOverrideTTL
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.snap.Store(snap)
	return s, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// loadSource membaca semua bahasa satu namespace. err = kesalahan konfigurasi (bukan isi katalog);
// issues = galat struktural isi katalog.
func loadSource(reg *Registry, src Source, lim sdki18n.Limits) (*nsData, []sdki18n.Issue, error) {
	if !namespacePattern.MatchString(src.Namespace) || len(src.Namespace) > 50 {
		return nil, nil, fmt.Errorf("namespace %q tidak valid", src.Namespace)
	}
	if src.FS == nil {
		return nil, nil, fmt.Errorf("namespace %q: FS katalog nil", src.Namespace)
	}
	def := src.Default
	if def == "" {
		def = reg.Default
	}
	locales := src.Locales
	if locales == nil {
		for _, code := range reg.Codes() {
			if _, err := fs.Stat(src.FS, code+".json"); err == nil || code == def {
				locales = append(locales, code)
			}
		}
	}
	for _, l := range locales {
		if !reg.Has(l) {
			return nil, nil, fmt.Errorf("namespace %q: bahasa %q tidak terdaftar di registri bahasa", src.Namespace, l)
		}
	}
	if !slices.Contains(locales, def) {
		return nil, nil, fmt.Errorf("namespace %q: bahasa default %q tidak ada di daftar bahasa", src.Namespace, def)
	}
	nd := &nsData{name: src.Namespace, module: src.Module, def: def, flats: map[string]sdki18n.Flat{}}
	var issues []sdki18n.Issue
	for _, l := range locales {
		file := src.Namespace + "/" + l + ".json"
		f, err := src.FS.Open(l + ".json")
		if err != nil {
			return nil, nil, fmt.Errorf("namespace %q: berkas katalog %s tidak ditemukan", src.Namespace, file)
		}
		flat, is := sdki18n.ParseCatalog(f, lim)
		_ = f.Close()
		for _, x := range is {
			x.File, x.Locale = file, l
			issues = append(issues, x)
		}
		if len(is) == 0 {
			nd.flats[l] = flat
		}
	}
	if len(issues) == 0 {
		nd.revision = revisionOf(nd)
	}
	return nd, issues, nil
}

// canonicalHash menghitung hash isi peta pesan yang tidak bergantung urutan.
func canonicalHash(m map[string]string) [sha256.Size]byte {
	h := sha256.New()
	for _, k := range sortedKeys(m) {
		fmt.Fprintf(h, "%d:%s%d:%s", len(k), k, len(m[k]), m[k])
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func revisionOf(nd *nsData) string {
	h := sha256.New()
	for _, l := range sortedKeys(nd.flats) {
		sum := canonicalHash(nd.flats[l])
		fmt.Fprintf(h, "%s=%x;", l, sum)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// warnParity mencatat ketidaksejajaran bahasa terhadap bahasa dasar namespace: kunci hilang (kecuali
// bahasa partial), kunci berlebih, dan argumen tidak cocok. Hanya peringatan (Requirement 8.1).
func warnParity(log *slog.Logger, reg *Registry, nd *nsData) {
	base := nd.flats[nd.def]
	const maxShown = 5
	for _, l := range sortedKeys(nd.flats) {
		if l == nd.def {
			continue
		}
		info, _ := reg.Info(l)
		flat := nd.flats[l]
		var missing, extra, mismatch []string
		for _, k := range sortedKeys(base) {
			other, ok := flat[k]
			if !ok {
				if !info.Partial {
					missing = append(missing, k)
				}
				continue
			}
			a, errA := sdki18n.Compile(base[k], sdki18n.DefaultLimits())
			b, errB := sdki18n.Compile(other, sdki18n.DefaultLimits())
			if errA == nil && errB == nil && !sdki18n.SameParams(a.Params(), b.Params()) {
				mismatch = append(mismatch, k)
			}
		}
		for _, k := range sortedKeys(flat) {
			if _, ok := base[k]; !ok {
				extra = append(extra, k)
			}
		}
		if len(missing)+len(extra)+len(mismatch) == 0 {
			continue
		}
		attrs := []any{"namespace", nd.name, "locale", l, "default_locale", nd.def}
		for _, g := range []struct {
			name string
			keys []string
		}{{"missing", missing}, {"extra", extra}, {"param_mismatch", mismatch}} {
			if len(g.keys) > 0 {
				attrs = append(attrs, g.name, trim(g.keys, maxShown), g.name+"_count", len(g.keys))
			}
		}
		log.Warn(WarnParityMessage, attrs...)
	}
}

func trim(keys []string, n int) []string {
	if len(keys) > n {
		return keys[:n]
	}
	return keys
}

// Registry mengembalikan registri bahasa yang dipakai Service.
func (s *Service) Registry() *Registry { return s.snap.Load().reg }

func (s *Service) isActive(t tenancy.TenantID, nd *nsData) bool {
	return !nd.module || s.active == nil || s.active(t, nd.name)
}

// chainFor menghasilkan urutan pencarian bahasa untuk satu namespace: rantai registri, ditambah bahasa dasar
// namespace bila belum termasuk (modul dapat berbahasa dasar selain default registri).
func (snap *snapshot) chainFor(nd *nsData, loc string) []string {
	if loc == "" {
		loc = snap.reg.Default
	}
	chain := snap.reg.Chain(loc)
	if !slices.Contains(chain, nd.def) {
		chain = append(chain, nd.def)
	}
	return chain
}

// Resolve mencari sumber pesan untuk (ns, key) pada bahasa loc: bahasa persis, fallback registri, lalu
// bahasa default (Requirement 3.2); bila tidak ada, mengembalikan "ns::key" dengan OriginMissing.
// Namespace modul nonaktif diperlakukan seperti tidak ada. Tidak pernah panik.
func (s *Service) Resolve(ctx context.Context, t tenancy.TenantID, ns, key string, loc sdki18n.Locale) (string, Origin) {
	snap := s.snap.Load()
	missing := ns + "::" + key
	nd := snap.ns[ns]
	if nd == nil || !s.isActive(t, nd) {
		return missing, OriginMissing
	}
	want := string(loc)
	if want == "" {
		want = snap.reg.Default
	}
	for _, l := range snap.chainFor(nd, want) {
		// Pada setiap bahasa dalam rantai: override dahulu, lalu katalog bawaan (Requirement 3.2).
		if set := s.overrideSet(ctx, t, ns, l); set != nil {
			if src, ok := set.msgs[key]; ok {
				if l == want {
					return src, OriginOverride
				}
				s.warnFallback(want, ns, key)
				return src, OriginFallback
			}
		}
		if src, ok := nd.flats[l][key]; ok {
			if l == want {
				return src, OriginBuiltin
			}
			s.warnFallback(want, ns, key)
			return src, OriginFallback
		}
	}
	return missing, OriginMissing
}

const warnedMax = 4096

// warnFallback mencatat peringatan satu kali per (bahasa, namespace, kunci) di mode development/test.
func (s *Service) warnFallback(loc, ns, key string) {
	if !s.dev || s.warnedSize.Load() >= warnedMax {
		return
	}
	if _, dup := s.warned.LoadOrStore(loc+"\x00"+ns+"\x00"+key, struct{}{}); dup {
		return
	}
	s.warnedSize.Add(1)
	s.log.Warn(WarnFallbackMessage, "locale", loc, "namespace", ns, "key", key)
}

// Catalog mengembalikan katalog efektif (override di atas bawaan, fallback sudah digabung) untuk ns dan loc.
// loc harus terdaftar di registri dan ns harus dikenal serta, untuk modul, aktif; selain itu ErrNotFound.
func (s *Service) Catalog(ctx context.Context, t tenancy.TenantID, ns string, loc sdki18n.Locale) (View, error) {
	snap := s.snap.Load()
	nd := snap.ns[ns]
	if nd == nil || !s.isActive(t, nd) || !snap.reg.Has(string(loc)) {
		return View{}, ErrNotFound
	}
	chain := snap.chainFor(nd, string(loc))
	sets := make([]*ovSet, len(chain))
	var sig strings.Builder
	for i, l := range chain {
		if sets[i] = s.overrideSet(ctx, t, ns, l); sets[i] != nil {
			fmt.Fprintf(&sig, "%s=%d;", l, sets[i].rev)
		}
	}
	// Revision namespace dihitung SELALU di luar cache (satu fungsi penghitung yang sama dengan Revisions), sebab
	// View di-cache per bahasa sedangkan revisi mencakup override semua bahasa namespace (R-8).
	rev := s.namespaceRevision(ctx, t, nd)
	k := t.String() + "\x00" + ns + "\x00" + string(loc) + "\x00" + sig.String()
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	if v, ok := s.views[k]; ok {
		out := *v
		out.Revision = rev
		return out, nil
	}
	msgs := map[string]string{}
	for i := len(chain) - 1; i >= 0; i-- { // dari fallback terjauh; bahasa yang lebih dekat menimpa
		for key, src := range nd.flats[chain[i]] {
			msgs[key] = src
		}
		if sets[i] != nil { // override menimpa bawaan pada bahasa yang sama
			for key, src := range sets[i].msgs {
				msgs[key] = src
			}
		}
	}
	sum := canonicalHash(msgs)
	v := &View{
		Locale: string(loc), Namespace: ns, Revision: rev,
		ETag: `W/"` + hex.EncodeToString(sum[:16]) + `"`, Messages: msgs,
	}
	if len(s.views) >= maxViews {
		s.views = map[string]*View{}
	}
	s.views[k] = v
	return *v, nil
}

// maxViews membatasi cache View; saat penuh seluruh isinya dibuang.
const maxViews = 512

// Revisions mengembalikan revisi katalog namespace yang aktif untuk tenant t (ui dan platform selalu ada).
// Revisi memuat revisi override tenant sehingga berubah bila override berubah.
func (s *Service) Revisions(ctx context.Context, t tenancy.TenantID) map[string]string {
	snap := s.snap.Load()
	out := make(map[string]string, len(snap.ns))
	for name, nd := range snap.ns {
		if s.isActive(t, nd) {
			out[name] = s.namespaceRevision(ctx, t, nd)
		}
	}
	return out
}

// namespaceRevision menggabungkan revisi katalog bawaan dengan revisi override (namespace, bahasa) tenant;
// tanpa override sama dengan revisi bawaan.
func (s *Service) namespaceRevision(ctx context.Context, t tenancy.TenantID, nd *nsData) string {
	if s.ovs == nil {
		return nd.revision
	}
	revs := s.overrideRevisions(ctx, t)
	var parts []string
	for _, l := range s.snap.Load().reg.Codes() {
		if r := revs[nd.name+"\x00"+l]; r > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", l, r))
		}
	}
	if len(parts) == 0 {
		return nd.revision
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s;%s", nd.revision, strings.Join(parts, ";"))
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// tenantResolver menyesuaikan Service ke sdki18n.Resolver untuk satu tenant.
type tenantResolver struct {
	s *Service
	t tenancy.TenantID
}

func (r tenantResolver) Resolve(ctx context.Context, ns, key string, loc sdki18n.Locale) (string, bool) {
	src, origin := r.s.Resolve(ctx, r.t, ns, key, loc)
	return src, origin != OriginMissing
}

// Resolver mengembalikan sdki18n.Resolver untuk tenant t; dipasang middleware di context request.
func (s *Service) Resolver(t tenancy.TenantID) sdki18n.Resolver { return tenantResolver{s: s, t: t} }
