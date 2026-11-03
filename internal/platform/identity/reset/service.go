package reset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
	sdkmail "erpora/sdk/mail"
)

// Konstanta alur reset (PRD-001 FR-101, ADR-0013).
const (
	// TokenTTL adalah umur tautan reset.
	TokenTTL = 30 * time.Minute
	// tokenBytes adalah panjang token acak (256 bit), dikodekan base64url tanpa padding (43 karakter).
	tokenBytes = 32
	// DefaultResetPath adalah jalur halaman reset di frontend; token dibawa di FRAGMEN (#token=...), bukan query:
	// fragmen tidak dikirim ke server, tidak masuk log akses/proksi, dan tidak ikut header Referer.
	DefaultResetPath = "/reset-password"
	// DefaultMinRequestDuration adalah lantai waktu jawaban Request (penyeragaman waktu, pola guard.Authenticator).
	DefaultMinRequestDuration = 100 * time.Millisecond
	// DefaultMaxInFlight membatasi pekerjaan pengiriman email yang berjalan serentak.
	DefaultMaxInFlight = 8
	// DefaultJobTimeout membatasi satu pekerjaan latar (cari akun, simpan tautan, kirim email).
	DefaultJobTimeout = 90 * time.Second
	maxNameRunes      = 100
	maxEmailLen       = 254
	defaultLocale     = "id"
	appName           = "ERPora"
)

// Galat layanan. ErrInvalidToken seragam untuk token cacat, tak dikenal, kedaluwarsa, atau sudah terpakai (kalah
// balapan pun sama): klien tidak dapat membedakan sebabnya.
var (
	ErrInvalidToken = errors.New("reset: tautan reset tidak valid atau sudah tidak berlaku")
	// ErrNoTenant: context tanpa tenant (gagal tertutup; kegagalan infrastruktur, bukan jawaban untuk klien).
	ErrNoTenant = errors.New("reset: context tanpa tenant")
	// ErrRateLimited adalah Cause pada *guard.RateLimitedError untuk batas laju forgot (IP atau email); jangan
	// dikirim ke klien (hanya 429 + Retry-After).
	ErrRateLimited = errors.New("reset: batas permintaan reset tercapai")
)

// PolicyError: sandi baru ditolak kebijakan FR-101. Tautan TIDAK terpakai. Tidak pernah memuat sandi.
type PolicyError struct{ Violations []password.Violation }

func (e *PolicyError) Error() string {
	codes := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		codes[i] = string(v.Code)
	}
	return "reset: sandi baru tidak memenuhi kebijakan: " + strings.Join(codes, ",")
}

// Store adalah bagian *store.Store yang dipakai layanan (paket ini bebas GORM).
type Store interface {
	FindUserByEmail(ctx context.Context, t tenancy.TenantID, email string) (store.User, error)
	GetPreferences(ctx context.Context, t tenancy.TenantID, userID string) (store.Preferences, error)
	InvalidatePasswordResets(ctx context.Context, t tenancy.TenantID, userID string, at time.Time) (int64, error)
	InsertPasswordReset(ctx context.Context, t tenancy.TenantID, r store.PasswordReset) error
	FindPasswordResetByHash(ctx context.Context, t tenancy.TenantID, hash []byte) (store.PasswordReset, error)
	ResetPassword(ctx context.Context, t tenancy.TenantID, resetID, newHash string, at time.Time) (store.ResetResult, bool, error)
}

// Sessions menyiarkan sesi yang dicabut store (dipenuhi *session.Service).
type Sessions interface {
	MarkRevoked(ctx context.Context, sessionIDs ...string)
}

// Hasher menghitung hash sandi lewat pembatas hash serentak (dipenuhi *guard.HashGate).
type Hasher interface {
	Hash(ctx context.Context, h *password.Hasher, plain string) (phc string, err error)
}

// Config merakit Service.
type Config struct {
	Store    Store          // wajib
	Sessions Sessions       // wajib
	Gate     Hasher         // wajib; biasanya *guard.HashGate yang sama dengan login
	Sender   sdkmail.Sender // wajib; driver smtp atau log (internal/platform/mail)
	// PublicURL adalah sumber URL publik aplikasi untuk tautan di email: settings platform (settings.Service). WAJIB.
	// Dibaca tiap permintaan dan divalidasi (http(s) absolut tanpa userinfo, query, fragmen); tidak pernah diambil dari
	// header permintaan (cegah host header injection).
	PublicURL PublicURLSource
	// ResetPath: jalur halaman reset di frontend (bawaan DefaultResetPath).
	ResetPath string
	// Argon2 adalah parameter hash (config.Config.Argon2); nol = password.DefaultParams().
	Argon2 password.Params
	// Email dan IP adalah pembatas laju forgot (instans TERPISAH dan lebih ketat dari login). Nil = bawaan
	// DefaultEmailLimiterConfig dan DefaultIPLimiterConfig.
	Email *guard.PrincipalLimiter
	IP    *guard.IPLimiter
	// MinRequestDuration: lantai waktu jawaban Request (0 = DefaultMinRequestDuration).
	MinRequestDuration time.Duration
	// MaxInFlight: batas pekerjaan latar serentak (0 = DefaultMaxInFlight). Penuh: pekerjaan dibuang (dicatat), jawaban
	// ke klien tetap sama.
	MaxInFlight int
	// JobTimeout: batas satu pekerjaan latar (0 = DefaultJobTimeout).
	JobTimeout time.Duration
	// Now (default time.Now) dan Sleep (default timer yang menghormati ctx) dapat diinjeksi untuk tes.
	Now    func() time.Time
	Sleep  func(ctx context.Context, d time.Duration) error
	Logger *slog.Logger // default slog.Default()
}

// DefaultEmailLimiterConfig: 3 permintaan forgot sekaligus per email (per tenant), lalu 1 per 10 menit.
func DefaultEmailLimiterConfig() guard.LimiterConfig {
	return guard.LimiterConfig{Burst: 3, Every: 10 * time.Minute}
}

// DefaultIPLimiterConfig: 10 permintaan forgot sekaligus per IP (IPv6 per /64), lalu 1 per 30 detik.
func DefaultIPLimiterConfig() guard.LimiterConfig {
	return guard.LimiterConfig{Burst: 10, Every: 30 * time.Second}
}

// Service menjalankan permintaan dan penyelesaian reset sandi. Aman dipakai serentak.
type Service struct {
	st        Store
	sessions  Sessions
	gate      Hasher
	sender    sdkmail.Sender
	urls      PublicURLSource
	resetPath string
	hasher    *password.Hasher
	email     *guard.PrincipalLimiter
	ip        *guard.IPLimiter
	floor     time.Duration
	jobTO     time.Duration
	now       func() time.Time
	sleep     func(ctx context.Context, d time.Duration) error
	log       *slog.Logger
	sem       chan struct{}
	wg        sync.WaitGroup
	tpls      templates
}

// New membuat Service. Galat bila dependensi wajib kosong atau ResetPath tidak sah.
func New(cfg Config) (*Service, error) {
	switch {
	case cfg.Store == nil:
		return nil, errors.New("reset: Config.Store wajib")
	case cfg.Sessions == nil:
		return nil, errors.New("reset: Config.Sessions wajib")
	case cfg.Gate == nil:
		return nil, errors.New("reset: Config.Gate wajib")
	case cfg.Sender == nil:
		return nil, errors.New("reset: Config.Sender wajib")
	}
	if cfg.PublicURL == nil {
		return nil, errors.New("reset: Config.PublicURL wajib")
	}
	resetPath, err := validateResetPath(cfg.ResetPath)
	if err != nil {
		return nil, err
	}
	params := cfg.Argon2
	if params == (password.Params{}) {
		params = password.DefaultParams()
	}
	s := &Service{
		st: cfg.Store, sessions: cfg.Sessions, gate: cfg.Gate, sender: cfg.Sender, urls: cfg.PublicURL, resetPath: resetPath,
		hasher: &password.Hasher{Params: params}, email: cfg.Email, ip: cfg.IP, floor: cfg.MinRequestDuration,
		jobTO: cfg.JobTimeout, now: cfg.Now, sleep: cfg.Sleep, log: cfg.Logger,
	}
	if s.email == nil {
		if s.email, err = guard.NewPrincipalLimiter(DefaultEmailLimiterConfig()); err != nil {
			return nil, err
		}
	}
	if s.ip == nil {
		if s.ip, err = guard.NewIPLimiter(DefaultIPLimiterConfig()); err != nil {
			return nil, err
		}
	}
	if s.floor <= 0 {
		s.floor = DefaultMinRequestDuration
	}
	if s.jobTO <= 0 {
		s.jobTO = DefaultJobTimeout
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.sleep == nil {
		s.sleep = sleepCtx
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	inflight := cfg.MaxInFlight
	if inflight <= 0 {
		inflight = DefaultMaxInFlight
	}
	s.sem = make(chan struct{}, inflight)
	if s.tpls, err = loadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublicURLSource menyediakan URL publik aplikasi (dipenuhi *settings.Service: PublicURL(ctx) mengembalikan
// settings.ErrNotConfigured bila belum diisi). Konteks membawa tenant.
type PublicURLSource interface {
	PublicURL(ctx context.Context) (string, error)
}

// validateResetPath memeriksa jalur halaman reset di frontend (bawaan DefaultResetPath).
func validateResetPath(p string) (string, error) {
	if p == "" {
		return DefaultResetPath, nil
	}
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "?#\\ \t\r\n") || strings.Contains(p, "//") {
		return "", errors.New("reset: Config.ResetPath harus diawali / tanpa query, fragmen, atau spasi")
	}
	return p, nil
}

// errBadPublicURL: URL publik dari sumber tidak sah (nilai tidak pernah dimasukkan ke galat atau log).
var errBadPublicURL = errors.New("URL publik tidak sah")

// linkBase membaca URL publik dari sumber, memvalidasinya, dan mengembalikan basis tautan (URL + jalur reset, tanpa
// garis miring ganda). Fragmen token ditambahkan saat tautan dibuat.
func (s *Service) linkBase(ctx context.Context) (string, error) {
	raw, err := s.urls.PublicURL(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return "", errBadPublicURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", errBadPublicURL
	}
	return strings.TrimRight(u.String(), "/") + s.resetPath, nil
}

// Wait menunggu pekerjaan latar pengiriman email yang sedang berjalan (tes dan penutupan proses).
func (s *Service) Wait() { s.wg.Wait() }

type clientIPKey struct{}

// WithClientIP memasang IP klien (hasil resolusi proxy tepercaya oleh middleware) pada context untuk batas laju per IP
// pada Request; pola yang sama dengan mfa.WithClientIP. Tanpa IP, permintaan berbagi satu ember "unknown" (gagal-tertutup).
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

func clientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return guard.NormalizeIP(ip)
}

// Request memproses permintaan lupa sandi. Dari luar selalu sukses (nil) tanpa membedakan akun ada, tak ada, atau
// nonaktif, dan waktu jawabannya seragam; satu-satunya galat bermakna bagi klien adalah *guard.RateLimitedError
// (guard.AsRateLimited: 429 + Retry-After), yang bergantung pada IP dan email yang DIMINTA, bukan pada keberadaan
// akun. Galat lain (mis. ErrNoTenant) adalah kegagalan infrastruktur.
//
// Alur: batas laju per IP lalu per email (sebelum pekerjaan apa pun) -> pekerjaan latar terbatas (cari akun, batalkan
// tautan lama, simpan hash token baru, render templat sesuai locale, kirim) -> tunggu sampai lantai MinRequestDuration.
// Pekerjaan yang bergantung pada keberadaan akun (DB, render, SMTP yang bisa berdetik-detik) sengaja dilepas dari
// jawaban agar waktu respons tidak membocorkan akun; kegagalan di latar hanya dicatat (tanpa email, token, atau tautan).
// IP klien dari WithClientIP. locale (mis. dari Accept-Language) dipakai bila pengguna belum punya preferensi.
func (s *Service) Request(ctx context.Context, email, locale string) error {
	start := s.now()
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return ErrNoTenant
	}
	if allowed, d := s.ip.Allow(clientIP(ctx)); !allowed {
		return &guard.RateLimitedError{RetryAfter: d, Cause: ErrRateLimited}
	}
	email = store.NormalizeEmail(email)
	if allowed, d := s.email.Allow(guard.EmailTargetKey("forgot", t, email)); !allowed {
		return &guard.RateLimitedError{RetryAfter: d, Cause: ErrRateLimited}
	}
	s.dispatch(ctx, t, email, locale)
	if rem := s.floor - s.now().Sub(start); rem > 0 {
		_ = s.sleep(ctx, rem) // ctx batal: pemanggil sudah pergi
	}
	return nil
}

// dispatch menjalankan pekerjaan latar bila ada slot; penuh = dibuang (dicatat) tanpa memengaruhi jawaban.
func (s *Service) dispatch(ctx context.Context, t tenancy.TenantID, email, locale string) {
	select {
	case s.sem <- struct{}{}:
	default:
		s.log.WarnContext(ctx, "reset: antrean pengiriman penuh; permintaan forgot dibuang")
		return
	}
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.jobTO)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { <-s.sem }()
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				s.log.ErrorContext(jobCtx, "reset: panik pada pekerjaan pengiriman", "panic", fmt.Sprint(r))
			}
		}()
		s.process(jobCtx, t, email, locale)
	}()
}

// process adalah pekerjaan latar: tidak pernah mencatat email, token, atau tautan.
func (s *Service) process(ctx context.Context, t tenancy.TenantID, email, locale string) {
	if email == "" || len(email) > maxEmailLen {
		return
	}
	u, err := s.st.FindUserByEmail(ctx, t, email)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.ErrorContext(ctx, "reset: gagal mencari akun", "error", err)
		}
		return
	}
	if u.Status != store.UserActive && u.Status != store.UserLocked {
		return // dinonaktifkan: tidak ada email; dari luar tak terbedakan
	}
	// Tanpa URL publik yang sah tautan tidak dapat dibangun: jangan membatalkan tautan lama atau menyimpan yang baru.
	// Dari luar tetap tak terbedakan (jawaban seragam); operator melihat peringatan tanpa data sensitif.
	base, err := s.linkBase(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "reset: URL publik belum dikonfigurasi atau tidak sah di settings; email reset tidak dikirim", "reason", urlReason(err))
		return
	}
	now := s.now()
	// Batalkan tautan lama LEBIH DULU, baru sisipkan yang baru (urutan terbalik akan membatalkan tautan baru).
	if _, err := s.st.InvalidatePasswordResets(ctx, t, u.ID, now); err != nil {
		s.log.ErrorContext(ctx, "reset: gagal membatalkan tautan lama", "user_id", u.ID, "error", err)
		return
	}
	raw, hash, err := newToken()
	if err != nil {
		s.log.ErrorContext(ctx, "reset: gagal membuat token", "user_id", u.ID, "error", err)
		return
	}
	rec := store.PasswordReset{ID: uuid.NewString(), UserID: u.ID, TokenHash: hash[:], CreatedAt: now, ExpiresAt: now.Add(TokenTTL)}
	if err := s.st.InsertPasswordReset(ctx, t, rec); err != nil {
		s.log.ErrorContext(ctx, "reset: gagal menyimpan tautan", "user_id", u.ID, "error", err)
		return
	}
	prefLocale := ""
	if prefs, err := s.st.GetPreferences(ctx, t, u.ID); err == nil {
		prefLocale = prefs.Locale
	}
	msg, err := s.tpls.render(pickLocale(prefLocale, locale), templateData{
		AppName: appName, Name: sanitizeName(u.Name), Link: base + "#token=" + raw, ExpiresMinutes: int(TokenTTL / time.Minute),
	})
	if err != nil {
		s.log.ErrorContext(ctx, "reset: gagal merender email", "user_id", u.ID, "error", err)
		return
	}
	msg.To = []string{u.Email}
	if err := s.sender.Send(ctx, msg); err != nil {
		s.log.ErrorContext(ctx, "reset: gagal mengirim email reset", "user_id", u.ID, "error", err)
	}
}

// Complete menyelesaikan reset: token dari tautan dan sandi baru. Urutan: token cacat (tanpa menyentuh DB) -> hash
// token -> cari (tak ada/terpakai/kedaluwarsa = ErrInvalidToken seragam) -> kebijakan sandi (*PolicyError; tautan tidak
// terpakai) -> hash argon2id lewat pembatas hash (argon2 baru berjalan setelah token nyata ditemukan, sehingga token
// acak tidak memperkuat DoS; 429 lewat guard.AsRateLimited, tautan tidak terpakai) -> store.ResetPassword atomik
// (pakai tautan sekali, ganti hash, buka locked, cabut SEMUA sesi, hapus percobaan; kalah balapan = ErrInvalidToken)
// -> session.MarkRevoked agar instance ini langsung menolak sesi tercabut.
func (s *Service) Complete(ctx context.Context, rawToken, newPassword string) error {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return ErrNoTenant
	}
	hash, ok := hashToken(rawToken)
	if !ok {
		return ErrInvalidToken
	}
	// Kebijakan lebih dulu pada sandi lemah agar probing token dengan sandi lemah tidak membuka status token.
	if v := password.CheckPolicy(newPassword); len(v) > 0 {
		return &PolicyError{Violations: v}
	}
	rec, err := s.st.FindPasswordResetByHash(ctx, t, hash[:])
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidToken
		}
		return fmt.Errorf("reset: cari tautan: %w", err)
	}
	now := s.now()
	if rec.UsedAt != nil || !rec.ExpiresAt.After(now) {
		return ErrInvalidToken
	}
	phc, err := s.gate.Hash(ctx, s.hasher, newPassword)
	if err != nil {
		if _, limited := guard.AsRateLimited(err); limited {
			return err
		}
		return fmt.Errorf("reset: hash sandi: %w", err)
	}
	res, won, err := s.st.ResetPassword(ctx, t, rec.ID, phc, now)
	if err != nil {
		return fmt.Errorf("reset: simpan sandi baru: %w", err)
	}
	if !won {
		return ErrInvalidToken
	}
	if len(res.RevokedSessionIDs) > 0 {
		s.sessions.MarkRevoked(ctx, res.RevokedSessionIDs...)
	}
	return nil
}

// newToken membuat token acak 32 byte dan hash SHA-256-nya.
func newToken() (raw string, hash [32]byte, err error) {
	b := make([]byte, tokenBytes)
	if _, err = io.ReadFull(rand.Reader, b); err != nil {
		return "", hash, err
	}
	return base64.RawURLEncoding.EncodeToString(b), sha256.Sum256(b), nil
}

// hashToken mendekode token base64url ketat (tepat 32 byte, tanpa padding, tanpa bit sisa) dan menghitung hash-nya.
func hashToken(raw string) ([32]byte, bool) {
	var zero [32]byte
	if len(raw) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return zero, false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil || len(b) != tokenBytes {
		return zero, false
	}
	return sha256.Sum256(b), true
}

// sanitizeName membuang karakter kontrol/format dan memotong nama untuk dipakai di email.
func sanitizeName(n string) string {
	n = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return -1
		}
		return r
	}, n)
	n = strings.TrimSpace(n)
	if r := []rune(n); len(r) > maxNameRunes {
		n = string(r[:maxNameRunes])
	}
	return n
}

// urlReason meringkas sebab kegagalan URL publik untuk log tanpa memuat nilai: nilai tidak sah, atau galat sumber
// (termasuk belum dikonfigurasi; teks galat sumber tidak berisi nilai URL).
func urlReason(err error) string {
	if errors.Is(err, errBadPublicURL) {
		return "invalid_value"
	}
	return "source_error: " + err.Error()
}
