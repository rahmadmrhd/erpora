package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/tenancy"
)

// Alasan pencabutan sesi yang dipakai store pada jalur atomik.
const (
	RevokeReasonPasswordReset   = "password_reset"
	RevokeReasonPasswordChanged = "password_changed"
)

type resetRow struct {
	ID        uuid.UUID  `gorm:"column:id"`
	TenantID  uuid.UUID  `gorm:"column:tenant_id"`
	UserID    uuid.UUID  `gorm:"column:user_id"`
	TokenHash []byte     `gorm:"column:token_hash"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
}

func (resetRow) TableName() string { return "password_resets" }

// InsertPasswordReset menyimpan tautan reset (hash token 32 byte). ErrNotFound bila pengguna bukan anggota tenant t.
func (s *Store) InsertPasswordReset(ctx context.Context, t tenancy.TenantID, r PasswordReset) error {
	id, ok1 := parseID(r.ID)
	uid, ok2 := parseID(r.UserID)
	if !ok1 || !ok2 || len(r.TokenHash) != 32 || r.ExpiresAt.IsZero() {
		return ErrInvalidArgument
	}
	created := r.CreatedAt
	if created.IsZero() {
		created = nowUTC()
	}
	n, err := exec(s.db(ctx), `INSERT INTO password_resets (id, tenant_id, user_id, token_hash, created_at, expires_at)
		SELECT ?::uuid, m.tenant_id, m.user_id, ?::bytea, ?::timestamptz, ?::timestamptz FROM tenant_memberships m
		WHERE m.tenant_id = ? AND m.user_id = ?`, id, r.TokenHash, created.UTC(), r.ExpiresAt.UTC(), tid(t), uid)
	if err != nil {
		return wrap("simpan reset sandi", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// FindPasswordResetByHash memuat tautan reset menurut hash token pada tenant t; ErrNotFound bila tidak dikenal.
func (s *Store) FindPasswordResetByHash(ctx context.Context, t tenancy.TenantID, hash []byte) (PasswordReset, error) {
	if len(hash) != 32 {
		return PasswordReset{}, ErrNotFound
	}
	var r resetRow
	res := s.db(ctx).Raw(`SELECT * FROM password_resets WHERE tenant_id = ? AND token_hash = ?`, tid(t), hash).Scan(&r)
	if res.Error != nil {
		return PasswordReset{}, wrap("muat reset sandi", res.Error)
	}
	if res.RowsAffected == 0 {
		return PasswordReset{}, ErrNotFound
	}
	return PasswordReset{
		ID: idString(r.ID), UserID: idString(r.UserID), TokenHash: r.TokenHash, CreatedAt: r.CreatedAt.UTC(),
		ExpiresAt: r.ExpiresAt.UTC(), UsedAt: utcPtr(r.UsedAt),
	}, nil
}

// ConsumePasswordReset memakai tautan secara bersyarat atomik (used_at IS NULL dan expires_at > at): tepat satu
// pemanggil serentak mendapat true; kedaluwarsa, sudah terpakai, tidak ada, atau milik tenant lain menghasilkan
// false. Untuk alur reset lengkap pakai ResetPassword.
func (s *Store) ConsumePasswordReset(ctx context.Context, t tenancy.TenantID, id string, at time.Time) (bool, error) {
	rid, ok := parseID(id)
	if !ok {
		return false, nil
	}
	n, err := exec(s.db(ctx), `UPDATE password_resets SET used_at = ? WHERE id = ? AND tenant_id = ? AND used_at IS NULL AND expires_at > ?`,
		at.UTC(), rid, tid(t), at.UTC())
	if err != nil {
		return false, wrap("pakai reset sandi", err)
	}
	return n == 1, nil
}

// InvalidatePasswordResets membatalkan semua tautan reset yang belum terpakai milik pengguna (dipanggil sebelum
// menerbitkan tautan baru agar hanya tautan terbaru yang berlaku) dan mengembalikan jumlahnya.
func (s *Store) InvalidatePasswordResets(ctx context.Context, t tenancy.TenantID, userID string, at time.Time) (int64, error) {
	uid, ok := parseID(userID)
	if !ok {
		return 0, nil
	}
	n, err := exec(s.db(ctx), `UPDATE password_resets SET used_at = ? WHERE tenant_id = ? AND user_id = ? AND used_at IS NULL`, at.UTC(), tid(t), uid)
	if err != nil {
		return 0, wrap("batalkan reset sandi", err)
	}
	return n, nil
}

// ResetPassword menyelesaikan reset sandi dalam satu transaksi atomik: memakai tautan (bersyarat used_at IS NULL
// dan belum kedaluwarsa pada at), mengganti hash sandi, membuka akun locked, membatalkan tautan lain milik
// pengguna, mencabut SEMUA sesi pengguna, dan menghapus catatan percobaan login. ok=false (tanpa efek) bila tautan
// tidak dapat dipakai atau pemiliknya berstatus disabled; hanya satu pemanggil serentak yang mendapat ok=true.
//
// Penjaga disabled berlapis dan atomik: syarat EXISTS pada UPDATE tautan memakai snapshot (READ COMMITTED) sehingga
// belum melihat penonaktifan yang belum commit; karena itu UPDATE users juga bersyarat status IN ('active','locked')
// (menunggu lock baris, lalu mengevaluasi ulang status terbaru) dan bila 0 baris berubah seluruh transaksi di-rollback
// (tautan tidak terpakai, sesi tidak dicabut) dan hasilnya ok=false.
func (s *Store) ResetPassword(ctx context.Context, t tenancy.TenantID, resetID, newHash string, at time.Time) (ResetResult, bool, error) {
	rid, ok := parseID(resetID)
	if !ok {
		return ResetResult{}, false, nil
	}
	if newHash == "" || len(newHash) > 512 {
		return ResetResult{}, false, ErrInvalidArgument
	}
	var res ResetResult
	won := false
	errOwnerInactive := errors.New("store: pemilik tautan reset tidak aktif") // sentinel rollback
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		// Pemilik berstatus disabled: tautan tidak dapat dipakai (PRD-001 Q30 butir 5); tanpa efek, tautan tidak terpakai.
		ids, err := pluckIDs(tx, `UPDATE password_resets SET used_at = ? WHERE id = ? AND tenant_id = ? AND used_at IS NULL AND expires_at > ?
			AND EXISTS (SELECT 1 FROM users WHERE users.id = password_resets.user_id AND users.status IN ('active', 'locked'))
			RETURNING user_id::text`, at.UTC(), rid, tid(t), at.UTC())
		if err != nil || len(ids) == 0 {
			return err
		}
		uid, _ := parseID(ids[0])
		// Bersyarat status: penonaktifan konkuren yang belum commit tidak boleh ditimpa (lihat komentar fungsi).
		n, err := exec(tx, `UPDATE users SET password_hash = ?,
				status = CASE WHEN status = 'locked' THEN 'active' ELSE status END,
				locked_at = CASE WHEN status = 'locked' THEN NULL ELSE locked_at END, updated_at = now()
			WHERE id = ? AND status IN ('active', 'locked')`, newHash, uid)
		if err != nil {
			return err
		}
		if n == 0 {
			return errOwnerInactive // rollback: tautan kembali belum terpakai
		}
		if _, err := exec(tx, `UPDATE password_resets SET used_at = ? WHERE tenant_id = ? AND user_id = ? AND used_at IS NULL`, at.UTC(), tid(t), uid); err != nil {
			return err
		}
		revoked, err := revokeUserSessions(tx, t, uid, "", RevokeReasonPasswordReset, at)
		if err != nil {
			return err
		}
		if _, err := exec(tx, `DELETE FROM login_attempts WHERE tenant_id = ? AND user_id = ?`, tid(t), uid); err != nil {
			return err
		}
		res = ResetResult{UserID: idString(uid), RevokedSessionIDs: revoked}
		won = true
		return nil
	})
	if errors.Is(err, errOwnerInactive) {
		return ResetResult{}, false, nil
	}
	if err != nil {
		return ResetResult{}, false, wrap("reset sandi", err)
	}
	return res, won, nil
}

// ChangePassword mengganti hash sandi pengguna dan mencabut semua sesi lain (kecuali exceptSessionID, sesi yang
// sedang dipakai; kosong = semua) dalam satu transaksi; mengembalikan id sesi yang baru tercabut. ErrNotFound bila
// bukan anggota tenant t.
func (s *Store) ChangePassword(ctx context.Context, t tenancy.TenantID, userID, newHash, exceptSessionID string, at time.Time) ([]string, error) {
	uid, ok := parseID(userID)
	if !ok {
		return nil, ErrNotFound
	}
	if newHash == "" || len(newHash) > 512 {
		return nil, ErrInvalidArgument
	}
	var revoked []string
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := exec(tx, `UPDATE users SET password_hash = ?, updated_at = now() WHERE id = ? AND `+memberExists("users.id"), newHash, uid, tid(t))
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		revoked, err = revokeUserSessions(tx, t, uid, exceptSessionID, RevokeReasonPasswordChanged, at)
		return err
	})
	if err != nil {
		if err == ErrNotFound || err == ErrInvalidArgument {
			return nil, err
		}
		return nil, wrap("ganti sandi", err)
	}
	return revoked, nil
}
