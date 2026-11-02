package identity

import (
	"context"
	"time"
)

// CreateAdminInput adalah masukan Admin.CreateAdmin (command core:create-admin).
type CreateAdminInput struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// UserSummary adalah ringkasan akun hasil operasi administratif (tanpa data rahasia).
type UserSummary struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Status siklus hidup kunci penandatangan: kunci baru `pending` (diterbitkan di JWKS lebih dulu agar
// instance lain sempat memuatnya), lalu `active` (menandatangani), `retiring` (hanya memverifikasi
// sampai token terakhirnya kedaluwarsa), dan `retired` (ditolak).
const (
	KeyStatusPending  = "pending"
	KeyStatusActive   = "active"
	KeyStatusRetiring = "retiring"
	KeyStatusRetired  = "retired"
)

// KeyInfo menjelaskan kunci penandatangan JWT (tanpa material kunci). ActivatesAt adalah saat kunci
// `pending` mulai menandatangani (nol bila sudah aktif), sehingga CLI dapat melaporkannya.
type KeyInfo struct {
	KeyID       string    `json:"keyId"`
	Algorithm   string    `json:"algorithm"`
	Status      string    `json:"status"` // KeyStatusPending | KeyStatusActive | KeyStatusRetiring | KeyStatusRetired
	CreatedAt   time.Time `json:"createdAt"`
	ActivatesAt time.Time `json:"activatesAt"`
}

// Admin adalah operasi identitas administratif (PRD-001 §3) untuk command CLI core dan layar admin.
// Berbeda dari Service, target dinyatakan eksplisit (email atau userID) sehingga implementasi wajib
// menegakkan otorisasinya sendiri: principal di ctx harus berhak, atau konteks hak sistem teraudit
// (authz.AsSystem) untuk CLI. Implementasi tidak mempercayai pemanggil dan mengaudit setiap operasi.
type Admin interface {
	CreateAdmin(ctx context.Context, in CreateAdminInput) (UserSummary, error)
	RotateSigningKeys(ctx context.Context) (KeyInfo, error)
	// ResetMFA menghapus enrolment MFA pengguna lain (kehilangan perangkat).
	ResetMFA(ctx context.Context, email string) error
	// UnlockAccount membuka akun berstatus locked.
	UnlockAccount(ctx context.Context, email string) error
	// LogoutAllDevices mencabut seluruh sesi pengguna.
	LogoutAllDevices(ctx context.Context, userID string) error
}

type adminKey struct{}

// adminBox: lihat principalBox; mencegah pemalsuan dan pembacaan Admin lewat kunci tiruan refleksi.
type adminBox struct {
	a  Admin
	ok bool
}

// WithAdmin memasang a di ctx; dipanggil platform saat merakit modul dan command.
func WithAdmin(ctx context.Context, a Admin) context.Context {
	return context.WithValue(ctx, adminKey{}, &adminBox{a: a, ok: true})
}

// AdminFrom mengembalikan Admin di ctx; ok false bila tidak ada, nil, nil bertipe, atau nilai di bawah
// kunci itu bukan kotak sah buatan WithAdmin.
func AdminFrom(ctx context.Context) (Admin, bool) {
	b, ok := ctx.Value(adminKey{}).(*adminBox)
	if !ok || b == nil || !b.ok || isNil(b.a) {
		return nil, false
	}
	return b.a, true
}
