package naming

import "strings"

// Target adalah bahasa atau sistem keluaran yang bentrokannya diperiksa.
type Target string

// Target yang diperiksa.
const (
	TargetSQL Target = "sql"
	TargetGo  Target = "go"
	TargetTS  Target = "typescript"
)

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// Pemeriksaan bentrokan nama bersifat PER TARGET dan hanya pada bentuk hasil konversi (keputusan pemilik
// produk, 2 Nov 2026; menggantikan daftar kata tercadang tetap). Kata kunci bahasa (type, order, user, group,
// limit, default, left, class, let, ...) sah sebagai nama field karena tidak ada keluaran bangkitan yang
// menaruh nama field pada posisi yang menafsirkannya sebagai kata kunci:
//
//   - SQL: setiap identifier dikutip lewat sqlgen.QuoteIdent (DDL, CHECK, indeks, pemicu, DO block migrasi),
//     sehingga "order" atau "user" valid. Yang tidak valid walau dikutip hanyalah nama kolom sistem PostgreSQL.
//   - Go: nama field struct dibangkitkan dalam PascalCase (Type, Order, Default tidak pernah kata kunci) dan
//     konstanta bernama <Resource>Field<Nama>; tidak ada variabel atau parameter lokal turunan nama field.
//     Yang bentrok hanyalah anggota struct DTO (GoDTOMembers: Present, Validate, FillRenamePairs; hanya untuk field tulis) dan identifier
//     tingkat paket bangkitan (<Resource>Spec dan sejenisnya terhadap tipe runtime seperti FieldSpec, serta
//     antar resource); yang terakhir diperiksa CheckGoPackage (generated.go) pada tingkat modul.
//   - TypeScript: nama field hanya muncul sebagai nilai string JSON pada resourceMeta dan sebagai kunci properti
//     objek (kunci properti boleh berupa kata kunci); tidak ada variabel atau destructuring turunan nama field.

// sqlSystemColumns adalah nama kolom sistem PostgreSQL; membuat kolom bernama itu ditolak PostgreSQL walau
// identifier-nya dikutip ("column name \"xmin\" conflicts with a system column name").
var sqlSystemColumns = set(`tableoid xmin cmin xmax cmax ctid`)

// tsGeneratedMembers kosong: tidak ada nama keluaran TypeScript yang dapat bentrok dengan nama field.
var tsGeneratedMembers = set(``)

// ReservedTargets mengembalikan target (urutan sql, go, typescript) yang keluaran bangkitannya bentrok dengan
// name pada bentuk hasil konversi target itu: SQL pada snake_case, TypeScript apa adanya. Go tidak diperiksa di sini:
// bentrok anggota DTO bergantung pada apakah field dapat ditulis, sehingga diperiksa CheckGoPackage (generated.go).
func ReservedTargets(name string) []Target {
	var out []Target
	snake := strings.ToLower(name)
	if s, err := ToSnake(name); err == nil {
		snake = s
	}
	if sqlSystemColumns[snake] {
		out = append(out, TargetSQL)
	}
	if tsGeneratedMembers[name] {
		out = append(out, TargetTS)
	}
	return out
}

func reservedError(name string, t Target) *Error {
	switch t {
	case TargetSQL:
		return errf(CodeReservedSQL, name, "bentrok dengan nama kolom sistem PostgreSQL (tableoid, xmin, cmin, xmax, cmax, ctid) yang ditolak walau dikutip; pilih nama lain")
	default:
		return errf(CodeReservedTS, name, "bentrok dengan anggota keluaran TypeScript bangkitan; pilih nama lain")
	}
}

// CheckReserved mengembalikan galat untuk target pertama yang bentrok dengan name, atau nil.
func CheckReserved(name string) error {
	ts := ReservedTargets(name)
	if len(ts) == 0 {
		return nil
	}
	return reservedError(name, ts[0])
}
