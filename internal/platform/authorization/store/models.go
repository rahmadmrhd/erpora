package store

import (
	"errors"
	"regexp"
	"time"
)

// Kunci role sistem yang dibenihkan migrasi (0003_roles_min dan 0005_authz).
const (
	// RoleAdministrator: semua izin, matriksnya tidak dapat diubah (FR-113); izin penuh dihitung mesin, bukan baris.
	RoleAdministrator = "administrator"
	// RoleAuthenticated: hak dasar semua pengguna login; berlaku implisit tanpa baris user_roles.
	RoleAuthenticated = "authenticated"
)

// Jenis role (roles.kind).
const (
	KindSystem        = "system"
	KindModuleDefault = "module_default"
	KindCustom        = "custom"
)

// Akses grup field (role_field_access.access).
const (
	AccessNone  = "none"
	AccessRead  = "read"
	AccessWrite = "write"
)

// Token scope (role_permissions.scope).
const (
	ScopeAll       = "all"
	ScopeOwner     = "owner"
	ScopeAssigned  = "assigned:"
	ScopeCustom    = "custom:"
	maxScopeTokens = 16
)

var (
	// ErrNotFound berarti baris tidak ada (role, atau role yang dirujuk). ID yang bukan UUID juga ErrNotFound.
	ErrNotFound = errors.New("authorization/store: tidak ditemukan")
	// ErrConflict berarti kunci role sudah dipakai, atau nilai berbenturan dengan baris lain.
	ErrConflict = errors.New("authorization/store: konflik")
	// ErrInvalidArgument berarti masukan melanggar aturan bentuk yang juga ditegakkan CHECK database (scope kosong
	// atau berisi token tak sah, aksi kosong, akses di luar none/read/write, dan seterusnya).
	ErrInvalidArgument = errors.New("authorization/store: argumen tidak valid")
	// ErrImmutable berarti operasi ditolak karena sifat role: matriks administrator tidak dapat diubah, role sistem
	// tidak dapat diubah namanya atau dihapus, dan hanya role custom yang dapat dihapus (FR-113).
	ErrImmutable = errors.New("authorization/store: role tidak dapat diubah")
	// ErrRoleInUse berarti role masih dipegang pengguna sehingga tidak dapat dihapus.
	ErrRoleInUse = errors.New("authorization/store: role masih dipegang pengguna")
	// ErrStaleVersion berarti role sudah berubah sejak versi (updated_at) yang diharapkan pemanggil.
	ErrStaleVersion = errors.New("authorization/store: versi role sudah berubah")
	// ErrNoTenant berarti context tidak membawa tenant.
	ErrNoTenant = errors.New("authorization/store: tenant tidak ada di context")
)

var (
	resourcePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)
	groupPattern    = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)
	actionPattern   = regexp.MustCompile(`^[a-z_]+$`)
	dimPattern      = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)
	roleKeyPattern  = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
)

// Role adalah satu baris roles.
type Role struct {
	ID           string
	Key          string
	Label        string
	Description  string
	Kind         string // KindSystem | KindModuleDefault | KindCustom
	SourceModule string // kosong kecuali KindModuleDefault
	Customized   bool
	CreatedAt    time.Time
	UpdatedAt    time.Time // juga versi untuk kendali konkurensi optimistik (RoleUpdate.IfUpdatedAt)
}

// PermissionRow adalah satu baris matriks (role_permissions). Scope adalah larik token tidak kosong yang digabung
// AND; antarbaris OR (union per baris, FR-111). ScopeInvalid hanya diisi pembaca bila nilai tersimpan tidak dapat
// diuraikan menjadi larik token sah (mustahil selama CHECK utuh); pemakai WAJIB menolak baris itu (gagal tertutup),
// bukan menganggapnya "all".
type PermissionRow struct {
	ID           string // diisi pembaca; diabaikan penulis
	Resource     string
	Actions      []string
	Scope        []string
	ScopeInvalid bool
}

// FieldAccessRow adalah akses satu grup field untuk satu role dan resource.
type FieldAccessRow struct {
	Resource   string
	FieldGroup string
	Access     string // AccessNone | AccessRead | AccessWrite
}

// ScopeAssignment adalah penugasan nilai dimensi scope kepada satu pengguna. IncludeDescendants hanya bermakna
// untuk dimensi core.org_unit (FR-118): true = unit beserta turunannya.
type ScopeAssignment struct {
	Dimension          string
	Values             []string
	IncludeDescendants bool
}

// RoleGrants adalah seluruh hak satu role: matriks dan akses grup field.
type RoleGrants struct {
	Permissions []PermissionRow
	FieldAccess []FieldAccessRow
}

// GrantedRole adalah role yang berlaku bagi pengguna. Implicit true untuk role authenticated yang berlaku tanpa
// baris user_roles; mesin boleh membuangnya untuk principal non-manusia.
type GrantedRole struct {
	Role
	Implicit bool
}

// Grants adalah seluruh masukan kompilasi hak efektif satu pengguna, dibaca dari satu snapshot konsisten (satu
// transaksi REPEATABLE READ read-only): Revision cocok dengan isi baris lain, sehingga kunci cache
// (tenant, user, Revision) tidak pernah memuat data yang lebih baru atau lebih lama dari revisinya.
type Grants struct {
	Revision    int64
	Roles       []GrantedRole
	Permissions []RolePermission
	FieldAccess []RoleFieldAccess
	Assignments []ScopeAssignment
}

// RolePermission menautkan baris matriks ke role pemiliknya (untuk penjelas akses dan hak efektif).
type RolePermission struct {
	RoleID string
	PermissionRow
}

// RoleFieldAccess menautkan akses grup field ke role pemiliknya.
type RoleFieldAccess struct {
	RoleID string
	FieldAccessRow
}

// NewRole adalah masukan CreateRole dan DuplicateRole. Kind hanya KindCustom atau KindModuleDefault (role sistem
// hanya dibenihkan migrasi). SourceModule wajib untuk KindModuleDefault dan harus kosong untuk KindCustom.
type NewRole struct {
	Key          string
	Label        string
	Description  string
	Kind         string
	SourceModule string
}

// RoleUpdate adalah masukan UpdateRole. Nil berarti tidak diubah. IfUpdatedAt (bila non-nil) harus sama dengan
// updated_at role saat ini (kendali konkurensi optimistik; ErrStaleVersion bila berbeda).
type RoleUpdate struct {
	Label       *string
	Description *string
	IfUpdatedAt *time.Time
}

// DefaultRoleDef adalah satu role bawaan manifest untuk SyncModuleDefaultRoles.
type DefaultRoleDef struct {
	Key         string
	Label       string
	Description string
	RoleGrants
}

// SyncResult merangkum SyncModuleDefaultRoles. Revision adalah revisi setelah sinkronisasi (tidak berubah bila
// tidak ada yang ditulis). SkippedCustomized memuat key role bawaan yang sudah customized dan karenanya tidak
// disentuh (dasar notifikasi "ada izin baru dari modul X yang belum diberikan", FR-113).
type SyncResult struct {
	Created           []string
	Updated           []string
	SkippedCustomized []string
	Revision          int64
}

func validResource(s string) bool { return resourcePattern.MatchString(s) }

// ValidateScope memeriksa larik token scope: tidak kosong, setiap token salah satu dari all, owner,
// assigned:<dimensi>, custom:<key> (tanpa duplikat). Aturan yang sama ditegakkan CHECK database.
func ValidateScope(scope []string) error {
	if len(scope) == 0 || len(scope) > maxScopeTokens {
		return ErrInvalidArgument
	}
	seen := make(map[string]bool, len(scope))
	for _, tok := range scope {
		if !validScopeToken(tok) || seen[tok] {
			return ErrInvalidArgument
		}
		seen[tok] = true
	}
	return nil
}

func validScopeToken(tok string) bool {
	switch {
	case tok == ScopeAll || tok == ScopeOwner:
		return true
	case len(tok) > len(ScopeAssigned) && tok[:len(ScopeAssigned)] == ScopeAssigned:
		return dimPattern.MatchString(tok[len(ScopeAssigned):])
	case len(tok) > len(ScopeCustom) && tok[:len(ScopeCustom)] == ScopeCustom:
		return dimPattern.MatchString(tok[len(ScopeCustom):])
	}
	return false
}

func validatePermission(p PermissionRow) error {
	if !validResource(p.Resource) || len(p.Actions) == 0 || len(p.Actions) > 64 {
		return ErrInvalidArgument
	}
	for _, a := range p.Actions {
		if !actionPattern.MatchString(a) {
			return ErrInvalidArgument
		}
	}
	return ValidateScope(p.Scope)
}

func validateFieldAccess(f FieldAccessRow) error {
	if !validResource(f.Resource) || !groupPattern.MatchString(f.FieldGroup) {
		return ErrInvalidArgument
	}
	switch f.Access {
	case AccessNone, AccessRead, AccessWrite:
		return nil
	}
	return ErrInvalidArgument
}

func validateGrants(g RoleGrants) error {
	for _, p := range g.Permissions {
		if err := validatePermission(p); err != nil {
			return err
		}
	}
	seen := make(map[[2]string]bool, len(g.FieldAccess))
	for _, f := range g.FieldAccess {
		if err := validateFieldAccess(f); err != nil {
			return err
		}
		k := [2]string{f.Resource, f.FieldGroup}
		if seen[k] {
			return ErrInvalidArgument // PK (role, resource, grup): satu akses per grup
		}
		seen[k] = true
	}
	return nil
}

func validateNewRole(n NewRole) error {
	if !roleKeyPattern.MatchString(n.Key) || len(n.Label) == 0 || len(n.Label) > 120 || len(n.Description) > 1000 {
		return ErrInvalidArgument
	}
	switch n.Kind {
	case KindCustom:
		if n.SourceModule != "" {
			return ErrInvalidArgument
		}
	case KindModuleDefault:
		if n.SourceModule == "" || len(n.SourceModule) > 64 {
			return ErrInvalidArgument
		}
	default:
		return ErrInvalidArgument
	}
	return nil
}
