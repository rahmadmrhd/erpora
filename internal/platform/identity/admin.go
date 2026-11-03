package identity

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/authz"
	sdkidentity "erpora/sdk/identity"
)

const maxAdminNameRunes = 200

// SystemReasonPrefix adalah awalan alasan hak sistem (authz.AsSystem) yang diterima operasi admin: command CLI `core:*`
// dipasang runtime sebagai "cli:<nama command>" (internal/app/command.go), mis. "cli:core:create-admin".
const SystemReasonPrefix = "cli:core:"

// admin adalah implementasi sdk/identity.Admin: operasi administratif untuk command CLI core dan layar admin.
//
// Otorisasi ditegakkan DI SINI, tidak dipercayakan pada pemanggil (CLAUDE.md, implementasi contract/):
//   - bila context membawa Principal: pemanggil wajib memegang role administrator dan sesi yang terverifikasi MFA;
//     hak sistem pada context TIDAK mengangkat Principal yang bukan administrator;
//   - bila tidak ada Principal: wajib hak sistem teraudit (authz.AsSystem) dengan alasan berawalan "cli:core:"
//     (SystemReasonPrefix; PRD-001 Q30 butir 1); pelakunya adalah alasan sistem itu. Alasan lain ditolak.
//
// Parameter tidak membawa tenant atau identitas pemanggil; email/ID hanyalah sasaran. Setiap aksi dicatat lewat slog
// terstruktur dan Auditor (opsional) dengan actor; tidak pernah memuat email, sandi, atau secret.
type admin struct {
	d   Deps
	now func() time.Time
	log *slog.Logger
}

// NewAdmin merakit sdk/identity.Admin dari dependensi d (Gate, Hasher, Lockout, Sessions, Keys, MFA, MFAState,
// Settings, Store wajib).
func NewAdmin(d Deps) (sdkidentity.Admin, error) {
	if err := require("Admin",
		need{"Gate", d.Gate != nil}, need{"Hasher", d.Hasher != nil}, need{"Lockout", d.Lockout != nil},
		need{"Sessions", d.Sessions != nil}, need{"Keys", d.Keys != nil}, need{"MFA", d.MFA != nil},
		need{"MFAState", d.MFAState != nil}, need{"Settings", d.Settings != nil}, need{"Store", d.Store != nil}); err != nil {
		return nil, err
	}
	return &admin{d: d, now: d.clock(), log: d.logger()}, nil
}

var _ sdkidentity.Admin = (*admin)(nil)

// authorize memeriksa hak pemanggil dan mengembalikan tenant serta pelaku (id administrator atau alasan sistem).
func (a *admin) authorize(ctx context.Context) (tenancy.TenantID, string, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return tenancy.TenantID{}, "", errors.New("identity: context tanpa tenant")
	}
	if p, has := sdkidentity.FromContext(ctx); has && p.UserID != "" {
		isAdmin, err := a.d.Store.HasRole(ctx, t, p.UserID, mfa.AdministratorRole)
		if err != nil {
			return t, "", mapErr("periksa role", err)
		}
		if !isAdmin || !p.MFAVerified() {
			a.log.WarnContext(ctx, "identity: operasi admin ditolak", "event", "identity.admin_denied", "user_id", p.UserID)
			return t, "", sdkidentity.ErrForbidden
		}
		return t, p.UserID, nil
	}
	reason, ok := authz.SystemReason(ctx)
	if !ok || strings.TrimSpace(reason) == "" {
		a.log.WarnContext(ctx, "identity: operasi admin ditolak tanpa principal maupun hak sistem", "event", "identity.admin_denied")
		return t, "", sdkidentity.ErrForbidden
	}
	// Hanya command CLI core yang boleh memakai hak sistem untuk operasi admin (PRD-001 Q30 butir 1). Alasan lain
	// (hook lifecycle, seed, modul lain) tidak dikenali di sini. Perbandingan persis (huruf besar-kecil dan spasi
	// dihitung); alasan dicatat di log (nilai slog terstruktur, bukan teks bebas) agar penyalahgunaan terlacak.
	if !strings.HasPrefix(reason, SystemReasonPrefix) {
		a.log.WarnContext(ctx, "identity: operasi admin ditolak: alasan hak sistem bukan command core", "event", "identity.admin_denied", "system_reason", reason)
		return t, "", sdkidentity.ErrForbidden
	}
	return t, reason, nil
}

// audit mencatat aksi administratif (slog terstruktur + Auditor bila terpasang).
func (a *admin) audit(ctx context.Context, action, actor, target string) {
	at := a.now().UTC()
	a.log.InfoContext(ctx, "identity: aksi administratif", "event", action, "actor", actor, "target", target)
	if a.d.Auditor != nil {
		a.d.Auditor.Record(ctx, AuditEvent{Action: action, Actor: actor, Target: target, At: at})
	}
}

// CreateAdmin membuat akun administrator aktif. Sandi memenuhi kebijakan FR-101 dan di-hash lewat pembatas hash;
// sandi tidak pernah dicatat atau dikembalikan.
//
// Membuat akun, memberi role, dan mengaktifkan bukan satu transaksi (tabel berada di database berbeda pada mode
// multi-tenant), jadi urutannya dibuat aman bila terputus: akun dibuat NONAKTIF, role diberikan, dan status active
// diberikan TERAKHIR. Akun tidak pernah aktif tanpa role; kegagalan di tengah menyisakan akun nonaktif tanpa role
// (sisa yang tidak dapat dipakai), dan memanggil CreateAdmin lagi dengan email yang sama MELANJUTKAN akun itu
// (idempoten, PRD-001 Q28): sandi diatur ulang dari masukan baru, role diberikan, akun diaktifkan.
//
// Hanya sisa seperti itu yang dapat dilanjutkan: akun nonaktif, tanpa role apa pun, belum pernah login
// (last_login_at kosong), dan tanpa sesi sama sekali (termasuk yang sudah dicabut). Akun aktif, akun yang punya role,
// atau akun yang pernah dipakai tetap ErrEmailTaken, sehingga operasi ini tidak pernah menjadi jalur pengambilalihan
// akun yang ada.
func (a *admin) CreateAdmin(ctx context.Context, in sdkidentity.CreateAdminInput) (sdkidentity.UserSummary, error) {
	t, actor, err := a.authorize(ctx)
	if err != nil {
		return sdkidentity.UserSummary{}, err
	}
	email, name := store.NormalizeEmail(in.Email), strings.TrimSpace(in.Name)
	if email == "" || name == "" || utf8.RuneCountInString(name) > maxAdminNameRunes {
		return sdkidentity.UserSummary{}, sdkidentity.ErrInvalidInput
	}
	if v := password.CheckPolicy(in.Password); len(v) > 0 {
		return sdkidentity.UserSummary{}, policyError(v)
	}
	phc, err := a.d.Gate.Hash(ctx, a.d.Hasher, in.Password)
	if err != nil {
		return sdkidentity.UserSummary{}, mapErr("hash sandi", err)
	}
	u, err := a.d.Store.CreateUser(ctx, t, store.NewUser{Email: email, Name: name, PasswordHash: phc, Status: store.UserDisabled})
	action := "identity.admin_created"
	switch {
	case errors.Is(err, store.ErrEmailTaken):
		if u, err = a.resumable(ctx, t, email); err != nil {
			return sdkidentity.UserSummary{}, err
		}
		if err := a.d.Store.SetPasswordHash(ctx, t, u.ID, phc); err != nil {
			return sdkidentity.UserSummary{}, mapErr("atur ulang sandi", err)
		}
		action = "identity.admin_resumed"
	case err != nil:
		return sdkidentity.UserSummary{}, mapErr("buat pengguna", err)
	}
	if err := a.promote(ctx, t, u.ID); err != nil {
		return sdkidentity.UserSummary{}, err
	}
	a.audit(ctx, action, actor, u.ID)
	return sdkidentity.UserSummary{ID: u.ID, Email: u.Email, Name: u.Name}, nil
}

// resumable memuat akun dengan email itu bila ia sisa CreateAdmin yang terputus (nonaktif, tanpa role, belum pernah
// login, tanpa sesi); selain itu ErrEmailTaken tanpa membedakan sebabnya.
func (a *admin) resumable(ctx context.Context, t tenancy.TenantID, email string) (store.User, error) {
	u, err := a.d.Store.FindUserByEmail(ctx, t, email)
	if errors.Is(err, store.ErrNotFound) { // hilang di antara dua panggilan: perlakukan seperti tidak tersedia
		return store.User{}, sdkidentity.ErrEmailTaken
	}
	if err != nil {
		return store.User{}, mapErr("muat pengguna", err)
	}
	if u.Status != store.UserDisabled || u.LastLoginAt != nil {
		return store.User{}, sdkidentity.ErrEmailTaken
	}
	roles, err := a.d.Store.UserRoleKeys(ctx, t, u.ID)
	if err != nil {
		return store.User{}, mapErr("muat role pengguna", err)
	}
	if len(roles) > 0 {
		return store.User{}, sdkidentity.ErrEmailTaken
	}
	n, err := a.d.Store.CountUserSessions(ctx, t, u.ID)
	if err != nil {
		return store.User{}, mapErr("hitung sesi pengguna", err)
	}
	if n > 0 {
		return store.User{}, sdkidentity.ErrEmailTaken
	}
	return u, nil
}

// promote memberi role administrator lalu mengaktifkan akun (status active selalu terakhir). Bila aktivasi gagal,
// role dicabut (upaya terbaik) agar akun kembali ke bentuk sisa yang dapat dilanjutkan.
func (a *admin) promote(ctx context.Context, t tenancy.TenantID, userID string) error {
	if _, err := a.d.Store.GrantRole(ctx, t, userID, mfa.AdministratorRole, a.now().UTC()); err != nil {
		a.log.ErrorContext(ctx, "identity: pemberian role administrator gagal; akun tetap nonaktif dan dapat dilanjutkan", "user_id", userID, "error", err.Error())
		return mapErr("beri role administrator", err)
	}
	if err := a.d.Store.SetUserStatus(ctx, t, userID, store.UserActive); err != nil {
		cctx := context.WithoutCancel(ctx)
		if _, rerr := a.d.Store.RevokeRole(cctx, t, userID, mfa.AdministratorRole); rerr != nil {
			a.log.ErrorContext(ctx, "identity: pencabutan role setelah aktivasi gagal juga gagal; akun nonaktif berrole administrator", "user_id", userID, "error", rerr.Error())
		}
		a.log.ErrorContext(ctx, "identity: aktivasi akun administrator gagal; akun tetap nonaktif dan dapat dilanjutkan", "user_id", userID, "error", err.Error())
		return mapErr("aktifkan akun", err)
	}
	return nil
}

// RotateSigningKeys membuat kunci penandatangan baru berstatus pending (terpublikasi di JWKS, belum menandatangani);
// promosi menjadi active oleh pekerja latar `serve` setelah JWKSMaxAge. Mengembalikan ringkasan kunci baru
// (Status dan ActivatesAt).
func (a *admin) RotateSigningKeys(ctx context.Context) (sdkidentity.KeyInfo, error) {
	_, actor, err := a.authorize(ctx)
	if err != nil {
		return sdkidentity.KeyInfo{}, err
	}
	k, err := a.d.Keys.Rotate(ctx)
	if err != nil {
		return sdkidentity.KeyInfo{}, mapErr("rotasi kunci", err)
	}
	info := k.Info()
	a.audit(ctx, "identity.signing_key_rotated", actor, info.KeyID)
	return info, nil
}

// target memuat pengguna sasaran menurut email pada tenant context; ErrNotFound bila tidak ada.
func (a *admin) target(ctx context.Context, t tenancy.TenantID, email string) (store.User, error) {
	u, err := a.d.Store.FindUserByEmail(ctx, t, email)
	if err != nil {
		return store.User{}, mapErr("cari pengguna", err)
	}
	return u, nil
}

// ResetMFA menghapus faktor TOTP dan recovery code pengguna (kehilangan perangkat), mencabut seluruh sesinya (sesi
// ber-otp lama tidak boleh bertahan setelah faktor hilang), dan membuang cache keadaan MFA-nya.
func (a *admin) ResetMFA(ctx context.Context, email string) error {
	t, actor, err := a.authorize(ctx)
	if err != nil {
		return err
	}
	u, err := a.target(ctx, t, email)
	if err != nil {
		return err
	}
	// mfa.Service.Reset menegakkan lagi: dengan Principal, actor harus Principal.UserID dan bukan sasaran sendiri.
	if err := a.d.MFA.Reset(ctx, actor, u.ID); err != nil {
		return mapErr("reset MFA", err)
	}
	a.d.MFAState.Invalidate(t, u.ID)
	if err := a.d.Sessions.RevokeAllForUser(ctx, u.ID); err != nil {
		return mapErr("cabut sesi", err)
	}
	a.audit(ctx, "identity.mfa_reset", actor, u.ID)
	return nil
}

// UnlockAccount membuka akun locked dan menghapus jeda/blokir pasangannya. Idempoten.
func (a *admin) UnlockAccount(ctx context.Context, email string) error {
	t, actor, err := a.authorize(ctx)
	if err != nil {
		return err
	}
	u, err := a.target(ctx, t, email)
	if err != nil {
		return err
	}
	if err := a.d.Lockout.Unlock(ctx, u.ID); err != nil {
		return mapErr("buka kunci", err)
	}
	a.audit(ctx, "identity.account_unlocked", actor, u.ID)
	return nil
}

// LogoutAllDevices mencabut seluruh sesi pengguna sasaran (id harus anggota tenant ini; selain itu ErrNotFound).
func (a *admin) LogoutAllDevices(ctx context.Context, userID string) error {
	t, actor, err := a.authorize(ctx)
	if err != nil {
		return err
	}
	u, err := a.d.Store.GetUser(ctx, t, userID)
	if err != nil {
		return mapErr("cari pengguna", err)
	}
	if err := a.d.Sessions.RevokeAllForUser(ctx, u.ID); err != nil {
		return mapErr("cabut sesi", err)
	}
	a.audit(ctx, "identity.sessions_revoked_all", actor, u.ID)
	return nil
}

// SetPublicURL mengisi platform.public_url (validasi dan normalisasi oleh settings.Service). updatedBy adalah
// pelaku (id administrator atau "cli:core:...").
func (a *admin) SetPublicURL(ctx context.Context, rawURL string) error {
	_, actor, err := a.authorize(ctx)
	if err != nil {
		return err
	}
	if err := a.d.Settings.SetPublicURL(ctx, rawURL, actor); err != nil {
		return mapErr("simpan URL publik", err)
	}
	a.audit(ctx, "identity.public_url_set", actor, "")
	return nil
}
