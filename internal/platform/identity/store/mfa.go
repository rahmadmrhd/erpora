package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/tenancy"
)

type mfaRow struct {
	ID           uuid.UUID  `gorm:"column:id"`
	UserID       uuid.UUID  `gorm:"column:user_id"`
	Type         string     `gorm:"column:type"`
	SecretSealed []byte     `gorm:"column:secret_sealed"`
	ConfirmedAt  *time.Time `gorm:"column:confirmed_at"`
	LastUsedStep int64      `gorm:"column:last_used_step"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
}

func (mfaRow) TableName() string { return "user_mfa_factors" }

// GetMFAFactor memuat faktor TOTP pengguna anggota tenant t; ErrNotFound bila belum ada atau bukan anggota.
func (s *Store) GetMFAFactor(ctx context.Context, t tenancy.TenantID, userID string) (MFAFactor, error) {
	uid, ok := parseID(userID)
	if !ok {
		return MFAFactor{}, ErrNotFound
	}
	var r mfaRow
	res := s.db(ctx).Raw(`SELECT f.* FROM user_mfa_factors f JOIN tenant_memberships m ON m.user_id = f.user_id AND m.tenant_id = ?
		WHERE f.user_id = ? AND f.type = 'totp'`, tid(t), uid).Scan(&r)
	if res.Error != nil {
		return MFAFactor{}, wrap("muat faktor MFA", res.Error)
	}
	if res.RowsAffected == 0 {
		return MFAFactor{}, ErrNotFound
	}
	return MFAFactor{
		ID: idString(r.ID), UserID: idString(r.UserID), Type: r.Type, SecretSealed: r.SecretSealed,
		ConfirmedAt: utcPtr(r.ConfirmedAt), LastUsedStep: r.LastUsedStep, CreatedAt: r.CreatedAt.UTC(),
	}, nil
}

// UpsertPendingMFAFactor menyimpan (atau mengganti) secret TOTP yang belum dikonfirmasi. true bila tersimpan;
// false bila pengguna sudah punya faktor terkonfirmasi (tidak boleh ditimpa lewat pendaftaran ulang; gunakan
// ResetMFA). ErrNotFound bila bukan anggota tenant t. secretSealed adalah hasil crypto.Sealer.
func (s *Store) UpsertPendingMFAFactor(ctx context.Context, t tenancy.TenantID, userID string, secretSealed []byte, at time.Time) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, ErrNotFound
	}
	if len(secretSealed) == 0 || len(secretSealed) > 1024 {
		return false, ErrInvalidArgument
	}
	id, err := uuid.NewV7()
	if err != nil {
		return false, wrap("buat id faktor", err)
	}
	n, err := exec(s.db(ctx), `INSERT INTO user_mfa_factors (id, user_id, type, secret_sealed, created_at)
		SELECT ?::uuid, m.user_id, 'totp', ?::bytea, ?::timestamptz FROM tenant_memberships m WHERE m.tenant_id = ? AND m.user_id = ?
		ON CONFLICT (user_id, type) DO UPDATE SET secret_sealed = EXCLUDED.secret_sealed, created_at = EXCLUDED.created_at
		WHERE user_mfa_factors.confirmed_at IS NULL`, id, secretSealed, at.UTC(), tid(t), uid)
	if err != nil {
		return false, wrap("simpan faktor MFA", err)
	}
	if n == 1 {
		return true, nil
	}
	member, err := isMember(s.db(ctx), t, uid)
	if err != nil {
		return false, wrap("simpan faktor MFA", err)
	}
	if !member {
		return false, ErrNotFound
	}
	return false, nil
}

// ConfirmMFAFactor mengonfirmasi faktor pending secara bersyarat atomik (confirmed_at IS NULL) dan menetapkan
// last_used_step = step kode konfirmasi, sehingga kode pendaftaran tidak dapat dipakai ulang untuk verifikasi.
// false bila tidak ada faktor pending (belum ada, sudah terkonfirmasi, atau bukan anggota tenant t).
func (s *Store) ConfirmMFAFactor(ctx context.Context, t tenancy.TenantID, userID string, step int64, at time.Time) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, nil
	}
	if step < 0 {
		return false, ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `UPDATE user_mfa_factors SET confirmed_at = ?, last_used_step = ?
		WHERE user_id = ? AND type = 'totp' AND confirmed_at IS NULL AND `+memberExists("user_mfa_factors.user_id"), at.UTC(), step, uid, tid(t))
	if err != nil {
		return false, wrap("konfirmasi faktor MFA", err)
	}
	return n == 1, nil
}

// AdvanceMFAStep adalah anti-replay TOTP: UPDATE ... SET last_used_step = step WHERE last_used_step < step pada
// faktor terkonfirmasi. true hanya bila step lebih besar dari last_used_step (tepat satu pemanggil serentak untuk
// step yang sama); false untuk kode yang sama, step lebih lama, faktor belum terkonfirmasi, atau bukan anggota.
func (s *Store) AdvanceMFAStep(ctx context.Context, t tenancy.TenantID, userID string, step int64) (bool, error) {
	uid, ok := parseID(userID)
	if !ok {
		return false, nil
	}
	if step < 0 {
		return false, ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `UPDATE user_mfa_factors SET last_used_step = ?
		WHERE user_id = ? AND type = 'totp' AND confirmed_at IS NOT NULL AND last_used_step < ? AND `+memberExists("user_mfa_factors.user_id"),
		step, uid, step, tid(t))
	if err != nil {
		return false, wrap("majukan step MFA", err)
	}
	return n == 1, nil
}

// ResetMFA menghapus faktor TOTP dan seluruh kode pemulihan pengguna (reset MFA oleh admin atau core:reset-mfa)
// dalam satu transaksi. Idempoten; ErrNotFound bila bukan anggota tenant t.
func (s *Store) ResetMFA(ctx context.Context, t tenancy.TenantID, userID string) error {
	uid, ok := parseID(userID)
	if !ok {
		return ErrNotFound
	}
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		member, err := isMember(tx, t, uid)
		if err != nil {
			return err
		}
		if !member {
			return ErrNotFound
		}
		if _, err := exec(tx, `DELETE FROM user_mfa_factors WHERE user_id = ?`, uid); err != nil {
			return err
		}
		_, err = exec(tx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, uid)
		return err
	})
	if err != nil {
		if err == ErrNotFound {
			return err
		}
		return wrap("reset MFA", err)
	}
	return nil
}

// ReplaceRecoveryCodes mengganti seluruh kode pemulihan pengguna dengan hashes (masing-masing 32 byte, unik)
// secara atomik: kode lama (terpakai atau tidak) tidak berlaku lagi; bila salah satu gagal disimpan, kode lama
// tetap utuh. ErrNotFound bila bukan anggota tenant t.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, t tenancy.TenantID, userID string, hashes [][]byte, at time.Time) error {
	uid, ok := parseID(userID)
	if !ok {
		return ErrNotFound
	}
	if len(hashes) == 0 || len(hashes) > 64 {
		return ErrInvalidArgument
	}
	for _, h := range hashes {
		if len(h) != 32 {
			return ErrInvalidArgument
		}
	}
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		member, err := isMember(tx, t, uid)
		if err != nil {
			return err
		}
		if !member {
			return ErrNotFound
		}
		if _, err := exec(tx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, uid); err != nil {
			return err
		}
		for _, h := range hashes {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			if _, err := exec(tx, `INSERT INTO user_recovery_codes (id, user_id, code_hash, created_at) VALUES (?, ?, ?, ?)`, id, uid, h, at.UTC()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if err == ErrNotFound {
			return err
		}
		return wrap("ganti kode pemulihan", err)
	}
	return nil
}

// UseRecoveryCode menandai kode terpakai secara bersyarat atomik (used_at IS NULL): tepat satu pemanggil serentak
// mendapat true.
func (s *Store) UseRecoveryCode(ctx context.Context, t tenancy.TenantID, userID string, hash []byte, at time.Time) (bool, error) {
	uid, ok := parseID(userID)
	if !ok || len(hash) != 32 {
		return false, nil
	}
	n, err := exec(s.db(ctx), `UPDATE user_recovery_codes SET used_at = ? WHERE user_id = ? AND code_hash = ? AND used_at IS NULL AND `+memberExists("user_recovery_codes.user_id"),
		at.UTC(), uid, hash, tid(t))
	if err != nil {
		return false, wrap("pakai kode pemulihan", err)
	}
	return n == 1, nil
}

// CountUnusedRecoveryCodes menghitung kode pemulihan yang belum terpakai (0 bila bukan anggota tenant t).
func (s *Store) CountUnusedRecoveryCodes(ctx context.Context, t tenancy.TenantID, userID string) (int, error) {
	uid, ok := parseID(userID)
	if !ok {
		return 0, nil
	}
	var n int
	if err := s.db(ctx).Raw(`SELECT count(*) FROM user_recovery_codes c JOIN tenant_memberships m ON m.user_id = c.user_id AND m.tenant_id = ?
		WHERE c.user_id = ? AND c.used_at IS NULL`, tid(t), uid).Scan(&n).Error; err != nil {
		return 0, wrap("hitung kode pemulihan", err)
	}
	return n, nil
}
