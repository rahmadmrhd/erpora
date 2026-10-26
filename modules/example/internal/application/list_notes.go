// Package application berisi use case modul example; tidak mengimpor infrastructure/presentation.
package application

import (
	"context"
	"errors"
	"fmt"

	"erpora/modules/example/internal/domain"
)

// Batas pagination (api-guidelines §6).
const (
	DefaultPageSize = 25
	MaxPageSize     = 200
)

// ErrInvalidPaging dikembalikan bila page/pageSize di luar batas.
var ErrInvalidPaging = errors.New("page harus >= 1 dan pageSize 1 sampai 200")

// Repository adalah port persistensi notes; diimplementasi infrastructure.
type Repository interface {
	List(ctx context.Context, offset, limit int) ([]domain.Note, error)
	Count(ctx context.Context) (int64, error)
	// AddMany menyimpan semua note dalam satu transaksi (semua atau tidak sama sekali).
	AddMany(ctx context.Context, notes []domain.Note) error
	Ping(ctx context.Context) error
}

// Service mengorkestrasi use case notes.
type Service struct{ repo Repository }

// NewService membuat Service di atas repo.
func NewService(repo Repository) *Service { return &Service{repo: repo} }

// ListNotes mengembalikan satu halaman notes (page mulai 1) beserta total.
func (s *Service) ListNotes(ctx context.Context, page, pageSize int) (items []domain.Note, total int64, err error) {
	if page < 1 || pageSize < 1 || pageSize > MaxPageSize {
		return nil, 0, ErrInvalidPaging
	}
	total, err = s.repo.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("hitung notes: %w", err)
	}
	items, err = s.repo.List(ctx, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("daftar notes: %w", err)
	}
	return items, total, nil
}

// Count mengembalikan jumlah notes tenant aktif.
func (s *Service) Count(ctx context.Context) (int64, error) { return s.repo.Count(ctx) }

// Health memastikan tabel notes dapat dibaca.
func (s *Service) Health(ctx context.Context) error { return s.repo.Ping(ctx) }
