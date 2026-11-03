// Package token menerbitkan dan memverifikasi access token JWT EdDSA (Ed25519), mengelola keyring kunci
// penandatangan (pending/active/retiring/retired), dan menyajikan JWKS (PRD-001 FR-102; ADR-0013 §3).
//
// Paket ini tidak mengimpor store GORM: persistensi lewat antarmuka KeyStore (dipenuhi adaptor tipis di
// perakit aplikasi) dan penyebaran perubahan lewat antarmuka Notifier. Kunci privat hanya ada di memori untuk
// kunci berstatus active, disimpan tersegel (crypto.Sealer) dengan AAD "signing_keys|private|<kid>", dan tidak
// pernah muncul di log, galat, atau hasil pencetakan (String/GoString/Format/LogValue tersamar).
package token

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"erpora/internal/platform/crypto"
	"erpora/sdk/identity"
)

// Konstanta umur (ADR-0013 §3).
const (
	// AccessTokenTTL adalah umur access token.
	AccessTokenTTL = 10 * time.Minute
	// Leeway adalah toleransi selisih jam verifier (exp, iat) dan tambahan masa retiring.
	Leeway = 30 * time.Second
	// JWKSMaxAge adalah max-age JWKS sekaligus masa minimal kunci pending sebelum boleh dipromosikan.
	JWKSMaxAge = 300 * time.Second
	// RetireAfter adalah masa minimal kunci retiring sebelum menjadi retired: umur access token + leeway.
	RetireAfter = AccessTokenTTL + Leeway

	// ChannelKeysChanged adalah kanal Notifier yang dipublikasikan setiap perubahan keyring.
	ChannelKeysChanged = "identity.keys_changed"

	defaultUnknownKIDReloadInterval = 10 * time.Second
	defaultTickInterval             = 30 * time.Second
	reloadTimeout                   = 5 * time.Second
)

// Status kunci (sama dengan sdk/identity.KeyStatus* dan nilai kolom signing_keys.status).
const (
	StatusPending  = identity.KeyStatusPending
	StatusActive   = identity.KeyStatusActive
	StatusRetiring = identity.KeyStatusRetiring
	StatusRetired  = identity.KeyStatusRetired
)

var (
	// ErrNoActiveKey berarti keyring tidak punya kunci active untuk menandatangani.
	ErrNoActiveKey = errors.New("token: tidak ada kunci penandatangan aktif")

	kidPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
)

// ValidKID melaporkan apakah kid berbentuk sah (^[A-Za-z0-9_-]{8,64}$, sama dengan batasan store). kid lain
// tidak pernah dicari di keyring maupun memicu muat ulang.
func ValidKID(kid string) bool { return kidPattern.MatchString(kid) }

// KeyRecord adalah bentuk simpan satu kunci (cermin store.SigningKey; adaptor memetakannya). Semua waktu UTC.
// PrivateKeySealed adalah hasil crypto.Sealer atas seed Ed25519 32 byte.
type KeyRecord struct {
	ID               string
	Status           string
	PublicKey        []byte
	PrivateKeySealed []byte
	CreatedAt        time.Time
	ActivatedAt      *time.Time
	RetiringAt       *time.Time
	RetiredAt        *time.Time
}

// KeyStore adalah persistensi kunci. Kontraknya sama dengan store.Store: TransitionSigningKey bersyarat dan atomik
// (false bila status saat ini bukan from atau kunci tidak ada).
type KeyStore interface {
	ListSigningKeys(ctx context.Context) ([]KeyRecord, error)
	InsertSigningKey(ctx context.Context, k KeyRecord) error
	TransitionSigningKey(ctx context.Context, id, from, to string, at time.Time) (bool, error)
}

// Notifier menyebarkan perubahan antar instance (kanal bernama). Implementasi produksi memetakan ke
// StateNotifier platform (perakit aplikasi); Subscribe mengembalikan fungsi pembatalan.
type Notifier interface {
	Publish(ctx context.Context, channel string)
	Subscribe(channel string, fn func(ctx context.Context)) (cancel func())
}

// Key adalah satu kunci penandatangan. Kunci privat hanya terisi (tidak diekspor) untuk status active.
// Nilai nol (ID kosong) berarti "tidak ada kunci".
type Key struct {
	ID          string
	Status      string
	Public      ed25519.PublicKey
	CreatedAt   time.Time
	ActivatedAt time.Time // nol bila belum pernah aktif
	RetiringAt  time.Time // nol bila belum retiring
	RetiredAt   time.Time // nol bila belum retired

	priv ed25519.PrivateKey
}

// IsZero melaporkan apakah k adalah nilai nol.
func (k Key) IsZero() bool { return k.ID == "" }

// ActivatesAt adalah saat kunci mulai (atau mulai boleh) menandatangani: CreatedAt+JWKSMaxAge bagi pending,
// ActivatedAt bagi kunci yang sudah pernah aktif.
func (k Key) ActivatesAt() time.Time {
	if k.Status == StatusPending {
		return k.CreatedAt.Add(JWKSMaxAge)
	}
	if !k.ActivatedAt.IsZero() {
		return k.ActivatedAt
	}
	return k.CreatedAt
}

// Info mengembalikan ringkasan tanpa material kunci (untuk CLI/admin).
func (k Key) Info() identity.KeyInfo {
	return identity.KeyInfo{KeyID: k.ID, Algorithm: "EdDSA", Status: k.Status, CreatedAt: k.CreatedAt, ActivatesAt: k.ActivatesAt()}
}

func (k Key) redacted() string {
	return fmt.Sprintf("token.Key{ID:%s Status:%s private:****}", k.ID, k.Status)
}

// String dan GoString menyamarkan material privat.
func (k Key) String() string   { return k.redacted() }
func (k Key) GoString() string { return k.redacted() }

// Format menyamarkan untuk semua verba (%v %+v %#v %s %x ...).
func (k Key) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(k.redacted())) }

// LogValue menyamarkan pada slog.
func (k Key) LogValue() slog.Value { return slog.StringValue(k.redacted()) }

func (k Key) newerActiveThan(o Key) bool {
	a, b := k.ActivatesAt(), o.ActivatesAt()
	if !a.Equal(b) {
		return a.After(b)
	}
	return k.ID > o.ID
}

func sealAAD(kid string) []byte { return []byte("signing_keys|private|" + kid) }

type snapshot struct {
	byKID  map[string]Key
	sorted []Key // terurut CreatedAt, lalu ID
	active Key
}

// KeyringConfig mengonfigurasi Keyring.
type KeyringConfig struct {
	Store    KeyStore
	Sealer   *crypto.Sealer
	Notifier Notifier // opsional
	// Now adalah jam (bawaan time.Now); diinjeksi pada tes.
	Now func() time.Time
	Log *slog.Logger
	// UnknownKIDReloadInterval membatasi muat ulang akibat kid tak dikenal (bawaan 10 detik).
	UnknownKIDReloadInterval time.Duration
}

// Keyring menyimpan kunci di memori sebagai snapshot tak berubah (copy-on-write) dan aman dipakai bersamaan.
type Keyring struct {
	store    KeyStore
	sealer   *crypto.Sealer
	notifier Notifier
	now      func() time.Time
	log      *slog.Logger
	interval time.Duration

	snap     atomic.Pointer[snapshot]
	reloadMu sync.Mutex // menserialkan list+swap agar snapshot lama tidak menimpa yang baru

	unkMu      sync.Mutex
	lastUnkRel time.Time

	cancelSub func()
}

// NewKeyring membuat Keyring kosong; panggil Reload (atau EnsureActive) sebelum dipakai. Bila Notifier ada,
// Keyring berlangganan ChannelKeysChanged dan memuat ulang setiap notifikasi; panggil Close untuk berhenti.
func NewKeyring(cfg KeyringConfig) (*Keyring, error) {
	if cfg.Store == nil || cfg.Sealer == nil {
		return nil, errors.New("token: Store dan Sealer wajib")
	}
	r := &Keyring{store: cfg.Store, sealer: cfg.Sealer, notifier: cfg.Notifier, now: cfg.Now, log: cfg.Log, interval: cfg.UnknownKIDReloadInterval}
	if r.now == nil {
		r.now = time.Now
	}
	if r.log == nil {
		r.log = slog.Default()
	}
	if r.interval <= 0 {
		r.interval = defaultUnknownKIDReloadInterval
	}
	r.snap.Store(&snapshot{byKID: map[string]Key{}})
	if r.notifier != nil {
		r.cancelSub = r.notifier.Subscribe(ChannelKeysChanged, func(ctx context.Context) {
			if err := r.Reload(ctx); err != nil {
				r.log.ErrorContext(ctx, "token: muat ulang keyring (notifikasi) gagal", "error", err.Error())
			}
		})
	}
	return r, nil
}

// Close menghentikan langganan Notifier.
func (r *Keyring) Close() {
	if r.cancelSub != nil {
		r.cancelSub()
		r.cancelSub = nil
	}
}

func tsOrZero(p *time.Time) time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.UTC()
}

func (r *Keyring) buildKey(rec KeyRecord) (Key, error) {
	if !ValidKID(rec.ID) || len(rec.PublicKey) != ed25519.PublicKeySize {
		return Key{}, errors.New("rekaman kunci tidak sah")
	}
	switch rec.Status {
	case StatusPending, StatusActive, StatusRetiring, StatusRetired:
	default:
		return Key{}, errors.New("status kunci tidak dikenal")
	}
	k := Key{
		ID: rec.ID, Status: rec.Status, Public: ed25519.PublicKey(append([]byte(nil), rec.PublicKey...)),
		CreatedAt: rec.CreatedAt.UTC(), ActivatedAt: tsOrZero(rec.ActivatedAt), RetiringAt: tsOrZero(rec.RetiringAt), RetiredAt: tsOrZero(rec.RetiredAt),
	}
	if rec.Status == StatusActive {
		seed, err := r.sealer.Open(rec.PrivateKeySealed, sealAAD(rec.ID))
		if err != nil {
			return Key{}, fmt.Errorf("buka kunci privat %s: %w", rec.ID, err)
		}
		defer clear(seed)
		if len(seed) != ed25519.SeedSize {
			return Key{}, fmt.Errorf("kunci privat %s: panjang seed salah", rec.ID)
		}
		priv := ed25519.NewKeyFromSeed(seed)
		if !priv.Public().(ed25519.PublicKey).Equal(k.Public) {
			return Key{}, fmt.Errorf("kunci privat %s tidak cocok dengan kunci publik", rec.ID)
		}
		k.priv = priv
	}
	return k, nil
}

// Reload memuat ulang semua kunci dari store dan mengganti snapshot secara atomik. Bila gagal (store atau
// kunci active tidak dapat dibuka), snapshot lama dipertahankan. Rekaman non-active yang rusak dilewati dan dicatat.
func (r *Keyring) Reload(ctx context.Context) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()
	recs, err := r.store.ListSigningKeys(ctx)
	if err != nil {
		return fmt.Errorf("token: muat kunci: %w", err)
	}
	s := &snapshot{byKID: make(map[string]Key, len(recs))}
	for _, rec := range recs {
		k, err := r.buildKey(rec)
		if err != nil {
			if rec.Status == StatusActive {
				return fmt.Errorf("token: %w", err)
			}
			r.log.WarnContext(ctx, "token: rekaman kunci dilewati", "kid", rec.ID, "error", err.Error())
			continue
		}
		s.byKID[k.ID] = k
		s.sorted = append(s.sorted, k)
		if k.Status == StatusActive && (s.active.IsZero() || k.newerActiveThan(s.active)) {
			s.active = k
		}
	}
	sort.Slice(s.sorted, func(i, j int) bool {
		if !s.sorted[i].CreatedAt.Equal(s.sorted[j].CreatedAt) {
			return s.sorted[i].CreatedAt.Before(s.sorted[j].CreatedAt)
		}
		return s.sorted[i].ID < s.sorted[j].ID
	})
	r.snap.Store(s)
	return nil
}

// Active mengembalikan kunci penandatangan: kunci active dengan aktivasi terbaru (tie-break ID), sama di semua
// instance. Nilai nol (IsZero) bila tidak ada.
func (r *Keyring) Active() Key { return r.snap.Load().active }

// Keys mengembalikan semua kunci (semua status) terurut CreatedAt. Kunci privat tidak terjangkau dari hasilnya.
func (r *Keyring) Keys() []Key { return append([]Key(nil), r.snap.Load().sorted...) }

func (r *Keyring) allowUnknownReload() bool {
	r.unkMu.Lock()
	defer r.unkMu.Unlock()
	now := r.now()
	if !r.lastUnkRel.IsZero() && now.Sub(r.lastUnkRel) < r.interval {
		return false
	}
	r.lastUnkRel = now
	return true
}

// ByKID mencari kunci menurut kid untuk verifikasi. kid berbentuk tak sah tidak dicari dan tidak memicu muat ulang.
// Bila kid tak dikenal (atau masih pending di snapshot ini, mis. instance lain sudah mempromosikannya), keyring
// dimuat ulang satu kali, dibatasi laju (UnknownKIDReloadInterval), lalu dicari lagi. Hasil dapat berstatus
// apa pun, termasuk pending dan retired: pemanggil wajib memeriksa Status (Verifier hanya menerima active dan retiring).
func (r *Keyring) ByKID(kid string) (Key, bool) {
	if !ValidKID(kid) {
		return Key{}, false
	}
	if k, ok := r.snap.Load().byKID[kid]; ok && k.Status != StatusPending {
		return k, true
	}
	if r.allowUnknownReload() {
		ctx, cancel := context.WithTimeout(context.Background(), reloadTimeout)
		defer cancel()
		if err := r.Reload(ctx); err != nil {
			r.log.Error("token: muat ulang keyring (kid tak dikenal) gagal", "error", err.Error())
		}
	}
	k, ok := r.snap.Load().byKID[kid]
	return k, ok
}

func (r *Keyring) publish(ctx context.Context) {
	if r.notifier != nil {
		r.notifier.Publish(ctx, ChannelKeysChanged)
	}
}

func newKID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token: kid acak: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newRecord membuat pasangan kunci baru dengan seed tersegel.
func (r *Keyring) newRecord(status string, now time.Time) (KeyRecord, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyRecord{}, fmt.Errorf("token: bangkitkan kunci: %w", err)
	}
	defer clear(priv)
	kid, err := newKID()
	if err != nil {
		return KeyRecord{}, err
	}
	seed := priv.Seed()
	defer clear(seed)
	sealed, err := r.sealer.Seal(seed, sealAAD(kid))
	if err != nil {
		return KeyRecord{}, errors.New("token: segel kunci privat gagal")
	}
	created := now.UTC().Truncate(time.Microsecond)
	rec := KeyRecord{ID: kid, Status: status, PublicKey: pub, PrivateKeySealed: sealed, CreatedAt: created}
	if status == StatusActive {
		rec.ActivatedAt = &created
	}
	return rec, nil
}

// Rotate hanya membuat kunci baru berstatus pending (terpublikasi di JWKS, tidak dipakai menandatangani),
// memuat ulang keyring, dan memicu identity.keys_changed. Kunci tidak pernah diaktifkan di sini: promosi oleh
// Promoter setelah JWKSMaxAge. Key hasilnya membawa ActivatesAt().
func (r *Keyring) Rotate(ctx context.Context) (Key, error) {
	rec, err := r.newRecord(StatusPending, r.now())
	if err != nil {
		return Key{}, err
	}
	if err := r.store.InsertSigningKey(ctx, rec); err != nil {
		return Key{}, fmt.Errorf("token: simpan kunci: %w", err)
	}
	if err := r.Reload(ctx); err != nil {
		return Key{}, err
	}
	r.publish(ctx)
	k, _ := r.snap.Load().byKID[rec.ID]
	return k, nil
}

// EnsureActive membuat kunci pertama langsung active bila keyring tidak punya kunci active maupun pending
// (instalasi baru atau semua kunci retired). Bila sudah ada active atau pending, tidak melakukan apa pun
// (pending menunggu promosi menurut JWKSMaxAge). Dua instance yang berlomba menghasilkan dua kunci active yang
// konvergen lewat Active() dan Promoter. Mengembalikan true bila membuat kunci.
func (r *Keyring) EnsureActive(ctx context.Context) (bool, error) {
	if err := r.Reload(ctx); err != nil {
		return false, err
	}
	return r.ensureActiveLoaded(ctx, r.now())
}

func (r *Keyring) ensureActiveLoaded(ctx context.Context, now time.Time) (bool, error) {
	for _, k := range r.snap.Load().sorted {
		if k.Status == StatusActive || k.Status == StatusPending {
			return false, nil
		}
	}
	rec, err := r.newRecord(StatusActive, now)
	if err != nil {
		return false, err
	}
	if err := r.store.InsertSigningKey(ctx, rec); err != nil {
		return false, fmt.Errorf("token: simpan kunci pertama: %w", err)
	}
	if err := r.Reload(ctx); err != nil {
		return true, err
	}
	r.publish(ctx)
	return true, nil
}

// Promoter adalah pekerja latar (dijalankan proses serve) yang menjalankan daur hidup kunci: mempromosikan
// pending menjadi active setelah JWKSMaxAge, menurunkan active lama menjadi retiring, dan mempensiunkan retiring
// setelah RetireAfter. Setiap tick juga memuat ulang keyring dari store (pengaman bila notifikasi lintas proses
// tidak sampai, mis. core:rotate-keys berjalan di proses lain).
type Promoter struct {
	ring     *Keyring
	now      func() time.Time
	interval time.Duration
	log      *slog.Logger
}

// PromoterConfig mengonfigurasi Promoter. Now dan Interval bawaan time.Now dan 30 detik.
type PromoterConfig struct {
	Keyring  *Keyring
	Now      func() time.Time
	Interval time.Duration
	Log      *slog.Logger
}

// NewPromoter membuat Promoter.
func NewPromoter(cfg PromoterConfig) (*Promoter, error) {
	if cfg.Keyring == nil {
		return nil, errors.New("token: Keyring wajib")
	}
	p := &Promoter{ring: cfg.Keyring, now: cfg.Now, interval: cfg.Interval, log: cfg.Log}
	if p.now == nil {
		p.now = time.Now
	}
	if p.interval <= 0 {
		p.interval = defaultTickInterval
	}
	if p.log == nil {
		p.log = slog.Default()
	}
	return p, nil
}

// Run menjalankan Tick segera (mencakup promosi yang sudah lewat saat serve start) lalu tiap Interval sampai
// ctx selesai. Galat dicatat dan tidak menghentikan pekerja.
func (p *Promoter) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		if err := p.Tick(ctx, p.now()); err != nil && ctx.Err() == nil {
			p.log.ErrorContext(ctx, "token: tick promoter gagal", "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick menjalankan satu putaran pada waktu now. Urutan: muat ulang; bootstrap kunci pertama bila kosong;
// promosikan pending terbaru yang sudah matang (yang lebih tua dibatalkan); turunkan active selain yang terbaru
// (promosi SEBELUM penurunan agar Issue tidak pernah tanpa kunci); pensiunkan retiring yang melewati RetireAfter.
// Semua transisi bersyarat atomik di store sehingga beberapa instance aman berlomba. Notifikasi hanya bila ada
// perubahan oleh tick ini.
func (p *Promoter) Tick(ctx context.Context, now time.Time) error {
	r := p.ring
	if err := r.Reload(ctx); err != nil {
		return err
	}
	now = now.UTC().Truncate(time.Microsecond)
	changed := false
	if created, err := r.ensureActiveLoaded(ctx, now); err != nil {
		return err
	} else if created {
		// ensureActiveLoaded sudah memuat ulang dan mempublikasikan.
		return nil
	}

	keys := r.Keys()
	// 1. promosikan pending terbaru yang matang; batalkan pending yang lebih tua (tidak pernah menandatangani).
	var promote Key
	for _, k := range keys {
		if k.Status == StatusPending && !k.ActivatesAt().After(now) {
			promote = k // sorted: yang terakhir adalah yang terbaru
		}
	}
	promotedID := ""
	if !promote.IsZero() {
		ok, err := r.store.TransitionSigningKey(ctx, promote.ID, StatusPending, StatusActive, now)
		if err != nil {
			return fmt.Errorf("token: promosi kunci: %w", err)
		}
		if ok {
			changed, promotedID = true, promote.ID
			r.log.InfoContext(ctx, "token: kunci dipromosikan menjadi active", "kid", promote.ID)
		}
		for _, k := range keys {
			if k.Status == StatusPending && k.ID != promote.ID && k.CreatedAt.Before(promote.CreatedAt) {
				if ok, err := r.store.TransitionSigningKey(ctx, k.ID, StatusPending, StatusRetired, now); err != nil {
					return fmt.Errorf("token: batalkan kunci pending: %w", err)
				} else if ok {
					changed = true
				}
			}
		}
	}
	// 2. turunkan semua active selain yang terbaru.
	var newest Key
	for _, k := range keys {
		cand := k
		if k.ID == promotedID {
			cand.Status, cand.ActivatedAt = StatusActive, now
		}
		if cand.Status == StatusActive && (newest.IsZero() || cand.newerActiveThan(newest)) {
			newest = cand
		}
	}
	for _, k := range keys {
		if k.Status == StatusActive && k.ID != newest.ID {
			ok, err := r.store.TransitionSigningKey(ctx, k.ID, StatusActive, StatusRetiring, now)
			if err != nil {
				return fmt.Errorf("token: turunkan kunci active: %w", err)
			}
			changed = changed || ok
		}
	}
	// 3. pensiunkan retiring yang melewati umur access token + leeway.
	for _, k := range keys {
		if k.Status == StatusRetiring && !k.RetiringAt.IsZero() && !now.Before(k.RetiringAt.Add(RetireAfter)) {
			ok, err := r.store.TransitionSigningKey(ctx, k.ID, StatusRetiring, StatusRetired, now)
			if err != nil {
				return fmt.Errorf("token: pensiunkan kunci: %w", err)
			}
			changed = changed || ok
		}
	}
	if changed {
		if err := r.Reload(ctx); err != nil {
			return err
		}
		r.publish(ctx)
	}
	return nil
}
