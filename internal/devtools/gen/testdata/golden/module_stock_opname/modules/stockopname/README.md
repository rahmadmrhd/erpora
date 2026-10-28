# Stock Opname (acme.stock-opname)

Modul ERPora yang dibuat `erpora make:module stock-opname` dan diturunkan dari modul referensi `example`.

- Kontrak API: `openapi.yaml` (lint: `erpora contracts:lint stock-opname`). Route, permission, dan klasifikasi respons dicocokkan dengan
  `module.go` oleh test di `internal/app`.
- Tabel: `stock_opname_items` (migrasi `migrations/`, tabel goose `goose_module_stock_opname`).
- Lapisan: `internal/domain` (tanpa I/O), `internal/application` (use case), `internal/infrastructure` (GORM lewat `sdk/data`),
  `internal/presentation/http` (handler, amplop `sdk/httpx`). Antarmodul hanya lewat `contract/`.
- Tambah kode dengan generator: `erpora make:handler`, `make:entity`, `make:usecase`, `make:contract`, `make:command`,
  `make:repository`, `make:migration`. Generator menyisipkan registrasi di atas penanda `erp:generated:*` di `module.go`.
- Tidak ada resource yang menerima ID di kerangka ini. Setiap endpoint ber-ID yang ditambahkan wajib membatasi record ke
  scope pemanggil (record di luar scope menjawab 404) dan punya test IDOR; `make:handler` membuat kerangkanya.
