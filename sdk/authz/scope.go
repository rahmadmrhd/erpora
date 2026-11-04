package authz

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ScopeProvider adalah scope kustom `custom:<Key>` yang disediakan modul (FR-111): Filter menerjemahkan
// scope ke kondisi GORM untuk query daftar, Match memeriksa satu record. Keduanya wajib dan harus
// konsisten. Didaftarkan lewat Registrar.Scope.
//
// Filter hanya boleh menambah kondisi WHERE pada db yang diterima (db adalah sesi bersih untuk
// membangun satu kelompok kondisi; JOIN dan klausa lain diabaikan). Filter yang tidak menambah kondisi
// apa pun, mengembalikan nil, atau panic berarti scope itu menolak semua baris, bukan "semua baris"
// (gagal tertutup); gunakan scope `all` untuk akses penuh. Match yang panic atau galat berarti tidak cocok.
type ScopeProvider struct {
	// Key diawali slug modul pemilik: "<slug>.<nama_snake>".
	Key string
	// Label adalah kunci i18n namespace modul (K-C2.5) yang ditampilkan di UI matriks.
	Label  string
	Filter func(ctx context.Context, db *gorm.DB) *gorm.DB
	Match  func(ctx context.Context, rec Record) (bool, error)
}

// Validate memeriksa provider untuk modul dengan slug tertentu: Key = "<slug>.<nama>" (nama
// `^[a-z][a-z0-9_]*$`), Label tidak kosong, Filter dan Match tidak nil.
func (p ScopeProvider) Validate(slug string) error {
	name, ok := strings.CutPrefix(p.Key, slug+".")
	if !ok || name == "" {
		return fmt.Errorf("key scope %q harus berbentuk %q", p.Key, slug+".<nama>")
	}
	if !validIdent(name) {
		return fmt.Errorf("key scope %q: bagian nama harus cocok ^[a-z][a-z0-9_]*$", p.Key)
	}
	if p.Label == "" {
		return fmt.Errorf("scope %q: label wajib diisi", p.Key)
	}
	if p.Filter == nil {
		return fmt.Errorf("scope %q: Filter wajib diisi", p.Key)
	}
	if p.Match == nil {
		return fmt.Errorf("scope %q: Match wajib diisi", p.Key)
	}
	return nil
}

// validIdent: ^[a-z][a-z0-9_]*$ (nama kolom dan nama scope dari manifest; lihat juga CanonicalField).
func validIdent(s string) bool {
	if s == "" || len(s) > maxFieldNameBytes || !isLowerASCII(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !isLowerASCII(c) && !isDigitASCII(c) && c != '_' {
			return false
		}
	}
	return true
}

// ConditionKind adalah jenis kondisi satu cabang RowFilter. Nilai nol sengaja tidak sah (ditolak).
type ConditionKind uint8

const (
	// CondAll: semua baris (scope `all`, dinyatakan eksplisit).
	CondAll ConditionKind = iota + 1
	// CondOwner: kolom pemilik sama dengan pengguna (scope `owner`).
	CondOwner
	// CondAssigned: kolom dimensi termasuk nilai penugasan pengguna (scope `assigned:<dimensi>`).
	CondAssigned
	// CondCustom: scope kustom modul (`custom:<key>`).
	CondCustom
)

// RowCondition adalah satu kondisi pada cabang RowFilter. Bangun lewat AllRows, OwnerIs, AssignedIn, atau
// CustomScope. Kondisi yang rusak (jenis tak dikenal, kolom tak sah, nilai kosong) bernilai FALSE.
type RowCondition struct {
	Kind ConditionKind
	// Column: nama kolom manifest (owner_field atau scope_fields[dimensi]), bukan input pengguna; tetap
	// divalidasi (^[a-z][a-z0-9_]*$) saat diterapkan.
	Column string
	// Values: untuk CondOwner tepat satu nilai (id pengguna); untuk CondAssigned nilai penugasan.
	Values []string
	// Provider: untuk CondCustom.
	Provider ScopeProvider
}

// AllRows adalah kondisi scope `all`.
func AllRows() RowCondition { return RowCondition{Kind: CondAll} }

// OwnerIs: kolom column sama dengan userID. userID kosong bernilai FALSE.
func OwnerIs(column, userID string) RowCondition {
	return RowCondition{Kind: CondOwner, Column: column, Values: []string{userID}}
}

// AssignedIn: kolom column termasuk values. values kosong bernilai FALSE (pengguna tanpa penugasan tidak
// melihat apa pun; FR-111), bukan klausa yang dilewati.
func AssignedIn(column string, values []string) RowCondition {
	return RowCondition{Kind: CondAssigned, Column: column, Values: append([]string(nil), values...)}
}

// CustomScope: scope kustom milik modul.
func CustomScope(p ScopeProvider) RowCondition { return RowCondition{Kind: CondCustom, Provider: p} }

// RowBranch adalah satu cabang (satu baris matriks): semua Conditions digabung AND. Cabang tanpa
// kondisi tidak sah dan bernilai FALSE; akses penuh harus dinyatakan eksplisit dengan AllRows.
type RowBranch struct{ Conditions []RowCondition }

// RowFilter adalah deskripsi netral (tanpa tipe GORM) hasil evaluasi scope sebuah (resource, aksi) bagi
// principal: antarcabang digabung OR (union per baris matriks; FR-111). RowFilter kosong berarti tolak
// semua baris (WHERE false). Mesin platform mengembalikannya dari Engine.Filter; Filter menerjemahkannya
// ke GORM sehingga paket mesin tidak mengimpor GORM (D3b).
type RowFilter struct{ Branches []RowBranch }
