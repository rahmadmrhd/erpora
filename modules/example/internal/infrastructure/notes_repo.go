// Package infrastructure berisi persistensi modul example (GORM).
package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/modules/example/internal/domain"
	"erpora/sdk/data"
)

// noteRow adalah model GORM tabel example_notes, terpisah dari entity domain.
type noteRow struct {
	ID        uuid.UUID `gorm:"column:id;primaryKey"`
	Title     string    `gorm:"column:title"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (noteRow) TableName() string { return "example_notes" }

func (r noteRow) toDomain() domain.Note {
	return domain.Note{ID: r.ID.String(), Title: r.Title, CreatedAt: r.CreatedAt.UTC()}
}

// NotesRepo adalah repository notes; koneksi diambil per panggilan dari context (tenant aktif).
type NotesRepo struct{}

// NewNotesRepo membuat repository.
func NewNotesRepo() NotesRepo { return NotesRepo{} }

// List mengembalikan notes terurut created_at lalu id.
func (NotesRepo) List(ctx context.Context, offset, limit int) ([]domain.Note, error) {
	db, err := data.DB(ctx)
	if err != nil {
		return nil, err
	}
	var rows []noteRow
	if err := db.WithContext(ctx).Order("created_at, id").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query notes: %w", err)
	}
	out := make([]domain.Note, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out, nil
}

// Count mengembalikan jumlah notes.
func (NotesRepo) Count(ctx context.Context) (int64, error) {
	db, err := data.DB(ctx)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.WithContext(ctx).Model(&noteRow{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("hitung notes: %w", err)
	}
	return n, nil
}

// Add menyimpan note baru dengan ID UUIDv7.
func (NotesRepo) Add(ctx context.Context, n domain.Note) error {
	db, err := data.DB(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("buat id: %w", err)
	}
	if err := db.WithContext(ctx).Create(&noteRow{ID: id, Title: n.Title}).Error; err != nil {
		return fmt.Errorf("simpan note: %w", err)
	}
	return nil
}

// AddMany menyimpan semua note dalam satu transaksi.
func (NotesRepo) AddMany(ctx context.Context, notes []domain.Note) error {
	db, err := data.DB(ctx)
	if err != nil {
		return err
	}
	rows := make([]noteRow, len(notes))
	for i, n := range notes {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("buat id: %w", err)
		}
		rows[i] = noteRow{ID: id, Title: n.Title}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return tx.Create(&rows).Error }); err != nil {
		return fmt.Errorf("simpan notes: %w", err)
	}
	return nil
}

// Ping menjalankan SELECT 1 FROM example_notes LIMIT 1.
func (NotesRepo) Ping(ctx context.Context) error {
	db, err := data.DB(ctx)
	if err != nil {
		return err
	}
	var one int
	res := db.WithContext(ctx).Raw("SELECT 1 FROM example_notes LIMIT 1").Scan(&one)
	if res.Error != nil {
		return fmt.Errorf("health notes: %w", res.Error)
	}
	return nil
}
