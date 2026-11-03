package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"erpora/internal/platform/tenancy"
)

// tenantDB memilih database tenant t lewat TenantConnector (roles dan user_roles ada di tenant DB, ADR-0004).
// Tenant yang tidak dikenal connector menghasilkan galat (gagal tertutup), bukan data tenant lain.
func (s *Store) tenantDB(ctx context.Context, t tenancy.TenantID) (*gorm.DB, error) {
	if s.tenants == nil {
		return nil, errors.New("store: TenantConnector tidak dikonfigurasi")
	}
	db, err := s.tenants.For(ctx, t)
	if err != nil {
		return nil, wrap("pilih database tenant", err)
	}
	return db.WithContext(ctx), nil
}

// GrantRole memberi role (menurut key, mis. "administrator") kepada pengguna. true bila baru diberikan, false bila
// sudah dimiliki; ErrNotFound bila role tidak ada. Keberadaan pengguna adalah tanggung jawab pemanggil (tabel
// users ada di control DB; tenant DB dapat berupa database terpisah).
func (s *Store) GrantRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string, at time.Time) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, ErrNotFound
	}
	db, err := s.tenantDB(ctx, t)
	if err != nil {
		return false, err
	}
	n, err := exec(db, `INSERT INTO user_roles (user_id, role_id, granted_at)
		SELECT ?::uuid, r.id, ?::timestamptz FROM roles r WHERE r.key = ? ON CONFLICT DO NOTHING`, uid, at.UTC(), roleKey)
	if err != nil {
		return false, wrap("beri role", err)
	}
	if n == 1 {
		return true, nil
	}
	var exists int64
	if err := db.Raw(`SELECT count(*) FROM roles WHERE key = ?`, roleKey).Scan(&exists).Error; err != nil {
		return false, wrap("beri role", err)
	}
	if exists == 0 {
		return false, ErrNotFound
	}
	return false, nil
}

// RevokeRole mencabut role dari pengguna; false bila tidak dimiliki atau role tidak ada. Larangan mencabut
// administrator terakhir adalah aturan service (FR-113), bukan store.
func (s *Store) RevokeRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, nil
	}
	db, err := s.tenantDB(ctx, t)
	if err != nil {
		return false, err
	}
	n, err := exec(db, `DELETE FROM user_roles WHERE user_id = ? AND role_id = (SELECT id FROM roles WHERE key = ?)`, uid, roleKey)
	if err != nil {
		return false, wrap("cabut role", err)
	}
	return n > 0, nil
}

// UserRoleKeys memuat key role milik pengguna, terurut.
func (s *Store) UserRoleKeys(ctx context.Context, t tenancy.TenantID, userID string) ([]string, error) {
	uid, ok := parseID(userID)
	if !ok {
		return nil, nil
	}
	db, err := s.tenantDB(ctx, t)
	if err != nil {
		return nil, err
	}
	keys, err := pluckIDs(db, `SELECT r.key FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = ? ORDER BY r.key`, uid)
	if err != nil {
		return nil, wrap("muat role pengguna", err)
	}
	return keys, nil
}

// HasRole melaporkan apakah pengguna memiliki role itu.
func (s *Store) HasRole(ctx context.Context, t tenancy.TenantID, userID, roleKey string) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, nil
	}
	db, err := s.tenantDB(ctx, t)
	if err != nil {
		return false, err
	}
	var n int64
	if err := db.Raw(`SELECT count(*) FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = ? AND r.key = ?`, uid, roleKey).Scan(&n).Error; err != nil {
		return false, wrap("periksa role", err)
	}
	return n > 0, nil
}

// CountRoleHolders menghitung pemegang role (mis. untuk melindungi administrator terakhir).
func (s *Store) CountRoleHolders(ctx context.Context, t tenancy.TenantID, roleKey string) (int, error) {
	db, err := s.tenantDB(ctx, t)
	if err != nil {
		return 0, err
	}
	var n int
	if err := db.Raw(`SELECT count(*) FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE r.key = ?`, roleKey).Scan(&n).Error; err != nil {
		return 0, wrap("hitung pemegang role", err)
	}
	return n, nil
}
