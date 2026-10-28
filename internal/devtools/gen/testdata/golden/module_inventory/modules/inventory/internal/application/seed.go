package application

import (
	"context"
	"fmt"

	"erpora/modules/inventory/internal/domain"
)

// SeedInitial menambah 3 items awal hanya bila tabel kosong (idempoten).
func (s *Service) SeedInitial(ctx context.Context) error {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	return s.add(ctx, "Item contoh", 3)
}

// SeedDev menambah 20 items contoh bila jumlah items < 20 (idempoten).
func (s *Service) SeedDev(ctx context.Context) error {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return err
	}
	if n >= 20 {
		return nil
	}
	return s.add(ctx, "Item dev", 20)
}

// add membuat count item lalu menyimpannya atomik; kegagalan tidak meninggalkan seed sebagian,
// sehingga pemanggilan ulang hook (count masih 0) melengkapi seed.
func (s *Service) add(ctx context.Context, prefix string, count int) error {
	items := make([]domain.Item, 0, count)
	for i := 1; i <= count; i++ {
		item, err := domain.NewItem(fmt.Sprintf("%s %d", prefix, i))
		if err != nil {
			return err
		}
		items = append(items, item)
	}
	return s.repo.AddMany(ctx, items)
}
