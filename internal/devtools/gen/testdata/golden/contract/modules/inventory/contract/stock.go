package contract

import "context"

// StockDTO adalah bentuk data Stock yang dibagikan ke modul lain. Hanya field yang memang perlu dibagikan;
// jangan membocorkan entity domain atau model GORM (tidak mengimpor internal/).
type StockDTO struct {
	ID      string
	Title   string
	Version int64
}

// Stock adalah port publik modul inventory atas Stock untuk modul lain.
//
// Implementasi kontrak WAJIB menegakkan otorisasi sendiri terhadap principal di context (jangan percaya pemanggil), dan
// record di luar scope pemanggil dilaporkan sebagai tidak ditemukan. Parameter kontrak tidak boleh membawa tenantId atau
// userId; tenant diambil dari context. Hak sistem hanya lewat authz.AsSystem(ctx, reason).
type Stock interface {
	// Get mengembalikan satu Stock menurut ID.
	Get(ctx context.Context, id string) (StockDTO, error)
}
