package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"erpora/internal/platform/config"
	"erpora/internal/platform/crypto"
	"erpora/internal/platform/database"
	"erpora/internal/platform/identity"
	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/reset"
	"erpora/internal/platform/identity/session"
	identitystore "erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/mail"
	"erpora/internal/platform/modules/runtime"
	"erpora/internal/platform/settings"
	settingsstore "erpora/internal/platform/settings/store"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
	sdkmail "erpora/sdk/mail"
)

const (
	// mfaIssuer adalah nama penerbit pada otpauth URI (tampil di aplikasi autentikator).
	mfaIssuer = "ERPora"
	// jwksPath adalah rute publik platform di luar /api/{slug} (K-C1.8).
	jwksPath = "/.well-known/jwks.json"
	// jwksRoute adalah pola router JWKS; satu sumber untuk pendaftaran, allowlist keamanan, dan tes kontrak.
	jwksRoute = "GET " + jwksPath
)

// DirectPublicRoutes adalah rute publik yang didaftarkan platform langsung di router (di luar registry modul) dan
// karenanya wajib tercantum di tests/security/public_routes.txt. Satu sumber untuk pendaftaran dan tes keamanan.
func DirectPublicRoutes() []string { return []string{jwksRoute} }

// PublicURLSource menyediakan URL publik aplikasi. Keputusan pemilik produk 3 Nov 2026: sumbernya tabel settings
// platform (settings.Service.PublicURL), BUKAN ERP_PUBLIC_URL; implementasi bawaan: newSettingsURLSource.
type PublicURLSource interface {
	PublicURL(ctx context.Context) (string, error)
}

// ErrPublicURLNotConfigured: URL publik belum diisi (settings.ErrNotConfigured). Pada production `serve` gagal start
// (lihat checkPublicURL).
var ErrPublicURLNotConfigured = settings.ErrNotConfigured

// publicURLFunc mengadaptasi fungsi menjadi PublicURLSource.
type publicURLFunc func(ctx context.Context) (string, error)

func (f publicURLFunc) PublicURL(ctx context.Context) (string, error) { return f(ctx) }

// fallbackDevURL dipakai di development bila ERP_HTTP_ADDR tidak berbentuk host:port (sama dengan perilaku lama).
const fallbackDevURL = "http://localhost:8080"

// newSettingsService membuat settings.Service di atas tabel settings tenant. Sealer hanya dibutuhkan nilai terenkripsi
// (public_url bukan); bila ERP_SECRET_KEY belum valid, sealer nil dan newIdentityStack yang menggagalkan startup.
func newSettingsService(cfg config.Config, db *database.DB, tid tenancy.TenantID) *settings.Service {
	var sealer *crypto.Sealer
	if sl, err := crypto.NewSealerFromMaterial([]byte(cfg.SecretKey), []byte(cfg.SecretKeyPrevious)); err == nil {
		sealer = sl
	}
	conn := database.SingleTenantConnector{Default: tid, DB: database.TenantDB{DB: db}}
	return settings.NewService(settingsstore.New(conn, sealer), settings.Options{Production: cfg.Env == config.EnvProduction})
}

// settingsURLSource adalah PublicURLSource di atas settings.Service yang terikat pada tenant default: pemanggil tanpa
// tenant di context (pemeriksa Origin, pemeriksaan start production) tetap membaca setting tenant itu. Di development
// dan hanya bila setting belum diisi, dipakai URL turunan ERP_HTTP_ADDR (PublicURLOrDevDefault); di production selalu
// settings.ErrNotConfigured bila belum diisi.
type settingsURLSource struct {
	svc        *settings.Service
	tid        tenancy.TenantID
	devDefault string
}

func newSettingsURLSource(cfg config.Config, svc *settings.Service, tid tenancy.TenantID) PublicURLSource {
	dev := ""
	if cfg.Env == config.EnvDevelopment {
		if dev = settings.DevURLFromHTTPAddr(cfg.HTTPAddr); dev == "" {
			dev = fallbackDevURL
		}
	}
	return settingsURLSource{svc: svc, tid: tid, devDefault: dev}
}

func (s settingsURLSource) PublicURL(ctx context.Context) (string, error) {
	if _, ok := tenancy.FromContext(ctx); !ok {
		ctx = tenancy.WithTenant(ctx, s.tid)
	}
	return s.svc.PublicURLOrDevDefault(ctx, s.devDefault)
}

// newMailSender memilih pengirim surat sesuai ERP_MAIL_DRIVER: smtp, atau log (isi surat penuh hanya di development).
func newMailSender(cfg config.Config, log *slog.Logger) (sdkmail.Sender, error) {
	if cfg.MailDriver == config.MailDriverSMTP {
		return mail.NewSMTP(cfg.SMTP, mail.WithSMTPLogger(log))
	}
	var opts []mail.LogOption
	if cfg.Env == config.EnvDevelopment {
		opts = append(opts, mail.WithBody())
	}
	return mail.NewLog(log, opts...), nil
}

// checkPublicURL dipanggil `serve` sebelum merakit: di production tanpa URL publik start gagal (ErrPublicURLNotConfigured).
func checkPublicURL(ctx context.Context, cfg config.Config, src PublicURLSource) error {
	if cfg.Env != config.EnvProduction {
		return nil
	}
	if _, err := src.PublicURL(ctx); err != nil {
		return fmt.Errorf("serve production membutuhkan URL publik (settings platform): %w", err)
	}
	return nil
}

// lazyOriginChecker menghitung origin yang diizinkan dari src per permintaan (di-cache per nilai URL), bukan sekali
// saat startup: perubahan setting berlaku tanpa restart. Galat atau URL tak sah: semua origin ditolak (gagal tertutup).
func lazyOriginChecker(cfg config.Config, src PublicURLSource) session.OriginChecker {
	var mu sync.Mutex
	var lastURL string
	var last session.OriginChecker
	return func(origin string) bool {
		raw, err := src.PublicURL(context.Background())
		if err != nil {
			return false
		}
		mu.Lock()
		if last == nil || raw != lastURL {
			chk, cerr := session.NewOriginChecker(allowedOrigins(cfg, raw))
			if cerr != nil {
				mu.Unlock()
				return false
			}
			last, lastURL = chk, raw
		}
		chk := last
		mu.Unlock()
		return chk(origin)
	}
}

// identityExtras adalah komponen luar identitas yang dibutuhkan Service dan Admin: settings (URL publik) dan pengirim
// surat (reset sandi).
type identityExtras struct {
	Settings *settings.Service
	Mail     sdkmail.Sender
	// CLI merakit varian command CLI (PRD-001 Q28, C1.12): hanya yang dibutuhkan identity.Admin. Tanpa pekerja latar
	// (promosi kunci, retensi login_attempts: milik `serve`), tanpa langganan notifikasi, tanpa Reset/Service (Mail boleh
	// nil); Service hasilnya nil.
	CLI bool
}

// identityStack adalah perakitan mesin identitas untuk satu proses serve (PRD-001 C1): kunci dan token, sesi dan
// revokasi, pembatas laju dan lockout, MFA, serta pembaca keadaan MFA untuk keputusan otorisasi.
//
// Titik pemasangan identity.Service dan identity.Admin ke modul core (C1.11/C1.12): Service dan Admin diisi
// newIdentityStack (C1.10b); installCoreIdentity (dipasang sebagai runtime.Services.Identity) memasangkannya pada context modul
// `erp.core` saja (route, command cli:core:*, hook, seed; Admin sendiri hanya menerima hak sistem berawalan "cli:core:", jadi hook/seed tidak dapat memakainya, PRD-001 Q30). runtime.Services.installIdentity menegakkan pembatasan
// itu di sisi platform, jadi modul lain tidak pernah menerimanya.
type identityStack struct {
	Store         *identitystore.Store
	Sealer        *crypto.Sealer
	Keyring       *token.Keyring
	Issuer        *token.Issuer
	Verifier      *token.Verifier
	Promoter      *token.Promoter
	Retention     *guard.RetentionWorker
	Revocations   *session.RevocationList
	Sessions      *session.Service
	Hasher        *password.Hasher
	Gate          *guard.HashGate
	Lockout       *guard.Lockout
	Authenticator *guard.Authenticator
	MFA           *mfa.Service
	MFAState      *identity.MFAStateReader
	IPLimiter     *guard.IPLimiter
	Principals    *guard.PrincipalLimiter

	// Reset adalah alur lupa/reset sandi (surat lewat pengirim dari identityExtras).
	Reset *reset.Service

	// Service dan Admin adalah implementasi sdk/identity.Service dan Admin (C1.10b); installCoreIdentity memasangnya
	// pada context modul core (C1.11) dan command cli:core:* (C1.12).
	Service sdkidentity.Service
	Admin   sdkidentity.Admin

	cancel []func()
}

// newIdentityStack merakit dan menyalakan mesin identitas. Gagal tertutup: ERP_SECRET_KEY wajib, keyring wajib
// dapat dimuat dengan satu kunci aktif, dan daftar sesi tercabut wajib termuat sebelum request pertama dilayani
// (tanpa daftar itu sesi tercabut bisa lolos). Pekerja latar (promosi kunci, retensi login_attempts, muat ulang daftar tercabut) berjalan
// sampai ctx selesai.
func newIdentityStack(ctx context.Context, cfg config.Config, log *slog.Logger, db *database.DB, tid tenancy.TenantID, urls PublicURLSource, ext identityExtras) (*identityStack, error) {
	if err := cfg.RequireSecretKey(); err != nil {
		return nil, err
	}
	sealer, err := crypto.NewSealerFromMaterial([]byte(cfg.SecretKey), []byte(cfg.SecretKeyPrevious))
	if err != nil {
		return nil, fmt.Errorf("rahasia identitas: %w", err)
	}
	conn := database.SingleTenantConnector{Default: tid, DB: database.TenantDB{DB: db}}
	st := identitystore.New(database.ControlDB{DB: db}, conn)
	notifier := newChannelNotifier(log)
	s := &identityStack{Store: st, Sealer: sealer}

	s.Keyring, err = token.NewKeyring(token.KeyringConfig{Store: keyStoreAdapter{st}, Sealer: sealer, Notifier: notifier, Log: log})
	if err != nil {
		return nil, fmt.Errorf("keyring: %w", err)
	}
	if err := s.Keyring.Reload(ctx); err != nil {
		return nil, fmt.Errorf("muat kunci penandatangan: %w", err)
	}
	// Juga di varian CLI: instalasi baru tanpa kunci aktif tidak boleh berakhir hanya dengan kunci pending setelah
	// core:rotate-keys (belum ada yang dapat menandatangani sampai promosi). Dua proses yang berlomba konvergen.
	if created, err := s.Keyring.EnsureActive(ctx); err != nil {
		return nil, fmt.Errorf("siapkan kunci penandatangan: %w", err)
	} else if created {
		log.Info("kunci penandatangan JWT pertama dibuat", "kid", s.Keyring.Active().ID)
	}
	if s.Issuer, err = token.NewIssuer(token.IssuerConfig{Keyring: s.Keyring, Issuer: identity.ClaimIssuer, Audience: identity.ClaimAudience}); err != nil {
		return nil, err
	}
	if s.Verifier, err = token.NewVerifier(token.VerifierConfig{Keyring: s.Keyring, Issuer: identity.ClaimIssuer, Audience: identity.ClaimAudience}); err != nil {
		return nil, err
	}
	if !ext.CLI {
		if s.Promoter, err = token.NewPromoter(token.PromoterConfig{Keyring: s.Keyring, Log: log}); err != nil {
			return nil, err
		}
		go s.Promoter.Run(ctx) // promosi pending -> active, retiring -> retired; berhenti bersama ctx
	}
	// Retensi login_attempts (data pribadi; PRD-001 Q30 butir 3): pangkas baris > 30 hari tiap jam; berhenti bersama ctx.
	if !ext.CLI {
		if s.Retention, err = guard.NewRetentionWorker(guard.RetentionConfig{Store: st, Tenant: tid, Log: log}); err != nil {
			return nil, err
		}
		go s.Retention.Run(ctx)
	}

	// Daftar sesi tercabut: muat awal (gagal = startup gagal), lalu langganan notifikasi + muat ulang berkala.
	tctx := tenancy.WithTenant(ctx, tid)
	s.Revocations = session.NewRevocationList(session.RevocationConfig{Loader: st, Log: log})
	if err := s.Revocations.Load(tctx); err != nil {
		return nil, fmt.Errorf("muat daftar sesi tercabut: %w", err)
	}
	if !ext.CLI {
		s.cancel = append(s.cancel, s.Revocations.Subscribe(tctx, notifier, 0))
	}
	go func() {
		<-ctx.Done()
		for _, c := range s.cancel {
			c()
		}
		s.Keyring.Close()
	}()

	origins := lazyOriginChecker(cfg, urls)
	if s.Sessions, err = session.New(session.Config{Store: st, Revocations: s.Revocations, Notifier: notifier, Origin: origins, Log: log}); err != nil {
		return nil, fmt.Errorf("sesi: %w", err)
	}

	params := cfg.Argon2
	if params.MemoryKiB == 0 {
		params = password.DefaultParams()
	}
	s.Hasher = &password.Hasher{Params: params}
	conc := cfg.Argon2MaxConcurrent
	if conc < 1 {
		conc = config.DefaultArgon2MaxConcurrent()
	}
	if s.Gate, err = guard.NewHashGate(conc, guard.DefaultHashGateWait); err != nil {
		return nil, fmt.Errorf("pembatas hash: %w", err)
	}
	if s.Lockout, err = guard.NewLockout(st, guard.DefaultLockoutConfig(), nil); err != nil {
		return nil, fmt.Errorf("lockout: %w", err)
	}
	if s.IPLimiter, err = guard.NewIPLimiter(guard.DefaultIPLimiterConfig()); err != nil {
		return nil, err
	}
	if s.Principals, err = guard.NewPrincipalLimiter(guard.DefaultTargetLimiterConfig()); err != nil {
		return nil, err
	}
	loginTarget, err := guard.NewPrincipalLimiter(guard.DefaultTargetLimiterConfig())
	if err != nil {
		return nil, err
	}
	// Batas IP dipasang middleware (urutan ADR-0013 §4), jadi Authenticator tidak diberi IP limiter (hindari hitung ganda).
	if s.Authenticator, err = guard.NewAuthenticator(guard.AuthenticatorConfig{
		Accounts: st, Lockout: s.Lockout, Gate: s.Gate, Target: loginTarget, Argon2: params, Logger: log,
	}); err != nil {
		return nil, fmt.Errorf("authenticator: %w", err)
	}
	if s.MFA, err = mfa.New(mfa.Deps{Store: st, Sealer: sealer, Lockout: mfaLockout{s.Lockout}, Issuer: mfaIssuer, Logger: log}); err != nil {
		return nil, fmt.Errorf("mfa: %w", err)
	}
	if s.MFAState, err = identity.NewMFAStateReader(st, 0, nil); err != nil {
		return nil, err
	}
	if ext.Settings == nil || (ext.Mail == nil && !ext.CLI) {
		return nil, errors.New("identitas: Settings dan Mail wajib")
	}
	if !ext.CLI {
		if s.Reset, err = reset.New(reset.Config{
			Store: st, Sessions: s.Sessions, Gate: s.Gate, Sender: ext.Mail, PublicURL: urls, Argon2: params, Logger: log,
		}); err != nil {
			return nil, fmt.Errorf("reset sandi: %w", err)
		}
	}
	deps := identity.Deps{
		Login: s.Authenticator, Gate: s.Gate, Hasher: s.Hasher, Lockout: s.Lockout, Sessions: s.Sessions, Issuer: s.Issuer,
		Keys: s.Keyring, MFA: s.MFA, MFAState: s.MFAState, Reset: s.Reset, Settings: ext.Settings, Store: st, Log: log,
	}
	if !ext.CLI {
		if s.Service, err = identity.NewService(deps); err != nil {
			return nil, err
		}
	}
	if s.Admin, err = identity.NewAdmin(deps); err != nil {
		return nil, err
	}
	return s, nil
}

// authenticate adalah middleware Authenticate nyata (menggantikan stub authN PRD-000).
func (s *identityStack) authenticate(log *slog.Logger) func(next http.Handler) http.Handler {
	return identity.Authenticate(identity.AuthenticateDeps{Verifier: s.Verifier, Revoked: s.Revocations, Log: log})
}

// installCoreIdentity memasang Service/Admin yang tersedia; dipakai sebagai runtime.Services.Identity.
func (s *identityStack) installCoreIdentity(ctx context.Context) context.Context {
	if s.Service != nil {
		ctx = sdkidentity.WithService(ctx, s.Service)
	}
	if s.Admin != nil {
		ctx = sdkidentity.WithAdmin(ctx, s.Admin)
	}
	return ctx
}

// mfaState memetakan MFAStateReader ke runtime.MFAStateReader.
type mfaState struct{ r *identity.MFAStateReader }

func (m mfaState) MFAState(ctx context.Context, userID string) (runtime.MFAState, error) {
	st, err := m.r.Status(ctx, userID)
	return runtime.MFAState{FactorActive: st.FactorActive, EnrollmentRequired: st.EnrollmentRequired}, err
}

// allowedOrigins menurunkan origin yang diizinkan memanggil endpoint refresh (cookie SameSite=Strict + pemeriksaan
// Origin; ADR-0013 §1) dari URL publik (PublicURLSource). Di development localhost dan 127.0.0.1 pada port yang sama
// ikut diizinkan agar kedua cara membuka aplikasi bekerja. URL kosong atau tak terurai: semua origin ditolak.
func allowedOrigins(cfg config.Config, rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	origins := []string{u.Scheme + "://" + u.Host}
	if cfg.Env == config.EnvDevelopment {
		if port := u.Port(); port != "" {
			for _, h := range []string{"localhost", "127.0.0.1"} {
				origins = append(origins, u.Scheme+"://"+h+":"+port)
			}
		}
	}
	return origins
}

// keyStoreAdapter memetakan *store.Store ke token.KeyStore (store tidak mengimpor token, dan sebaliknya).
type keyStoreAdapter struct{ st *identitystore.Store }

func (a keyStoreAdapter) ListSigningKeys(ctx context.Context) ([]token.KeyRecord, error) {
	keys, err := a.st.ListSigningKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]token.KeyRecord, len(keys))
	for i, k := range keys {
		out[i] = token.KeyRecord{
			ID: k.ID, Status: k.Status, PublicKey: k.PublicKey, PrivateKeySealed: k.PrivateKeySealed,
			CreatedAt: k.CreatedAt, ActivatedAt: k.ActivatedAt, RetiringAt: k.RetiringAt, RetiredAt: k.RetiredAt,
		}
	}
	return out, nil
}

func (a keyStoreAdapter) InsertSigningKey(ctx context.Context, k token.KeyRecord) error {
	return a.st.InsertSigningKey(ctx, identitystore.SigningKey{
		ID: k.ID, Status: k.Status, PublicKey: k.PublicKey, PrivateKeySealed: k.PrivateKeySealed,
		CreatedAt: k.CreatedAt, ActivatedAt: k.ActivatedAt, RetiringAt: k.RetiringAt, RetiredAt: k.RetiredAt,
	})
}

func (a keyStoreAdapter) TransitionSigningKey(ctx context.Context, id, from, to string, at time.Time) (bool, error) {
	return a.st.TransitionSigningKey(ctx, id, from, to, at)
}

// mfaLockout memetakan guard.Lockout ke mfa.Lockout/LockChecker (guard.State dan mfa.LockState berbentuk sama).
// Kegagalan kode MFA dicatat dengan kind `mfa` (RegisterMFAFailure), bukan kind `password`.
type mfaLockout struct{ l *guard.Lockout }

func (m mfaLockout) RegisterFailure(ctx context.Context, userID, ip string) (mfa.LockState, error) {
	st, err := m.l.RegisterMFAFailure(ctx, userID, ip)
	return mfa.LockState(st), err
}

func (m mfaLockout) RegisterSuccess(ctx context.Context, userID, ip string) error {
	return m.l.RegisterSuccess(ctx, userID, ip)
}

func (m mfaLockout) Check(ctx context.Context, userID, ip string) (mfa.LockState, error) {
	st, err := m.l.Check(ctx, userID, ip)
	return mfa.LockState(st), err
}

// channelNotifier adalah token.Notifier in-process berkanal nama (identity.keys_changed, identity.session_revoked).
//
// Mengapa bukan adaptor ke runtime.StateNotifier: StateNotifier membawa (tenant, revision int64) milik status modul,
// dan pelanggan lifecycle me-refresh snapshot setiap revision lebih besar dari yang dikenal, jadi memultipleks kanal
// identitas ke dalamnya akan memicu refresh palsu dan mencampur dua semantik. Notifier ini sengaja terpisah dengan
// bentuk sama sederhananya (in-process, sinkron); kelak keduanya diganti LISTEN/NOTIFY PostgreSQL tanpa mengubah
// pemanggil. Notifikasi lintas proses tidak ada sampai itu, dan celahnya ditutup pengaman yang sudah ada: Promoter
// memuat ulang keyring tiap tick, RevocationList memuat ulang berkala (1 menit) dan kid tak dikenal memicu muat
// ulang terbatas laju.
type channelNotifier struct {
	log  *slog.Logger
	mu   sync.Mutex
	next int
	subs map[string]map[int]func(context.Context)
}

var _ token.Notifier = (*channelNotifier)(nil)

func newChannelNotifier(log *slog.Logger) *channelNotifier {
	if log == nil {
		log = slog.Default()
	}
	return &channelNotifier{log: log, subs: map[string]map[int]func(context.Context){}}
}

func (n *channelNotifier) Subscribe(channel string, fn func(ctx context.Context)) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.subs[channel] == nil {
		n.subs[channel] = map[int]func(context.Context){}
	}
	id := n.next
	n.next++
	n.subs[channel][id] = fn
	return func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		delete(n.subs[channel], id)
	}
}

// Publish memanggil pelanggan kanal secara sinkron; panic satu pelanggan dicatat dan tidak menghentikan yang lain.
func (n *channelNotifier) Publish(ctx context.Context, channel string) {
	n.mu.Lock()
	fns := make([]func(context.Context), 0, len(n.subs[channel]))
	for _, fn := range n.subs[channel] {
		fns = append(fns, fn)
	}
	n.mu.Unlock()
	for _, fn := range fns {
		func() {
			defer func() {
				if r := recover(); r != nil {
					n.log.ErrorContext(ctx, "subscriber notifier identitas panic", "channel", channel, "panic", r)
				}
			}()
			fn(ctx)
		}()
	}
}

// Reserve, Fail, dan Release memenuhi mfa.LockReserver: percobaan kode direservasi (dicatat sebagai kegagalan kind
// `mfa`, pasangan (akun, IP) yang sama dengan login sandi) secara atomik lintas instance sebelum kode diverifikasi.
func (m mfaLockout) Reserve(ctx context.Context, userID, ip string) (any, mfa.LockState, error) {
	res, st, err := m.l.Reserve(ctx, identitystore.AttemptMFA, userID, ip)
	if err != nil || !res.Reserved() {
		return nil, mfa.LockState(st), err
	}
	return res, mfa.LockState{}, nil
}

func (m mfaLockout) Fail(ctx context.Context, token any) (mfa.LockState, error) {
	res, ok := token.(guard.Reservation)
	if !ok {
		return mfa.LockState{}, errors.New("identity: token reservasi MFA tidak valid")
	}
	st, err := m.l.Fail(ctx, res)
	return mfa.LockState(st), err
}

func (m mfaLockout) Release(ctx context.Context, token any) error {
	res, ok := token.(guard.Reservation)
	if !ok {
		return errors.New("identity: token reservasi MFA tidak valid")
	}
	return m.l.Release(ctx, res)
}

var _ mfa.LockReserver = mfaLockout{}
