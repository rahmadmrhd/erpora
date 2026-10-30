// Package store adalah persistensi override terjemahan per tenant (PRD-000 FR-25, Q34/Q35 D-1): tabel
// i18n_overrides dan i18n_revisions di control plane. Satu-satunya pemakai GORM untuk tabel itu (D3b).
// Semua query memfilter tenant_id; baris milik tenant lain tidak pernah terlihat (ErrNotFound / kosong).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
)

// ErrNotFound dikembalikan bila override tidak ada atau milik tenant lain.
var ErrNotFound = errors.New("store: override tidak ditemukan")

// Override adalah satu teks pengganti untuk (tenant, namespace, locale, key).
type Override struct {
	TenantID                                   tenancy.TenantID
	Namespace, Locale, Key, Message, UpdatedBy string
	UpdatedAt                                  time.Time
}

// Revision adalah revisi satu (namespace, locale) milik tenant; naik pada setiap perubahan override.
type Revision struct {
	Namespace, Locale string
	Revision          int64
}

// Filter membatasi List; nilai kosong = tanpa pembatasan.
type Filter struct{ Namespace, Locale string }

// Precondition dijalankan di dalam transaksi setelah baris terkunci (FOR UPDATE); cur nil bila baris belum
// ada. Galat yang dikembalikan membatalkan operasi tanpa mengubah apa pun (dipakai untuk If-Match).
type Precondition func(cur *Override) error

// Store menyimpan override. Nilai nol tidak valid; gunakan New.
type Store struct{ control database.ControlDB }

// New membuat Store di atas control plane.
func New(control database.ControlDB) *Store { return &Store{control: control} }

func tid(t tenancy.TenantID) uuid.UUID { return uuid.UUID(t) }

type overrideRow struct {
	TenantID  uuid.UUID `gorm:"column:tenant_id"`
	Namespace string    `gorm:"column:namespace"`
	Locale    string    `gorm:"column:locale"`
	Key       string    `gorm:"column:key"`
	Message   string    `gorm:"column:message"`
	UpdatedBy string    `gorm:"column:updated_by"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (overrideRow) TableName() string { return "i18n_overrides" }

func (r overrideRow) model() Override {
	return Override{
		TenantID: tenancy.TenantID(r.TenantID), Namespace: r.Namespace, Locale: r.Locale, Key: r.Key,
		Message: r.Message, UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt.UTC(),
	}
}

type revisionRow struct {
	Namespace string `gorm:"column:namespace"`
	Locale    string `gorm:"column:locale"`
	Revision  int64  `gorm:"column:revision"`
}

func (revisionRow) TableName() string { return "i18n_revisions" }

// Get memuat satu override tenant.
func (s *Store) Get(ctx context.Context, t tenancy.TenantID, ns, locale, key string) (Override, error) {
	var rows []overrideRow
	if err := s.control.WithContext(ctx).
		Where("tenant_id = ? AND namespace = ? AND locale = ? AND key = ?", tid(t), ns, locale, key).
		Limit(1).Find(&rows).Error; err != nil {
		return Override{}, fmt.Errorf("store: muat override: %w", err)
	}
	if len(rows) == 0 {
		return Override{}, ErrNotFound
	}
	return rows[0].model(), nil
}

// List memuat override tenant terurut (namespace, locale, key) dengan pagination halaman (page >= 1).
func (s *Store) List(ctx context.Context, t tenancy.TenantID, f Filter, page, pageSize int) ([]Override, int64, error) {
	q := s.control.WithContext(ctx).Model(&overrideRow{}).Where("tenant_id = ?", tid(t))
	if f.Namespace != "" {
		q = q.Where("namespace = ?", f.Namespace)
	}
	if f.Locale != "" {
		q = q.Where("locale = ?", f.Locale)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("store: hitung override: %w", err)
	}
	if page < 1 {
		page = 1
	}
	var rows []overrideRow
	if err := q.Order("namespace, locale, key").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("store: daftar override: %w", err)
	}
	out := make([]Override, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.model())
	}
	return out, total, nil
}

// ListLocale memuat semua override tenant untuk satu (namespace, locale); dipakai cache Service.
func (s *Store) ListLocale(ctx context.Context, t tenancy.TenantID, ns, locale string) ([]Override, error) {
	var rows []overrideRow
	if err := s.control.WithContext(ctx).
		Where("tenant_id = ? AND namespace = ? AND locale = ?", tid(t), ns, locale).
		Order("key").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: muat override namespace: %w", err)
	}
	out := make([]Override, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.model())
	}
	return out, nil
}

// Revisions memuat semua revisi (namespace, locale) tenant dengan satu query ringan.
func (s *Store) Revisions(ctx context.Context, t tenancy.TenantID) ([]Revision, error) {
	var rows []revisionRow
	if err := s.control.WithContext(ctx).Where("tenant_id = ?", tid(t)).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: muat revisi i18n: %w", err)
	}
	out := make([]Revision, 0, len(rows))
	for _, r := range rows {
		out = append(out, Revision(r))
	}
	return out, nil
}

const bumpRevisionSQL = `INSERT INTO i18n_revisions (tenant_id, namespace, locale, revision) VALUES (?, ?, ?, 1)
	ON CONFLICT (tenant_id, namespace, locale) DO UPDATE SET revision = i18n_revisions.revision + 1, updated_at = now()
	RETURNING revision`

func currentRevision(tx *gorm.DB, t tenancy.TenantID, ns, locale string) (int64, error) {
	var revs []int64
	if err := tx.Raw(`SELECT revision FROM i18n_revisions WHERE tenant_id = ? AND namespace = ? AND locale = ?`, tid(t), ns, locale).
		Scan(&revs).Error; err != nil {
		return 0, fmt.Errorf("store: muat revisi: %w", err)
	}
	if len(revs) == 0 {
		return 0, nil
	}
	return revs[0], nil
}

func lockRow(tx *gorm.DB, t tenancy.TenantID, ns, locale, key string) (*Override, error) {
	var rows []overrideRow
	if err := tx.Raw(`SELECT tenant_id, namespace, locale, key, message, updated_by, updated_at FROM i18n_overrides
		WHERE tenant_id = ? AND namespace = ? AND locale = ? AND key = ? FOR UPDATE`, tid(t), ns, locale, key).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: kunci override: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	m := rows[0].model()
	return &m, nil
}

// Upsert menyimpan override. Dalam SATU transaksi: kunci baris, jalankan pre (bila ada), lalu simpan dan,
// hanya bila nilainya berubah, naikkan revisi (namespace, locale). Nilai yang sama tidak mengubah apa pun
// (idempoten; changed=false) dan revisi tidak naik. Mengembalikan revisi sesudahnya.
func (s *Store) Upsert(ctx context.Context, t tenancy.TenantID, o Override, pre Precondition) (changed bool, rev int64, err error) {
	err = s.control.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cur, err := lockRow(tx, t, o.Namespace, o.Locale, o.Key)
		if err != nil {
			return err
		}
		if pre != nil {
			if err := pre(cur); err != nil {
				return err
			}
		}
		if cur != nil && cur.Message == o.Message {
			rev, err = currentRevision(tx, t, o.Namespace, o.Locale)
			return err
		}
		// Satu pernyataan bersyarat: baris hanya ditulis (dan RETURNING hanya mengembalikan baris) bila isinya benar-benar
		// berubah. Dua PUT serentak dengan nilai sama pada baris yang belum ada: yang kedua menunggu yang pertama
		// commit, lalu WHERE mendapati nilai sama sehingga tidak menulis dan tidak menaikkan revisi (R-7b; FOR UPDATE
		// saja tidak mengunci baris yang belum ada).
		var written []int
		if err := tx.Raw(`INSERT INTO i18n_overrides (tenant_id, namespace, locale, key, message, updated_by, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, now())
			ON CONFLICT (tenant_id, namespace, locale, key)
			DO UPDATE SET message = EXCLUDED.message, updated_by = EXCLUDED.updated_by, updated_at = now()
			WHERE i18n_overrides.message IS DISTINCT FROM EXCLUDED.message
			RETURNING 1`,
			tid(t), o.Namespace, o.Locale, o.Key, o.Message, o.UpdatedBy).Scan(&written).Error; err != nil {
			return fmt.Errorf("store: simpan override: %w", err)
		}
		if len(written) == 0 {
			rev, err = currentRevision(tx, t, o.Namespace, o.Locale)
			return err
		}
		var revs []int64
		if err := tx.Raw(bumpRevisionSQL, tid(t), o.Namespace, o.Locale).Scan(&revs).Error; err != nil {
			return fmt.Errorf("store: naikkan revisi: %w", err)
		}
		if len(revs) == 0 {
			return errNoRevision
		}
		changed, rev = true, revs[0]
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	return changed, rev, nil
}

var errNoRevision = errors.New("store: baris revisi tidak dikembalikan")

// Delete menghapus override. Dalam SATU transaksi: kunci baris, jalankan pre (bila ada), hapus, dan hanya bila
// baris ada naikkan revisi. existed=false bila barisnya tidak ada (tidak ada perubahan, revisi tetap).
func (s *Store) Delete(ctx context.Context, t tenancy.TenantID, ns, locale, key string, pre Precondition) (existed bool, rev int64, err error) {
	err = s.control.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cur, err := lockRow(tx, t, ns, locale, key)
		if err != nil {
			return err
		}
		if pre != nil {
			if err := pre(cur); err != nil {
				return err
			}
		}
		if cur == nil {
			rev, err = currentRevision(tx, t, ns, locale)
			return err
		}
		if err := tx.Exec(`DELETE FROM i18n_overrides WHERE tenant_id = ? AND namespace = ? AND locale = ? AND key = ?`,
			tid(t), ns, locale, key).Error; err != nil {
			return fmt.Errorf("store: hapus override: %w", err)
		}
		var revs []int64
		if err := tx.Raw(bumpRevisionSQL, tid(t), ns, locale).Scan(&revs).Error; err != nil {
			return fmt.Errorf("store: naikkan revisi: %w", err)
		}
		if len(revs) == 0 {
			return errNoRevision
		}
		existed, rev = true, revs[0]
		return nil
	})
	if err != nil {
		return false, 0, err
	}
	return existed, rev, nil
}
