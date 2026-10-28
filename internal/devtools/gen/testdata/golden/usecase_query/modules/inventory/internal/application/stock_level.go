package application

import "context"

// StockLevelQuery adalah masukan use case StockLevel (baca saja, tanpa efek samping). Tambahkan field dan validasi.
type StockLevelQuery struct{}

// StockLevelResult adalah hasil use case StockLevel. Bentuk DTO respons HTTP ditentukan di presentation, bukan di sini.
type StockLevelResult struct{}

// StockLevelHandler mengorkestrasi use case StockLevel; dependensinya adalah port (Repository), bukan infrastructure.
type StockLevelHandler struct{ repo Repository }

// NewStockLevelHandler membuat StockLevelHandler di atas repo.
func NewStockLevelHandler(repo Repository) *StockLevelHandler { return &StockLevelHandler{repo: repo} }

// Handle menjalankan use case dan membungkus error dengan konteks (fmt.Errorf("...: %w", err)).
func (h *StockLevelHandler) Handle(ctx context.Context, q StockLevelQuery) (StockLevelResult, error) {
	_, _, _ = h.repo, ctx, q
	return StockLevelResult{}, nil
}
