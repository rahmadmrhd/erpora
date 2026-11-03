package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

// DefaultMinFailureDuration adalah lantai waktu respons untuk setiap login yang gagal (401 seragam): selisih kecil sisa
// (mis. satu tulis fsync pada akun nyata) tertutup karena semua jalur gagal baru menjawab setelah lantai ini, selama
// pemrosesan normal lebih singkat darinya.
const DefaultMinFailureDuration = 200 * time.Millisecond

// LoginAccounts mencari akun berdasarkan email dalam tenant. *store.Store memenuhinya (store.ErrNotFound = tak dikenal).
type LoginAccounts interface {
	FindUserByEmail(ctx context.Context, t tenancy.TenantID, email string) (store.User, error)
}

// OutcomeKind adalah hasil verifikasi login yang dilihat pemanggil.
type OutcomeKind int

const (
	// Success: sandi benar pada akun aktif yang tidak sedang dijeda. Pemanggil melanjutkan ke MFA/penerbitan sesi dan
	// memanggil RegisterSuccess setelah autentikasi selesai penuh.
	Success OutcomeKind = iota + 1
	// InvalidCredentials: jawab 401 `unauthenticated` seragam. Berlaku sama untuk email tak dikenal, sandi salah, akun
	// terkunci, akun dinonaktifkan, dan hash tersimpan cacat; Reason hanya untuk log/audit internal.
	InvalidCredentials
	// RateLimited: jawab 429 `rate_limited` dengan Retry-After = RetryAfter (httpx.TooManyRequests). Tidak
	// membedakan akun ada atau tidak.
	RateLimited
)

func (k OutcomeKind) String() string {
	switch k {
	case Success:
		return "Success"
	case InvalidCredentials:
		return "InvalidCredentials"
	case RateLimited:
		return "RateLimited"
	}
	return "OutcomeKind(?)"
}

// Reason menyebut sebab internal sebuah Outcome. HANYA untuk log keamanan/audit; tidak boleh dikirim ke klien dan tidak
// boleh membedakan respons (itu tujuan Kind).
type Reason string

// Sebab internal.
const (
	ReasonUnknownEmail    Reason = "unknown_email"
	ReasonWrongPassword   Reason = "wrong_password"
	ReasonAccountLocked   Reason = "account_locked"
	ReasonAccountDisabled Reason = "account_disabled"
	ReasonInvalidHash     Reason = "invalid_stored_hash"
	ReasonPairBlocked     Reason = "pair_blocked"
	ReasonIPLimited       Reason = "ip_limited"
	ReasonTargetLimited   Reason = "target_limited"
	ReasonHashBusy        Reason = "hash_busy"
)

// Outcome adalah hasil seragam Authenticator.Verify.
type Outcome struct {
	Kind OutcomeKind
	// User terisi hanya pada Success (PasswordHash dikosongkan).
	User store.User
	// NeedsRehash (Success): hash tersimpan berparameter lama; pemanggil menyimpan hash baru lewat HashGate.Hash.
	NeedsRehash bool
	// RetryAfter terisi pada RateLimited.
	RetryAfter time.Duration
	// Reason: sebab internal (lihat Reason).
	Reason Reason
	// LockedNow (InvalidCredentials): kegagalan ini yang membuat akun berstatus locked (untuk event keamanan).
	LockedNow bool
}

// LoginInput adalah masukan Verify. IP adalah IP klien yang sudah diselesaikan (mis. lewat proksi tepercaya) oleh
// pemanggil; tenant diambil dari context.
type LoginInput struct {
	Email    string
	Password string
	IP       string
}

// AuthenticatorConfig merakit Authenticator.
type AuthenticatorConfig struct {
	Accounts LoginAccounts // wajib
	Lockout  *Lockout      // wajib
	Gate     HashRunner    // wajib; biasanya *HashGate (semua Verify/dummy lewat sini)
	Target   *PrincipalLimiter
	// Argon2 adalah parameter dari config.Config.Argon2. Hasher sungguhan dan hash tiruan keduanya berparameter ini
	// (password.NewDummy(cfg.Argon2), bukan password.Dummy()) sehingga biaya identik. Nol = password.DefaultParams().
	Argon2 password.Params
	// IP opsional: batas laju per IP juga dipasang middleware (ADR-0013 §4); isi hanya bila Authenticator dipakai tanpa
	// middleware itu (hindari penghitungan ganda).
	IP *IPLimiter
	// MinFailureDuration: lantai waktu respons login gagal (0 = DefaultMinFailureDuration).
	MinFailureDuration time.Duration
	// Now (default time.Now) dan Sleep (default timer yang menghormati ctx) dapat diinjeksi untuk tes.
	Now    func() time.Time
	Sleep  func(ctx context.Context, d time.Duration) error
	Logger *slog.Logger // default slog.Default()
}

// Authenticator menggabungkan batas IP, jeda pasangan (akun, IP), batas per email, pembatas hash, verifikasi sandi
// (dengan hash tiruan untuk email tak dikenal dan akun yang tak boleh masuk), dan pencatatan kegagalan menjadi SATU
// hasil seragam. Dipakai service login (C1.10/C1.11). Aman untuk dipakai serentak.
//
// Urutan: batas IP -> cari akun -> jeda pasangan (429 sebelum hash; tidak dicatat) -> batas per email -> verifikasi
// (hash tiruan bila perlu) -> catat kegagalan. Semua jalur gagal mengeluarkan urutan panggilan database yang sama (akun
// tak dikenal memakai UUID acak untuk panggilan tiruan dan pelacak jeda di memori) dan baru menjawab setelah lantai
// MinFailureDuration.
type Authenticator struct {
	accounts LoginAccounts
	lockout  *Lockout
	gate     HashRunner
	target   *PrincipalLimiter
	ip       *IPLimiter
	hasher   *password.Hasher
	dummy    *password.Hasher
	floor    time.Duration
	now      func() time.Time
	sleep    func(ctx context.Context, d time.Duration) error
	log      *slog.Logger
}

// NewAuthenticator membuat Authenticator; hash tiruan dihitung di sini (bukan pada login pertama).
func NewAuthenticator(cfg AuthenticatorConfig) (*Authenticator, error) {
	switch {
	case cfg.Accounts == nil:
		return nil, errors.New("guard: AuthenticatorConfig.Accounts wajib")
	case cfg.Lockout == nil:
		return nil, errors.New("guard: AuthenticatorConfig.Lockout wajib")
	case cfg.Gate == nil:
		return nil, errors.New("guard: AuthenticatorConfig.Gate wajib")
	case cfg.Target == nil:
		return nil, errors.New("guard: AuthenticatorConfig.Target wajib")
	}
	params := cfg.Argon2
	if params == (password.Params{}) {
		params = password.DefaultParams()
	}
	a := &Authenticator{
		accounts: cfg.Accounts, lockout: cfg.Lockout, gate: cfg.Gate, target: cfg.Target, ip: cfg.IP,
		hasher: &password.Hasher{Params: params}, dummy: password.NewDummy(params),
		floor: cfg.MinFailureDuration, now: cfg.Now, sleep: cfg.Sleep, log: cfg.Logger,
	}
	if a.floor <= 0 {
		a.floor = DefaultMinFailureDuration
	}
	if a.now == nil {
		a.now = time.Now
	}
	if a.sleep == nil {
		a.sleep = sleepCtx
	}
	if a.log == nil {
		a.log = slog.Default()
	}
	return a, nil
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

func limited(d time.Duration, r Reason) Outcome {
	return Outcome{Kind: RateLimited, RetryAfter: d, Reason: r}
}

// Verify memeriksa kredensial login. Galat (bukan Outcome) hanya untuk kegagalan infrastruktur (database, tenant
// tidak ada di context, ctx batal): jawab 500/abaikan. Sandi benar pada pasangan yang sedang dijeda tetap ditolak (429).
func (a *Authenticator) Verify(ctx context.Context, in LoginInput) (Outcome, error) {
	start := a.now()
	t, err := tenantFrom(ctx)
	if err != nil {
		return Outcome{}, err
	}
	email := store.NormalizeEmail(in.Email)
	ipKey := NormalizeIP(in.IP)

	if a.ip != nil {
		if ok, d := a.ip.Allow(ipKey); !ok {
			return limited(d, ReasonIPLimited), nil
		}
	}

	user, err := a.accounts.FindUserByEmail(ctx, t, email)
	known := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Outcome{}, fmt.Errorf("guard: cari akun: %w", err)
	}

	// Jeda pasangan (akun, IP): ditolak sebelum hash dan tanpa dicatat. Email tak dikenal diperlakukan sama lewat
	// pelacak di memori dan satu bacaan tiruan (jumlah round-trip DB sama).
	var st State
	if known {
		if st, _, err = a.lockout.checkPair(ctx, t, user.ID, ipKey); err != nil {
			return Outcome{}, err
		}
	} else {
		a.lockout.dummyCheck(ctx, t)
		st = a.lockout.unknownCheck(t, email, ipKey)
	}
	if st.PairBlocked {
		return limited(st.RetryAfter, ReasonPairBlocked), nil
	}

	if ok, d := a.target.Allow(EmailTargetKey("login", t, email)); !ok {
		return limited(d, ReasonTargetLimited), nil
	}

	// Verifikasi. Akun yang tak boleh masuk (tak dikenal, locked, disabled) diverifikasi terhadap hash tiruan
	// berparameter terkonfigurasi: biaya argon2 sama, hasil dipaksa gagal.
	usable := known && user.Status == store.UserActive
	hasher, phc := a.dummy, a.dummy.DummyHash()
	if usable {
		hasher, phc = a.hasher, user.PasswordHash
	}
	reason := ReasonWrongPassword
	switch {
	case !known:
		reason = ReasonUnknownEmail
	case user.Status == store.UserLocked:
		reason = ReasonAccountLocked
	case user.Status != store.UserActive:
		reason = ReasonAccountDisabled
	}
	ok, rehash, verr := runVerify(ctx, a.gate, hasher, in.Password, phc)
	if errors.Is(verr, password.ErrInvalidHash) {
		// Hash tersimpan cacat (mis. kosong untuk akun SSO-only): ErrInvalidHash gagal SEBELUM argon2 berjalan, jadi
		// jalankan verifikasi tiruan agar waktunya sama, lalu perlakukan seperti sandi salah.
		a.log.WarnContext(ctx, "identity: hash sandi tersimpan cacat; diperlakukan sebagai sandi salah", "user_id", user.ID)
		ok, reason = false, ReasonInvalidHash
		_, _, verr = runVerify(ctx, a.gate, a.dummy, in.Password, a.dummy.DummyHash())
	}
	if retry, limitedErr := AsRateLimited(verr); limitedErr {
		return limited(retry, ReasonHashBusy), nil
	}
	if verr != nil {
		return Outcome{}, fmt.Errorf("guard: verifikasi sandi: %w", verr)
	}

	if ok && usable {
		user.PasswordHash = ""
		return Outcome{Kind: Success, User: user, NeedsRehash: rehash}, nil
	}

	out := Outcome{Kind: InvalidCredentials, Reason: reason}
	if known {
		fst, err := a.lockout.RegisterFailure(ctx, user.ID, ipKey)
		if err != nil {
			return Outcome{}, err
		}
		if fst.AccountLocked && user.Status == store.UserActive {
			out.LockedNow = true
			a.log.WarnContext(ctx, "identity: akun dikunci (kegagalan lintas IP melampaui ambang)", "user_id", user.ID)
		}
	} else {
		a.lockout.unknownFail(t, email, ipKey)
		a.lockout.dummyFailure(ctx, t)
	}
	if rem := a.floor - a.now().Sub(start); rem > 0 {
		_ = a.sleep(ctx, rem) // ctx batal: pemanggil sudah pergi, jawaban tak dibaca
	}
	return out, nil
}

// RegisterSuccess mencatat sukses (mereset hitungan pasangan). Panggil setelah autentikasi selesai penuh (setelah MFA
// bila akun punya faktor), bukan pada Success sandi saja.
func (a *Authenticator) RegisterSuccess(ctx context.Context, userID, ip string) error {
	return a.lockout.RegisterSuccess(ctx, userID, ip)
}
