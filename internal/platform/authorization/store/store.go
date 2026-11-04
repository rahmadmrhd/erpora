package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
)

// Store adalah persistensi otorisasi di tenant DB. Satu-satunya pemakai GORM untuk tabel otorisasi (D3b). Nilai nol
// tidak valid; gunakan New. Tenant selalu diambil dari context (tenancy.FromContext) dan database dipilih lewat
// TenantConnector (isolasi per database, ADR-0004; Q18): tabel tidak punya kolom tenant_id, sehingga tidak ada
// kueri yang dapat menjangkau tenant lain. Tenant yang tidak dikenal connector menghasilkan galat (gagal tertutup).
type Store struct {
	tenants database.TenantConnector
}

// New membuat Store.
func New(tenants database.TenantConnector) *Store { return &Store{tenants: tenants} }

func wrap(op string, err error) error { return fmt.Errorf("authorization/store: %s: %w", op, err) }

func parseID(s string) (uuid.UUID, bool) {
	if len(s) != 36 {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	return id, err == nil
}

func (s *Store) db(ctx context.Context) (*gorm.DB, error) {
	t, ok := tenancy.FromContext(ctx)
	if !ok {
		return nil, ErrNoTenant
	}
	if s.tenants == nil {
		return nil, errors.New("authorization/store: TenantConnector tidak dikonfigurasi")
	}
	db, err := s.tenants.For(ctx, t)
	if err != nil {
		return nil, wrap("pilih database tenant", err)
	}
	return db.WithContext(ctx), nil
}

func exec(db *gorm.DB, q string, args ...any) (int64, error) {
	r := db.Exec(q, args...)
	return r.RowsAffected, r.Error
}

// Revision membaca permission_revision saat ini. Baris authz_state yang hilang adalah galat (gagal tertutup),
// bukan revisi 0.
func (s *Store) Revision(ctx context.Context) (int64, error) {
	db, err := s.db(ctx)
	if err != nil {
		return 0, err
	}
	return readRevision(db)
}

func readRevision(db *gorm.DB) (int64, error) {
	var rev []int64
	if err := db.Raw(`SELECT permission_revision FROM authz_state WHERE id = 1`).Scan(&rev).Error; err != nil {
		return 0, wrap("baca revisi", err)
	}
	if len(rev) != 1 {
		return 0, errors.New("authorization/store: baris authz_state tidak ada")
	}
	return rev[0], nil
}

// BumpRevision menaikkan permission_revision satu dan mengembalikan nilai barunya. tx WAJIB transaksi yang sama
// dengan perubahan role, matriks, field access, user_roles, penugasan scope, atau sinkronisasi default_roles
// (Global Constraints C2): revisi dan data berubah bersama atau tidak sama sekali. Pembaruan mengunci baris
// authz_state sampai commit, sehingga dua transaksi serentak selalu menerima nilai berbeda. Paket identity/store
// memanggil fungsi ini pada user_roles miliknya (GrantRole/RevokeRole) agar tidak ada jalur ubah hak tanpa revisi.
func BumpRevision(tx *gorm.DB) (int64, error) {
	var rev []int64
	if err := tx.Raw(`UPDATE authz_state SET permission_revision = permission_revision + 1 WHERE id = 1 RETURNING permission_revision`).Scan(&rev).Error; err != nil {
		return 0, wrap("naikkan revisi", err)
	}
	if len(rev) != 1 {
		return 0, errors.New("authorization/store: baris authz_state tidak ada")
	}
	return rev[0], nil
}

// write menjalankan fn dalam satu transaksi tenant lalu menaikkan revisi pada transaksi yang sama; galat apa pun
// (termasuk dari fn) membatalkan keduanya. Mengembalikan revisi baru.
func (s *Store) write(ctx context.Context, fn func(tx *gorm.DB) error) (int64, error) {
	db, err := s.db(ctx)
	if err != nil {
		return 0, err
	}
	var rev int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := fn(tx); err != nil {
			return err
		}
		r, err := BumpRevision(tx)
		rev = r
		return err
	})
	if err != nil {
		return 0, err
	}
	return rev, nil
}

type roleRow struct {
	ID           string    `gorm:"column:id"`
	Key          string    `gorm:"column:key"`
	Label        string    `gorm:"column:label"`
	Description  string    `gorm:"column:description"`
	Kind         string    `gorm:"column:kind"`
	SourceModule string    `gorm:"column:source_module"`
	Customized   bool      `gorm:"column:customized"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
	Implicit     bool      `gorm:"column:implicit"`
}

const roleCols = `r.id::text AS id, r.key, r.label, r.description, r.kind, COALESCE(r.source_module, '') AS source_module,
	r.customized, r.created_at, r.updated_at`

func (r roleRow) role() Role {
	return Role{ID: r.ID, Key: r.Key, Label: r.Label, Description: r.Description, Kind: r.Kind, SourceModule: r.SourceModule,
		Customized: r.Customized, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

type permRow struct {
	ID       string `gorm:"column:id"`
	RoleID   string `gorm:"column:role_id"`
	Resource string `gorm:"column:resource"`
	Actions  string `gorm:"column:actions"`
	Scope    string `gorm:"column:scope"`
}

type faRow struct {
	RoleID     string `gorm:"column:role_id"`
	Resource   string `gorm:"column:resource"`
	FieldGroup string `gorm:"column:field_group"`
	Access     string `gorm:"column:access"`
}

type asgRow struct {
	Dimension          string `gorm:"column:dimension"`
	Vals               string `gorm:"column:vals"`
	IncludeDescendants bool   `gorm:"column:include_descendants"`
}

const permCols = `id::text AS id, role_id::text AS role_id, resource, to_json(actions)::text AS actions, scope::text AS scope`

// decodePermission menguraikan baris matriks. Nilai yang tidak dapat diuraikan menjadi aksi dan token scope sah
// menandai ScopeInvalid (dan mengosongkan Scope): pemakai wajib menolak baris itu, tidak pernah menganggapnya
// "all" (gagal tertutup, NFR-106). Duplikat token tidak mengubah arti (AND idempoten) sehingga tidak ditolak.
func decodePermission(r permRow) RolePermission {
	p := PermissionRow{ID: r.ID, Resource: r.Resource}
	var actions, scope []string
	if err := json.Unmarshal([]byte(r.Actions), &actions); err != nil || len(actions) == 0 {
		p.ScopeInvalid = true
	}
	p.Actions = actions
	if err := json.Unmarshal([]byte(r.Scope), &scope); err != nil || len(scope) == 0 {
		p.ScopeInvalid = true
	} else {
		for _, tok := range scope {
			if !validScopeToken(tok) {
				p.ScopeInvalid = true
			}
		}
	}
	if p.ScopeInvalid {
		p.Scope = nil
	} else {
		p.Scope = scope
	}
	return RolePermission{RoleID: r.RoleID, PermissionRow: p}
}

func decodeAssignment(r asgRow) (ScopeAssignment, error) {
	var vals []string
	if err := json.Unmarshal([]byte(r.Vals), &vals); err != nil {
		return ScopeAssignment{}, wrap("uraikan nilai penugasan", err)
	}
	return ScopeAssignment{Dimension: r.Dimension, Values: vals, IncludeDescendants: r.IncludeDescendants}, nil
}

// LoadUserGrants membaca seluruh masukan hak efektif pengguna dari satu snapshot konsisten (transaksi REPEATABLE
// READ read-only): role eksplisit (user_roles) ditambah role authenticated yang berlaku implisit (Implicit=true),
// baris matriks dan field access semua role itu, dan penugasan scope pengguna. Satu kueri per tabel. userID yang
// bukan UUID adalah ErrNotFound. Keberadaan pengguna di control DB bukan urusan store; pengguna tanpa role hanya
// menerima hak authenticated.
func (s *Store) LoadUserGrants(ctx context.Context, userID string) (Grants, error) {
	uid, ok := parseID(userID)
	if !ok {
		return Grants{}, ErrNotFound
	}
	db, err := s.db(ctx)
	if err != nil {
		return Grants{}, err
	}
	var g Grants
	err = db.Transaction(func(tx *gorm.DB) error {
		// Harus pernyataan pertama transaksi: snapshot dibuat saat kueri pertama dijalankan.
		if err := tx.Exec(`SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY`).Error; err != nil {
			return wrap("atur isolasi snapshot", err)
		}
		rev, err := readRevision(tx)
		if err != nil {
			return err
		}
		g.Revision = rev

		var roles []roleRow
		if err := tx.Raw(`SELECT `+roleCols+`, false AS implicit
			FROM user_roles ur JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = ? AND r.key <> ?
			UNION ALL
			SELECT `+roleCols+`, true AS implicit FROM roles r WHERE r.key = ?
			ORDER BY implicit, key`, uid, RoleAuthenticated, RoleAuthenticated).Scan(&roles).Error; err != nil {
			return wrap("muat role pengguna", err)
		}
		for _, r := range roles {
			g.Roles = append(g.Roles, GrantedRole{Role: r.role(), Implicit: r.Implicit})
		}

		const effective = `role_id IN (SELECT role_id FROM user_roles WHERE user_id = ?
			UNION SELECT id FROM roles WHERE key = ?)`
		var perms []permRow
		if err := tx.Raw(`SELECT `+permCols+` FROM role_permissions WHERE `+effective+` ORDER BY role_id, resource, id`,
			uid, RoleAuthenticated).Scan(&perms).Error; err != nil {
			return wrap("muat matriks", err)
		}
		for _, p := range perms {
			g.Permissions = append(g.Permissions, decodePermission(p))
		}

		var fas []faRow
		if err := tx.Raw(`SELECT role_id::text AS role_id, resource, field_group, access FROM role_field_access WHERE `+effective+
			` ORDER BY role_id, resource, field_group`, uid, RoleAuthenticated).Scan(&fas).Error; err != nil {
			return wrap("muat field access", err)
		}
		for _, f := range fas {
			g.FieldAccess = append(g.FieldAccess, RoleFieldAccess{RoleID: f.RoleID,
				FieldAccessRow: FieldAccessRow{Resource: f.Resource, FieldGroup: f.FieldGroup, Access: f.Access}})
		}

		asg, err := loadAssignments(tx, uid)
		g.Assignments = asg
		return err
	})
	if err != nil {
		return Grants{}, err
	}
	return g, nil
}

func loadAssignments(db *gorm.DB, uid uuid.UUID) ([]ScopeAssignment, error) {
	var rows []asgRow
	if err := db.Raw(`SELECT dimension, to_json("values")::text AS vals, include_descendants
		FROM user_scope_assignments WHERE user_id = ? ORDER BY dimension`, uid).Scan(&rows).Error; err != nil {
		return nil, wrap("muat penugasan scope", err)
	}
	out := make([]ScopeAssignment, 0, len(rows))
	for _, r := range rows {
		a, err := decodeAssignment(r)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// ListRoles memuat semua role (sistem, bawaan modul, kustom), terurut jenis lalu key.
func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var rows []roleRow
	if err := db.Raw(`SELECT ` + roleCols + ` FROM roles r ORDER BY CASE r.kind WHEN 'system' THEN 0 WHEN 'module_default' THEN 1 ELSE 2 END, r.key`).Scan(&rows).Error; err != nil {
		return nil, wrap("daftar role", err)
	}
	out := make([]Role, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.role())
	}
	return out, nil
}

// GetRole memuat satu role menurut id; ErrNotFound bila tidak ada.
func (s *Store) GetRole(ctx context.Context, id string) (Role, error) {
	rid, ok := parseID(id)
	if !ok {
		return Role{}, ErrNotFound
	}
	db, err := s.db(ctx)
	if err != nil {
		return Role{}, err
	}
	return getRole(db, rid, false)
}

func getRole(db *gorm.DB, rid uuid.UUID, lock bool) (Role, error) {
	q := `SELECT ` + roleCols + ` FROM roles r WHERE r.id = ?`
	if lock {
		q += ` FOR UPDATE`
	}
	var rows []roleRow
	if err := db.Raw(q, rid).Scan(&rows).Error; err != nil {
		return Role{}, wrap("baca role", err)
	}
	if len(rows) == 0 {
		return Role{}, ErrNotFound
	}
	return rows[0].role(), nil
}

// RoleGrantsOf memuat matriks dan field access satu role; ErrNotFound bila role tidak ada.
func (s *Store) RoleGrantsOf(ctx context.Context, roleID string) (RoleGrants, error) {
	rid, ok := parseID(roleID)
	if !ok {
		return RoleGrants{}, ErrNotFound
	}
	db, err := s.db(ctx)
	if err != nil {
		return RoleGrants{}, err
	}
	var out RoleGrants
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY`).Error; err != nil {
			return wrap("atur isolasi snapshot", err)
		}
		if _, err := getRole(tx, rid, false); err != nil {
			return err
		}
		var perms []permRow
		if err := tx.Raw(`SELECT `+permCols+` FROM role_permissions WHERE role_id = ? ORDER BY resource, id`, rid).Scan(&perms).Error; err != nil {
			return wrap("muat matriks role", err)
		}
		for _, p := range perms {
			out.Permissions = append(out.Permissions, decodePermission(p).PermissionRow)
		}
		var fas []faRow
		if err := tx.Raw(`SELECT role_id::text AS role_id, resource, field_group, access FROM role_field_access
			WHERE role_id = ? ORDER BY resource, field_group`, rid).Scan(&fas).Error; err != nil {
			return wrap("muat field access role", err)
		}
		for _, f := range fas {
			out.FieldAccess = append(out.FieldAccess, FieldAccessRow{Resource: f.Resource, FieldGroup: f.FieldGroup, Access: f.Access})
		}
		return nil
	})
	if err != nil {
		return RoleGrants{}, err
	}
	return out, nil
}

// UserRoleIDs memuat id role eksplisit milik pengguna (tanpa role authenticated implisit), terurut.
func (s *Store) UserRoleIDs(ctx context.Context, userID string) ([]string, error) {
	uid, ok := parseID(userID)
	if !ok {
		return nil, nil
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	if err := db.Raw(`SELECT role_id::text FROM user_roles WHERE user_id = ? ORDER BY role_id`, uid).Scan(&ids).Error; err != nil {
		return nil, wrap("muat role pengguna", err)
	}
	return ids, nil
}

// UserScopeAssignments memuat penugasan scope pengguna, terurut dimensi.
func (s *Store) UserScopeAssignments(ctx context.Context, userID string) ([]ScopeAssignment, error) {
	uid, ok := parseID(userID)
	if !ok {
		return nil, nil
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	return loadAssignments(db, uid)
}

// BumpRevision menaikkan revisi pada transaksi tx milik pemanggil; padanan metode dari fungsi paket BumpRevision
// (antarmuka rencana C2.2: Store.BumpRevision(tx)).
func (s *Store) BumpRevision(tx *gorm.DB) (int64, error) { return BumpRevision(tx) }
