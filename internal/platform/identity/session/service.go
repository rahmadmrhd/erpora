// Package session adalah sesi, refresh token berotasi, dan revokasi PRD-001 C1.6 (FR-102, FR-106; ADR-0013 §1-2;
// security-guidelines §3). Paket ini hanya memakai interface kecil (Store, dipenuhi *store.Store) dan tidak
// mengimpor GORM.
//
// # Refresh token
//
// String acak 32 byte (crypto/rand) dikodekan base64url tanpa padding (43 karakter); yang disimpan hanya SHA-256
// dari 32 byte itu. Umur 14 hari sliding (diperpanjang tiap rotasi) dan 90 hari absolut sejak login (sesi dan token
// tidak pernah melewatinya). Tiap pemakaian merotasi token dalam satu transaksi (Store.RotateRefreshToken: pemenang
// tunggal lewat UPDATE ... WHERE used_at IS NULL). Memakai ulang token yang sudah terpakai, atau kalah pada
// balapan rotasi, mencabut seluruh family beserta sesinya dan mencatat event keamanan (log terstruktur tanpa
// token). Semua kegagalan token menghasilkan ErrInvalidRefresh yang sama (401 generik); nilai token tidak pernah
// masuk log atau galat, dan RefreshToken menyamarkan dirinya pada fmt/slog.
//
// Upgrade (kenaikan sesi ke otp, PRD-001 Q27) mengganti refresh token belum terpakai milik sesi dengan satu token
// baru dalam family yang sama, atomik bersama kenaikan AMR. Token pra-upgrade menjadi "tidak dikenal": ditolak
// seragam tanpa mencabut sesi; leluhur yang sudah terpakai tetap memicu deteksi reuse.
//
// Rotate memeriksa header Origin lebih dulu (cek CSRF cookie refresh): hilang atau lintas-origin
// ditolak (ErrOriginNotAllowed) tanpa menyentuh token.
//
// Setelah token dikenal dan lolos deteksi reuse (reuse mendahului batas laju), Rotate membatasi laju per family (Config.RefreshLimiter, kunci guard.TargetKey("refresh", ...);
// ADR-0013 §4): melebihi batas mengembalikan *guard.RateLimitedError (429 + Retry-After) tanpa menyentuh token.
//
// # Revokasi
//
// Service menulis pencabutan ke DB lebih dulu, lalu menambahkannya ke RevocationList lokal dan menyiarkan sinyal
// lewat token.Notifier kanal ChannelSessionRevoked. Notifier tidak membawa muatan, jadi instance lain memuat ulang
// daftar dari DB (RevocationList.Subscribe). Lihat RevocationList untuk retensi (10 menit 30 detik).
//
// # Cara merakit (C1.10 dan C1.12; assemble.go tidak diubah oleh C1.6)
//
//	st := store.New(control, tenants)                         // *store.Store memenuhi session.Store dan RevokedLoader
//	list := session.NewRevocationList(session.RevocationConfig{Loader: st, Now: clock, Log: log})
//	tctx := tenancy.WithTenant(ctx, tenantID)                  // mode single-tenant
//	if err := list.Load(tctx); err != nil { /* gagal tertutup: jangan layani permintaan */ }
//	cancel := list.Subscribe(tctx, notifier, 0)                // notifier: token.Notifier (adaptor ke StateNotifier)
//	defer cancel()                                             // saat shutdown
//	origins, err := session.NewOriginChecker([]string{publicURL})
//	svc, err := session.New(session.Config{Store: st, Revocations: list, Notifier: notifier, Origin: origins, Now: clock, Log: log})
//	// middleware Authenticate memakai list.IsRevoked(claims.SessionID); modul lain yang mencabut sesi lewat
//	// store (reset/ganti sandi) memanggil svc.MarkRevoked(ctx, ids...) dengan id yang dikembalikan store.
//
// Penerbit access token (C1.10) wajib menerbitkan JWT hanya untuk sesi yang baru dikembalikan Start/Rotate, segera
// setelahnya, agar token tidak terbit setelah sesi dicabut (jendela retensi dihitung dari saat pencabutan).
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/tenancy"
)

const (
	// ChannelSessionRevoked adalah kanal Notifier yang dipublikasikan setiap pencabutan sesi.
	ChannelSessionRevoked = "identity.session_revoked"

	// RefreshSlidingTTL adalah umur token refresh (diperpanjang tiap rotasi, dibatasi umur absolut sesi).
	RefreshSlidingTTL = 14 * 24 * time.Hour
	// SessionAbsoluteTTL adalah umur absolut sesi sejak login.
	SessionAbsoluteTTL = 90 * 24 * time.Hour
	// RevocationRetention adalah lama entri sesi tercabut disimpan: umur access token + leeway verifier.
	RevocationRetention = token.AccessTokenTTL + token.Leeway

	refreshSecretBytes = 32
	refreshRawLen      = 43 // base64url tanpa padding dari 32 byte
	maxUserAgent       = 512
	maxIP              = 45
)

// Alasan pencabutan (kolom auth_sessions.revoke_reason, maksimum 64 karakter).
const (
	ReasonLogout        = "logout"
	ReasonUserRevoked   = "user_revoked"
	ReasonRevokeAll     = "revoke_all"
	ReasonRefreshReuse  = "refresh_reuse"
	ReasonUserDisabled  = "user_disabled"
	ReasonUserNotActive = "user_not_active"
)

var (
	// ErrInvalidRefresh adalah galat generik semua kegagalan token refresh (tidak dikenal, salah bentuk,
	// kedaluwarsa, terpakai/reuse, kalah balapan, sesi tercabut, pengguna tidak aktif); handler memetakannya ke
	// 401 tanpa membedakan sebab.
	ErrInvalidRefresh = errors.New("session: refresh token tidak valid")
	// ErrRefreshRateLimited adalah Cause pada *guard.RateLimitedError untuk batas laju refresh per family; jangan
	// dibandingkan langsung, gunakan guard.AsRateLimited.
	ErrRefreshRateLimited = errors.New("session: batas refresh per family tercapai")
	// ErrOriginNotAllowed berarti header Origin hilang atau tidak diizinkan pada permintaan refresh.
	ErrOriginNotAllowed = errors.New("session: origin tidak diizinkan")
	// ErrSessionNotFound berarti sesi tidak ada, milik tenant lain, atau milik pengguna lain (404).
	ErrSessionNotFound = errors.New("session: sesi tidak ditemukan")
	// ErrUserNotActive berarti pengguna bukan anggota aktif tenant atau berstatus disabled/locked.
	ErrUserNotActive = errors.New("session: pengguna tidak aktif")
	// ErrNoTenant berarti context tidak membawa tenant.
	ErrNoTenant = errors.New("session: context tanpa tenant")
	// ErrInvalidInput berarti masukan Start tidak memenuhi aturan bentuk.
	ErrInvalidInput = errors.New("session: masukan tidak valid")
	// ErrAlreadyUpgraded berarti Upgrade tidak menerapkan apa pun: sesi sudah memuat faktor itu, atau upgrade
	// serentak lain menang (tepat satu pemanggil berhasil). Tidak ada token baru yang terbit.
	ErrAlreadyUpgraded = errors.New("session: sesi sudah dinaikkan")
)

// Session adalah sesi per perangkat (id = klaim sid).
type Session = store.Session

// RefreshToken adalah token refresh yang baru diterbitkan. Value adalah nilai mentah untuk cookie HttpOnly
// (tidak pernah di body respons) dan tidak pernah dicatat: fmt dan slog menyamarkannya.
type RefreshToken struct {
	Value     string
	FamilyID  string
	ExpiresAt time.Time
}

func (RefreshToken) redacted() string { return "session.RefreshToken{value:****}" }

// String dan GoString menyamarkan nilai token.
func (r RefreshToken) String() string   { return r.redacted() }
func (r RefreshToken) GoString() string { return r.redacted() }

// Format menyamarkan untuk semua verba (%v %+v %#v %s %q ...).
func (r RefreshToken) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(r.redacted())) }

// LogValue menyamarkan pada slog.
func (r RefreshToken) LogValue() slog.Value { return slog.StringValue(r.redacted()) }

// Store adalah persistensi yang dibutuhkan Service (dipenuhi *store.Store). Kontrak atomisitasnya sama dengan
// store: RotateRefreshToken mengembalikan true untuk tepat satu pemanggil serentak.
type Store interface {
	RevokedLoader
	GetUser(ctx context.Context, t tenancy.TenantID, id string) (store.User, error)
	GetMembership(ctx context.Context, t tenancy.TenantID, userID string) (store.Membership, error)
	CreateSessionWithRefresh(ctx context.Context, t tenancy.TenantID, sess store.Session, rt store.RefreshToken) error
	GetSession(ctx context.Context, t tenancy.TenantID, id string) (store.Session, error)
	FindRefreshTokenByHash(ctx context.Context, t tenancy.TenantID, hash []byte) (store.RefreshToken, error)
	RotateRefreshToken(ctx context.Context, t tenancy.TenantID, usedID string, at time.Time, next store.RefreshToken) (bool, error)
	RevokeSession(ctx context.Context, t tenancy.TenantID, id, reason string, at time.Time) (bool, error)
	RevokeUserSessions(ctx context.Context, t tenancy.TenantID, userID, exceptSessionID, reason string, at time.Time) ([]string, error)
	RevokeSessionsOfFamily(ctx context.Context, t tenancy.TenantID, familyID, reason string, at time.Time) ([]string, error)
	UpgradeSession(ctx context.Context, t tenancy.TenantID, sessionID string, oldAMR, newAMR []string, next store.RefreshToken) (bool, error)
}

var _ Store = (*store.Store)(nil)

// Config mengonfigurasi Service.
type Config struct {
	Store       Store
	Revocations *RevocationList
	// Notifier menyiarkan pencabutan ke instance lain (opsional; nil = hanya lokal).
	Notifier token.Notifier
	// Origin memeriksa header Origin pada Rotate (wajib; lihat NewOriginChecker).
	Origin OriginChecker
	// RefreshLimiter membatasi laju Rotate per family (ADR-0013 §4: identitas sasaran endpoint refresh publik; kunci
	// guard.TargetKey("refresh", tenant, family)). Nil = bawaan guard.DefaultTargetLimiterConfig. Diperiksa SETELAH token
	// dikenal dan SEBELUM rotasi; penolakan tidak mengonsumsi token dan tidak mencabut sesi.
	RefreshLimiter *guard.PrincipalLimiter
	Now            func() time.Time
	Log            *slog.Logger
}

// Service mengelola sesi, refresh token, dan revokasi. Aman dipakai serentak.
type Service struct {
	st      Store
	list    *RevocationList
	notify  token.Notifier
	origin  OriginChecker
	refresh *guard.PrincipalLimiter
	now     func() time.Time
	log     *slog.Logger
}

// New membuat Service.
func New(cfg Config) (*Service, error) {
	if cfg.Store == nil || cfg.Revocations == nil || cfg.Origin == nil {
		return nil, errors.New("session: Store, Revocations, dan Origin wajib")
	}
	s := &Service{st: cfg.Store, list: cfg.Revocations, notify: cfg.Notifier, origin: cfg.Origin, refresh: cfg.RefreshLimiter, now: cfg.Now, log: cfg.Log}
	if s.now == nil {
		s.now = time.Now
	}
	if s.refresh == nil {
		l, err := guard.NewPrincipalLimiter(guard.DefaultTargetLimiterConfig())
		if err != nil {
			return nil, wrap("pembatas refresh", err)
		}
		s.refresh = l
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	return s, nil
}

func (s *Service) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }

func tenantOf(ctx context.Context) (tenancy.TenantID, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return tenancy.TenantID{}, ErrNoTenant
	}
	return t, nil
}

func wrap(op string, err error) error { return fmt.Errorf("session: %s: %w", op, err) }

// StartInput adalah masukan Start (setelah kredensial dan MFA pengguna diverifikasi pemanggil). Tenant diambil
// dari context. Nilai kosong: ClientKind web, AuthMethod password, AMR ["pwd"].
type StartInput struct {
	UserID     string
	ClientKind string
	AuthMethod string
	AMR        []string
	UserAgent  string
	IP         string
}

func newRefreshSecret() (raw string, hash []byte, err error) {
	var b [refreshSecretBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, errors.New("session: pembangkit acak gagal")
	}
	sum := sha256.Sum256(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:]), sum[:], nil
}

// hashRaw memvalidasi bentuk kanonik token (43 karakter base64url, 32 byte) dan mengembalikan SHA-256 dari
// 32 byte itu. Selain bentuk itu tidak pernah mencapai DB.
func hashRaw(raw string) ([]byte, bool) {
	if len(raw) != refreshRawLen {
		return nil, false
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		isAlnum := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if !isAlnum && c != '-' && c != '_' {
			return nil, false
		}
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(b) != refreshSecretBytes {
		return nil, false
	}
	sum := sha256.Sum256(b)
	return sum[:], true
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", errors.New("session: pembangkit id gagal")
	}
	return id.String(), nil
}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// userActive: anggota aktif tenant dengan status users.active (syarat membuat sesi baru: login).
func (s *Service) userActive(ctx context.Context, t tenancy.TenantID, userID string) (bool, error) {
	return s.userUsable(ctx, t, userID, false)
}

// userUsable: anggota aktif tenant dengan status users.active; allowLocked juga menerima users.locked. Status locked
// hanya menolak LOGIN baru dan TIDAK mencabut sesi yang sudah ada (PRD-001 Q30 butir 4, 4 Nov 2026); pencabutan sesi
// khusus status disabled (dan keanggotaan tenant nonaktif), jadi Rotate memakai allowLocked=true sedangkan Start tidak.
func (s *Service) userUsable(ctx context.Context, t tenancy.TenantID, userID string, allowLocked bool) (bool, error) {
	u, err := s.st.GetUser(ctx, t, userID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, wrap("muat pengguna", err)
	}
	if u.Status != store.UserActive && (!allowLocked || u.Status != store.UserLocked) {
		return false, nil
	}
	m, err := s.st.GetMembership(ctx, t, userID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, wrap("muat keanggotaan", err)
	}
	return m.Status == store.MembershipActive, nil
}

// Start membuat sesi dan token refresh akarnya secara atomik (login). Pengguna harus aktif (ErrUserNotActive).
func (s *Service) Start(ctx context.Context, in StartInput) (Session, RefreshToken, error) {
	t, err := tenantOf(ctx)
	if err != nil {
		return Session{}, RefreshToken{}, err
	}
	active, err := s.userActive(ctx, t, in.UserID)
	if err != nil {
		return Session{}, RefreshToken{}, err
	}
	if !active {
		return Session{}, RefreshToken{}, ErrUserNotActive
	}
	kind, method, amr := in.ClientKind, in.AuthMethod, in.AMR
	if kind == "" {
		kind = store.ClientWeb
	}
	if method == "" {
		method = "password"
	}
	if len(amr) == 0 {
		amr = []string{"pwd"}
	}
	ip := in.IP
	if len(ip) > maxIP {
		ip = ""
	}
	now := s.clock()
	sid, err1 := newID()
	rid, err2 := newID()
	fam, err3 := newID()
	raw, hash, err4 := newRefreshSecret()
	if err := errors.Join(err1, err2, err3, err4); err != nil {
		return Session{}, RefreshToken{}, err
	}
	sess := store.Session{
		ID: sid, UserID: in.UserID, ClientKind: kind, AuthMethod: method, AMR: amr,
		UserAgent: truncateUTF8(in.UserAgent, maxUserAgent), IP: ip,
		CreatedAt: now, LastSeenAt: now, AbsoluteExpiresAt: now.Add(SessionAbsoluteTTL),
	}
	rt := store.RefreshToken{ID: rid, SessionID: sid, FamilyID: fam, TokenHash: hash, CreatedAt: now, ExpiresAt: now.Add(RefreshSlidingTTL)}
	if err := s.st.CreateSessionWithRefresh(ctx, t, sess, rt); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return Session{}, RefreshToken{}, ErrUserNotActive
		case errors.Is(err, store.ErrInvalidArgument):
			return Session{}, RefreshToken{}, ErrInvalidInput
		}
		return Session{}, RefreshToken{}, wrap("buat sesi", err)
	}
	return sess, RefreshToken{Value: raw, FamilyID: fam, ExpiresAt: rt.ExpiresAt}, nil
}

// Rotate menukar token refresh dengan token baru (rotasi tiap pakai). origin adalah nilai header Origin
// permintaan (kosong bila hilang). Semua kegagalan token mengembalikan ErrInvalidRefresh; Origin salah
// mengembalikan ErrOriginNotAllowed tanpa menyentuh token. Reuse atau kalah balapan mencabut family dan sesinya.
// Pengguna harus masih aktif atau locked (disabled atau keanggotaan nonaktif: semua sesinya dicabut; locked tidak
// mencabut sesi, PRD-001 Q30 butir 4).
func (s *Service) Rotate(ctx context.Context, rawRefresh, origin string) (Session, RefreshToken, error) {
	if s.origin == nil || !s.origin(origin) {
		return Session{}, RefreshToken{}, ErrOriginNotAllowed
	}
	hash, ok := hashRaw(rawRefresh)
	if !ok {
		return Session{}, RefreshToken{}, ErrInvalidRefresh
	}
	t, err := tenantOf(ctx)
	if err != nil {
		return Session{}, RefreshToken{}, err
	}
	now := s.clock()
	tok, err := s.st.FindRefreshTokenByHash(ctx, t, hash)
	if errors.Is(err, store.ErrNotFound) {
		return Session{}, RefreshToken{}, ErrInvalidRefresh
	}
	if err != nil {
		return Session{}, RefreshToken{}, wrap("muat refresh token", err)
	}
	// Deteksi reuse MENDAHULUI batas laju family (ADR-0013 §2 dan §4): bila tidak, penyerang yang merotasi token curian
	// pada laju isi ulang menjaga bucket family hampir kosong sehingga pemakaian token lama oleh pengguna sah selalu 429
	// dan reuse tak pernah terdeteksi. reuse() idempoten dan murah; pembanjir token terpakai tetap dibatasi limiter IP.
	if tok.UsedAt != nil {
		return Session{}, RefreshToken{}, s.reuse(ctx, t, tok, "used_token", now)
	}
	// Batas laju per family (ADR-0013 §4): hanya untuk token dikenal dan belum terpakai, sebelum rotasi. Penolakan:
	// 429 + Retry-After via *guard.RateLimitedError; token tidak terpakai.
	if ok, retry := s.refresh.Allow(guard.TargetKey("refresh", t, tok.FamilyID)); !ok {
		return Session{}, RefreshToken{}, &guard.RateLimitedError{RetryAfter: retry, Cause: ErrRefreshRateLimited}
	}
	sess, err := s.st.GetSession(ctx, t, tok.SessionID)
	if errors.Is(err, store.ErrNotFound) {
		return Session{}, RefreshToken{}, ErrInvalidRefresh
	}
	if err != nil {
		return Session{}, RefreshToken{}, wrap("muat sesi", err)
	}
	if sess.RevokedAt != nil || !now.Before(sess.AbsoluteExpiresAt) || !now.Before(tok.ExpiresAt) {
		return Session{}, RefreshToken{}, ErrInvalidRefresh
	}
	active, err := s.userUsable(ctx, t, sess.UserID, true) // locked: sesi yang ada tetap hidup (Q30 butir 4)
	if err != nil {
		return Session{}, RefreshToken{}, err
	}
	if !active {
		// pengaman bila hook OnUserDisabled terlewat: cabut semua sesi pengguna itu, bukan hanya yang ini
		if err := s.revokeAll(ctx, sess.UserID, ReasonUserNotActive); err != nil {
			return Session{}, RefreshToken{}, err
		}
		return Session{}, RefreshToken{}, ErrInvalidRefresh
	}
	rid, err1 := newID()
	raw, nh, err2 := newRefreshSecret()
	if err := errors.Join(err1, err2); err != nil {
		return Session{}, RefreshToken{}, err
	}
	exp := now.Add(RefreshSlidingTTL)
	if exp.After(sess.AbsoluteExpiresAt) {
		exp = sess.AbsoluteExpiresAt
	}
	next := store.RefreshToken{ID: rid, SessionID: sess.ID, FamilyID: tok.FamilyID, ParentID: tok.ID, TokenHash: nh, CreatedAt: now, ExpiresAt: exp}
	won, err := s.st.RotateRefreshToken(ctx, t, tok.ID, now, next)
	if err != nil {
		return Session{}, RefreshToken{}, wrap("rotasi refresh token", err)
	}
	if !won {
		// Token lenyap di antara pemuatan dan rotasi (Upgrade menggantikannya): bukan reuse, sesi tidak dicabut.
		if _, ferr := s.st.FindRefreshTokenByHash(ctx, t, hash); errors.Is(ferr, store.ErrNotFound) {
			return Session{}, RefreshToken{}, ErrInvalidRefresh
		}
		return Session{}, RefreshToken{}, s.reuse(ctx, t, tok, "lost_race", now)
	}
	return sess, RefreshToken{Value: raw, FamilyID: tok.FamilyID, ExpiresAt: exp}, nil
}

// Upgrade menaikkan sesi userID dengan faktor addAMR (mis. "otp" setelah MFA terverifikasi; pemanggil yang
// memverifikasinya) dan MENGGANTI token refresh sesi dalam satu transaksi atomik (Store.UpgradeSession): token refresh
// pra-upgrade (milik cookie lama, mungkin tercuri sebelum MFA) mati dan token baru pada family yang sama
// dikembalikan untuk cookie baru. Token pra-upgrade ditolak sebagai ErrInvalidRefresh tanpa mencabut sesi (bukan
// reuse); reuse leluhur yang sudah terpakai tetap mencabut family. userID HARUS berasal dari sumber tepercaya (Principal);
// sesi milik pengguna lain, tak ada, tercabut, atau kedaluwarsa menghasilkan ErrSessionNotFound. Sesi yang sudah memuat
// addAMR atau kalah balapan upgrade: ErrAlreadyUpgraded (tepat satu pemanggil serentak berhasil).
func (s *Service) Upgrade(ctx context.Context, userID, sessionID, addAMR string) (Session, RefreshToken, error) {
	if addAMR == "" || len(addAMR) > 32 {
		return Session{}, RefreshToken{}, ErrInvalidInput
	}
	t, err := tenantOf(ctx)
	if err != nil {
		return Session{}, RefreshToken{}, err
	}
	now := s.clock()
	sess, err := s.st.GetSession(ctx, t, sessionID)
	if errors.Is(err, store.ErrNotFound) {
		return Session{}, RefreshToken{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, RefreshToken{}, wrap("muat sesi", err)
	}
	if userID == "" || sess.UserID != userID || sess.RevokedAt != nil || !now.Before(sess.AbsoluteExpiresAt) {
		return Session{}, RefreshToken{}, ErrSessionNotFound
	}
	if slices.Contains(sess.AMR, addAMR) {
		return Session{}, RefreshToken{}, ErrAlreadyUpgraded
	}
	newAMR := append(slices.Clone(sess.AMR), addAMR)
	rid, err1 := newID()
	fam, err2 := newID() // cadangan; Store memakai family yang sudah ada
	raw, nh, err3 := newRefreshSecret()
	if err := errors.Join(err1, err2, err3); err != nil {
		return Session{}, RefreshToken{}, err
	}
	exp := now.Add(RefreshSlidingTTL)
	if exp.After(sess.AbsoluteExpiresAt) {
		exp = sess.AbsoluteExpiresAt
	}
	next := store.RefreshToken{ID: rid, SessionID: sess.ID, FamilyID: fam, TokenHash: nh, CreatedAt: now, ExpiresAt: exp}
	won, err := s.st.UpgradeSession(ctx, t, sess.ID, sess.AMR, newAMR, next)
	if err != nil {
		if errors.Is(err, store.ErrInvalidArgument) {
			return Session{}, RefreshToken{}, ErrInvalidInput
		}
		return Session{}, RefreshToken{}, wrap("naikkan sesi", err)
	}
	if !won {
		cur, gerr := s.st.GetSession(ctx, t, sess.ID)
		if gerr == nil && cur.RevokedAt != nil {
			return Session{}, RefreshToken{}, ErrSessionNotFound
		}
		return Session{}, RefreshToken{}, ErrAlreadyUpgraded
	}
	sess.AMR = newAMR
	// family token baru = family sesi (Store menentukannya); dibaca ulang agar RefreshToken.FamilyID akurat.
	famID := fam
	if got, ferr := s.st.FindRefreshTokenByHash(ctx, t, nh); ferr == nil {
		famID = got.FamilyID
	}
	return sess, RefreshToken{Value: raw, FamilyID: famID, ExpiresAt: exp}, nil
}

// reuse mencabut seluruh family (dan sesinya), mencatat event keamanan tanpa token, dan mengembalikan
// ErrInvalidRefresh (atau galat bila pencabutan gagal sehingga klien tidak mengira sudah aman).
func (s *Service) reuse(ctx context.Context, t tenancy.TenantID, tok store.RefreshToken, how string, now time.Time) error {
	ids, err := s.st.RevokeSessionsOfFamily(ctx, t, tok.FamilyID, ReasonRefreshReuse, now)
	if err != nil {
		return wrap("cabut family", err)
	}
	s.announce(ctx, ids, now)
	level := slog.LevelInfo
	if len(ids) > 0 {
		level = slog.LevelWarn
	}
	s.log.Log(ctx, level, "refresh token dipakai ulang atau kalah balapan", "event", "identity.refresh_reuse", "how", how,
		"session_id", tok.SessionID, "family_id", tok.FamilyID, "tenant_id", t.String(), "revoked_sessions", len(ids))
	return ErrInvalidRefresh
}

// announce mencatat sesi tercabut di daftar lokal sampai at+RevocationRetention dan menyiarkan sinyal.
func (s *Service) announce(ctx context.Context, ids []string, at time.Time) {
	if len(ids) == 0 {
		return
	}
	until := at.Add(RevocationRetention)
	for _, id := range ids {
		s.list.Add(id, until)
	}
	if s.notify != nil {
		s.notify.Publish(ctx, ChannelSessionRevoked)
	}
}

// MarkRevoked mencatat dan menyiarkan sesi yang sudah dicabut lewat store oleh pihak lain (mis. reset/ganti
// sandi yang mengembalikan id sesi tercabut) agar instance ini dan lainnya menolaknya segera.
func (s *Service) MarkRevoked(ctx context.Context, sessionIDs ...string) {
	s.announce(ctx, sessionIDs, s.clock())
}

func (s *Service) revokeSession(ctx context.Context, t tenancy.TenantID, sessionID, reason string, now time.Time) error {
	ok, err := s.st.RevokeSession(ctx, t, sessionID, reason, now)
	if err != nil {
		if errors.Is(err, store.ErrInvalidArgument) {
			return ErrInvalidInput
		}
		return wrap("cabut sesi", err)
	}
	if ok {
		s.announce(ctx, []string{sessionID}, now)
		return nil
	}
	// sudah tercabut atau tidak ada
	sess, err := s.st.GetSession(ctx, t, sessionID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrSessionNotFound
	}
	if err != nil {
		return wrap("muat sesi", err)
	}
	if sess.RevokedAt != nil { // pastikan tercatat di daftar lokal (pencabut lain mungkin di instance lain)
		s.list.Add(sessionID, sess.RevokedAt.Add(RevocationRetention))
	}
	return nil
}

// Revoke mencabut satu sesi (idempoten: sudah tercabut berarti nil). sessionID HARUS berasal dari sumber tepercaya
// (klaim sid principal, atau id yang sudah dicek kepemilikannya); id dari klien wajib lewat RevokeOwn.
// ErrSessionNotFound bila sesi tidak ada pada tenant context.
func (s *Service) Revoke(ctx context.Context, sessionID, reason string) error {
	t, err := tenantOf(ctx)
	if err != nil {
		return err
	}
	return s.revokeSession(ctx, t, sessionID, reason, s.clock())
}

// RevokeOwn mencabut sesi yang id-nya diberikan klien: hanya bila milik userID (principal pemanggil); sesi milik
// pengguna lain atau tidak ada menghasilkan ErrSessionNotFound (404, tanpa membedakan; PRD-001 FR-119).
func (s *Service) RevokeOwn(ctx context.Context, userID, sessionID string) error {
	t, err := tenantOf(ctx)
	if err != nil {
		return err
	}
	sess, err := s.st.GetSession(ctx, t, sessionID)
	if errors.Is(err, store.ErrNotFound) {
		return ErrSessionNotFound
	}
	if err != nil {
		return wrap("muat sesi", err)
	}
	if userID == "" || sess.UserID != userID {
		return ErrSessionNotFound
	}
	return s.revokeSession(ctx, t, sessionID, ReasonUserRevoked, s.clock())
}

func (s *Service) revokeAll(ctx context.Context, userID, reason string) error {
	t, err := tenantOf(ctx)
	if err != nil {
		return err
	}
	now := s.clock()
	ids, err := s.st.RevokeUserSessions(ctx, t, userID, "", reason, now)
	if err != nil {
		return wrap("cabut sesi pengguna", err)
	}
	s.announce(ctx, ids, now)
	return nil
}

// RevokeAllForUser mencabut semua sesi aktif pengguna pada tenant context. userID dari sumber tepercaya.
func (s *Service) RevokeAllForUser(ctx context.Context, userID string) error {
	return s.revokeAll(ctx, userID, ReasonRevokeAll)
}

// OnUserDisabled adalah hook yang dipanggil saat pengguna dinonaktifkan (status disabled): semua sesinya dicabut segera
// (Rotate juga menolak pengguna yang tidak aktif sebagai pengaman bila hook terlewat). Pengguna yang hanya dikunci
// (locked) TIDAK dicabut sesinya (PRD-001 Q30 butir 4).
func (s *Service) OnUserDisabled(ctx context.Context, userID string) error {
	return s.revokeAll(ctx, userID, ReasonUserDisabled)
}
