package i18n

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/trace"

	"erpora/internal/platform/i18n/store"
	"erpora/internal/platform/tenancy"
	sdki18n "erpora/sdk/i18n"
)

// DefaultOverrideTTL adalah selang pemeriksaan revisi override ke DB (satu query ringan per tenant per
// selang). Konsistensi antar-instance bersifat eventual dalam selang ini; pada instance yang sama mutasi
// langsung berlaku.
const DefaultOverrideTTL = 5 * time.Second

// MaxOverrideLen adalah panjang maksimal pesan override (sama dengan batas pesan katalog dan CHECK tabel).
const MaxOverrideLen = 2000

// WarnOverrideStoreMessage adalah pesan log saat DB override tidak terjangkau.
const WarnOverrideStoreMessage = "i18n: override tidak dapat dimuat; katalog bawaan disajikan"

// Kode OverrideError; menjadi error.details[].code pada kode galat invalid_i18n_override (api-guidelines §8).
const (
	CodeUnknownKey     = "unknown_key"
	CodeInvalidSyntax  = "invalid_syntax"
	CodeParamMismatch  = "param_mismatch"
	CodeHTMLNotAllowed = "html_not_allowed"
	CodeTooLong        = "too_long"
)

// ErrOverridesUnavailable berarti Service dibangun tanpa penyimpanan override.
var ErrOverridesUnavailable = errors.New("i18n: override tidak tersedia")

var overrideKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// OverrideError adalah penolakan validasi override (Requirement 12.3). Field menyebut bagian yang salah
// (namespace, locale, key, message).
type OverrideError struct{ Code, Field, Detail string }

// Error mengimplementasi error.
func (e *OverrideError) Error() string {
	return fmt.Sprintf("i18n: override ditolak (%s) pada %s: %s", e.Code, e.Field, e.Detail)
}

// Override adalah isi permintaan simpan override; tenant dan aktor berasal dari pemanggil, bukan dari sini.
type Override struct{ Namespace, Locale, Key, Message string }

// OverrideStore dipenuhi *store.Store; fake pada test.
type OverrideStore interface {
	Revisions(ctx context.Context, t tenancy.TenantID) ([]store.Revision, error)
	ListLocale(ctx context.Context, t tenancy.TenantID, ns, locale string) ([]store.Override, error)
	Get(ctx context.Context, t tenancy.TenantID, ns, locale, key string) (store.Override, error)
	List(ctx context.Context, t tenancy.TenantID, f store.Filter, page, pageSize int) ([]store.Override, int64, error)
	Upsert(ctx context.Context, t tenancy.TenantID, o store.Override, pre store.Precondition) (changed bool, revision int64, err error)
	Delete(ctx context.Context, t tenancy.TenantID, ns, locale, key string, pre store.Precondition) (existed bool, revision int64, err error)
}

// ovSet adalah isi override satu (namespace, bahasa) pada revisi tertentu.
type ovSet struct {
	rev  int64
	msgs map[string]string
}

// Parameter pemuatan override. Query ke DB TIDAK PERNAH dilakukan sambil menahan kunci per tenant (R-6): satu
// pemuat bekerja, pemanggil lain memakai cache lama atau menunggu paling lama overrideWait. Query dibatasi
// overrideQueryTimeout, dan kegagalan diikuti backoff (negative cache) overrideBackoff agar DB bermasalah tidak
// dihujani ulang dan katalog bawaan tetap tersaji.
var (
	overrideWait         = 500 * time.Millisecond
	overrideQueryTimeout = 2 * time.Second
	overrideBackoff      = 30 * time.Second
)

// tenantOv adalah cache override satu tenant. revs adalah peta immutable (diganti utuh saat berubah)
// sehingga dapat dibaca pemanggil tanpa salinan. mu hanya melindungi memori; tidak ditahan selama I/O.
type tenantOv struct {
	mu         sync.Mutex
	nextCheck  time.Time // revisi diperiksa ulang sesudah ini (TTL setelah sukses, backoff setelah gagal); nol = belum pernah
	revs       map[string]int64
	sets       map[string]*ovSet
	refreshing chan struct{}            // tidak nil selama pemeriksaan revisi berlangsung
	loading    map[string]chan struct{} // pemuatan isi per (ns, bahasa) yang berlangsung
	failUntil  map[string]time.Time     // backoff pemuatan isi per (ns, bahasa)
}

func ovKey(ns, loc string) string { return ns + "\x00" + loc }

func (s *Service) tenantOv(t tenancy.TenantID) *tenantOv {
	s.ovMu.Lock()
	defer s.ovMu.Unlock()
	to := s.ov[t]
	if to == nil {
		to = &tenantOv{sets: map[string]*ovSet{}, loading: map[string]chan struct{}{}, failUntil: map[string]time.Time{}}
		s.ov[t] = to
	}
	return to
}

// warnStore mencatat peringatan DB override tidak terjangkau paling sering satu kali per 30 detik.
func (s *Service) warnStore(err error, reason string) {
	s.countOverrideError(reason)
	nowNs := s.now().UnixNano()
	last := s.lastWarnStore.Load()
	if last != 0 && nowNs-last < int64(30*time.Second) {
		return
	}
	if s.lastWarnStore.CompareAndSwap(last, nowNs) {
		s.log.Warn(WarnOverrideStoreMessage, "error", err.Error())
	}
}

// isCallerCancel melaporkan apakah err hanyalah akibat pembatalan context PEMANGGIL (bukan kegagalan nyata store):
// tidak boleh memicu backoff atau peringatan "DB tak terjangkau" (F-1).
func isCallerCancel(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, ctx.Err())
}

// waitFlight menunggu pekerjaan yang sedang berlangsung paling lama overrideWait (atau ctx selesai).
func waitFlight(ctx context.Context, ch chan struct{}) {
	timer := time.NewTimer(overrideWait)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	case <-ctx.Done():
	}
}

// refreshLocked memeriksa revisi override tenant ke DB paling sering sekali per TTL (dan sekali per backoff bila
// gagal). Dipanggil dengan to.mu terkunci dan kembali dengan terkunci, tetapi melepasnya selama I/O.
// Selama pemeriksaan berlangsung pemanggil lain memakai revisi lama; hanya bila belum ada data sama sekali mereka
// menunggu paling lama overrideWait.
func (s *Service) refreshLocked(ctx context.Context, to *tenantOv, t tenancy.TenantID) {
	now := s.now()
	if !to.nextCheck.IsZero() && now.Before(to.nextCheck) {
		return
	}
	if ch := to.refreshing; ch != nil {
		if to.revs == nil {
			to.mu.Unlock()
			waitFlight(ctx, ch)
			to.mu.Lock()
		}
		return
	}
	ch := make(chan struct{})
	to.refreshing = ch
	to.mu.Unlock()
	// Query bersama milik SEMUA penunggu, bukan satu pemanggil: dijalankan di luar pembatalan request pemicu
	// (klien memutus koneksi atau React Query membatalkan fetch) dengan timeout sendiri (F-1).
	qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), overrideQueryTimeout)
	rows, err := s.ovs.Revisions(qctx, t)
	cancel()
	to.mu.Lock()
	to.refreshing = nil
	close(ch)
	if err != nil {
		if isCallerCancel(ctx, err) { // bukan kegagalan DB: tanpa backoff dan tanpa peringatan
			return
		}
		s.warnStore(err, "revisions")
		to.nextCheck = now.Add(overrideBackoff)
		return
	}
	// Gabung dengan max per kunci: revisi di DB hanya naik, dan hasil query yang dimulai SEBELUM sebuah mutasi
	// (afterMutation sudah mengisi revisi lebih baru) tidak boleh menurunkannya (monoton di semua jalur; F-2).
	revs := make(map[string]int64, len(rows)+len(to.revs))
	for k, v := range to.revs {
		revs[k] = v
	}
	for _, r := range rows {
		if k := ovKey(r.Namespace, r.Locale); r.Revision > revs[k] {
			revs[k] = r.Revision
		}
	}
	to.revs = revs
	to.nextCheck = now.Add(s.ttl)
}

// overrideRevisions mengembalikan peta revisi override (kunci ns\x00locale); tidak boleh diubah pemanggil.
func (s *Service) overrideRevisions(ctx context.Context, t tenancy.TenantID) map[string]int64 {
	if s.ovs == nil {
		return nil
	}
	to := s.tenantOv(t)
	to.mu.Lock()
	defer to.mu.Unlock()
	s.refreshLocked(ctx, to, t)
	return to.revs
}

// overrideSet mengembalikan override tenant untuk (ns, loc) yang segar dalam TTL, atau nil bila tidak ada (atau
// belum dapat dimuat: DB lambat atau gagal; pemanggil lalu menyajikan bawaan).
func (s *Service) overrideSet(ctx context.Context, t tenancy.TenantID, ns, loc string) *ovSet {
	if s.ovs == nil {
		return nil
	}
	to := s.tenantOv(t)
	to.mu.Lock()
	defer to.mu.Unlock()
	s.refreshLocked(ctx, to, t)
	k := ovKey(ns, loc)
	rev := to.revs[k]
	if rev == 0 {
		delete(to.sets, k)
		return nil
	}
	if set := to.sets[k]; set != nil && set.rev == rev {
		return set
	}
	if ch := to.loading[k]; ch != nil { // pemuat lain bekerja: tunggu terbatas, lalu pakai hasilnya atau yang ada
		to.mu.Unlock()
		waitFlight(ctx, ch)
		to.mu.Lock()
		return to.sets[k]
	}
	now := s.now()
	if until, ok := to.failUntil[k]; ok && now.Before(until) {
		return to.sets[k]
	}
	ch := make(chan struct{})
	to.loading[k] = ch
	to.mu.Unlock()
	qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), overrideQueryTimeout)
	rows, err := s.ovs.ListLocale(qctx, t, ns, loc)
	cancel()
	to.mu.Lock()
	delete(to.loading, k)
	close(ch)
	if err != nil {
		if isCallerCancel(ctx, err) {
			return to.sets[k]
		}
		s.warnStore(err, "load")
		to.failUntil[k] = now.Add(overrideBackoff)
		return to.sets[k]
	}
	delete(to.failUntil, k)
	set := &ovSet{rev: rev, msgs: make(map[string]string, len(rows))}
	for _, r := range rows {
		set.msgs[r.Key] = r.Message
	}
	to.sets[k] = set
	return set
}

// afterMutation membuat perubahan langsung berlaku pada instance ini: revisi diperbarui (monoton: tidak pernah
// turun walau dua mutasi selesai terbalik) dan isi (ns, loc) dimuat ulang pada pemakaian berikutnya.
func (s *Service) afterMutation(t tenancy.TenantID, ns, loc string, rev int64) {
	to := s.tenantOv(t)
	to.mu.Lock()
	defer to.mu.Unlock()
	if to.revs != nil {
		revs := make(map[string]int64, len(to.revs)+1)
		for k, v := range to.revs {
			revs[k] = v
		}
		if rev > revs[ovKey(ns, loc)] {
			revs[ovKey(ns, loc)] = rev
		}
		to.revs = revs
	}
	delete(to.sets, ovKey(ns, loc))
	delete(to.failUntil, ovKey(ns, loc))
}

// validateTarget memeriksa namespace (dikenal dan aktif) dan bahasa (terdaftar).
func (s *Service) validateTarget(t tenancy.TenantID, ns, loc string) (*nsData, *OverrideError) {
	snap := s.snap.Load()
	nd := snap.ns[ns]
	if nd == nil || !s.isActive(t, nd) {
		return nil, &OverrideError{Code: CodeUnknownKey, Field: "namespace", Detail: "namespace tidak dikenal atau tidak aktif"}
	}
	if !snap.reg.Has(loc) {
		return nil, &OverrideError{Code: CodeUnknownKey, Field: "locale", Detail: "bahasa tidak terdaftar di registri"}
	}
	return nd, nil
}

// validateOverride menerapkan Requirement 12.3: kunci ada di katalog bawaan bahasa DEFAULT namespace itu,
// pesan tidak berisi HTML, sintaks ICU valid dalam batas, dan argumen sama dengan pesan bawaan itu.
func (s *Service) validateOverride(t tenancy.TenantID, o Override) *OverrideError {
	nd, oerr := s.validateTarget(t, o.Namespace, o.Locale)
	if oerr != nil {
		return oerr
	}
	if len(o.Key) > 200 || !overrideKeyPattern.MatchString(o.Key) {
		return &OverrideError{Code: CodeUnknownKey, Field: "key", Detail: "kunci tidak sah"}
	}
	baseSrc, ok := nd.flats[nd.def][o.Key]
	if !ok {
		return &OverrideError{Code: CodeUnknownKey, Field: "key", Detail: "kunci tidak ada di katalog bawaan bahasa default"}
	}
	if utf8.RuneCountInString(o.Message) > MaxOverrideLen {
		return &OverrideError{Code: CodeTooLong, Field: "message", Detail: fmt.Sprintf("pesan melebihi %d karakter", MaxOverrideLen)}
	}
	if o.Message == "" {
		return &OverrideError{Code: CodeInvalidSyntax, Field: "message", Detail: "pesan tidak boleh kosong"}
	}
	msg, err := sdki18n.ValidateMessage(o.Message, sdki18n.DefaultLimits())
	if err != nil {
		var ce *sdki18n.CompileError
		code, detail := CodeInvalidSyntax, err.Error()
		if errors.As(err, &ce) {
			detail = fmt.Sprintf("posisi %d: %s", ce.Pos, ce.Detail)
			switch ce.Code {
			case sdki18n.CodeHTMLNotAllowed:
				code = CodeHTMLNotAllowed
			case sdki18n.CodeTooLarge:
				code = CodeTooLong
			}
		}
		return &OverrideError{Code: code, Field: "message", Detail: detail}
	}
	base, err := sdki18n.Compile(baseSrc, sdki18n.DefaultLimits())
	if err != nil { // katalog bawaan sudah divalidasi saat New; tidak mungkin
		return &OverrideError{Code: CodeInvalidSyntax, Field: "message", Detail: "pesan bawaan tidak dapat dibandingkan"}
	}
	if !sdki18n.SameParamsStrict(base.Params(), msg.Params()) {
		return &OverrideError{Code: CodeParamMismatch, Field: "message", Detail: "argumen pesan (nama dan jenis) harus sama dengan pesan bawaan"}
	}
	return nil
}

// SetOverride memvalidasi lalu menyimpan override tenant. pre (opsional) dijalankan di dalam transaksi setelah
// baris terkunci (dipakai If-Match). Nilai yang sama tidak mengubah apa pun (changed=false, revisi tetap).
// Setiap perubahan yang berefek dicatat sebagai log terstruktur i18n_override_changed (tanpa isi pesan)
// sampai audit writer PRD-001 tersedia (Requirement 12.7).
func (s *Service) SetOverride(ctx context.Context, t tenancy.TenantID, o Override, actor string, pre store.Precondition) (bool, error) {
	if s.ovs == nil {
		return false, ErrOverridesUnavailable
	}
	if oerr := s.validateOverride(t, o); oerr != nil {
		return false, oerr
	}
	changed, rev, err := s.ovs.Upsert(ctx, t, store.Override{
		TenantID: t, Namespace: o.Namespace, Locale: o.Locale, Key: o.Key, Message: o.Message, UpdatedBy: actor,
	}, pre)
	if err != nil {
		return false, err
	}
	if changed {
		s.afterMutation(t, o.Namespace, o.Locale, rev)
		s.logChange(ctx, t, "set", o.Namespace, o.Locale, o.Key, actor)
	}
	return changed, nil
}

// DeleteOverride menghapus override tenant (kembali ke bawaan). Namespace atau bahasa tak dikenal
// menghasilkan ErrNotFound. existed=false bila barisnya tidak ada (revisi tidak naik).
func (s *Service) DeleteOverride(ctx context.Context, t tenancy.TenantID, ns, loc, key, actor string, pre store.Precondition) (bool, error) {
	if s.ovs == nil {
		return false, ErrOverridesUnavailable
	}
	if _, oerr := s.validateTarget(t, ns, loc); oerr != nil {
		return false, ErrNotFound
	}
	existed, rev, err := s.ovs.Delete(ctx, t, ns, loc, key, pre)
	if err != nil {
		return false, err
	}
	if existed {
		s.afterMutation(t, ns, loc, rev)
		s.logChange(ctx, t, "delete", ns, loc, key, actor)
	}
	return existed, nil
}

// GetOverride memuat satu override tenant (store.ErrNotFound bila tidak ada).
func (s *Service) GetOverride(ctx context.Context, t tenancy.TenantID, ns, loc, key string) (store.Override, error) {
	if s.ovs == nil {
		return store.Override{}, ErrOverridesUnavailable
	}
	return s.ovs.Get(ctx, t, ns, loc, key)
}

// ListOverrides memuat override tenant dengan pagination halaman.
func (s *Service) ListOverrides(ctx context.Context, t tenancy.TenantID, f store.Filter, page, pageSize int) ([]store.Override, int64, error) {
	if s.ovs == nil {
		return nil, 0, ErrOverridesUnavailable
	}
	return s.ovs.List(ctx, t, f, page, pageSize)
}

// OverridesEnabled melaporkan apakah penyimpanan override terpasang.
func (s *Service) OverridesEnabled() bool { return s.ovs != nil }

func (s *Service) logChange(ctx context.Context, t tenancy.TenantID, action, ns, loc, key, actor string) {
	attrs := []any{"tenant", t.String(), "namespace", ns, "locale", loc, "key", key, "actor", actor, "action", action}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		attrs = append(attrs, "trace_id", sc.TraceID().String())
	}
	s.log.InfoContext(ctx, "i18n_override_changed", attrs...)
}
