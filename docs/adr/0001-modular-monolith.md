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

- Root module `erpora` berisi platform (`internal/platform`), composition root (`internal/app`), dan `cmd/erpora`.
- `sdk/` adalah Go module publik (`erpora/sdk`) yang tidak bergantung pada root maupun `modules/*`.
- Setiap `modules/<name>/` adalah Go module sendiri (`erpora/modules/<name>`) dengan tag versi independen.
- `go.work` di root menyatukan semua module untuk pengembangan lokal. Build `GOWORK=off` per module wajib lulus (lihat ADR-002).
- Frontend setiap modul di-co-locate di `modules/<name>/web` (package `@erpora/module-<name>`). Komponen UI generik berada di `packages/ui`, yang dibuat pada M3, dan app shell perakit berada di `web/`. Batas frontend ditegakkan lewat `package.json` (F1–F3).
- Binary platform memuat modul pada build-time (`internal/app/modules.go`), sedangkan enable/disable modul terjadi pada runtime (FR-19).
- **Catatan path module (keputusan 22 Okt 2026, PRD-000 Q24):** path Go `erpora` (tanpa domain) sengaja dipakai untuk saat ini. Path tanpa domain tidak dapat di-`go get` oleh pihak ketiga, jadi harus dipindah ke vanity domain (mis. `erpora.dev/...`) **sebelum SDK dipublikasikan** untuk modul pihak ketiga/marketplace. Perpindahan itu mengubah semua import, `go.mod`, dan `replace`, dan dilakukan sekali dengan rename mekanis seperti yang dilakukan pada Q24.

## Konsekuensi

- Satu binary dan satu database tetap menjadi default. Operasional tetap sederhana.
- Batas modul tidak bisa dilanggar tanpa gagal build atau gagal arch test.
- Setiap modul memiliki `go.mod`, `module.json`, dan versi sendiri. Ada biaya koordinasi versi dan `replace` untuk sibling yang belum dirilis.
- `go.work` dan `go.work.sum` harus di-commit. Toolchain Go harus seragam di semua `go.mod` (diperiksa `task check:gomod`).

## Pemicu evaluasi ulang

- Ada kebutuhan skala atau isolasi (mis. tenant besar, persyaratan residensi data) yang tidak bisa dipenuhi satu binary. Pertimbangkan pemisahan deployment, tetapi tetap pertahankan kontrak modul.
- Jumlah modul atau tim membuat koordinasi `go.work` dan rilis per modul menjadi hambatan utama.
