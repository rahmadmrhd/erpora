package infrastructure

import (
	"fmt"

	"github.com/google/uuid"

	"erpora/modules/inventory/internal/domain"
)

// categoryToDomain memetakan model GORM ke entity domain (waktu dalam UTC).
func categoryToDomain(m CategoryModel) domain.Category {
	return domain.Category{
		ID: m.ID.String(), Title: m.Title, Version: m.Version,
		CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(),
	}
}

// categoryFromDomain memetakan entity domain ke model GORM; ID kosong menghasilkan uuid.Nil (diisi saat Save).
func categoryFromDomain(e domain.Category) (CategoryModel, error) {
	m := CategoryModel{Title: e.Title, Version: e.Version, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
	if e.ID != "" {
		id, err := uuid.Parse(e.ID)
		if err != nil {
			return CategoryModel{}, fmt.Errorf("id category bukan UUID: %w", err)
		}
		m.ID = id
	}
	return m, nil
}
