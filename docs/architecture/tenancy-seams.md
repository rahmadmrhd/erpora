# Seam Tenancy (implementasi M1)

Status: implementasi M1. Target desain database-per-tenant ada di [tenancy-design.md](tenancy-design.md);
dokumen ini hanya mencatat titik yang sudah ada di kode dan apa yang berubah saat PRD Tenancy dikerjakan.

M1 berjalan **single-tenant**: satu database, satu tenant default (`platform_state.default_tenant_id`),
`ERP_TENANCY_MODE=single` (satu-satunya nilai yang diterima `config.Load`).

## Yang diganti saat PRD Tenancy

| Titik | Kode M1 | Perubahan di PRD Tenancy |
| --- | --- | --- |
| Penentuan tenant per request | `tenancy.Resolver` -> `SingleTenantResolver` (selalu tenant default) | Implementasi baru (mis. subdomain/header/token) |
| Akses database tenant | `database.TenantConnector` -> `SingleTenantConnector` (satu `TenantDB`, tenant lain -> `ErrUnknownTenant`) | Connector multi-pool: satu pool per database tenant |
| Migrasi tenant | Target `tenant` goose dijalankan pada satu database yang sama | Dijalankan per database tenant saat provisioning/upgrade |
| Registri tenant | Hanya `platform_state.default_tenant_id`; belum ada tabel `tenants` | Tabel `tenants` di control DB; FK dari tabel control ke `tenants` |
| Mode konfigurasi | `ERP_TENANCY_MODE=single` | `ERP_TENANCY_MODE=multi` diterima dan mengaktifkan resolver/connector baru |

## Yang tidak berubah

- Kode modul: modul hanya memakai `sdk/data.DB(ctx)` dan tidak pernah memegang pool atau koneksi.
- `sdk/data` (`Provider`, `WithProvider`, `DB`, `ErrNoTenantContext`): kontrak tetap; hanya implementasi `Provider` di platform yang berubah.
- Kolom `tenant_id` pada tabel tenant-scoped dan pengambilan tenant dari context (`tenancy.FromContext`).
