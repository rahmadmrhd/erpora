package application

import (
	"context"
	"errors"
	"testing"

	"erpora/modules/inventory/internal/domain"
)

type fakeRepo struct {
	items        []domain.Item
	offset, size int
	added        []domain.Item
	addManyCalls int
	err          error
	addManyErr   error
}

func (f *fakeRepo) List(_ context.Context, offset, limit int) ([]domain.Item, error) {
	f.offset, f.size = offset, limit
	return f.items, f.err
}
func (f *fakeRepo) Count(context.Context) (int64, error) { return int64(len(f.items)), f.err }
func (f *fakeRepo) AddMany(_ context.Context, items []domain.Item) error {
	f.addManyCalls++
	if f.addManyErr != nil {
		return f.addManyErr // atomik: tidak ada item yang tersimpan
	}
	f.added = append(f.added, items...)
	f.items = append(f.items, items...)
	return f.err
}
func (f *fakeRepo) Ping(context.Context) error { return f.err }

func TestListItems_OffsetFromPage(t *testing.T) {
	repo := &fakeRepo{items: make([]domain.Item, 3)}
	s := NewService(repo)
	items, total, err := s.ListItems(context.Background(), 3, 25)
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("items=%d total=%d err=%v", len(items), total, err)
	}
	if repo.offset != 50 || repo.size != 25 {
		t.Errorf("offset=%d size=%d", repo.offset, repo.size)
	}
}

func TestListItems_RejectsInvalidPaging(t *testing.T) {
	s := NewService(&fakeRepo{})
	if _, _, err := s.ListItems(context.Background(), 0, 25); err == nil {
		t.Error("page 0 harus ditolak")
	}
	if _, _, err := s.ListItems(context.Background(), 1, 201); err == nil {
		t.Error("pageSize 201 harus ditolak")
	}
}

func TestListItems_RepoError(t *testing.T) {
	s := NewService(&fakeRepo{err: errors.New("db")})
	if _, _, err := s.ListItems(context.Background(), 1, 25); err == nil {
		t.Error("error repo harus diteruskan")
	}
}

func TestSeedInitial_OnlyWhenEmpty(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo)
	for range 2 {
		if err := s.SeedInitial(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.added) != 3 {
		t.Errorf("seed idempoten: ditambahkan %d, ingin 3", len(repo.added))
	}
}

func TestSeedDev_OnlyBelowTwenty(t *testing.T) {
	repo := &fakeRepo{}
	s := NewService(repo)
	for range 2 {
		if err := s.SeedDev(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.added) != 20 {
		t.Errorf("ditambahkan %d, ingin 20", len(repo.added))
	}
}

// Seed awal atomik lewat satu AddMany; kegagalan tidak meninggalkan seed sebagian dan dapat diulang.
func TestSeedInitial_FailureLeavesNothingAndIsRetryable(t *testing.T) {
	repo := &fakeRepo{addManyErr: errors.New("insert gagal")}
	s := NewService(repo)
	if err := s.SeedInitial(context.Background()); err == nil {
		t.Fatal("error diharapkan")
	}
	if len(repo.added) != 0 || repo.addManyCalls != 1 {
		t.Fatalf("seed sebagian tersimpan: added=%d calls=%d", len(repo.added), repo.addManyCalls)
	}
	repo.addManyErr = nil
	if err := s.SeedInitial(context.Background()); err != nil || len(repo.added) != 3 {
		t.Fatalf("ulang gagal melengkapi seed: err=%v added=%d", err, len(repo.added))
	}
}
