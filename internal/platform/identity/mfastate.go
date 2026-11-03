package identity

import (
	"context"
	"errors"
	"sync"
	"time"

	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

// MFAStatus adalah keadaan MFA pengguna untuk keputusan otorisasi (ADR-0013 §4); internal/app memetakannya ke
// runtime.MFAState.
type MFAStatus struct {
	// FactorActive: pengguna punya faktor TOTP terkonfirmasi (pendaftaran yang belum dikonfirmasi tidak dihitung).
	FactorActive bool
	// EnrollmentRequired: pemegang role administrator (wajib punya faktor; FR-107, Q9).
	EnrollmentRequired bool
}

// MFAStore adalah bagian *store.Store yang dibaca MFAStateReader.
type MFAStore interface {
	GetMFAFactor(ctx context.Context, t tenancy.TenantID, userID string) (store.MFAFactor, error)
	HasRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string) (bool, error)
}

// DefaultMFAStateTTL adalah masa berlaku cache keadaan MFA per pengguna. Menjadi batas atas keterlambatan sebuah
// instance melihat perubahan yang dibuat instance lain (pendaftaran, reset MFA, pemberian role); instance yang sama
// segera konsisten lewat Invalidate.
const DefaultMFAStateTTL = 15 * time.Second

// maxMFAStateEntries membatasi ukuran cache; melampauinya cache dikosongkan (bukan dibuang satu per satu).
const maxMFAStateEntries = 10_000

type mfaKey struct {
	tenant tenancy.TenantID
	user   string
}

type mfaEntry struct {
	st      MFAStatus
	expires time.Time
}

// MFAStateReader membaca MFAStatus dari store dengan cache TTL singkat. Hanya dipanggil untuk sesi yang belum
// memverifikasi faktor kedua (sesi ber-otp melompati pencarian; lihat runtime.TypedAuthorizer), jadi biaya terbesar
// ada pada pengguna yang hanya login dengan sandi. Tidak ada pengguna atau faktor di tenant itu berarti status nol
// (bukan galat): pengguna tak dikenal tidak punya faktor dan bukan administrator.
type MFAStateReader struct {
	st  MFAStore
	ttl time.Duration
	now func() time.Time

	mu    sync.Mutex
	cache map[mfaKey]mfaEntry
}

// NewMFAStateReader membuat pembaca. ttl <= 0 memakai DefaultMFAStateTTL; now nil memakai time.Now.
func NewMFAStateReader(st MFAStore, ttl time.Duration, now func() time.Time) (*MFAStateReader, error) {
	if st == nil {
		return nil, errors.New("identity: MFAStore wajib")
	}
	if ttl <= 0 {
		ttl = DefaultMFAStateTTL
	}
	if now == nil {
		now = time.Now
	}
	return &MFAStateReader{st: st, ttl: ttl, now: now, cache: map[mfaKey]mfaEntry{}}, nil
}

// Status mengembalikan keadaan MFA userID pada tenant context. Galat store dikembalikan (pemanggil gagal tertutup).
func (r *MFAStateReader) Status(ctx context.Context, userID string) (MFAStatus, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return MFAStatus{}, errors.New("identity: context tanpa tenant")
	}
	k := mfaKey{t, userID}
	now := r.now()
	r.mu.Lock()
	if e, hit := r.cache[k]; hit && now.Before(e.expires) {
		r.mu.Unlock()
		return e.st, nil
	}
	r.mu.Unlock()

	var st MFAStatus
	f, err := r.st.GetMFAFactor(ctx, t, userID)
	switch {
	case err == nil:
		st.FactorActive = f.ConfirmedAt != nil
	case errors.Is(err, store.ErrNotFound):
	default:
		return MFAStatus{}, err
	}
	admin, err := r.st.HasRole(ctx, t, userID, mfa.AdministratorRole)
	if err != nil {
		return MFAStatus{}, err
	}
	st.EnrollmentRequired = admin

	r.mu.Lock()
	if len(r.cache) >= maxMFAStateEntries {
		clear(r.cache)
	}
	r.cache[k] = mfaEntry{st: st, expires: now.Add(r.ttl)}
	r.mu.Unlock()
	return st, nil
}

// Invalidate membuang entri cache userID pada tenant t. Dipanggil Service identity setelah pendaftaran dikonfirmasi,
// MFA direset, atau role berubah agar instance ini segera melihat keadaan baru.
func (r *MFAStateReader) Invalidate(t tenancy.TenantID, userID string) {
	r.mu.Lock()
	delete(r.cache, mfaKey{t, userID})
	r.mu.Unlock()
}
