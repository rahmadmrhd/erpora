// Package identity memuat primitif identitas yang dipakai modul: Principal di context, antarmuka
// Service (login, refresh, reset sandi, MFA) dan Admin (operasi administratif/CLI) yang diimplementasi
// platform (PRD-001 FR-101, FR-102, FR-106, FR-107, §3). Paket ini daun (D9): hanya stdlib, tanpa tipe
// platform.
package identity

import (
	"context"
	"errors"
	"reflect"
	"slices"
)

// ErrUnauthenticated: operasi butuh Principal di context tetapi tidak ada (atau kosong). Handler
// memetakannya ke 401 `unauthenticated`.
var ErrUnauthenticated = errors.New("identity: tidak terautentikasi")

// Kind membedakan jenis pelaku.
type Kind string

const (
	// KindHuman adalah pengguna manusia (login interaktif).
	KindHuman Kind = "human"
	// KindService adalah akun layanan/integrasi (token mesin).
	KindService Kind = "service"
)

// AuthMethod adalah cara Principal diautentikasi (bukan faktor yang dipakai; itu AMR).
type AuthMethod string

const (
	// AuthMethodPassword: login lokal email+sandi (dengan atau tanpa TOTP; lihat Principal.AMR).
	AuthMethodPassword AuthMethod = "password"
	// AuthMethodOIDC: login lewat penyedia identitas eksternal (OpenID Connect).
	AuthMethodOIDC AuthMethod = "oidc"
	// AuthMethodPAT: personal access token.
	AuthMethodPAT AuthMethod = "pat"
	// AuthMethodClientCredentials: client credentials akun layanan.
	AuthMethodClientCredentials AuthMethod = "client_credentials"
)

// Nilai AMR (Authentication Methods References, RFC 8176) yang dipakai pada Principal.AMR dan klaim `amr` JWT.
const (
	AMRPassword = "pwd" // sandi diverifikasi
	AMROTP      = "otp" // TOTP atau recovery code diverifikasi
)

// Principal adalah identitas pemanggil yang sudah terautentikasi. Dibangun platform (middleware
// Authenticate) dari token; modul membacanya lewat FromContext dan tidak pernah membangunnya sendiri
// kecuali di test (identitytest). Tenant sengaja tidak ada di sini: selalu dari tenancy.FromContext.
type Principal struct {
	UserID     string
	Kind       Kind
	SessionID  string
	AuthMethod AuthMethod
	// AMR adalah faktor yang sudah diverifikasi pada sesi ini (AMRPassword, AMROTP); dasar RequireMFA.
	AMR []string
	// TokenScope membatasi token; kosong = tanpa pembatasan tambahan.
	TokenScope []string
}

// MFAVerified melaporkan apakah faktor kedua (AMROTP) sudah diverifikasi pada sesi ini. Diturunkan dari
// AMR, bukan field terpisah, agar tidak ada dua sumber kebenaran yang dapat tak sinkron.
func (p Principal) MFAVerified() bool { return slices.Contains(p.AMR, AMROTP) }

type principalKey struct{}

// principalBox membungkus Principal di context. Tipe dan field-nya tak diekspor dengan sengaja: modul
// jahat dapat menelusuri rantai context lewat refleksi, meniru tipe kunci principalKey, dan memasang
// nilai Principal mentah dengan kunci yang sama. Karena yang diterima FromContext hanya *principalBox
// dengan ok true, nilai mentah atau kotak nol buatan reflect.New ditolak (field tak diekspor tidak dapat
// diisi lewat API refleksi biasa), dan isi kotak sah tidak terbaca lewat ctx.Value(kunciTiruan).
//
// Batas yang dinyatakan jujur: ini mempersulit pemalsuan murni-refleksi, tetapi TIDAK menghalangi kode
// yang memakai unsafe.Pointer, termasuk lewat reflect.NewAt (nilai unsafe.Pointer dapat diperoleh dari
// reflect.Value.UnsafePointer tanpa menyebut paket unsafe), yang dapat mengisi kotak nol maupun
// memutasi kotak sah di tempat. Itu batas bahasa Go, bukan sesuatu yang dapat diperbaiki di sini.
// Mitigasinya adalah aturan arsitektur IDENT yang menandai ekspresi unsafe.Pointer di kode modul
// (ditegakkan archcheck), bukan kotak ini.
type principalBox struct {
	p  Principal
	ok bool
}

// WithPrincipal memasang p di ctx. Salinan disimpan (termasuk slice AMR dan TokenScope) sehingga
// mutasi berikutnya oleh pemanggil tidak mengubah identitas di context.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, &principalBox{p: p.clone(), ok: true})
}

// FromContext mengembalikan Principal di ctx. Hasilnya salinan: mengubahnya (termasuk AMR dan
// TokenScope) tidak mengubah isi context. ok false bila tidak ada principal (tidak terautentikasi)
// atau nilai di bawah kunci itu bukan kotak sah buatan WithPrincipal (gagal tertutup).
func FromContext(ctx context.Context) (Principal, bool) {
	b, ok := ctx.Value(principalKey{}).(*principalBox)
	if !ok || b == nil || !b.ok {
		return Principal{}, false
	}
	return b.p.clone(), true
}

// RequirePrincipal mengembalikan Principal di ctx, atau ErrUnauthenticated bila tidak ada atau tanpa
// UserID. Dipakai Service yang menurunkan pengguna/sesi dari context, bukan dari parameter pemanggil.
func RequirePrincipal(ctx context.Context) (Principal, error) {
	p, ok := FromContext(ctx)
	if !ok || p.UserID == "" {
		return Principal{}, ErrUnauthenticated
	}
	return p, nil
}

func (p Principal) clone() Principal {
	p.AMR = slices.Clone(p.AMR)
	p.TokenScope = slices.Clone(p.TokenScope)
	return p
}

// isNil melaporkan nil biasa maupun nil bertipe pada tipe yang dapat nil, agar From* tidak
// mengembalikan antarmuka yang membungkus nil dan panik saat dipanggil.
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Chan, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	}
	return false
}
