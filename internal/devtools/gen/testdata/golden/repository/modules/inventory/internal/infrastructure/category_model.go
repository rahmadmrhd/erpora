package infrastructure

import (
	"time"

	"github.com/google/uuid"
)

// CategoryModel adalah model GORM tabel inventory_categories, terpisah dari entity domain. Tag gorm eksplisit.
// Tabelnya dibuat lewat `erpora make:migration inventory create_inventory_categories`; isolasi tenant ditangani
// lapisan database (ADR-004), sehingga tidak ada kolom tenant_id.
type CategoryModel struct {
	ID        uuid.UUID `gorm:"column:id;primaryKey"`
	Title     string    `gorm:"column:title"`
	Version   int64     `gorm:"column:version"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName menamai tabel dengan prefiks modul.
func (CategoryModel) TableName() string { return "inventory_categories" }
