package guard

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

// ErrNoTenant berarti context tidak membawa tenant (tenancy.FromContext). Lockout gagal tertutup: tidak ada tenant
// bawaan.
var ErrNoTenant = errors.New("guard: context tanpa tenant")

// AttemptStore adalah bagian store identitas yang dipakai Lockout (dan Authenticator untuk operasi tiruan). Dipenuhi
// *store.Store; tes unit memakai fake. Penghitung hidup di database (login_attempts) sehingga berlaku lintas instance.
type AttemptStore interface {
	RecordLoginAttempt(ctx context.Context, t tenancy.TenantID, userID, ip, kind string, success bool, at time.Time) error
	PairFailureStateSince(ctx context.Context, t tenancy.TenantID, userID, ip string, since time.Time) (store.PairState, error)
	CrossIPFailures(ctx context.Context, t tenancy.TenantID, userID string, since time.Time) (store.CrossIPState, error)
	GetUser(ctx context.Context, t tenancy.TenantID, id string) (store.User, error)
	LockUser(ctx context.Context, t tenancy.TenantID, userID string, at time.Time) (bool, error)
	UnlockUser(ctx context.Context, t tenancy.TenantID, userID string) (bool, error)
}

// LockoutConfig adalah ambang lockout (PRD-001 Q16, ADR-0013 §5). Nilai bawaan: DefaultLockoutConfig.
type LockoutConfig struct {
	// PairThreshold kegagalan berturut-turut pada satu pasangan (akun, IP) memicu jeda: BaseDelay, berlipat dua tiap
	// kegagalan berikutnya, paling lama MaxDelay (diukur dari kegagalan terakhir).
	PairThreshold int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	// PairBlockThreshold kegagalan pada satu pasangan memblokirnya selama PairBlockDuration (dari kegagalan terakhir).
	PairBlockThreshold int
	PairBlockDuration  time.Duration
	// PairWindow: hanya kegagalan pasangan dalam PairWindow terakhir yang dihitung (jeda bertingkat dan blokir 20
	// gagal); yang lebih tua diabaikan, sukses tetap mereset. Keputusan pemilik produk 3 Nov 2026 (Q19): 24 jam.
	// Harus >= PairBlockDuration agar hitungan blokir tidak jatuh selagi blokir berlaku.
	PairWindow time.Duration
	// Akun berstatus locked bila terkumpul AccountFailures kegagalan lintas >= AccountDistinctIPs IP berbeda dalam
	// AccountWindow terakhir. Tidak terbuka sendiri.
	AccountFailures    int
	AccountDistinctIPs int
	AccountWindow      time.Duration
}

// DefaultLockoutConfig adalah nilai Q16: jeda 30 detik berlipat dua sejak gagal ke-5 (maksimum 15 menit), blokir pasangan
// 1 jam pada gagal ke-20, kunci akun pada >= 50 gagal lintas >= 5 IP dalam 1 jam.
func DefaultLockoutConfig() LockoutConfig {
	return LockoutConfig{
		PairThreshold: 5, BaseDelay: 30 * time.Second, MaxDelay: 15 * time.Minute,
		PairBlockThreshold: 20, PairBlockDuration: time.Hour, PairWindow: 24 * time.Hour,
		AccountFailures: 50, AccountDistinctIPs: 5, AccountWindow: time.Hour,
	}
}

func (c LockoutConfig) validate() error {
	switch {
	case c.PairThreshold < 1:
		return errors.New("guard: LockoutConfig.PairThreshold harus >= 1")
	case c.BaseDelay <= 0:
		return errors.New("guard: LockoutConfig.BaseDelay harus > 0")
	case c.MaxDelay < c.BaseDelay:
		return errors.New("guard: LockoutConfig.MaxDelay harus >= BaseDelay")
	case c.PairBlockThreshold <= c.PairThreshold:
		return errors.New("guard: LockoutConfig.PairBlockThreshold harus > PairThreshold")
	case c.PairBlockDuration <= 0:
		return errors.New("guard: LockoutConfig.PairBlockDuration harus > 0")
	case c.PairWindow < c.PairBlockDuration:
		return errors.New("guard: LockoutConfig.PairWindow harus >= PairBlockDuration")
	case c.AccountFailures < 1:
		return errors.New("guard: LockoutConfig.AccountFailures harus >= 1")
	case c.AccountDistinctIPs < 1:
		return errors.New("guard: LockoutConfig.AccountDistinctIPs harus >= 1")
	case c.AccountWindow <= 0:
		return errors.New("guard: LockoutConfig.AccountWindow harus > 0")
	}
	return nil
}

// pairPenalty adalah lama jeda/blokir setelah n kegagalan berturut-turut pada satu pasangan: 0 di bawah ambang;
// BaseDelay x 2^(n-PairThreshold) dibatasi MaxDelay; PairBlockDuration sejak PairBlockThreshold.
func (c LockoutConfig) pairPenalty(n int) time.Duration {
	switch {
	case n < c.PairThreshold:
		return 0
	case n >= c.PairBlockThreshold:
		return c.PairBlockDuration
	}
	d := c.BaseDelay
	for i := c.PairThreshold; i < n && d < c.MaxDelay; i++ {
		d *= 2
	}
	return min(d, c.MaxDelay)
}

// State adalah keadaan lockout setelah sebuah pencatatan atau pemeriksaan.
type State struct {
	// PairBlocked: pasangan (akun, IP) sedang dijeda atau diblokir; tolak tanpa memverifikasi sandi (429 + RetryAfter).
	PairBlocked bool
	// AccountLocked: akun melampaui ambang lintas IP dan berstatus locked (diisi RegisterFailure; Check tidak membaca
	// status akun, pemanggil membacanya dari catatan pengguna).
	AccountLocked bool
	// RetryAfter: sisa jeda/blokir pasangan (0 bila tidak diblokir).
	RetryAfter time.Duration
}

// Bentuk State (PairBlocked, AccountLocked, RetryAfter; urutan dan tipe tetap) sengaja identik dengan mfa.LockState
// sehingga perekat (internal/app) cukup mengonversi: mfa.LockState(st). Jangan menambah field tanpa mengubah keduanya.

// Lockout menegakkan jeda bertingkat per pasangan (akun, IP) dan kunci akun lintas IP (Q16). Aman untuk dipakai
// serentak; keadaan di database lewat AttemptStore sehingga berlaku lintas instance. Tenant selalu dari context.
type Lockout struct {
	store   AttemptStore
	cfg     LockoutConfig
	now     func() time.Time
	unknown *unknownPairs
}

// NewLockout membuat Lockout. now (opsional, default time.Now) adalah jam yang dapat diinjeksi; cap waktu yang
// ditulis ke database memakai jam ini (UTC).
func NewLockout(s AttemptStore, cfg LockoutConfig, now func() time.Time) (*Lockout, error) {
	if s == nil {
		return nil, errors.New("guard: NewLockout: store nil")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	l := &Lockout{store: s, cfg: cfg, now: now}
	l.unknown = newUnknownPairs(DefaultMaxUnknownPairs, cfg.PairBlockThreshold)
	return l, nil
}

func tenantFrom(ctx context.Context) (tenancy.TenantID, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return tenancy.TenantID{}, ErrNoTenant
	}
	return t, nil
}

// stateFor menurunkan State pasangan dari hitungan kegagalan berturut-turut n dan waktu kegagalan terakhir.
func (l *Lockout) stateFor(n int, last *time.Time, now time.Time) State {
	var st State
	d := l.cfg.pairPenalty(n)
	if d == 0 || last == nil {
		return st
	}
	if remaining := last.Add(d).Sub(now); remaining > 0 {
		st.PairBlocked, st.RetryAfter = true, remaining
	}
	return st
}

// Check membaca keadaan saat ini TANPA menulis atau menambah hitungan gagal: jeda/blokir pasangan (akun, IP) dan
// apakah akun berstatus locked. Dipakai pemeriksa guess-limit sebelum memverifikasi kredensial (sandi di Authenticator,
// kode TOTP/recovery di MFA): selama PairBlocked atau AccountLocked tolak TANPA memverifikasi, TANPA mencatat percobaan
// itu (agar kegagalan tidak memperpanjang jeda tanpa batas dan hitungan lintas IP tidak membengkak), dan jangan
// membedakan sebabnya ke klien. Memakai satu bacaan pengguna dan satu bacaan pasangan.
func (l *Lockout) Check(ctx context.Context, userID, ip string) (State, error) {
	t, err := tenantFrom(ctx)
	if err != nil {
		return State{}, err
	}
	st, _, err := l.checkPair(ctx, t, userID, ip)
	if err != nil {
		return State{}, err
	}
	u, err := l.store.GetUser(ctx, t, userID)
	if err != nil {
		return State{}, fmt.Errorf("guard: baca status akun: %w", err)
	}
	st.AccountLocked = u.Status == store.UserLocked
	return st, nil
}

// checkPair membaca jeda pasangan saja (Authenticator sudah memegang status akun dari FindUserByEmail) dan jumlah
// kegagalan berturut-turut.
func (l *Lockout) checkPair(ctx context.Context, t tenancy.TenantID, userID, ip string) (State, int, error) {
	now := l.now().UTC()
	ps, err := l.store.PairFailureStateSince(ctx, t, userID, NormalizeIP(ip), now.Add(-l.cfg.PairWindow))
	if err != nil {
		return State{}, 0, fmt.Errorf("guard: baca keadaan pasangan: %w", err)
	}
	return l.stateFor(ps.ConsecutiveFailures, ps.LastFailureAt, now), ps.ConsecutiveFailures, nil
}

// RegisterFailure mencatat satu kegagalan sandi untuk (userID, ip) lalu menghitung ulang jeda pasangan dan ambang kunci
// akun lintas IP; bila ambang terlampaui akun dikunci (store.LockUser) dan State.AccountLocked true. ip dinormalisasi
// lewat NormalizeIP (IPv6 menjadi /64).
func (l *Lockout) RegisterFailure(ctx context.Context, userID, ip string) (State, error) {
	return l.registerFailure(ctx, store.AttemptPassword, userID, ip)
}

// RegisterMFAFailure sama dengan RegisterFailure untuk kegagalan kode MFA/recovery (kind mfa; FR-107): memakai
// hitungan pasangan dan ambang yang sama.
func (l *Lockout) RegisterMFAFailure(ctx context.Context, userID, ip string) (State, error) {
	return l.registerFailure(ctx, store.AttemptMFA, userID, ip)
}

func (l *Lockout) registerFailure(ctx context.Context, kind, userID, ip string) (State, error) {
	t, err := tenantFrom(ctx)
	if err != nil {
		return State{}, err
	}
	ipKey := NormalizeIP(ip)
	now := l.now().UTC()
	if err := l.store.RecordLoginAttempt(ctx, t, userID, ipKey, kind, false, now); err != nil {
		return State{}, fmt.Errorf("guard: catat kegagalan: %w", err)
	}
	ps, err := l.store.PairFailureStateSince(ctx, t, userID, ipKey, now.Add(-l.cfg.PairWindow))
	if err != nil {
		return State{}, fmt.Errorf("guard: baca keadaan pasangan: %w", err)
	}
	st := l.stateFor(ps.ConsecutiveFailures, ps.LastFailureAt, now)
	cs, err := l.store.CrossIPFailures(ctx, t, userID, now.Add(-l.cfg.AccountWindow))
	if err != nil {
		return st, fmt.Errorf("guard: baca kegagalan lintas IP: %w", err)
	}
	if cs.Failures >= l.cfg.AccountFailures && cs.DistinctIPs >= l.cfg.AccountDistinctIPs {
		st.AccountLocked = true
		// false = sudah locked/disabled: tidak ada yang berubah, hasilnya tetap sama.
		if _, err := l.store.LockUser(ctx, t, userID, now); err != nil {
			return st, fmt.Errorf("guard: kunci akun: %w", err)
		}
	}
	return st, nil
}

// RegisterSuccess mencatat sukses untuk (userID, ip), yang mereset hitungan pasangan itu (bukan jendela lintas IP).
// Panggil HANYA setelah autentikasi selesai penuh (setelah MFA bila ada), bukan pada sandi benar saja (doc store).
func (l *Lockout) RegisterSuccess(ctx context.Context, userID, ip string) error {
	t, err := tenantFrom(ctx)
	if err != nil {
		return err
	}
	if err := l.store.RecordLoginAttempt(ctx, t, userID, NormalizeIP(ip), store.AttemptPassword, true, l.now().UTC()); err != nil {
		return fmt.Errorf("guard: catat sukses: %w", err)
	}
	return nil
}

// Unlock membuka akun locked (bila locked) dan SELALU menghapus seluruh catatan percobaan akun itu (store.UnlockUser),
// sehingga jeda/blokir pasangan (akun, IP) pada akun yang masih active juga hilang; dipakai reset sandi,
// `core:unlock`, dan unlock admin. Idempoten: akun yang tidak berstatus locked bukan galat.
func (l *Lockout) Unlock(ctx context.Context, userID string) error {
	t, err := tenantFrom(ctx)
	if err != nil {
		return err
	}
	if _, err := l.store.UnlockUser(ctx, t, userID); err != nil {
		return fmt.Errorf("guard: buka kunci akun: %w", err)
	}
	return nil
}

// --- Operasi tiruan dan pelacak email tak dikenal (keseragaman respons dan jejak DB; dipakai Authenticator) ---

// dummyCheck mengeluarkan kueri baca yang sama dengan Check untuk pengguna fiktif: email tak dikenal menghasilkan jumlah
// dan bentuk round-trip database yang sama dengan akun nyata. Hasil dan galat diabaikan.
func (l *Lockout) dummyCheck(ctx context.Context, t tenancy.TenantID) {
	_, _ = l.store.PairFailureStateSince(ctx, t, uuid.NewString(), UnknownIP, l.now().UTC().Add(-l.cfg.PairWindow))
}

// dummyFailure meniru registerFailure: satu INSERT yang tidak memasukkan baris (ErrNotFound pada pengguna fiktif)
// diikuti dua kueri baca. Hasil dan galat diabaikan.
func (l *Lockout) dummyFailure(ctx context.Context, t tenancy.TenantID) {
	id, now := uuid.NewString(), l.now().UTC()
	_ = l.store.RecordLoginAttempt(ctx, t, id, UnknownIP, store.AttemptPassword, false, now)
	_, _ = l.store.PairFailureStateSince(ctx, t, id, UnknownIP, now.Add(-l.cfg.PairWindow))
	_, _ = l.store.CrossIPFailures(ctx, t, id, now.Add(-l.cfg.AccountWindow))
}

// unknownPairs melacak kegagalan per (tenant, email, IP) untuk email yang TIDAK dikenal, di memori, dengan jadwal jeda
// dan jendela PairWindow yang sama dengan akun nyata: tanpanya akun nyata menjawab 429 setelah 5 gagal sedangkan email
// tak dikenal terus menjawab 401, sehingga keberadaan akun dapat dienumerasi. Per instance dan tidak dibagikan lewat
// database (login_attempts mensyaratkan user_id NOT NULL, Q18): pada penerapan multi-instance di balik penyeimbang
// beban, hitungan tersebar per instance (residual terdokumentasi di doc.go). Menyimpan waktu kegagalan (paling banyak
// capPerKey terbaru: jadwal jenuh pada PairBlockThreshold) agar jendela bergeser per kegagalan seperti kueri DB.
type unknownPairs struct {
	mu        sync.Mutex
	m         *lruMap[[]time.Time]
	capPerKey int
}

// DefaultMaxUnknownPairs membatasi jumlah pasangan email tak dikenal yang dilacak di memori.
const DefaultMaxUnknownPairs = 100_000

func newUnknownPairs(maxEntries, capPerKey int) *unknownPairs {
	return &unknownPairs{m: newLRU[[]time.Time](maxEntries), capPerKey: capPerKey}
}

func (u *unknownPairs) key(t tenancy.TenantID, email, ipKey string) string {
	return TargetKey("pair", t, email+"\x00"+ipKey)
}

// pruneBefore membuang kegagalan sebelum since dari ts (urut naik) dan mengembalikan sisanya.
func pruneBefore(ts []time.Time, since time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(since) {
		i++
	}
	return ts[i:]
}

// state mengembalikan hitungan dan waktu gagal terakhir dalam jendela; 0 dan nil bila tak ada.
func (u *unknownPairs) state(key string, since time.Time) (int, *time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	p, ok := u.m.get(key)
	if !ok {
		return 0, nil
	}
	*p = pruneBefore(*p, since)
	if len(*p) == 0 {
		return 0, nil
	}
	last := (*p)[len(*p)-1]
	return len(*p), &last
}

func (u *unknownPairs) fail(key string, now, since time.Time) (int, *time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	p, ok := u.m.get(key)
	if !ok {
		p = u.m.add(key, nil)
	}
	ts := append(pruneBefore(*p, since), now)
	if len(ts) > u.capPerKey {
		ts = ts[len(ts)-u.capPerKey:]
	}
	*p = ts
	last := now
	return len(ts), &last
}

func (l *Lockout) unknownCheck(t tenancy.TenantID, email, ipKey string) State {
	now := l.now().UTC()
	n, last := l.unknown.state(l.unknown.key(t, email, ipKey), now.Add(-l.cfg.PairWindow))
	return l.stateFor(n, last, now)
}

func (l *Lockout) unknownFail(t tenancy.TenantID, email, ipKey string) State {
	now := l.now().UTC()
	n, last := l.unknown.fail(l.unknown.key(t, email, ipKey), now, now.Add(-l.cfg.PairWindow))
	return l.stateFor(n, last, now)
}
