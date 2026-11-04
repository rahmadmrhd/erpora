package authz

import "context"

// Admin adalah antarmuka administrasi hak akses (role, matriks, field access, penugasan scope, unit
// organisasi, hak efektif, penjelas akses; FR-113, FR-114, FR-118) yang dipasang platform hanya pada
// context modul `core`, seperti identity.Admin.
//
// PLACEHOLDER: deklarasi ini baru menetapkan tipe dan pemasangan di context (Task C2.3). Daftar metodenya
// didefinisikan dan diimplementasi Task C2.10 (Interfaces C2.10); setiap metode kelak menegakkan
// permission sendiri lewat Require terhadap principal di context.
type Admin interface{}

type adminKey struct{}

// adminBox: lihat engineBox.
type adminBox struct {
	a  Admin
	ok bool
}

// WithAdmin memasang a di ctx; dipanggil platform saat merakit context modul `core`.
func WithAdmin(ctx context.Context, a Admin) context.Context {
	return context.WithValue(ctx, adminKey{}, &adminBox{a: a, ok: true})
}

// AdminFrom mengembalikan Admin di ctx; ok false bila tidak ada, nil, nil bertipe, atau nilai di bawah
// kunci itu bukan kotak sah buatan WithAdmin.
func AdminFrom(ctx context.Context) (Admin, bool) {
	b, ok := ctx.Value(adminKey{}).(*adminBox)
	if !ok || b == nil || !b.ok || isNilValue(b.a) {
		return nil, false
	}
	return b.a, true
}
