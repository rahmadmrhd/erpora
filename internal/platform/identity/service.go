package identity

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/reset"
	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	sdkidentity "erpora/sdk/identity"
)

// service adalah implementasi sdk/identity.Service (PRD-001 FR-101, FR-102, FR-106, FR-107; ADR-0013).
//
// Prinsip yang ditegakkan di sini:
//   - Metode tanpa parameter pengguna/sesi menurunkan pengguna dan sesi HANYA dari Principal di context
//     (RequirePrincipal), tidak pernah dari input, sehingga tidak ada jalur IDOR (user A tidak dapat menyentuh user B).
//   - IP klien selalu dari httpx.ClientIP(ctx) (dipasang platform dari alamat peer tepercaya); field ClientIP pada
//     input hanya cadangan bila context tidak membawanya.
//   - Kegagalan login seragam (ErrInvalidCredentials): email tak dikenal, sandi salah, akun terkunci/nonaktif tidak
//     terbedakan; sebab sebenarnya hanya di log keamanan.
//   - RegisterSuccess (reset hitungan lockout pasangan) hanya setelah autentikasi selesai penuh.
//   - Refresh token mentah hanya lewat LoginResult.RefreshToken (json:"-"); tidak pernah masuk log atau galat.
type service struct {
	d   Deps
	now func() time.Time
	log *slog.Logger
}

// NewService merakit sdk/identity.Service dari dependensi d (Login, Gate, Hasher, Lockout, Sessions, Issuer, MFA,
// MFAState, Reset, Store wajib).
func NewService(d Deps) (sdkidentity.Service, error) {
	if err := require("Service",
		need{"Login", d.Login != nil}, need{"Gate", d.Gate != nil}, need{"Hasher", d.Hasher != nil},
		need{"Lockout", d.Lockout != nil}, need{"Sessions", d.Sessions != nil}, need{"Issuer", d.Issuer != nil},
		need{"MFA", d.MFA != nil}, need{"MFAState", d.MFAState != nil}, need{"Reset", d.Reset != nil},
		need{"Store", d.Store != nil}); err != nil {
		return nil, err
	}
	return &service{d: d, now: d.clock(), log: d.logger()}, nil
}

var _ sdkidentity.Service = (*service)(nil)

func (s *service) tenant(ctx context.Context) (tenancy.TenantID, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return tenancy.TenantID{}, errors.New("identity: context tanpa tenant")
	}
	return t, nil
}

// clientIP: httpx.ClientIP(ctx) lebih dulu (sumber tepercaya platform); fallback ke nilai input.
func clientIPOf(ctx context.Context, fallback string) string {
	if ip := httpx.ClientIP(ctx); ip != "" {
		return ip
	}
	return fallback
}

// principalCtx memastikan Principal ada dan memasang IP klien untuk lockout MFA. Mengembalikan principal, tenant, ip.
func (s *service) principalCtx(ctx context.Context, fallbackIP string) (context.Context, sdkidentity.Principal, tenancy.TenantID, string, error) {
	p, err := sdkidentity.RequirePrincipal(ctx)
	if err != nil {
		return ctx, sdkidentity.Principal{}, tenancy.TenantID{}, "", err
	}
	t, err := s.tenant(ctx)
	if err != nil {
		return ctx, p, t, "", err
	}
	ip := clientIPOf(ctx, fallbackIP)
	return mfa.WithClientIP(ctx, ip), p, t, ip, nil
}

// --- Login, refresh, logout ---

// Login memverifikasi kredensial dan memulai sesi. Sandi benar menghasilkan sesi (AMR ["pwd"]) dan access token
// walau MFA masih tersisa: MFAState memberi tahu klien layar mana yang dibuka (enroll_required / verify_required) dan
// otorisasi route (TypedAuthorizer) membatasi token itu pada endpoint MFA sampai amr memuat otp.
func (s *service) Login(ctx context.Context, in sdkidentity.LoginInput) (sdkidentity.LoginResult, error) {
	t, err := s.tenant(ctx)
	if err != nil {
		return sdkidentity.LoginResult{}, err
	}
	ip := clientIPOf(ctx, in.ClientIP)
	out, err := s.d.Login.Verify(ctx, guard.LoginInput{Email: in.Email, Password: in.Password, IP: ip})
	if err != nil {
		return sdkidentity.LoginResult{}, mapErr("login", err)
	}
	switch out.Kind {
	case guard.Success:
	case guard.RateLimited:
		s.log.InfoContext(ctx, "identity: login dibatasi laju", "event", "identity.login_rate_limited", "reason", string(out.Reason), "ip", ip)
		return sdkidentity.LoginResult{}, &sdkidentity.RateLimitedError{RetryAfter: out.RetryAfter}
	default:
		s.log.InfoContext(ctx, "identity: login gagal", "event", "identity.login_failed", "reason", string(out.Reason), "locked_now", out.LockedNow, "ip", ip)
		return sdkidentity.LoginResult{}, sdkidentity.ErrInvalidCredentials
	}
	u := out.User
	if out.NeedsRehash {
		s.rehash(ctx, t, u.ID, in.Password, out.RehashFrom)
	}
	sess, rt, err := s.d.Sessions.Start(ctx, session.StartInput{
		UserID: u.ID, ClientKind: store.ClientWeb, AuthMethod: string(sdkidentity.AuthMethodPassword),
		AMR: []string{sdkidentity.AMRPassword}, UserAgent: in.UserAgent, IP: ip,
	})
	if err != nil {
		if errors.Is(err, session.ErrUserNotActive) { // anggota tenant tidak aktif: tak dibedakan dari sandi salah
			s.log.InfoContext(ctx, "identity: login gagal", "event", "identity.login_failed", "reason", "membership_inactive", "ip", ip)
			return sdkidentity.LoginResult{}, sdkidentity.ErrInvalidCredentials
		}
		return sdkidentity.LoginResult{}, mapErr("mulai sesi", err)
	}
	state, err := s.mfaState(ctx, t, u.ID, true)
	var res sdkidentity.LoginResult
	if err == nil {
		res, err = s.issue(t, u.ID, sess.ID, sess.AMR, state)
	}
	if err != nil {
		s.revokeQuietly(ctx, sess.ID)
		return sdkidentity.LoginResult{}, mapErr("terbitkan token", err)
	}
	res.RefreshToken, res.RefreshExpiresAt = rt.Value, rt.ExpiresAt
	if state == sdkidentity.MFANone {
		s.fullyAuthenticated(ctx, t, u.ID, ip)
	}
	return res, nil
}

// rehash menyimpan hash dengan parameter terbaru setelah sandi terverifikasi, HANYA bila hash tersimpan masih oldPHC
// (compare-and-set): sandi yang diganti serentak oleh reset/ganti sandi tidak tertimpa; hash sudah berubah = rehash
// dilewati tanpa galat. Kegagalan hanya dicatat: login sah.
func (s *service) rehash(ctx context.Context, t tenancy.TenantID, userID, plain, oldPHC string) {
	if oldPHC == "" {
		return
	}
	phc, err := s.d.Gate.Hash(ctx, s.d.Hasher, plain)
	if err == nil {
		_, err = s.d.Store.RehashPassword(ctx, t, userID, oldPHC, phc)
	}
	if err != nil {
		s.log.WarnContext(ctx, "identity: hash ulang sandi gagal; login tetap berhasil", "user_id", userID, "error", err.Error())
	}
}

// fullyAuthenticated mencatat sukses lockout (reset hitungan pasangan) dan waktu login terakhir. Galat hanya dicatat:
// autentikasi sudah sah.
func (s *service) fullyAuthenticated(ctx context.Context, t tenancy.TenantID, userID, ip string) {
	if err := s.d.Lockout.RegisterSuccess(ctx, userID, ip); err != nil {
		s.log.WarnContext(ctx, "identity: gagal mencatat sukses lockout", "user_id", userID, "error", err.Error())
	}
	if err := s.d.Store.TouchLastLogin(ctx, t, userID, s.now().UTC()); err != nil {
		s.log.WarnContext(ctx, "identity: gagal memperbarui login terakhir", "user_id", userID, "error", err.Error())
	}
}

// revokeQuietly mencabut sesi yang baru dibuat bila langkah berikutnya gagal (agar tidak ada sesi tanpa token).
func (s *service) revokeQuietly(ctx context.Context, sid string) {
	if err := s.d.Sessions.Revoke(context.WithoutCancel(ctx), sid, session.ReasonLogout); err != nil {
		s.log.WarnContext(ctx, "identity: gagal mencabut sesi yatim", "session_id", sid, "error", err.Error())
	}
}

// mfaState menurunkan keadaan MFA: faktor aktif -> verify_required; administrator tanpa faktor -> enroll_required;
// selain itu none. fresh true membuang cache lebih dulu (login membaca keadaan terbaru, bukan cache 15 detik).
// Pendaftaran yang belum dikonfirmasi tidak dihitung sebagai faktor aktif.
func (s *service) mfaState(ctx context.Context, t tenancy.TenantID, userID string, fresh bool) (sdkidentity.MFAState, error) {
	if fresh {
		s.d.MFAState.Invalidate(t, userID)
	}
	st, err := s.d.MFAState.Status(ctx, userID)
	if err != nil {
		return "", err
	}
	switch {
	case st.FactorActive:
		return sdkidentity.MFAVerifyRequired, nil
	case st.EnrollmentRequired:
		return sdkidentity.MFAEnrollRequired, nil
	}
	return sdkidentity.MFANone, nil
}

// issue menerbitkan access token 10 menit untuk sesi. Scope sengaja kosong: pembatasan token parsial dilakukan
// TypedAuthorizer lewat amr + MFAState (Authenticate menolak token ber-scope di luar AcceptedScopes). perm_rev = 0
// sampai mesin permission C2 menyediakan revisinya.
func (s *service) issue(t tenancy.TenantID, userID, sid string, amr []string, state sdkidentity.MFAState) (sdkidentity.LoginResult, error) {
	raw, err := s.d.Issuer.Issue(token.Claims{Subject: userID, SessionID: sid, TenantID: t.String(), AMR: slices.Clone(amr)})
	if err != nil {
		return sdkidentity.LoginResult{}, err
	}
	return sdkidentity.LoginResult{AccessToken: raw, ExpiresIn: int(token.AccessTokenTTL / time.Second), MFAState: state}, nil
}

// Refresh menukar refresh token (rotasi tiap pakai; reuse mencabut family) dan menerbitkan access token baru
// dengan AMR yang dipertahankan dari sesi: refresh tidak pernah menaikkan sesi ke otp tanpa verifikasi.
func (s *service) Refresh(ctx context.Context, in sdkidentity.RefreshInput) (sdkidentity.LoginResult, error) {
	t, err := s.tenant(ctx)
	if err != nil {
		return sdkidentity.LoginResult{}, err
	}
	sess, rt, err := s.d.Sessions.Rotate(ctx, in.RefreshToken, in.Origin)
	if err != nil {
		return sdkidentity.LoginResult{}, mapErr("refresh", err)
	}
	state := sdkidentity.MFANone
	if !slices.Contains(sess.AMR, sdkidentity.AMROTP) {
		if state, err = s.mfaState(ctx, t, sess.UserID, false); err != nil {
			return sdkidentity.LoginResult{}, mapErr("keadaan MFA", err)
		}
	}
	res, err := s.issue(t, sess.UserID, sess.ID, sess.AMR, state)
	if err != nil {
		return sdkidentity.LoginResult{}, mapErr("terbitkan token", err)
	}
	res.RefreshToken, res.RefreshExpiresAt = rt.Value, rt.ExpiresAt
	return res, nil
}

// Logout mencabut sesi milik Principal (sesi dan pengguna dari context; tanpa parameter).
func (s *service) Logout(ctx context.Context) error {
	p, err := sdkidentity.RequirePrincipal(ctx)
	if err != nil {
		return err
	}
	if p.SessionID == "" {
		return sdkidentity.ErrUnauthenticated
	}
	if err := s.d.Sessions.Revoke(ctx, p.SessionID, session.ReasonLogout); err != nil && !errors.Is(err, session.ErrSessionNotFound) {
		return mapErr("logout", err)
	}
	return nil
}

// --- Lupa, reset, dan ganti sandi ---

// ForgotPassword meminta tautan reset. Dari luar selalu nil kecuali *RateLimitedError; waktu jawaban seragam.
func (s *service) ForgotPassword(ctx context.Context, in sdkidentity.ForgotInput) error {
	ctx = reset.WithClientIP(ctx, clientIPOf(ctx, in.ClientIP))
	return mapErr("lupa sandi", s.d.Reset.Request(ctx, in.Email, in.Locale))
}

// ResetPassword menyelesaikan reset dari tautan sekali pakai (mencabut semua sesi dan membuka akun locked).
func (s *service) ResetPassword(ctx context.Context, in sdkidentity.ResetInput) error {
	return mapErr("reset sandi", s.d.Reset.Complete(ctx, in.Token, in.NewPassword))
}

// ChangePassword mengganti sandi Principal sendiri: kebijakan sandi baru, verifikasi sandi lama (lewat pembatas hash
// dan lockout yang sama dengan login), lalu simpan dan cabut semua sesi LAIN (sesi ini tetap hidup).
func (s *service) ChangePassword(ctx context.Context, in sdkidentity.ChangeInput) error {
	p, err := sdkidentity.RequirePrincipal(ctx)
	if err != nil {
		return err
	}
	t, err := s.tenant(ctx)
	if err != nil {
		return err
	}
	ip := clientIPOf(ctx, in.ClientIP)
	if v := password.CheckPolicy(in.NewPassword); len(v) > 0 {
		return policyError(v)
	}
	// Reservasi atomik per pasangan (akun, IP) SEBELUM verifikasi argon2: percobaan langsung dicatat sebagai kegagalan
	// sehingga permintaan serentak saling melihat dan hanya sebanyak yang diizinkan jadwal jeda yang sampai ke verifikasi.
	// Semua keluar selain sandi lama salah (sukses, galat, ctx batal) melepas reservasi. Urutan kebijakan sandi baru ->
	// reservasi dipertahankan: sandi baru yang lemah tidak memakai jatah tebakan.
	res, st, err := s.d.Lockout.Reserve(ctx, store.AttemptPassword, p.UserID, ip)
	if err != nil {
		return mapErr("periksa lockout", err)
	}
	if st.AccountLocked {
		return sdkidentity.ErrAccountLocked
	}
	if st.PairBlocked || st.RetryAfter > 0 {
		return &sdkidentity.RateLimitedError{RetryAfter: st.RetryAfter}
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		if rerr := s.d.Lockout.Release(ctx, res); rerr != nil {
			s.log.WarnContext(ctx, "identity: gagal melepas reservasi percobaan", "user_id", p.UserID, "error", rerr.Error())
		}
	}
	defer release()
	u, err := s.d.Store.GetUser(ctx, t, p.UserID)
	if err != nil {
		return mapErr("muat pengguna", err)
	}
	ok := false
	if u.PasswordHash != "" {
		ok, _, err = s.d.Gate.Verify(ctx, s.d.Hasher, in.CurrentPassword, u.PasswordHash)
		if err != nil && !errors.Is(err, password.ErrInvalidHash) {
			return mapErr("verifikasi sandi", err)
		}
	}
	if !ok {
		released = true // kegagalan sudah tercatat oleh reservasi; jangan dilepas
		st, ferr := s.d.Lockout.Fail(ctx, res)
		if ferr != nil {
			return mapErr("catat kegagalan", ferr)
		}
		if st.AccountLocked {
			return sdkidentity.ErrAccountLocked
		}
		if st.PairBlocked || st.RetryAfter > 0 {
			return &sdkidentity.RateLimitedError{RetryAfter: st.RetryAfter}
		}
		return sdkidentity.ErrWrongCurrentPassword
	}
	release() // sandi lama benar bukan kegagalan
	phc, err := s.d.Gate.Hash(ctx, s.d.Hasher, in.NewPassword)
	if err != nil {
		return mapErr("hash sandi", err)
	}
	revoked, err := s.d.Store.ChangePassword(ctx, t, p.UserID, phc, p.SessionID, s.now().UTC())
	if err != nil {
		return mapErr("ganti sandi", err)
	}
	if len(revoked) > 0 {
		s.d.Sessions.MarkRevoked(ctx, revoked...)
	}
	if err := s.d.Lockout.RegisterSuccess(ctx, p.UserID, ip); err != nil {
		s.log.WarnContext(ctx, "identity: gagal mencatat sukses lockout", "user_id", p.UserID, "error", err.Error())
	}
	s.log.InfoContext(ctx, "identity: sandi diganti", "event", "identity.password_changed", "user_id", p.UserID, "revoked_sessions", len(revoked))
	return nil
}

// --- MFA ---

// BeginMFAEnrollment membuat secret TOTP pending untuk Principal (pengguna dari context, bukan input).
func (s *service) BeginMFAEnrollment(ctx context.Context) (sdkidentity.EnrollmentInfo, error) {
	ctx, p, t, _, err := s.principalCtx(ctx, "")
	if err != nil {
		return sdkidentity.EnrollmentInfo{}, err
	}
	info, err := s.d.MFA.BeginEnrollment(ctx, p.UserID)
	if err != nil {
		return sdkidentity.EnrollmentInfo{}, mapErr("daftar MFA", err)
	}
	s.d.MFAState.Invalidate(t, p.UserID)
	return info, nil
}

// ConfirmMFA mengaktifkan faktor dengan kode pertama dan menerbitkan recovery code (tampil sekali). Pemilik faktor
// baru saja membuktikan kepemilikannya, sehingga sesi dinaikkan ke amr [pwd, otp] dengan MEROTASI refresh token
// (PRD-001 Q27, ADR-0013 §4): token pra-MFA mati dan hasil memuat refresh token baru yang wajib ditulis ulang
// handler ke cookie. Hasil juga memuat access token otp, jadi klien tidak perlu memanggil Refresh lagi. Ini juga
// menyelesaikan autentikasi: hitungan lockout pasangan direset.
//
// Recovery code sudah dibuat dan hanya tampil sekali; kegagalan menaikkan sesi dicatat dan hasilnya tetap membawa
// kode (Session kosong, refresh token lama masih berlaku sampai klien memanggil VerifyMFA).
func (s *service) ConfirmMFA(ctx context.Context, in sdkidentity.ConfirmMFAInput) (sdkidentity.ConfirmMFAResult, error) {
	ctx, p, t, ip, err := s.principalCtx(ctx, in.ClientIP)
	if err != nil {
		return sdkidentity.ConfirmMFAResult{}, err
	}
	codes, err := s.d.MFA.ConfirmEnrollment(ctx, p.UserID, in.Code)
	if err != nil {
		return sdkidentity.ConfirmMFAResult{}, mapErr("konfirmasi MFA", err)
	}
	s.d.MFAState.Invalidate(t, p.UserID)
	res := sdkidentity.ConfirmMFAResult{Codes: codes}
	if sess, uerr := s.upgradeToOTP(ctx, t, p); uerr != nil {
		s.log.WarnContext(ctx, "identity: gagal menaikkan sesi setelah konfirmasi MFA", "user_id", p.UserID, "session_id", p.SessionID, "error", uerr.Error())
	} else {
		res.Session = sess
	}
	s.fullyAuthenticated(ctx, t, p.UserID, ip)
	return res, nil
}

// VerifyMFA memverifikasi kode TOTP (atau recovery code bila in.Recovery) untuk sesi Principal, menaikkan sesi ke
// amr [pwd, otp] dengan merotasi refresh token, dan menerbitkan access token baru (PRD-001 Q27). LoginResult
// memuat RefreshToken baru yang WAJIB ditulis ulang handler ke cookie; token pra-MFA tidak berlaku lagi. Verifikasi
// yang gagal tidak pernah menghasilkan token otp maupun mengubah sesi.
//
// Bila sesi ternyata sudah otp (verifikasi serentak, atau access token pra-MFA yang masih berlaku), pemanggil tetap
// baru saja membuktikan faktornya: access token otp diterbitkan tanpa RefreshToken (cookie yang ada, hasil rotasi
// pemenang, tidak boleh ditimpa).
func (s *service) VerifyMFA(ctx context.Context, in sdkidentity.VerifyInput) (sdkidentity.LoginResult, error) {
	ctx, p, t, _, err := s.principalCtx(ctx, in.ClientIP)
	if err != nil {
		return sdkidentity.LoginResult{}, err
	}
	if in.Recovery {
		err = s.d.MFA.UseRecoveryCode(ctx, p.UserID, in.Code)
	} else {
		err = s.d.MFA.Verify(ctx, p.UserID, in.Code)
	}
	if err != nil {
		return sdkidentity.LoginResult{}, mapErr("verifikasi MFA", err)
	}
	// (mfa.Service sudah memanggil Lockout.RegisterSuccess pada verifikasi sukses; tidak dihitung ganda di sini.)
	res, err := s.upgradeToOTP(ctx, t, p)
	if err != nil {
		return sdkidentity.LoginResult{}, err
	}
	if terr := s.d.Store.TouchLastLogin(ctx, t, p.UserID, s.now().UTC()); terr != nil {
		s.log.WarnContext(ctx, "identity: gagal memperbarui login terakhir", "user_id", p.UserID, "error", terr.Error())
	}
	return res, nil
}

// upgradeToOTP menaikkan sesi Principal ke otp secara atomik dan merotasi refresh tokennya, lalu menerbitkan access
// token otp. Access token diterbitkan LEBIH DULU (tanpa efek samping) supaya kegagalan penandatanganan tidak
// meninggalkan sesi yang sudah dirotasi tanpa token baru yang sampai ke klien. Sesi harus milik Principal dan belum
// tercabut/kedaluwarsa (pertahanan berlapis di atas klaim sid yang sudah diverifikasi middleware): selain itu
// ErrUnauthenticated. RefreshToken terisi hanya bila rotasi terjadi (pemanggil ini yang menaikkan sesi).
func (s *service) upgradeToOTP(ctx context.Context, t tenancy.TenantID, p sdkidentity.Principal) (sdkidentity.LoginResult, error) {
	amr := slices.Clone(p.AMR)
	if !slices.Contains(amr, sdkidentity.AMROTP) {
		amr = append(amr, sdkidentity.AMROTP)
	}
	res, err := s.issue(t, p.UserID, p.SessionID, amr, sdkidentity.MFANone)
	if err != nil {
		return sdkidentity.LoginResult{}, mapErr("terbitkan token", err)
	}
	_, rt, err := s.d.Sessions.Upgrade(ctx, p.UserID, p.SessionID, sdkidentity.AMROTP)
	switch {
	case err == nil:
		res.RefreshToken, res.RefreshExpiresAt = rt.Value, rt.ExpiresAt
		return res, nil
	case errors.Is(err, session.ErrAlreadyUpgraded):
		return res, nil
	case errors.Is(err, session.ErrSessionNotFound):
		return sdkidentity.LoginResult{}, sdkidentity.ErrUnauthenticated
	}
	return sdkidentity.LoginResult{}, mapErr("naikkan sesi", err)
}

// RegenerateRecoveryCodes menerbitkan set recovery code baru dan mencabut yang lama. Menuntut sesi yang sudah
// terverifikasi MFA (amr memuat otp), sesuai kewajiban pemanggil pada mfa.Service.RegenerateRecoveryCodes.
func (s *service) RegenerateRecoveryCodes(ctx context.Context) (sdkidentity.RecoveryCodes, error) {
	ctx, p, _, _, err := s.principalCtx(ctx, "")
	if err != nil {
		return sdkidentity.RecoveryCodes{}, err
	}
	if !p.MFAVerified() {
		return sdkidentity.RecoveryCodes{}, sdkidentity.ErrMFARequired
	}
	codes, err := s.d.MFA.RegenerateRecoveryCodes(ctx, p.UserID)
	if err != nil {
		return sdkidentity.RecoveryCodes{}, mapErr("recovery code", err)
	}
	return sdkidentity.RecoveryCodes{Codes: codes}, nil
}
