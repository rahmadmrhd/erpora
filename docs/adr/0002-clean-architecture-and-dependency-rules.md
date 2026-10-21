# ADR-002: Clean Architecture selektif dan aturan dependensi (D1–D11, F1–F3)

- **Status:** Accepted
- **Sumber:** PRD-000 §4.1 dan §4.2 (aturan D dan F), Blueprint §5.1 (dirangkum dalam PRD-000)

## Konteks

Modul ERP harus bisa diubah, diuji, dan diaktifkan tanpa merusak modul lain. Tanpa aturan dependensi yang tegas, kode akan berangsur saling bergantung secara tersembunyi, dan lifecycle modul (FR-19) tidak bisa dipercaya. Aturan ini harus diuji otomatis, bukan hanya didokumentasikan.

## Opsi yang dipertimbangkan

1. **Layering bebas, tanpa aturan tertulis**: cepat di awal, tetapi tidak bisa diverifikasi dan akan terkikis.
2. **Clean Architecture penuh di setiap modul**: konsisten, tetapi berlebihan untuk modul CRUD sederhana.
3. **Clean Architecture selektif dengan aturan dependensi yang diuji otomatis**: lapisan `domain`, `application`, `infrastructure`, `presentation` tetap ada, tetapi modul CRUD sederhana boleh meringkas `application`/`domain`. Batas yang tidak boleh dilanggar adalah aturan `internal/` dan aturan D/F.

## Keputusan

Memilih opsi 3. Aturan berikut wajib berlaku dan diuji di CI:

| # | Aturan | Penegak |
| --- | --- | --- |
| D1 | Modul lain hanya boleh mengimpor `modules/<x>/contract`; `modules/<x>/internal/**` tertutup | Compiler (`internal/`) |
| D2 | `internal/platform/**` tidak boleh mengimpor `modules/**` | Arch test |
| D3 | `*/internal/domain` tidak boleh mengimpor `net/http`, `gorm.io/**`, `pgx`, `database/sql`, `infrastructure`, `presentation` | Arch test |
| D3b | `gorm.io/**` hanya boleh diimpor oleh `modules/*/internal/infrastructure/**`, `internal/platform/database`, `internal/platform/modules/store`, dan `sdk/data` | Arch test |
| D4 | `*/internal/application` tidak boleh mengimpor `presentation` atau `infrastructure` | Arch test |
| D5 | `sdk` tidak boleh mengimpor root module maupun `modules/*` | Compiler (`sdk/go.mod`) |
| D6 | `contract/` modul tidak boleh mengimpor `internal/` modul lain | Compiler |
| D7 | Hanya `internal/app` yang boleh mengimpor seluruh modul (composition root) | Arch test |
| D8 | Modul hanya boleh mengimpor modul yang tercantum di `dependencies` manifest-nya | `GOWORK=off go build ./...` per modul + arch test (`require` di `go.mod` == `dependencies` di `module.json`) |
| D9 | `modules/**` tidak boleh mengimpor root module; akses platform hanya lewat `sdk` | Compiler + arch test |
| D10 | `modules/**` dan `sdk` tidak boleh mengimpor library router pihak ketiga | Arch test |
| D11 | Dependensi antar-Go-module tidak boleh siklik | Compiler/`go mod` + arch test |

Aturan frontend (dicek lewat dependency-cruiser dan `package.json`):

| # | Aturan |
| --- | --- |
| F1 | `@erp/ui`, `@erp/platform-web`, `@erp/module-sdk` tidak boleh bergantung pada `@erp/module-*` |
| F2 | Frontend modul hanya boleh mengimpor paket platform/UI bersama dan `@erp/module-<x>` yang ada di `manifest.dependencies`; antarmodul hanya lewat `index.ts` |
| F3 | Hanya app shell `web/` yang boleh bergantung pada semua `@erp/module-*` dan merakitnya |

Penegak di repo:

- `internal/archcheck` berisi logika pemeriksaan. `tests/architecture` menjalankannya (`task arch`).
- Build `GOWORK=off` per module (`task check:gowork-off`).
- Pemeriksaan versi `go` dan `toolchain` seragam (`task check:gomod`, rule GOVERSION).
- `package.json` (Bun workspaces) dan `.dependency-cruiser.cjs` untuk aturan F, dijalankan lewat `task web:check`.

## Konsekuensi

- Pelanggaran batas terdeteksi di CI dengan pesan yang menyebut nomor aturan.
- Kode modul CRUD sederhana boleh memakai model GORM langsung di `application` hanya jika ADR modulnya mencatat pengecualian (PRD-000 FR-07 guardrail 1).
- Setiap aturan baru wajib masuk tabel ini dan memiliki penegak otomatis. Aturan tanpa penegak tidak diterima.
- Biaya: arch test harus dipelihara seiring perubahan struktur repo.

## Pemicu evaluasi ulang

- Kebutuhan modul yang secara sah membutuhkan impor lintas `internal/` (perlu mekanisme khusus).
- Pembukaan modul ekstensi pihak ketiga (ADR-008) yang mengubah model trust dan batas dependensi.
