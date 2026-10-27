// Package infrastructure berisi persistensi modul stock-opname (GORM). Akses database hanya lewat sdk/data.
package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/modules/stockopname/internal/domain"
	"erpora/sdk/data"
)

// itemRow adalah model GORM tabel stock_opname_items, terpisah dari entity domain.
type itemRow struct {
	ID        uuid.UUID `gorm:"column:id;primaryKey"`
	Title     string    `gorm:"column:title"`
	Version   int64     `gorm:"column:version"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (itemRow) TableName() string { return "stock_opname_items" }

func (r itemRow) toDomain() domain.Item {
	return domain.Item{
		ID: r.ID.String(), Title: r.Title, Version: r.Version,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

// ItemsRepo adalah repository items; koneksi diambil per panggilan dari context (tenant aktif).
type ItemsRepo struct{}

// NewItemsRepo membuat repository.
func NewItemsRepo() ItemsRepo { return ItemsRepo{} }

// List mengembalikan items terurut created_at lalu id.
func (ItemsRepo) List(ctx context.Context, offset, limit int) ([]domain.Item, error) {
	db, err := data.DB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []itemRow
	if err := db.WithContext(ctx).Order("created_at, id").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	out := make([]domain.Item, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out, nil
}

// Count mengembalikan jumlah items.
func (ItemsRepo) Count(ctx context.Context) (int64, error) {
	db, err := data.DB(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.WithContext(ctx).Model(&itemRow{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("hitung items: %w", err)
	}
	return n, nil
}

// AddMany menyimpan semua item dalam satu transaksi.
func (ItemsRepo) AddMany(ctx context.Context, items []domain.Item) error {
	db, err := data.DB(ctx)
	if err != nil {
		return err
	}
	rows := make([]itemRow, len(items))
	for i, n := range items {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("buat id: %w", err)
		}
		rows[i] = itemRow{ID: id, Title: n.Title, Version: 1}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return tx.Create(&rows).Error }); err != nil {
		return fmt.Errorf("simpan items: %w", err)
	}
	return nil
}

// Ping menjalankan SELECT 1 FROM stock_opname_items LIMIT 1.
func (ItemsRepo) Ping(ctx context.Context) error {
	db, err := data.DB(ctx)
	if err != nil {
		return err
	}
	var one int
	res := db.WithContext(ctx).Raw("SELECT 1 FROM stock_opname_items LIMIT 1").Scan(&one)
	if res.Error != nil {
		return fmt.Errorf("health items: %w", res.Error)
	}
	return nil
}
