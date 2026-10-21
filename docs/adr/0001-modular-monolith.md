# ADR-001: Modular monolith dengan multi Go module dan go.work

- **Status:** Accepted
- **Sumber:** PRD-000 Q15 (struktur Go module), PRD-000 Q21 (lokasi frontend modul)

## Konteks

ERP ini akan berisi banyak modul bisnis (keuangan, HR, inventori, dan seterusnya). Tim kecil membutuhkan deployment sederhana, satu binary, dan transaksi lintas domain yang mudah. Di sisi lain, batas antarmodul harus tegas agar modul dapat diaktifkan, dinonaktifkan, dan diganti tanpa menyentuh kode platform. Modul juga perlu versi dan rilis sendiri-sendiri.

## Opsi yang dipertimbangkan

1. **Microservices**: setiap modul adalah service dan binary terpisah. Batas kuat, tetapi biaya operasional, transaksi terdistribusi, dan latensi antarservice terlalu besar untuk tahap ini.
2. **Monolit dengan satu Go module**: sederhana, tetapi batas modul hanya berupa konvensi folder. Tidak ada yang mencegah impor lintas modul.
3. **Modular monolith dengan multi Go module dan `go.work`**: satu binary dan satu deployment, tetapi setiap modul adalah Go module sendiri. Batas ditegakkan compiler (`internal/`) dan `go.mod` (`require`).

## Keputusan

Memilih opsi 3:

- Root module `github.com/rahmadmrhd/erp-go` berisi platform (`internal/platform`), composition root (`internal/app`), dan `cmd/erp`.
- `sdk/` adalah Go module publik (`github.com/rahmadmrhd/erp-go/sdk`) yang tidak bergantung pada root maupun `modules/*`.
- Setiap `modules/<name>/` adalah Go module sendiri (`github.com/rahmadmrhd/erp-go/modules/<name>`) dengan tag versi independen.
- `go.work` di root menyatukan semua module untuk pengembangan lokal. Build `GOWORK=off` per module wajib lulus (lihat ADR-002).
- Frontend setiap modul di-co-locate di `modules/<name>/web` (package `@erp/module-<name>`). Komponen UI generik berada di `packages/ui`, yang dibuat pada M3, dan app shell perakit berada di `web/`. Batas frontend ditegakkan lewat `package.json` (F1–F3).
- Binary platform memuat modul pada build-time (`internal/app/modules.go`), sedangkan enable/disable modul terjadi pada runtime (FR-19).

## Konsekuensi

- Satu binary dan satu database tetap menjadi default. Operasional tetap sederhana.
- Batas modul tidak bisa dilanggar tanpa gagal build atau gagal arch test.
- Setiap modul memiliki `go.mod`, `module.json`, dan versi sendiri. Ada biaya koordinasi versi dan `replace` untuk sibling yang belum dirilis.
- `go.work` dan `go.work.sum` harus di-commit. Toolchain Go harus seragam di semua `go.mod` (diperiksa `task check:gomod`).

## Pemicu evaluasi ulang

- Ada kebutuhan skala atau isolasi (mis. tenant besar, persyaratan residensi data) yang tidak bisa dipenuhi satu binary. Pertimbangkan pemisahan deployment, tetapi tetap pertahankan kontrak modul.
- Jumlah modul atau tim membuat koordinasi `go.work` dan rilis per modul menjadi hambatan utama.
