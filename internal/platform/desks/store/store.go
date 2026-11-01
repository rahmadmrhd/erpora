// Package store membaca timpaan Desk per tenant (revisi M3a, design.md section 6): tabel desk_overrides di
// control plane. Satu-satunya pemakai GORM untuk tabel itu (D3b). Revisi M3a hanya MEMBACA (R13.5); jalur tulis
// datang di M3b. Setiap kueri menyaring tenant_id; baris milik tenant lain tidak pernah terlihat.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
)

// Override adalah timpaan satu Desk pada tingkat situs. Config adalah JSON mentah {"hidden":[...],"order":[...]};
// pemanggil (desks.ParseConfig) memvalidasinya sebelum dipakai.
type Override struct {
	DeskID    string
	Config    json.RawMessage
	Revision  int64
	UpdatedBy string
	UpdatedAt time.Time
}

// Store membaca timpaan. Nilai nol tidak valid; gunakan New.
type Store struct{ control database.ControlDB }

// New membuat Store di atas control plane.
func New(control database.ControlDB) *Store { return &Store{control: control} }

type row struct {
	DeskID    string    `gorm:"column:desk_id"`
	Config    []byte    `gorm:"column:config"`
	Revision  int64     `gorm:"column:revision"`
	UpdatedBy string    `gorm:"column:updated_by"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (row) TableName() string { return "desk_overrides" }

// ListSite memuat timpaan tingkat situs (scope "site", tanpa subjek) milik tenant t, terurut desk_id. Scope
// "user" tidak dibaca di sini: butuh identitas pengguna (PRD-001).
func (s *Store) ListSite(ctx context.Context, t tenancy.TenantID) ([]Override, error) {
	var rows []row
	if err := s.control.WithContext(ctx).
		Where("tenant_id = ? AND scope = 'site' AND subject = ''", uuid.UUID(t)).
		Order("desk_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: muat timpaan Desk: %w", err)
	}
	out := make([]Override, 0, len(rows))
	for _, r := range rows {
		out = append(out, Override{
			DeskID: r.DeskID, Config: json.RawMessage(r.Config), Revision: r.Revision,
			UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt.UTC(),
		})
	}
	return out, nil
}
