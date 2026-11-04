package authz

import (
	"context"
	"log/slog"

	"erpora/sdk/identity"
)

// Engine diimplementasi platform (internal/platform/authorization, C2.4) dan dipasang di context lewat
// WithEngine. Fungsi paket Require, Filter, Check, Fields, MaskFields, dan CheckWrite hanya
// mendelegasikan ke Engine setelah memastikan ada principal atau hak sistem; modul tidak pernah
// memanggil Engine langsung.
//
// Kontrak untuk implementer (wajib, semuanya gagal tertutup, NFR-106): setiap galat evaluasi, cache
// rusak, nilai scope tersimpan yang tidak sah, panic ScopeProvider, dan nilai record yang tak terbaca
// berarti ditolak. Hak sistem (AsSystem) diperlakukan sebagai izin penuh dan dicatat (authz.system_call).
// Engine tidak mengimpor GORM (D3b): scope baris dikembalikan sebagai RowFilter netral.
type Engine interface {
	// Require memeriksa lapis aksi ("<resource>.<aksi>") tanpa data record. Tanpa hak: ErrForbidden.
	Require(ctx context.Context, permission string) error
	// Filter mengevaluasi scope baris untuk (resource, aksi): satu RowBranch per baris matriks yang memuat
	// aksi itu (union per baris, FR-111), kondisi dalam cabang digabung AND: scope `all` menjadi
	// AllRows(), `owner` menjadi OwnerIs(owner_field, userID), `assigned:<dim>` menjadi
	// AssignedIn(scope_fields[dim], nilai penugasan) dan nilai kosong tetap dikirim kosong (FALSE),
	// `custom:<key>` menjadi CustomScope(provider). RowFilter kosong (tanpa cabang) berarti tolak semua.
	// Galat apa pun berarti tolak; hasil RowFilter diabaikan bila galat non-nil. Nama kolom berasal dari
	// manifest tervalidasi, tidak pernah dari input.
	Filter(ctx context.Context, resource, action string) (RowFilter, error)
	// Check memeriksa aksi atas satu record. Tanpa baris yang memuat aksi: ErrForbidden; ada baris yang
	// memuat aksi tetapi tak satu pun scope-nya cocok dengan record: ErrNotFound. Union dihitung per baris.
	Check(ctx context.Context, permission string, rec Record) error
	// Fields mengembalikan akses grup field principal pada resource; tak pernah nil pada implementasi
	// yang benar (nil diperlakukan sebagai semua none).
	Fields(ctx context.Context, resource string) FieldAccess
	// MaskFields menghapus field berakses none; kunci fields boleh camelCase atau snake_case (gunakan
	// ApplyMask pada hasil Fields). fields adalah salinan yang boleh diubah.
	MaskFields(ctx context.Context, resource string, fields map[string]any) map[string]any
	// CheckWrite menolak perubahan field tanpa akses write dengan *FieldForbiddenError berisi semua
	// field yang ditolak (gunakan CheckWriteAccess pada hasil Fields).
	CheckWrite(ctx context.Context, resource string, changedFields []string) error
}

type engineKey struct{}

// engineBox membungkus Engine di context agar nilai mentah di bawah kunci tiruan (lewat refleksi) tidak
// diterima; pola yang sama dengan principalBox di sdk/identity. Penegakan lanjutan (hanya platform yang
// boleh memanggil WithEngine) lewat aturan arsitektur IDENT (C2.7).
type engineBox struct {
	e  Engine
	ok bool
}

// WithEngine memasang e di ctx. HANYA platform (internal/app/internal/platform) yang boleh memanggilnya;
// kode modul dilarang (aturan arsitektur IDENT, C2.7).
func WithEngine(ctx context.Context, e Engine) context.Context {
	return context.WithValue(ctx, engineKey{}, &engineBox{e: e, ok: true})
}

func engineFrom(ctx context.Context) (Engine, bool) {
	b, ok := ctx.Value(engineKey{}).(*engineBox)
	if !ok || b == nil || !b.ok || isNilValue(b.e) {
		return nil, false
	}
	return b.e, true
}

// hasActor melaporkan apakah ctx membawa principal sah (UserID tidak kosong) atau hak sistem.
func hasActor(ctx context.Context) bool {
	if p, ok := identity.FromContext(ctx); ok && p.UserID != "" {
		return true
	}
	_, ok := SystemReason(ctx)
	return ok
}

func recoverLog(ctx context.Context, op string) {
	if r := recover(); r != nil {
		slog.ErrorContext(ctx, "authz: panic pada Engine", "op", op, "panic", r)
	}
}

// Require memeriksa lapis aksi: apakah principal boleh melakukan permission ("<resource>.<aksi>")
// pada sebagian data (aturan scope bersyarat tetap wajib ditegakkan dengan Filter atau Check).
// Tanpa principal dan tanpa hak sistem: ErrUnauthenticated. Tanpa mesin, galat atau panic mesin:
// ErrForbidden atau galat dari mesin (gagal tertutup).
func Require(ctx context.Context, permission string) (err error) {
	if !hasActor(ctx) {
		return ErrUnauthenticated
	}
	e, ok := engineFrom(ctx)
	if !ok {
		return ErrForbidden
	}
	defer func() {
		if r := recover(); r != nil {
			recoverLog(ctx, "Require")
			err = ErrForbidden
		}
	}()
	return e.Require(ctx, permission)
}

// Check memeriksa lapis record: apakah principal boleh melakukan permission atas satu record. record
// berupa Record atau map[string]any (kunci boleh camelCase atau snake_case); tipe lain ditolak dengan
// ErrUnsupportedRecord. Record di luar scope: ErrNotFound (HTTP 404). Tanpa principal dan tanpa hak
// sistem atau tanpa mesin: ErrNotFound (bukan 401) agar implementasi kontrak yang dipanggil dengan
// context.Background() menjawab "tidak ditemukan" sesuai FR-119; di handler HTTP middleware sudah
// menjawab 401 lebih dulu.
func Check(ctx context.Context, permission string, record any) (err error) {
	rec, ok := asRecord(record)
	if !ok {
		return ErrUnsupportedRecord
	}
	if !hasActor(ctx) {
		return ErrNotFound
	}
	e, ok := engineFrom(ctx)
	if !ok {
		return ErrNotFound
	}
	defer func() {
		if r := recover(); r != nil {
			recoverLog(ctx, "Check")
			err = ErrNotFound
		}
	}()
	return e.Check(ctx, permission, rec)
}

// Fields mengembalikan akses grup field principal pada resource (FR-112). Selalu non-nil; tanpa
// principal, tanpa mesin, atau galat mesin: semua none.
func Fields(ctx context.Context, resource string) (fa FieldAccess) {
	deny := denyAllFields{resource: resource}
	if !hasActor(ctx) {
		return deny
	}
	e, ok := engineFrom(ctx)
	if !ok {
		return deny
	}
	defer func() {
		if r := recover(); r != nil {
			recoverLog(ctx, "Fields")
			fa = deny
		}
	}()
	if fa = e.Fields(ctx, resource); fa == nil || isNilValue(fa) {
		return deny
	}
	return fa
}

// CheckWrite memeriksa bahwa principal boleh menulis semua changedFields pada resource; selain itu
// *FieldForbiddenError berisi seluruh field yang ditolak (HTTP 403 field_forbidden; FR-112). Nama field
// boleh camelCase atau snake_case. Tanpa principal, tanpa mesin, atau panic mesin: semua field ditolak.
func CheckWrite(ctx context.Context, resource string, changedFields []string) (err error) {
	deny := func() error { return newFieldForbidden(resource, changedFields) }
	if !hasActor(ctx) {
		return deny()
	}
	e, ok := engineFrom(ctx)
	if !ok {
		return deny()
	}
	defer func() {
		if r := recover(); r != nil {
			recoverLog(ctx, "CheckWrite")
			err = deny()
		}
	}()
	return e.CheckWrite(ctx, resource, changedFields)
}

// maskWithEngine menjalankan Engine.MaskFields; ok false bila tidak ada mesin atau ctx tanpa aktor.
// hasil nil atau panic menjadi map kosong (gagal tertutup).
func maskWithEngine(ctx context.Context, resource string, fields map[string]any) (out map[string]any, used bool) {
	e, ok := engineFrom(ctx)
	if !ok {
		return nil, false
	}
	if !hasActor(ctx) {
		return map[string]any{}, true
	}
	defer func() {
		if r := recover(); r != nil {
			recoverLog(ctx, "MaskFields")
			out, used = map[string]any{}, true
		}
	}()
	out = e.MaskFields(ctx, resource, fields)
	if out == nil {
		out = map[string]any{}
	}
	return out, true
}
