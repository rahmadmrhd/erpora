package inventory

// Blank import kontrak dependensi manifest. Menjaga aturan arsitektur D8 dan `go mod tidy` tetap stabil sampai
// kontrak dependensi benar-benar dipakai; hapus baris yang sudah digantikan impor sungguhan.
import (
	_ "erpora/modules/example/contract"
)
