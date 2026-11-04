package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func validLabel(s string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= 1 && n <= 120
}

// permJSON/faJSON adalah bentuk JSON baris untuk penyisipan massal lewat jsonb_to_recordset (satu pernyataan per
// tabel; nilai selalu parameter terikat).
type permJSON struct {
	Resource string          `json:"resource"`
	Actions  []string        `json:"actions"`
	Scope    json.RawMessage `json:"scope"`
}

type faJSON struct {
	Resource   string `json:"resource"`
	FieldGroup string `json:"field_group"`
	Access     string `json:"access"`
}

// insertGrants menyisipkan matriks dan field access untuk role rid (sudah divalidasi pemanggil).
func insertGrants(tx *gorm.DB, rid uuid.UUID, g RoleGrants) error {
	if len(g.Permissions) > 0 {
		rows := make([]permJSON, 0, len(g.Permissions))
		for _, p := range g.Permissions {
			sc, err := json.Marshal(p.Scope)
			if err != nil {
				return wrap("serialisasi scope", err)
			}
			rows = append(rows, permJSON{Resource: p.Resource, Actions: p.Actions, Scope: sc})
		}
		b, err := json.Marshal(rows)
		if err != nil {
			return wrap("serialisasi matriks", err)
		}
		if _, err := exec(tx, `INSERT INTO role_permissions (role_id, resource, actions, scope)
			SELECT ?::uuid, x.resource, ARRAY(SELECT a FROM jsonb_array_elements_text(x.actions) WITH ORDINALITY AS t(a, n) ORDER BY n), x.scope
			FROM jsonb_to_recordset(?::jsonb) AS x(resource text, actions jsonb, scope jsonb)`, rid, string(b)); err != nil {
			return wrap("simpan matriks", err)
		}
	}
	if len(g.FieldAccess) > 0 {
		rows := make([]faJSON, 0, len(g.FieldAccess))
		for _, f := range g.FieldAccess {
			rows = append(rows, faJSON(f))
		}
		b, err := json.Marshal(rows)
		if err != nil {
			return wrap("serialisasi field access", err)
		}
		if _, err := exec(tx, `INSERT INTO role_field_access (role_id, resource, field_group, access)
			SELECT ?::uuid, x.resource, x.field_group, x.access
			FROM jsonb_to_recordset(?::jsonb) AS x(resource text, field_group text, access text)`, rid, string(b)); err != nil {
			return wrap("simpan field access", err)
		}
	}
	return nil
}

func deleteGrants(tx *gorm.DB, rid uuid.UUID) error {
	if _, err := exec(tx, `DELETE FROM role_permissions WHERE role_id = ?`, rid); err != nil {
		return wrap("hapus matriks", err)
	}
	if _, err := exec(tx, `DELETE FROM role_field_access WHERE role_id = ?`, rid); err != nil {
		return wrap("hapus field access", err)
	}
	return nil
}

// touchRole menaikkan updated_at (versi role untuk kendali konkurensi optimistik); markCustomized menandai role
// bawaan modul yang diubah admin sebagai customized sehingga tidak ditimpa sinkronisasi (FR-113).
func touchRole(tx *gorm.DB, rid uuid.UUID, markCustomized bool) error {
	q := `UPDATE roles SET updated_at = clock_timestamp()`
	if markCustomized {
		q += `, customized = customized OR kind = 'module_default'`
	}
	if _, err := exec(tx, q+` WHERE id = ?`, rid); err != nil {
		return wrap("perbarui versi role", err)
	}
	return nil
}

// CreateRole membuat role KindCustom atau KindModuleDefault tanpa hak apa pun dan menaikkan revisi pada
// transaksi yang sama. Key yang sudah ada adalah ErrConflict.
func (s *Store) CreateRole(ctx context.Context, n NewRole) (Role, int64, error) {
	if err := validateNewRole(n); err != nil || !validLabel(n.Label) {
		return Role{}, 0, ErrInvalidArgument
	}
	var out Role
	rev, err := s.write(ctx, func(tx *gorm.DB) error {
		r, err := insertRole(tx, n)
		out = r
		return err
	})
	if err != nil {
		return Role{}, 0, err
	}
	return out, rev, nil
}

func insertRole(tx *gorm.DB, n NewRole) (Role, error) {
	var rows []roleRow
	if err := tx.Raw(`WITH r AS (
			INSERT INTO roles (key, label, description, kind, source_module)
			VALUES (?, ?, ?, ?, NULLIF(?, ''))
			ON CONFLICT (key) DO NOTHING
			RETURNING *
		) SELECT `+roleCols+` FROM r`, n.Key, strings.TrimSpace(n.Label), n.Description, n.Kind, n.SourceModule).Scan(&rows).Error; err != nil {
		return Role{}, wrap("buat role", err)
	}
	if len(rows) == 0 {
		return Role{}, ErrConflict
	}
	return rows[0].role(), nil
}

// UpdateRole mengubah label dan/atau deskripsi. Role sistem tidak dapat diubah (ErrImmutable); role bawaan modul
// yang diubah ditandai customized. IfUpdatedAt yang tidak cocok adalah ErrStaleVersion. Menaikkan revisi.
func (s *Store) UpdateRole(ctx context.Context, id string, u RoleUpdate) (Role, int64, error) {
	rid, ok := parseID(id)
	if !ok {
		return Role{}, 0, ErrNotFound
	}
	if (u.Label != nil && !validLabel(*u.Label)) || (u.Description != nil && len(*u.Description) > 1000) {
		return Role{}, 0, ErrInvalidArgument
	}
	var out Role
	rev, err := s.write(ctx, func(tx *gorm.DB) error {
		cur, err := getRole(tx, rid, true)
		if err != nil {
			return err
		}
		if cur.Kind == KindSystem {
			return ErrImmutable
		}
		if u.IfUpdatedAt != nil && !u.IfUpdatedAt.Equal(cur.UpdatedAt) {
			return ErrStaleVersion
		}
		label, desc := cur.Label, cur.Description
		if u.Label != nil {
			label = strings.TrimSpace(*u.Label)
		}
		if u.Description != nil {
			desc = *u.Description
		}
		if _, err := exec(tx, `UPDATE roles SET label = ?, description = ? WHERE id = ?`, label, desc, rid); err != nil {
			return wrap("ubah role", err)
		}
		if err := touchRole(tx, rid, true); err != nil {
			return err
		}
		out, err = getRole(tx, rid, false)
		return err
	})
	if err != nil {
		return Role{}, 0, err
	}
	return out, rev, nil
}

// DeleteRole menghapus role KindCustom beserta matriks dan field access-nya (ON DELETE CASCADE). Role bukan
// custom adalah ErrImmutable; role yang masih dipegang pengguna adalah ErrRoleInUse. Menaikkan revisi.
func (s *Store) DeleteRole(ctx context.Context, id string) (int64, error) {
	rid, ok := parseID(id)
	if !ok {
		return 0, ErrNotFound
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		cur, err := getRole(tx, rid, true) // FOR UPDATE: pemberian role serentak (FK) menunggu
		if err != nil {
			return err
		}
		if cur.Kind != KindCustom {
			return ErrImmutable
		}
		var holders int64
		if err := tx.Raw(`SELECT count(*) FROM user_roles WHERE role_id = ?`, rid).Scan(&holders).Error; err != nil {
			return wrap("hitung pemegang role", err)
		}
		if holders > 0 {
			return ErrRoleInUse
		}
		if _, err := exec(tx, `DELETE FROM roles WHERE id = ?`, rid); err != nil {
			return wrap("hapus role", err)
		}
		return nil
	})
}

// ReplaceRoleGrants mengganti seluruh matriks dan field access satu role dengan g (penggantian utuh, bukan
// gabung) dan menaikkan revisi. Matriks administrator tidak dapat diubah (ErrImmutable; izin penuhnya dihitung
// mesin, bukan baris). Role bawaan modul yang diubah ditandai customized. ifUpdatedAt (opsional) adalah kendali
// konkurensi optimistik. Scope kosong atau berisi token tak sah adalah ErrInvalidArgument (tanpa menyentuh DB).
func (s *Store) ReplaceRoleGrants(ctx context.Context, roleID string, g RoleGrants, ifUpdatedAt *time.Time) (int64, error) {
	rid, ok := parseID(roleID)
	if !ok {
		return 0, ErrNotFound
	}
	if err := validateGrants(g); err != nil {
		return 0, err
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		cur, err := getRole(tx, rid, true)
		if err != nil {
			return err
		}
		if cur.Key == RoleAdministrator {
			return ErrImmutable
		}
		if ifUpdatedAt != nil && !ifUpdatedAt.Equal(cur.UpdatedAt) {
			return ErrStaleVersion
		}
		if err := deleteGrants(tx, rid); err != nil {
			return err
		}
		if err := insertGrants(tx, rid, g); err != nil {
			return err
		}
		return touchRole(tx, rid, true)
	})
}

// DuplicateRole membuat role baru n dengan menyalin matriks dan field access role sumber (satu transaksi, satu
// kenaikan revisi). Menyalin administrator menghasilkan role tanpa baris (izin penuhnya bukan baris).
func (s *Store) DuplicateRole(ctx context.Context, sourceID string, n NewRole) (Role, int64, error) {
	sid, ok := parseID(sourceID)
	if !ok {
		return Role{}, 0, ErrNotFound
	}
	if err := validateNewRole(n); err != nil || !validLabel(n.Label) {
		return Role{}, 0, ErrInvalidArgument
	}
	var out Role
	rev, err := s.write(ctx, func(tx *gorm.DB) error {
		var locked []string
		if err := tx.Raw(`SELECT id::text FROM roles WHERE id = ? FOR SHARE`, sid).Scan(&locked).Error; err != nil {
			return wrap("kunci role sumber", err)
		}
		if len(locked) == 0 {
			return ErrNotFound
		}
		r, err := insertRole(tx, n)
		if err != nil {
			return err
		}
		out = r
		if _, err := exec(tx, `INSERT INTO role_permissions (role_id, resource, actions, scope)
			SELECT ?::uuid, resource, actions, scope FROM role_permissions WHERE role_id = ?`, r.ID, sid); err != nil {
			return wrap("salin matriks", err)
		}
		if _, err := exec(tx, `INSERT INTO role_field_access (role_id, resource, field_group, access)
			SELECT ?::uuid, resource, field_group, access FROM role_field_access WHERE role_id = ?`, r.ID, sid); err != nil {
			return wrap("salin field access", err)
		}
		return nil
	})
	if err != nil {
		return Role{}, 0, err
	}
	return out, rev, nil
}

// SetUserRoles mengganti himpunan role eksplisit pengguna dengan roleIDs (role yang sudah dipegang dipertahankan
// berikut granted_at-nya) dan menaikkan revisi. Role yang tidak ada, atau role authenticated (berlaku implisit,
// tidak pernah diberikan lewat baris), adalah ErrNotFound dan membatalkan seluruh perubahan. Keberadaan pengguna
// adalah tanggung jawab pemanggil (tabel users di control DB). Aturan administrator terakhir dan eskalasi hak
// (FR-113, K-C2.6) adalah aturan service (C2.10), bukan store.
func (s *Store) SetUserRoles(ctx context.Context, userID string, roleIDs []string) (int64, error) {
	uid, ok := parseID(userID)
	if !ok {
		return 0, ErrNotFound
	}
	want := make([]uuid.UUID, 0, len(roleIDs))
	seen := make(map[uuid.UUID]bool, len(roleIDs))
	for _, id := range roleIDs {
		rid, ok := parseID(id)
		if !ok {
			return 0, ErrNotFound
		}
		if !seen[rid] {
			seen[rid] = true
			want = append(want, rid)
		}
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		if len(want) > 0 {
			var found []string
			if err := tx.Raw(`SELECT id::text FROM roles WHERE id IN ? AND key <> ? FOR SHARE`, want, RoleAuthenticated).Scan(&found).Error; err != nil {
				return wrap("periksa role", err)
			}
			if len(found) != len(want) {
				return ErrNotFound
			}
			if _, err := exec(tx, `DELETE FROM user_roles WHERE user_id = ? AND role_id NOT IN ?`, uid, want); err != nil {
				return wrap("cabut role", err)
			}
			if _, err := exec(tx, `INSERT INTO user_roles (user_id, role_id)
				SELECT ?::uuid, r.id FROM roles r WHERE r.id IN ? ON CONFLICT DO NOTHING`, uid, want); err != nil {
				return wrap("beri role", err)
			}
			return nil
		}
		if _, err := exec(tx, `DELETE FROM user_roles WHERE user_id = ?`, uid); err != nil {
			return wrap("cabut role", err)
		}
		return nil
	})
}

const (
	maxAssignmentValues = 1000
	maxAssignmentLen    = 256
)

// SetScopeAssignments mengganti seluruh penugasan scope pengguna dengan as dan menaikkan revisi. Dimensi yang
// tidak ada di as dihapus; dimensi dengan Values kosong juga dihapus (tidak ada penugasan = FALSE bagi scope
// assigned, bukan baris berisi larik kosong). Dimensi ganda, nilai kosong atau ganda adalah ErrInvalidArgument.
func (s *Store) SetScopeAssignments(ctx context.Context, userID string, as []ScopeAssignment) (int64, error) {
	uid, ok := parseID(userID)
	if !ok {
		return 0, ErrNotFound
	}
	dims := map[string]bool{}
	var keep []ScopeAssignment
	for _, a := range as {
		if !dimPattern.MatchString(a.Dimension) || dims[a.Dimension] || len(a.Values) > maxAssignmentValues {
			return 0, ErrInvalidArgument
		}
		dims[a.Dimension] = true
		vals := map[string]bool{}
		for _, v := range a.Values {
			if v == "" || len(v) > maxAssignmentLen || vals[v] {
				return 0, ErrInvalidArgument
			}
			vals[v] = true
		}
		if len(a.Values) > 0 {
			keep = append(keep, a)
		}
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		names := make([]string, 0, len(keep))
		for _, a := range keep {
			names = append(names, a.Dimension)
		}
		if len(names) > 0 {
			if _, err := exec(tx, `DELETE FROM user_scope_assignments WHERE user_id = ? AND dimension NOT IN ?`, uid, names); err != nil {
				return wrap("hapus penugasan", err)
			}
		} else if _, err := exec(tx, `DELETE FROM user_scope_assignments WHERE user_id = ?`, uid); err != nil {
			return wrap("hapus penugasan", err)
		}
		for _, a := range keep {
			vb, err := json.Marshal(a.Values)
			if err != nil {
				return wrap("serialisasi nilai penugasan", err)
			}
			if _, err := exec(tx, `INSERT INTO user_scope_assignments (user_id, dimension, "values", include_descendants)
				VALUES (?::uuid, ?, ARRAY(SELECT v FROM jsonb_array_elements_text(?::jsonb) WITH ORDINALITY AS t(v, n) ORDER BY n), ?)
				ON CONFLICT (user_id, dimension) DO UPDATE
				SET "values" = EXCLUDED."values", include_descendants = EXCLUDED.include_descendants`,
				uid, a.Dimension, string(vb), a.IncludeDescendants); err != nil {
				return wrap("simpan penugasan", err)
			}
		}
		return nil
	})
}

// SyncModuleDefaultRoles menyelaraskan role bawaan modul (manifest default_roles) pada install/upgrade (FR-113):
// role baru dibuat (kind module_default, source_module = module), role yang belum customized diperbarui mengikuti
// manifest (label, deskripsi, dan seluruh hak diganti), role customized TIDAK disentuh dan dilaporkan di
// SkippedCustomized. Role dengan key yang sama tetapi bukan milik modul itu adalah ErrConflict (seluruh sinkronisasi
// dibatalkan). Seluruhnya dalam satu transaksi; revisi naik tepat satu kali bila ada role dibuat atau diperbarui,
// dan tidak berubah bila tidak ada yang ditulis.
func (s *Store) SyncModuleDefaultRoles(ctx context.Context, module string, defs []DefaultRoleDef) (SyncResult, error) {
	if module == "" || len(module) > 64 {
		return SyncResult{}, ErrInvalidArgument
	}
	keys := map[string]bool{}
	for _, d := range defs {
		nr := NewRole{Key: d.Key, Label: d.Label, Description: d.Description, Kind: KindModuleDefault, SourceModule: module}
		if err := validateNewRole(nr); err != nil || !validLabel(d.Label) || keys[d.Key] {
			return SyncResult{}, ErrInvalidArgument
		}
		keys[d.Key] = true
		if err := validateGrants(d.RoleGrants); err != nil {
			return SyncResult{}, err
		}
	}
	db, err := s.db(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	var res SyncResult
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, d := range defs {
			var rows []roleRow
			if err := tx.Raw(`SELECT `+roleCols+` FROM roles r WHERE r.key = ? FOR UPDATE`, d.Key).Scan(&rows).Error; err != nil {
				return wrap("baca role bawaan", err)
			}
			if len(rows) == 0 {
				r, err := insertRole(tx, NewRole{Key: d.Key, Label: d.Label, Description: d.Description, Kind: KindModuleDefault, SourceModule: module})
				if err != nil {
					return err
				}
				rid, _ := parseID(r.ID)
				if err := insertGrants(tx, rid, d.RoleGrants); err != nil {
					return err
				}
				res.Created = append(res.Created, d.Key)
				continue
			}
			cur := rows[0].role()
			if cur.Kind != KindModuleDefault || cur.SourceModule != module {
				return ErrConflict
			}
			if cur.Customized {
				res.SkippedCustomized = append(res.SkippedCustomized, d.Key)
				continue
			}
			rid, _ := parseID(cur.ID)
			if _, err := exec(tx, `UPDATE roles SET label = ?, description = ? WHERE id = ?`, strings.TrimSpace(d.Label), d.Description, rid); err != nil {
				return wrap("perbarui role bawaan", err)
			}
			if err := deleteGrants(tx, rid); err != nil {
				return err
			}
			if err := insertGrants(tx, rid, d.RoleGrants); err != nil {
				return err
			}
			if err := touchRole(tx, rid, false); err != nil {
				return err
			}
			res.Updated = append(res.Updated, d.Key)
		}
		if len(res.Created)+len(res.Updated) > 0 {
			rev, err := BumpRevision(tx)
			res.Revision = rev
			return err
		}
		rev, err := readRevision(tx)
		res.Revision = rev
		return err
	})
	if err != nil {
		return SyncResult{}, err
	}
	return res, nil
}
