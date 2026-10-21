# Desain Multi-Tenancy (Target)

| Atribut | Nilai |
| --- | --- |
| Status | **Draft desain** (diekstrak dari dokumen awal `Blueprint.md` §1, §3, §9, §15). Akan diformalkan di **PRD Tenancy** |
| Kondisi saat ini | Mode single-tenant dengan seam (PRD-000 FR-21, `docs/architecture/tenancy-seams.md`) |

## 1. Keputusan

**Database per tenant.** Control plane menyimpan metadata platform; setiap tenant memiliki database bisnis sendiri, dengan kredensial dan proses provisioning yang terisolasi.

Alasan: isolasi data antarperusahaan, backup/restore per perusahaan, dan fleksibilitas self-hosted. Konsekuensinya memengaruhi migrasi, koneksi database, background job, pelaporan lintas perusahaan, dan biaya operasional.

**Batas penting:** database terpisah mengisolasi data, tetapi **tidak otomatis menjamin keamanan tenant**. Pemilihan tenant, kredensial database, cache key, file storage, queue, dan proses background tetap harus memakai konteks tenant yang benar.

Jangan memakai schema-per-tenant PostgreSQL sebagai pengganti database-per-tenant tanpa alasan khusus: database dan schema adalah batas yang berbeda, dan satu koneksi terhubung ke satu database.

## 2. Tiga area sistem

1. **Control Plane**: organisasi, tenant, database, lisensi, instalasi, katalog modul.
2. **ERP Runtime**: transaksi bisnis pada konteks tenant tertentu.
3. **Extension Platform**: distribusi dan pengelolaan modul internal & pihak ketiga.

Ini batas logis, bukan keharusan service terpisah; di fase awal berada dalam satu binary.

## 3. Control DB vs Tenant DB

**Control DB** (metadata plane — bukan sumber kebenaran saldo stok, invoice, atau jurnal): `tenants`, `tenant_databases`, `users`, `tenant_memberships`, `module_catalog`, `tenant_modules`, `module_dependencies`, `provisioning_operations`, `tenant_releases`.

**Tenant DB** (satu per tenant): data bisnis dan konfigurasi tenant — perusahaan, role & permission, master data, dokumen transaksi, jurnal, audit, outbox.

`users` berada di Control DB agar satu akun platform dapat bergabung ke beberapa tenant; peran bisnis ditetapkan per tenant. Pada self-hosted satu perusahaan, model yang sama tetap dipakai dengan provisioning yang disederhanakan.

### ERD konseptual Control DB

| Tabel | Kolom utama |
| --- | --- |
| `tenants` | `id` PK, `slug` UK, `name`, `deployment_mode`, `status`, `created_at` |
| `tenant_databases` | `id` PK, `tenant_id` FK, `connection_key`, `engine`, `database_name`, `secret_reference`, `status` |
| `users` | `id` PK, `email` UK, `password_hash`, `status` |
| `tenant_memberships` | `id` PK, `tenant_id` FK, `user_id` FK, `status`, `platform_role` |
| `module_catalog` | `id` PK, `module_key` UK, `current_version`, `publisher_id`, `trust_status` |
| `tenant_modules` | `id` PK, `tenant_id` FK, `module_id` FK, `installed_version`, `status`, `settings` |
| `module_dependencies` | `id` PK, `module_id` FK, `dependency_key`, `version_constraint`, `optional` |
| `provisioning_operations` | `id` PK, `tenant_id` FK, `operation`, `status`, `progress` |
| `tenant_releases` | `id` PK, `tenant_id` FK, `core_version`, `schema_version`, `status` |

Catatan desain:
- `tenant_databases.secret_reference` menunjuk ke secret manager atau mekanisme secret terenkripsi — **tidak pernah** menyimpan password database plaintext atau kredensial sebagai JSON biasa.
- `tenant_modules` unique `(tenant_id, module_id)`.
- `module_catalog` idealnya menyimpan identitas publisher, versi, checksum artifact, signature, dan hasil pemeriksaan keamanan (tabel terkait).
- `tenant_releases` melacak versi core dan status migrasi setiap tenant.
- `provisioning_operations` diperlukan karena pembuatan database, migrasi, dan pemasangan modul berjalan lama dan dapat gagal di tengah proses.

## 4. Alur tenant resolver

```
Request masuk
  → resolve tenant dari hostname / route mapping TEPERCAYA
  → autentikasi user
  → verifikasi membership & status tenant
  → muat referensi tenant database
  → ambil kredensial database secara aman
  → inisialisasi koneksi tenant
  → muat konfigurasi modul tenant
  → jalankan use case yang terotorisasi
  → bersihkan state berskala tenant
```

SaaS: domain seperti `company-a.example.com` dipetakan ke tenant terdaftar. Self-hosted: konfigurasi instalasi menentukan tenant database lokal.

### Pengamanan wajib

1. Tenant tidak boleh menentukan host, nama database, atau kredensial lewat request (URL, header tak tervalidasi, input pengguna).
2. Koneksi tenant tidak disimpan sebagai global state yang bertahan tanpa batas.
3. Bersihkan/pulihkan koneksi setelah request, terutama pada persistent worker.
4. Setiap job antrean membawa `tenant_id` tervalidasi, bukan hanya model atau nama koneksi.
5. Cache, file storage, log context, dan idempotency key dibatasi per tenant.
6. Scheduled job memilih tenant secara eksplisit dan terisolasi.
7. Database account memakai hak akses minimum.

## 5. Tenant context untuk background job

Konteks tenant HTTP **tidak** otomatis tersedia di worker. Kontrak job (Go):

```go
type RebuildInventoryProjection struct {
    TenantID    tenancy.TenantID // tervalidasi saat job dibuat
    WarehouseID string
}

func (j RebuildInventoryProjection) Handle(ctx context.Context, tc TenantContextInitializer) error {
    ctx, release, err := tc.Initialize(ctx, j.TenantID) // verifikasi tenant aktif + koneksi
    if err != nil { return err }
    defer release() // tenant context lama tidak boleh bocor ke job berikutnya
    return rebuildProjection(ctx, j.WarehouseID)
}
```

Worker juga harus memeriksa status modul dan tenant saat mengeksekusi job (job yang sudah di antrean sebelum modul/tenant dinonaktifkan).

## 6. Test isolasi tenant (adversarial)

Bukan hanya "tenant A memakai DB A". Uji:

1. User tenant A mengubah ID di URL untuk mengakses dokumen tenant B.
2. Job tenant A dimanipulasi agar memakai referensi tenant B.
3. Cache key tenant A dipakai untuk membaca cache tenant B.
4. Worker memproses beberapa tenant berurutan (kebocoran context).
5. Tenant dinonaktifkan ketika request/job masih berjalan.

## 7. Risiko & kebutuhan saat skala tumbuh

Jumlah koneksi, migrasi, backup, monitoring, dan upgrade bertambah seiring jumlah tenant. Rancang sejak awal:

- Penempatan database dan kapasitas per cluster.
- Connection pooling dan batas koneksi per worker.
- Scheduler migrasi tenant dengan concurrency limit — **jangan** migrasi semua tenant paralel tanpa batas; upgrade harus bisa di-resume, di-throttle, dan dipantau.
- Backup & restore per tenant.
- Metrik berbasis tenant (latensi query, backlog antrean).
- Prosedur memindahkan tenant antar server database.
- Jalur khusus untuk tenant besar yang butuh resource terisolasi.
