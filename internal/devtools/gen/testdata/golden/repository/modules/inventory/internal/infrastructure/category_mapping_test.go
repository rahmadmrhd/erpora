package infrastructure

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/modules/inventory/internal/domain"
)

// Test unit mapping model <-> entity tanpa database. Test repository terhadap DB ditulis lewat platform
// (pola internal/app/reference_modules_integration_test.go); lihat keputusan rencana M2d butir 2.
func TestCategoryMapping_RoundTrip(t *testing.T) {
	id := uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b")
	ts := time.Date(2026, 10, 22, 7, 0, 0, 0, time.FixedZone("x", 7*3600))
	model := CategoryModel{ID: id, Title: "Halo", Version: 3, CreatedAt: ts, UpdatedAt: ts.Add(time.Hour)}

	e := categoryToDomain(model)
	if e.ID != id.String() || e.Title != "Halo" || e.Version != 3 {
		t.Fatalf("entity = %+v", e)
	}
	if e.CreatedAt.Location() != time.UTC || !e.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt harus UTC dan setara: %v", e.CreatedAt)
	}
	back, err := categoryFromDomain(e)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != id || back.Title != "Halo" || back.Version != 3 || !back.UpdatedAt.Equal(ts.Add(time.Hour)) {
		t.Errorf("model = %+v", back)
	}
}

func TestCategoryMapping_NewAndInvalidID(t *testing.T) {
	m, err := categoryFromDomain(domain.Category{Title: "Baru"})
	if err != nil || m.ID != uuid.Nil {
		t.Errorf("ID kosong harus uuid.Nil: %+v %v", m, err)
	}
	if _, err := categoryFromDomain(domain.Category{ID: "bukan-uuid"}); err == nil {
		t.Error("ID bukan UUID harus error")
	}
}

func TestCategoryModel_TableName(t *testing.T) {
	if got := (CategoryModel{}).TableName(); got != "inventory_categories" {
		t.Errorf("TableName = %q", got)
	}
}
