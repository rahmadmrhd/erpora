package runtime

import (
	"context"
	"errors"
	"log/slog"

	"erpora/sdk/identity"
)

// Decision adalah keputusan otorisasi bertipe (ADR-0013 §4). Nilai nol sengaja Unauthenticated: Decision yang
// tidak diinisialisasi (atau Authorizer yang lupa mengisinya) gagal tertutup, bukan mengizinkan.
type Decision int

const (
	// Unauthenticated: tidak ada Principal pada context. Dijawab 401 `unauthenticated`.
	Unauthenticated Decision = iota
	// TokenExpired: token sah tetapi kedaluwarsa. Dijawab 401 `token_expired`. Authenticate (identity) sudah
	// menjawabnya langsung untuk route non-Public; nilai ini untuk Authorizer yang diberi tahu sebab lain.
	TokenExpired
	// Forbidden: terautentikasi tetapi tidak berhak (permission, atau token ber-scope terbatas). Dijawab 403
	// `permission_denied`.
	Forbidden
	// MFARequired: aturan MFA ADR-0013 §4 belum terpenuhi. Dijawab 403 `mfa_required`.
	MFARequired
	// Allow: izinkan.
	Allow
)

// String untuk log dan pesan tes.
func (d Decision) String() string {
	switch d {
	case Unauthenticated:
		return "unauthenticated"
	case TokenExpired:
		return "token_expired"
	case Forbidden:
		return "forbidden"
	case MFARequired:
		return "mfa_required"
	case Allow:
		return "allow"
	}
	return "unknown"
}

// RouteOptions adalah sifat route yang memengaruhi keputusan. Diisi Mount dari registry.RouteInfo; modul tidak
// pernah mengisinya langsung (kecuali RequireMFA lewat module.RequireMFA).
type RouteOptions struct {
	// RequireMFA menuntut amr memuat otp apa pun status faktor pengguna (module.RequireMFA).
	RequireMFA bool
	// PendingVerify: route boleh dipanggil sesi yang faktor MFA-nya aktif tetapi belum terverifikasi
	// (mfa/verify, mfa/recovery, logout). Hanya untuk route auth modul erp.core (lihat mfaPending).
	PendingVerify bool
	// PendingEnroll: route boleh dipanggil administrator yang belum punya faktor (mfa/enroll, mfa/confirm, logout).
	PendingEnroll bool
}

// MFAState adalah keadaan MFA pengguna yang dibutuhkan keputusan; diperoleh dari MFAStateReader per request
// (klaim token saja tidak cukup: reset MFA atau pendaftaran baru mengubahnya di tengah sesi).
type MFAState struct {
	// FactorActive: pengguna punya faktor TOTP terkonfirmasi.
	FactorActive bool
	// EnrollmentRequired: pengguna wajib punya faktor (pemegang role administrator; FR-107, Q9).
	EnrollmentRequired bool
}

// MFAStateReader membaca MFAState pengguna pada tenant context. Galat membuat Decide gagal tertutup (MFARequired).
type MFAStateReader interface {
	MFAState(ctx context.Context, userID string) (MFAState, error)
}

// Decider memutuskan akses bertipe. Authorizer.Allow lama tetap ada; Mount memakai Decide bila Authorizer juga
// mengimplementasikannya, selain itu Allow false dipetakan ke Forbidden (stub dan modul tidak berubah).
type Decider interface {
	Decide(ctx context.Context, permission string, opts RouteOptions) Decision
}

// TypedAuthorizer menggabungkan pemeriksa permission (Authorizer biasa: stub sampai C2) dengan aturan identitas:
// Principal wajib, aturan MFA ADR-0013 §4 untuk SEMUA pengguna manusia, dan RequireMFA.
//
// Urutan Decide: tanpa Principal -> Unauthenticated; token ber-scope terbatas -> Forbidden; aturan MFA ->
// MFARequired; permission -> Forbidden; selain itu Allow. MFA diperiksa SEBELUM permission agar sesi yang belum
// memenuhi MFA tidak dapat menebak permission lewat 403 permission_denied.
type TypedAuthorizer struct {
	permissions Authorizer
	mfa         MFAStateReader
	log         *slog.Logger
}

var (
	_ Authorizer = (*TypedAuthorizer)(nil)
	_ Decider    = (*TypedAuthorizer)(nil)
)

// ErrNilPermissionAuthorizer: NewTypedAuthorizer tanpa pemeriksa permission.
var ErrNilPermissionAuthorizer = errors.New("runtime: pemeriksa permission wajib")

// NewTypedAuthorizer membuat TypedAuthorizer. permissions wajib (stub atau mesin permission C2); mfa wajib
// (tanpa pembaca keadaan MFA aturan §4 tidak dapat ditegakkan, jadi tidak ada mode "lewati").
func NewTypedAuthorizer(permissions Authorizer, mfa MFAStateReader, log *slog.Logger) (*TypedAuthorizer, error) {
	if permissions == nil {
		return nil, ErrNilPermissionAuthorizer
	}
	if mfa == nil {
		return nil, errors.New("runtime: MFAStateReader wajib")
	}
	if log == nil {
		log = slog.Default()
	}
	return &TypedAuthorizer{permissions: permissions, mfa: mfa, log: log}, nil
}

// Allow adalah adaptor Authorizer lama: true hanya bila Decide(ctx, permission, RouteOptions{}) == Allow.
func (a *TypedAuthorizer) Allow(ctx context.Context, permission string) bool {
	return a.Decide(ctx, permission, RouteOptions{}) == Allow
}

// Decide: lihat doc TypedAuthorizer.
func (a *TypedAuthorizer) Decide(ctx context.Context, permission string, opts RouteOptions) Decision {
	p, ok := identity.FromContext(ctx)
	if !ok || p.UserID == "" {
		return Unauthenticated
	}
	// Token ber-scope terbatas (mis. scope refresh atau PAT) tidak berlaku di route biasa. Authenticate sudah menolak
	// scope yang tidak dikenal; ini pertahanan berlapis bila Principal sampai di sini lewat jalur lain.
	if len(p.TokenScope) > 0 {
		return Forbidden
	}
	if d := a.mfaDecision(ctx, p, opts); d != Allow {
		return d
	}
	if !a.permissions.Allow(ctx, permission) {
		return Forbidden
	}
	return Allow
}

// mfaDecision menerapkan ADR-0013 §4. Hanya Principal manusia yang tunduk pada MFA (akun layanan memakai token
// mesin tanpa faktor kedua).
func (a *TypedAuthorizer) mfaDecision(ctx context.Context, p identity.Principal, opts RouteOptions) Decision {
	if p.Kind != identity.KindHuman {
		if opts.RequireMFA {
			return MFARequired // faktor kedua tidak pernah terverifikasi pada token mesin
		}
		return Allow
	}
	verified := p.MFAVerified()
	if verified && !opts.RequireMFA {
		// Sesi yang sudah memverifikasi faktor kedua tidak membayar pencarian status (jalur panas). Konsekuensinya,
		// reset MFA oleh admin lain harus mencabut sesi pengguna itu (Admin.ResetMFA/LogoutAllDevices; C1.12).
		return Allow
	}
	st, err := a.mfa.MFAState(ctx, p.UserID)
	if err != nil {
		a.log.ErrorContext(ctx, "keadaan MFA tidak terbaca; akses ditolak (gagal tertutup)", "error", err)
		return MFARequired
	}
	switch {
	case opts.RequireMFA && !verified:
		return MFARequired
	case st.FactorActive && !verified:
		// Faktor aktif tetapi belum terverifikasi pada sesi ini: hanya mfa/verify, mfa/recovery, logout.
		if opts.PendingVerify {
			return Allow
		}
		return MFARequired
	case st.EnrollmentRequired && !st.FactorActive:
		// Administrator tanpa faktor: hanya mfa/enroll, mfa/confirm, logout.
		if opts.PendingEnroll {
			return Allow
		}
		return MFARequired
	}
	return Allow
}
