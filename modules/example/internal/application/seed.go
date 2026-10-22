package application

import (
	"context"
	"fmt"

	"erpora/modules/example/internal/domain"
)

// SeedInitial menambah 3 notes awal hanya bila tabel kosong (idempoten).
func (s *Service) SeedInitial(ctx context.Context) error {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return s.add(ctx, "Catatan contoh", 3)
}

// SeedDev menambah 20 notes contoh bila jumlah notes < 20 (idempoten).
func (s *Service) SeedDev(ctx context.Context) error {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return err
	}
	if n >= 20 {
		return nil
	}
	return s.add(ctx, "Catatan dev", 20)
}

// add membuat count note lalu menyimpannya atomik; kegagalan tidak meninggalkan seed sebagian,
// sehingga pemanggilan ulang hook (count masih 0) melengkapi seed.
func (s *Service) add(ctx context.Context, prefix string, count int) error {
	notes := make([]domain.Note, 0, count)
	for i := 1; i <= count; i++ {
		note, err := domain.NewNote(fmt.Sprintf("%s %d", prefix, i))
		if err != nil {
			return err
		}
		notes = append(notes, note)
	}
	return s.repo.AddMany(ctx, notes)
}
