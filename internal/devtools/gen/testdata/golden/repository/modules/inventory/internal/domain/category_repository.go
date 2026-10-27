package domain

import "context"

// CategoryRepository adalah port persistensi Category; diimplementasi infrastructure. Implementasi membatasi
// query ke scope pemanggil (tenant dari context) dan mengembalikan ErrNotFound untuk record yang tidak ada atau di luar scope.
type CategoryRepository interface {
	// Get mengembalikan satu Category; ErrNotFound bila tidak ada atau di luar scope.
	Get(ctx context.Context, id string) (Category, error)
	// List mengembalikan satu halaman (page mulai 1) beserta total.
	List(ctx context.Context, page, size int) ([]Category, int64, error)
	// Save membuat (ID kosong) atau memperbarui dengan pemeriksaan Version (optimistic concurrency).
	Save(ctx context.Context, e *Category) error
}
