package application

import "context"

// AdjustStockCommand adalah masukan use case AdjustStock (perintah yang mengubah state). Tambahkan field dan validasi.
type AdjustStockCommand struct{}

// AdjustStockHandler mengorkestrasi use case AdjustStock; dependensinya adalah port (Repository), bukan infrastructure.
type AdjustStockHandler struct{ repo Repository }

// NewAdjustStockHandler membuat AdjustStockHandler di atas repo.
func NewAdjustStockHandler(repo Repository) *AdjustStockHandler {
	return &AdjustStockHandler{repo: repo}
}

// Handle menjalankan use case. Validasi masukan dan aturan bisnis berada di domain; handler hanya mengorkestrasi dan
// membungkus error dengan konteks (fmt.Errorf("...: %w", err)).
func (h *AdjustStockHandler) Handle(ctx context.Context, cmd AdjustStockCommand) error {
	_, _, _ = h.repo, ctx, cmd
	return nil
}
