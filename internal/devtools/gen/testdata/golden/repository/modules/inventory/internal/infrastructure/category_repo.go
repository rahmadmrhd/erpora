package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/modules/inventory/internal/domain"
	"erpora/sdk/data"
)

// CategoryRepo mengimplementasi domain.CategoryRepository. Koneksi diambil per panggilan dari context lewat
// sdk/data (tenant aktif); repository tidak pernah memegang pool DB.
type CategoryRepo struct{}

var _ domain.CategoryRepository = CategoryRepo{}

// NewCategoryRepo membuat repository.
func NewCategoryRepo() CategoryRepo { return CategoryRepo{} }

// Get mengembalikan satu Category; ErrNotFound bila tidak ada, ID bukan UUID, atau di luar scope tenant.
func (CategoryRepo) Get(ctx context.Context, id string) (domain.Category, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return domain.Category{}, domain.ErrNotFound
	}
	db, err := data.DB(ctx)
	if err != nil {
		return domain.Category{}, err
	}
	var m CategoryModel
	if err := db.WithContext(ctx).Where("id = ?", uid).Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Category{}, domain.ErrNotFound
		}
		return domain.Category{}, fmt.Errorf("query category: %w", err)
	}
	return categoryToDomain(m), nil
}

// List mengembalikan satu halaman (page mulai 1) terurut created_at lalu id, beserta total.
func (CategoryRepo) List(ctx context.Context, page, size int) ([]domain.Category, int64, error) {
	db, err := data.DB(ctx)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := db.WithContext(ctx).Model(&CategoryModel{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("hitung category: %w", err)
	}
	var rows []CategoryModel
	if err := db.WithContext(ctx).Order("created_at, id").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("daftar category: %w", err)
	}
	out := make([]domain.Category, len(rows))
	for i, m := range rows {
		out[i] = categoryToDomain(m)
	}
	return out, total, nil
}

// Save membuat (ID kosong; Version 1) atau memperbarui dengan pemeriksaan Version (optimistic concurrency, api-guidelines §7.1).
// Pembaruan yang tidak mengenai baris (tidak ada, di luar scope, atau Version basi) mengembalikan ErrNotFound; bedakan
// Version basi (412 version_mismatch) di use case bila diperlukan.
func (r CategoryRepo) Save(ctx context.Context, e *domain.Category) error {
	db, err := data.DB(ctx)
	if err != nil {
		return err
	}
	m, err := categoryFromDomain(*e)
	if err != nil {
		return err
	}
	if m.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("buat id: %w", err)
		}
		m.ID, m.Version = id, 1
		if err := db.WithContext(ctx).Create(&m).Error; err != nil {
			return fmt.Errorf("simpan category: %w", err)
		}
		*e = categoryToDomain(m)
		return nil
	}
	res := db.WithContext(ctx).Model(&CategoryModel{}).Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]any{"title": m.Title, "version": m.Version + 1})
	if res.Error != nil {
		return fmt.Errorf("perbarui category: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	saved, err := r.Get(ctx, e.ID)
	if err != nil {
		return err
	}
	*e = saved
	return nil
}
