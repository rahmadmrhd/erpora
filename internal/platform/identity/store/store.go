package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
)

var (
	// ErrNotFound berarti baris tidak ada, atau ada tetapi bukan milik tenant pemanggil (keduanya sengaja tidak
	// dibedakan: record di luar scope = 404, PRD-001 FR-119). ID yang bukan UUID juga menghasilkan ErrNotFound.
	ErrNotFound = errors.New("store: tidak ditemukan")
	// ErrEmailTaken berarti email (setelah normalisasi) sudah dipakai akun lain.
	ErrEmailTaken = errors.New("store: email sudah dipakai")
	// ErrConflict berarti baris dengan kunci yang sama sudah ada (mis. kid kunci penandatangan).
	ErrConflict = errors.New("store: konflik kunci")
	// ErrInvalidArgument berarti masukan melanggar aturan bentuk yang juga ditegakkan CHECK database.
	ErrInvalidArgument = errors.New("store: argumen tidak valid")
	// ErrInvalidTransition berarti pasangan status kunci (dari, ke) bukan tepi yang diizinkan.
	ErrInvalidTransition = errors.New("store: transisi status kunci tidak diizinkan")
)

// Store adalah persistensi identitas. Satu-satunya pemakai GORM untuk tabel identitas (D3b). Nilai nol tidak valid;
// gunakan New. Tabel control (users, sesi, token, kunci, MFA, reset, percobaan login) lewat ControlDB; tabel tenant
// (roles, user_roles) lewat TenantConnector. Setiap kueri berscope tenant menyaring tenant_id atau keanggotaan
// tenant_memberships; baris milik tenant lain tidak pernah terlihat. Pengecualian terdokumentasi: signing_keys
// bersifat global platform.
type Store struct {
	control database.ControlDB
	tenants database.TenantConnector
}

// New membuat Store. tenants boleh nil bila operasi peran tidak dipakai; operasi peran lalu gagal tertutup.
func New(control database.ControlDB, tenants database.TenantConnector) *Store {
	return &Store{control: control, tenants: tenants}
}

// NormalizeEmail adalah satu-satunya bentuk kanonik email: trim spasi tepi dan huruf kecil. CHECK pada users.email
// menolak bentuk lain, sehingga UNIQUE (email) berarti unik tanpa memandang huruf atau spasi.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func tid(t tenancy.TenantID) uuid.UUID { return uuid.UUID(t) }

// parseID memeriksa bentuk UUID kanonik (36 karakter). ID lain tidak pernah mencapai database.
func parseID(s string) (uuid.UUID, bool) {
	if len(s) != 36 {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	return id, err == nil
}

func nowUTC() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func (s *Store) db(ctx context.Context) *gorm.DB { return s.control.WithContext(ctx) }

// exec menjalankan pernyataan dan mengembalikan jumlah baris terpengaruh.
func exec(db *gorm.DB, q string, args ...any) (int64, error) {
	r := db.Exec(q, args...)
	return r.RowsAffected, r.Error
}

// pluckIDs menjalankan pernyataan yang mengembalikan satu kolom teks (mis. UPDATE ... RETURNING id::text).
func pluckIDs(db *gorm.DB, q string, args ...any) ([]string, error) {
	var ids []string
	if err := db.Raw(q, args...).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// memberExists adalah subkueri keanggotaan: pengguna (kolom col) adalah anggota tenant (placeholder pertama).
func memberExists(col string) string {
	return "EXISTS (SELECT 1 FROM tenant_memberships m WHERE m.tenant_id = ? AND m.user_id = " + col + ")"
}

func isMember(db *gorm.DB, t tenancy.TenantID, userID uuid.UUID) (bool, error) {
	var n int64
	if err := db.Raw(`SELECT count(*) FROM tenant_memberships WHERE tenant_id = ? AND user_id = ?`, tid(t), userID).Scan(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func wrap(op string, err error) error { return fmt.Errorf("store: %s: %w", op, err) }
