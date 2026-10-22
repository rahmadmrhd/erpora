# ADR-003: PostgreSQL 18, GORM, dan goose

- **Status:** Accepted
- **Sumber:** PRD-000 Q3 (akses data), PRD-000 FR-07 (database dan migrasi)

## Konteks

ERP membutuhkan transaksi ACID, locking eksplisit, dan query yang kompleks (laporan, CTE). Tim perlu produktif untuk CRUD dengan filter dinamis, sehingga pilihan akses data harus seimbang antara kecepatan pengembangan dan kontrol. Skema harus dikelola versioned, bukan lewat auto-migrasi.

## Opsi yang dipertimbangkan

**Database**

1. **PostgreSQL**: transaksi dan tipe data kuat, ekosistem matang, dan dapat dipakai untuk database-per-tenant (ADR-004).
2. **Database lain (MySQL, SQL Server)**: tidak memberi keunggulan yang dibutuhkan.

**Akses data**

1. **GORM** (`gorm.io/gorm` + `gorm.io/driver/postgres`, pgx v5): fleksibel dan cepat untuk pengembangan, dengan guardrail (FR-07).
2. **sqlc**: query type-safe, tetapi lambat untuk CRUD dan filter dinamis. Ditolak.
3. **ent**: codegen dan kurva belajar yang besar. Ditolak.

**Migrasi**

1. **goose** (Provider API, migrasi di-embed) dengan tabel versi per target.
2. **AutoMigrate GORM**: tidak versioned dan tidak dapat diaudit. Ditolak untuk kode non-test.

## Keputusan

- Database: **PostgreSQL 18** (`postgres:18-alpine` di `infrastructure/docker/docker-compose.dev.yml`). Versi major dikunci. Naikkan versi hanya lewat ADR baru atau pembaruan ADR ini.
- Akses data: **GORM** dengan enam guardrail wajib (PRD-000 FR-07):
  1. GORM hanya di lapisan infrastructure (D3/D3b). Model GORM terpisah dari entity domain, dan repository melakukan mapping.
  2. `AutoMigrate` dilarang di luar test. Skema dikelola migrasi SQL versioned.
  3. Tipe koneksi terpisah: `database.ControlDB` dan `database.TenantDB` membungkus `*gorm.DB`.
  4. Transaksi eksplisit (`db.Transaction`) dan locking eksplisit (`clause.Locking`). Query kompleks memakai `db.Raw` dengan parameter binding.
  5. Logger GORM diarahkan ke `slog`, mencatat slow query dengan ambang yang dapat dikonfigurasi (`ERP_DB_SLOW_QUERY_MS`).
  6. Preload, bukan lazy load. Relasi lintas modul dilarang.
- Migrasi: **goose** dengan tabel versi per target:

| Target | Sumber | Tabel versi |
| --- | --- | --- |
| control | `migrations/control/` | `goose_control` |
| tenant | `migrations/tenant/` | `goose_tenant` |
| modul | `modules/<x>/migrations/` | `goose_module_<slug_snake>` |

- Migrasi tidak pernah berjalan otomatis pada HTTP request atau saat `serve`. Migrasi hanya berjalan lewat `erp migrate` dan operasi lifecycle modul.

## Konsekuensi

- Produktivitas CRUD tinggi, dengan batas yang dijaga oleh guardrail dan arch test (D3, D3b).
- Setiap perubahan skema membutuhkan file migrasi goose baru. Tidak ada perubahan skema diam-diam.
- Ada dua tabel versi aktif per database (`goose_control` dan `goose_tenant`) dan satu per modul. Operator harus memahami ini saat troubleshooting.
- Pembaruan PostgreSQL major membutuhkan rencana migrasi data dan dicatat di ADR ini.

## Pemicu evaluasi ulang

- Query utama modul membutuhkan fitur yang sulit dicapai GORM, dan `db.Raw` menjadi mayoritas kode data.
- Persyaratan kinerja tidak terpenuhi oleh GORM setelah profiling.
- PostgreSQL major baru dirilis dan dibutuhkan fitur tertentu, atau versi lama mencapai akhir dukungan.
