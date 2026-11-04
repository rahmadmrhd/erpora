package store

import (
	"context"
	"regexp"
	"time"
)

var kidPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

type signingKeyRow struct {
	Kid              string     `gorm:"column:kid"`
	Status           string     `gorm:"column:status"`
	PublicKey        []byte     `gorm:"column:public_key"`
	PrivateKeySealed []byte     `gorm:"column:private_key_sealed"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	ActivatedAt      *time.Time `gorm:"column:activated_at"`
	RetiringAt       *time.Time `gorm:"column:retiring_at"`
	RetiredAt        *time.Time `gorm:"column:retired_at"`
}

func (signingKeyRow) TableName() string { return "signing_keys" }

// ValidKeyTransition melaporkan apakah (from, to) tepi yang diizinkan: pending->active, active->retiring,
// retiring->retired, dan pending->retired (kunci pending yang dibatalkan).
func ValidKeyTransition(from, to string) bool {
	switch from {
	case KeyPending:
		return to == KeyActive || to == KeyRetired
	case KeyActive:
		return to == KeyRetiring
	case KeyRetiring:
		return to == KeyRetired
	}
	return false
}

// normalizeKey mengisi cap waktu yang diwajibkan status bila pemanggil tidak mengisinya (bawaan CreatedAt) dan
// menyeragamkan ke UTC.
func normalizeKey(k *SigningKey) {
	k.CreatedAt = k.CreatedAt.UTC()
	k.ActivatedAt, k.RetiringAt, k.RetiredAt = utcPtr(k.ActivatedAt), utcPtr(k.RetiringAt), utcPtr(k.RetiredAt)
	c := k.CreatedAt
	switch k.Status {
	case KeyActive:
		if k.ActivatedAt == nil {
			k.ActivatedAt = &c
		}
	case KeyRetiring:
		if k.ActivatedAt == nil {
			k.ActivatedAt = &c
		}
		if k.RetiringAt == nil {
			k.RetiringAt = &c
		}
	case KeyRetired:
		if k.RetiredAt == nil {
			k.RetiredAt = &c
		}
	}
}

func validKey(k SigningKey) bool {
	switch k.Status {
	case KeyPending:
		if k.ActivatedAt != nil || k.RetiringAt != nil || k.RetiredAt != nil {
			return false
		}
	case KeyActive, KeyRetiring, KeyRetired:
	default:
		return false
	}
	return kidPattern.MatchString(k.ID) && len(k.PublicKey) == 32 && len(k.PrivateKeySealed) > 0 && len(k.PrivateKeySealed) <= 4096
}

// ListSigningKeys memuat semua kunci (semua status), terurut created_at lalu kid. signing_keys global platform:
// tidak ada filter tenant (satu JWKS; klaim tid yang menyatakan tenant).
func (s *Store) ListSigningKeys(ctx context.Context) ([]SigningKey, error) {
	var rows []signingKeyRow
	if err := s.db(ctx).Order("created_at, kid").Find(&rows).Error; err != nil {
		return nil, wrap("muat kunci penandatangan", err)
	}
	out := make([]SigningKey, 0, len(rows))
	for _, r := range rows {
		out = append(out, SigningKey{
			ID: r.Kid, Status: r.Status, PublicKey: r.PublicKey, PrivateKeySealed: r.PrivateKeySealed,
			CreatedAt: r.CreatedAt.UTC(), ActivatedAt: utcPtr(r.ActivatedAt), RetiringAt: utcPtr(r.RetiringAt), RetiredAt: utcPtr(r.RetiredAt),
		})
	}
	return out, nil
}

// InsertSigningKey menyimpan kunci baru. CreatedAt nol berarti sekarang; cap waktu yang diwajibkan status dan
// kosong diisi CreatedAt (kunci pertama boleh langsung active). ErrInvalidArgument untuk bentuk salah,
// ErrConflict bila kid sudah ada.
func (s *Store) InsertSigningKey(ctx context.Context, k SigningKey) error {
	if k.CreatedAt.IsZero() {
		k.CreatedAt = nowUTC()
	}
	normalizeKey(&k)
	if !validKey(k) {
		return ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `INSERT INTO signing_keys (kid, status, public_key, private_key_sealed, created_at, activated_at, retiring_at, retired_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (kid) DO NOTHING`,
		k.ID, k.Status, k.PublicKey, k.PrivateKeySealed, k.CreatedAt, k.ActivatedAt, k.RetiringAt, k.RetiredAt)
	if err != nil {
		return wrap("simpan kunci penandatangan", err)
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// TransitionSigningKey memindahkan kunci dari status from ke to secara bersyarat atomik (satu UPDATE ... WHERE
// kid = ? AND status = from) dan mengisi cap waktu status baru dengan at. Mengembalikan false (tanpa efek) bila
// status saat ini bukan from atau kunci tidak ada; ErrInvalidTransition bila (from, to) bukan tepi yang diizinkan.
func (s *Store) TransitionSigningKey(ctx context.Context, id, from, to string, at time.Time) (bool, error) {
	if !ValidKeyTransition(from, to) {
		return false, ErrInvalidTransition
	}
	col := map[string]string{KeyActive: "activated_at", KeyRetiring: "retiring_at", KeyRetired: "retired_at"}[to]
	n, err := exec(s.db(ctx), `UPDATE signing_keys SET status = ?, `+col+` = ? WHERE kid = ? AND status = ?`, to, at.UTC(), id, from)
	if err != nil {
		return false, wrap("ubah status kunci", err)
	}
	return n == 1, nil
}
