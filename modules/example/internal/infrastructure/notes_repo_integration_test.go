//go:build integration

package infrastructure

import (
	"context"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"erpora/modules/example/internal/domain"
	"erpora/sdk/data"
)

type testProvider struct{ db *gorm.DB }

func (p testProvider) DB(context.Context) (*gorm.DB, error) { return p.db, nil }

// TestNotesRepo_AddManyThenListHasVersionAndUpdatedAt memakai database yang sudah dimigrasi (0001 + 0002).
// DSN dari ERP_EXAMPLE_TEST_DSN; test di tests/integration (TestExampleModule_RepoIntegration) menyiapkan
// container PostgreSQL, menerapkan migrasi modul, lalu menjalankan test ini. Tanpa DSN, test dilewati.
func TestNotesRepo_AddManyThenListHasVersionAndUpdatedAt(t *testing.T) {
	dsn := os.Getenv("ERP_EXAMPLE_TEST_DSN")
	if dsn == "" {
		t.Skip("ERP_EXAMPLE_TEST_DSN tidak diset; dijalankan lewat tests/integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s, err := db.DB(); err == nil {
			_ = s.Close()
		}
	})
	ctx := data.WithProvider(context.Background(), testProvider{db: db})

	repo := NewNotesRepo()
	n1, _ := domain.NewNote("satu")
	n2, _ := domain.NewNote("dua")
	if err := repo.AddMany(ctx, []domain.Note{n1, n2}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.List(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for _, n := range got {
		if n.Version != 1 {
			t.Errorf("%s: Version = %d, want 1", n.Title, n.Version)
		}
		if n.CreatedAt.IsZero() || n.UpdatedAt.IsZero() {
			t.Errorf("%s: CreatedAt/UpdatedAt harus terisi: %+v", n.Title, n)
		}
		if n.ID == "" {
			t.Errorf("%s: ID kosong", n.Title)
		}
	}
	if c, err := repo.Count(ctx); err != nil || c != 2 {
		t.Fatalf("Count = %d err = %v", c, err)
	}
}
