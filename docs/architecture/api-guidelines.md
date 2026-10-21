# Pedoman API (HTTP/JSON)

| Atribut | Nilai |
| --- | --- |
| Status | **Accepted v1** (21 Oktober 2026): berlaku untuk semua endpoint platform & modul |
| Dirujuk oleh | PRD-000 FR-04, FR-05; PRD-001 FR-110–FR-117 |
| Ruang lingkup | API yang dipanggil browser/PWA, aplikasi mobile/desktop, dan sistem integrasi |

Pedoman ini mengatur **bentuk kontrak** API supaya setiap modul terlihat seperti satu API yang sama. Kata **WAJIB**, **JANGAN**, dan **SEBAIKNYA** dipakai dengan arti normatif.

---

## 1. Prinsip

1. **REST untuk data, endpoint aksi untuk proses bisnis.** CRUD memakai metode HTTP standar; operasi seperti `submit`, `cancel`, `approve` memakai `POST .../{id}/{aksi}`.
2. **Satu endpoint = satu permission.** Setiap endpoint dipetakan ke tepat satu `<resource>.<action>` (PRD-001 FR-110). Bila sebuah endpoint terasa butuh dua permission, kemungkinan besar ia harus dipecah.
3. **Kontrak dulu, kode kemudian.** Endpoint baru ditulis di OpenAPI sebelum diimplementasikan; tipe TS dibangkitkan dari situ (`erp contracts:generate`).
4. **Aman secara default.** Data di luar scope tidak terlihat (404), field terlarang tidak muncul, dan operasi tulis tahan retry (idempotency).
5. **Perubahan bersifat aditif.** Klien lama tidak boleh rusak karena rilis baru (§14).

## 2. Struktur URL

```
/api/{slug-modul}/{koleksi}[/{id}[/{sub-koleksi}|/{aksi}]]
```

| Contoh | Keterangan |
| --- | --- |
| `/api/inventory/items` | Koleksi |
| `/api/inventory/items/0193a6c2-…` | Satu record |
| `/api/inventory/items/0193a6c2-…/prices` | Sub-koleksi (maksimal **satu** tingkat) |
| `/api/inventory/adjustments/0193a6c2-…/submit` | Aksi pada record |
| `/api/inventory/items/import` | Aksi pada koleksi |
| `/api/core/users`, `/api/platform/modules` | Modul `core` dan platform mengikuti aturan yang sama |

Aturan:
- `{slug-modul}` dipasang otomatis oleh platform (PRD-000 FR-05); modul hanya menulis path relatifnya.
- Koleksi: **kata benda jamak, kebab-case**, bahasa Inggris (`purchase-orders`, bukan `purchaseOrder` atau `pesanan-pembelian`).
- Aksi: **kata kerja, kebab-case** (`submit`, `adjust-price`). Nama aksi di URL = key aksi di manifest dengan `_` diganti `-` (`adjust_price` → `adjust-price`).
- **JANGAN** memakai kata kerja untuk CRUD (`/items/create`, `/items/{id}/delete`).
- **JANGAN** nesting lebih dari satu tingkat. `/orders/{id}/lines/{lineId}/taxes` → jadikan `/order-line-taxes?lineId=…` atau perlakukan sebagai bagian dari dokumen induk.
- Trailing slash tidak dipakai; `/items/` → 404.

## 3. Metode HTTP, status, dan permission

| Operasi | Metode & path | Sukses | Permission |
| --- | --- | --- | --- |
| Daftar | `GET /items` | `200` | `inventory.item.read` |
| Detail | `GET /items/{id}` | `200` | `inventory.item.read` |
| Buat | `POST /items` | `201` + header `Location` | `inventory.item.create` |
| Ubah sebagian | `PATCH /items/{id}` | `200` | `inventory.item.update` |
| Hapus | `DELETE /items/{id}` | `204` | `inventory.item.delete` |
| Aksi record | `POST /adjustments/{id}/submit` | `200` (record terbaru) | `inventory.adjustment.submit` |
| Aksi koleksi / massal | `POST /items/bulk-delete` | `200` (hasil per item, §9) | `inventory.item.delete` |
| Proses panjang | `POST /items/export` | `202` + job (§10) | `inventory.item.export` |

- **`PUT` tidak dipakai** untuk resource bisnis (ganti penuh berisiko menghapus field yang tidak dikirim/tidak terlihat oleh user). Pengecualian: dokumen konfigurasi utuh (mis. settings satu modul), dan harus dinyatakan di OpenAPI.
- `PATCH` memakai semantik **JSON Merge Patch** (RFC 7396): hanya field yang dikirim yang diubah; `null` berarti mengosongkan field.
- `DELETE` pada dokumen yang sudah `submitted` **ditolak** `409 invalid_state`; gunakan aksi `cancel`.

## 4. Konvensi JSON

### 4.1. Penamaan & tipe

| Hal | Aturan | Contoh |
| --- | --- | --- |
| Nama field | **camelCase** | `createdAt`, `warehouseId` |
| ID | UUID (v7, terurut waktu) sebagai string | `"0193a6c2-7c1e-7d4a-…"` |
| Nomor dokumen | Field terpisah `number`, bukan ID | `"number": "ADJ-2026-00042"` |
| Waktu | ISO 8601 **UTC** dengan `Z` | `"2026-10-21T07:15:00Z"` |
| Tanggal saja | `YYYY-MM-DD` | `"postingDate": "2026-10-21"` |
| **Uang & kuantitas** | **String desimal**, bukan number | `"total": "1250000.00"`, `"quantity": "12.500"` |
| Mata uang | ISO 4217, field terpisah | `"currency": "IDR"` |
| Enum | snake_case huruf kecil | `"status": "pending_approval"` |
| Referensi ke record lain | `<nama>Id`, plus objek ringkas opsional lewat `expand` | `"supplierId": "…"` |

Uang dan kuantitas **WAJIB** string: number JSON di JavaScript adalah float 64-bit, sehingga `0.1 + 0.2 ≠ 0.3` dan nilai besar kehilangan presisi. Server memakai tipe desimal (mis. `shopspring/decimal`), dan klien memakai library desimal.

Zona waktu: server selalu mengirim UTC. Konversi ke zona waktu user (PRD-001 FR-120) dilakukan di klien.

### 4.2. Amplop respons

**Satu record:**
```json
{
  "data": {
    "id": "0193a6c2-…",
    "number": "ADJ-2026-00042",
    "status": "draft",
    "warehouseId": "…",
    "version": 3,
    "createdAt": "2026-10-21T07:15:00Z",
    "updatedAt": "2026-10-21T07:20:11Z"
  }
}
```

**Daftar** (lihat §6 untuk `page`):
```json
{
  "data": [ { "id": "…" }, { "id": "…" } ],
  "page": { "page": 1, "pageSize": 25, "total": 132, "hasMore": true }
}
```

- Respons sukses **selalu** dibungkus `data`, sehingga metadata (`page`, `meta`) bisa ditambahkan tanpa breaking change.
- Respons error **selalu** berbentuk `{"error": {...}}` (§8). Sebuah respons tidak pernah berisi `data` dan `error` sekaligus.

### 4.3. Field wajib pada setiap resource

| Field | Keterangan |
| --- | --- |
| `id` | UUID |
| `version` | Integer, naik setiap perubahan (optimistic concurrency, §7) |
| `createdAt`, `updatedAt` | UTC |
| `createdBy`, `updatedBy` | ID user (dipakai juga oleh scope `owner`, PRD-001 FR-111) |
| `status` | Wajib untuk resource berstatus dokumen (`draft`, `submitted`, `cancelled`, …) |

### 4.4. Field yang tidak boleh dilihat

Field dari field group berakses `none` **dihilangkan dari JSON** (tidak dikirim sama sekali), bukan dikirim sebagai `null` (PRD-001 FR-112). Klien **WAJIB** memperlakukan field yang tidak ada sebagai "tidak tersedia", bukan "kosong".

## 5. Membaca: filter, sort, pencarian, field

### 5.1. Filter

```
GET /api/inventory/items?status=active&categoryId[in]=a,b&createdAt[gte]=2026-01-01T00:00:00Z
```

| Operator | Arti | Contoh |
| --- | --- | --- |
| *(tanpa)* / `[eq]` | Sama dengan | `status=active` |
| `[ne]` | Tidak sama | `status[ne]=cancelled` |
| `[gt]` `[gte]` `[lt]` `[lte]` | Perbandingan | `total[gte]=1000000` |
| `[in]` | Salah satu (dipisah koma, maks. 100 nilai) | `warehouseId[in]=…,…` |
| `[contains]` | Mengandung teks (case-insensitive) | `name[contains]=baut` |
| `[isnull]` | `true`/`false` | `supplierId[isnull]=true` |

- Hanya field yang dinyatakan **filterable** di OpenAPI yang boleh difilter; field lain → `400 invalid_filter`.
- Filter pada field yang tidak boleh dilihat user → `403 field_forbidden` (PRD-001 FR-112).
- Pencarian teks bebas memakai `q=` (field yang dicari ditentukan endpoint dan didokumentasikan).
- Scope data (owner, gudang, unit organisasi) diterapkan server **sebelum** filter klien; klien tidak perlu dan tidak bisa mematikannya.

### 5.2. Sort

`sort=-createdAt,name` → tanda `-` berarti menurun. Hanya field **sortable**; default sort tiap endpoint didokumentasikan dan selalu deterministik (diakhiri `id`).

### 5.3. Field parsial & ekspansi

- `fields=id,number,status` mengembalikan subset field (opsional per endpoint; berguna untuk dropdown/relation select).
- `expand=supplier` menambahkan objek ringkas `{"supplier": {"id", "name"}}`. Hanya relasi yang didokumentasikan; maksimal satu tingkat. Ekspansi tunduk pada permission modul pemilik data: tanpa `purchasing.supplier.read`, objek ekspansi tidak disertakan.

## 6. Pagination

| Jenis | Parameter | Dipakai untuk |
| --- | --- | --- |
| **Halaman** (default) | `page` (mulai 1), `pageSize` (default 25, maks. 200) | Data grid UI, daftar biasa |
| **Cursor** | `cursor`, `limit` (maks. 1000) | Sinkronisasi offline, audit log, integrasi yang menarik data besar |

Respons halaman: `"page": {"page", "pageSize", "total", "hasMore"}`. `total` dapat dimatikan endpoint dengan data sangat besar (`"total": null`) dan didokumentasikan.

Respons cursor: `"page": {"nextCursor": "…", "hasMore": true}`. Cursor bersifat opaque; klien **JANGAN** membentuk atau mengurai isinya.

## 7. Menulis: validasi, konkurensi, idempotency

### 7.1. Optimistic concurrency

- Setiap respons record menyertakan `version` dan header `ETag: "3"`.
- `PATCH`, `DELETE`, dan aksi record **SEBAIKNYA** mengirim `If-Match: "3"`. Bila versi di server sudah berbeda → `412 version_mismatch` beserta record terbaru di `error.details`.
- Endpoint yang dipakai sync offline **WAJIB** mensyaratkan `If-Match` (setara `baseVersion` di arsitektur.md §5).

### 7.2. Idempotency

- Header `Idempotency-Key` (UUID dari klien) **WAJIB** untuk `POST` yang dikirim oleh sync engine/outbox dan **SEBAIKNYA** dipakai semua integrasi.
- Server menyimpan kunci per principal selama 24 jam:
  - Kunci sama + body sama → mengembalikan **respons pertama** apa adanya (tidak memproses ulang).
  - Kunci sama + body berbeda → `422 idempotency_key_reused`.
  - Permintaan pertama masih diproses → `409 request_in_progress`.
- `operationId` di outbox arsitektur.md §5 dikirim sebagai `Idempotency-Key`.

### 7.3. Validasi

- Validasi struktur (tipe, wajib, format) → `422 validation_failed` dengan detail per field.
- Pelanggaran aturan bisnis (stok tidak cukup, periode ditutup) → `422 business_rule_violated` dengan `code` spesifik di `details`.
- Transisi status yang tidak sah (submit dokumen yang sudah submitted) → `409 invalid_state`.
- Mengubah field tanpa akses `write` → `403 field_forbidden` (tidak diabaikan diam-diam).

## 8. Error

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Data tidak valid.",
    "details": [
      { "field": "quantity", "code": "must_be_positive", "message": "Kuantitas harus lebih dari 0." },
      { "field": "lines[2].itemId", "code": "not_found", "message": "Barang tidak ditemukan." }
    ],
    "requestId": "01J…",
    "traceId": "4bf92f3577b34da6a3ce929d0e0e4736"
  }
}
```

- `code` **stabil** dan menjadi kontrak (klien boleh bergantung padanya). `message` untuk manusia, mengikuti `Accept-Language` / preferensi user, dan **JANGAN** di-parse oleh klien.
- `field` memakai path JSON (`lines[2].itemId`).
- Error 5xx tidak pernah memuat detail internal (stack trace, SQL); detail ada di log dengan `requestId`/`traceId` yang sama.

| Status | `code` | Kapan |
| --- | --- | --- |
| 400 | `bad_request`, `invalid_filter`, `invalid_sort` | Parameter/format request salah |
| 401 | `unauthenticated`, `token_expired` | Token tidak ada/tidak valid/kedaluwarsa |
| 403 | `permission_denied` | Tidak punya permission aksi |
| 403 | `field_forbidden` | Membaca/memfilter/mengubah field terlarang |
| 403 | `mfa_required` | Endpoint mensyaratkan MFA (PRD-001 FR-107) |
| 403 | `module_disabled` | Modul `installed`/`disabled` (PRD-000 FR-19.7) |
| 404 | `not_found` | Record tidak ada **atau di luar scope user** |
| 404 | `module_not_found` | Modul belum terpasang |
| 409 | `invalid_state`, `request_in_progress`, `conflict` | Status dokumen / proses paralel / bentrok unik |
| 412 | `version_mismatch` | `If-Match` tidak cocok |
| 422 | `validation_failed`, `business_rule_violated`, `idempotency_key_reused` | Data ditolak |
| 429 | `rate_limited` | Dengan header `Retry-After` |
| 500 | `internal_error` | Kesalahan server |
| 503 | `module_unavailable` | Modul `upgrade_pending`/`failed` |

**Record di luar scope → 404, bukan 403.** Membalas 403 berarti mengonfirmasi bahwa record itu ada, misalnya bahwa dokumen bernomor tertentu ada di cabang lain. Pengecualian: record yang **terlihat** oleh user tetapi aksinya tidak diizinkan → 403 `permission_denied`.

## 9. Operasi massal

```http
POST /api/inventory/items/bulk-delete
{ "ids": ["…", "…", "…"] }
```
```json
{
  "data": {
    "results": [
      { "id": "…", "ok": true },
      { "id": "…", "ok": false, "error": { "code": "invalid_state", "message": "…" } }
    ],
    "succeeded": 1,
    "failed": 1
  }
}
```

- Maksimal 500 ID per request; lebih dari itu → gunakan proses panjang (§10).
- Status HTTP `200` selama request valid, walaupun sebagian item gagal; hasil per item yang menentukan.
- Permission & scope diperiksa **per item**.
- Default: setiap item diproses dalam transaksinya sendiri. Endpoint yang butuh "semua atau tidak sama sekali" mendokumentasikannya dan memakai `?atomic=true`.

## 10. Proses panjang (import, export, print massal)

```http
POST /api/inventory/items/export     { "format": "xlsx", "filter": { … } }
→ 202 Accepted
  Location: /api/platform/jobs/0193…
  { "data": { "jobId": "0193…", "status": "queued" } }

GET /api/platform/jobs/0193…
→ { "data": { "status": "succeeded", "progress": 100, "resultUrl": "/api/platform/files/…" } }
```

- Status job: `queued` → `running` → `succeeded` | `failed` | `cancelled`.
- Job membawa principal & tenant pemicunya; permission diperiksa saat dibuat **dan** saat dijalankan.
- Hasil (file) hanya bisa diunduh oleh pemicunya atau user dengan permission yang sama, dan kedaluwarsa (default 24 jam).
- Print satu dokumen yang cepat boleh sinkron: `GET /api/sales/invoices/{id}/print?format=pdf` → `application/pdf` (permission `sales.invoice.print`).

> Infrastruktur job (antrean, worker) disediakan PRD Events/Jobs. Sebelum itu, endpoint proses panjang belum boleh dibuat.

## 11. Dokumen ERP (resource berstatus)

Resource dokumen (pesanan, penyesuaian stok, jurnal) mengikuti siklus standar yang selaras dengan aksi PRD-001:

```
draft ──submit──▶ submitted ──cancel──▶ cancelled ──amend──▶ (draft baru, amendedFromId)
  │                    │
  └─ PATCH/DELETE      └─ approve (bila modul memakai approval)
```

- `PATCH` hanya diizinkan pada `draft`; dokumen `submitted` bersifat tidak dapat diubah (`409 invalid_state`).
- `amend` membuat dokumen **baru** dengan `amendedFromId`, tidak mengubah dokumen lama (jejak audit tetap utuh).
- Aksi mengembalikan dokumen dalam status terbarunya.
- Dokumen yang memiliki baris (`lines`) dikirim dan diubah **sebagai satu kesatuan** bersama induknya, bukan lewat endpoint baris terpisah, agar validasi dan total selalu konsisten.

## 12. Header

| Header | Arah | Keterangan |
| --- | --- | --- |
| `Authorization: Bearer …` | Request | JWT / PAT / token client (PRD-001) |
| `Accept-Language` | Request | Bahasa `error.message`; default preferensi user |
| `Idempotency-Key` | Request | §7.2 |
| `If-Match` | Request | §7.1 |
| `traceparent` | Request | Propagasi trace OTel (PRD-000 FR-17) |
| `X-Request-Id` | Respons | Selalu ada (PRD-000 NFR-07) |
| `ETag` | Respons | Versi record |
| `Location` | Respons | Pada `201`/`202` |
| `Retry-After` | Respons | Pada `429`/`503` |
| `Deprecation`, `Sunset` | Respons | Endpoint yang akan dihapus (§14) |

## 13. Dokumentasi kontrak (OpenAPI)

- **Setiap modul memiliki kontraknya sendiri** di `modules/<x>/openapi.yaml`. Kontrak platform berada di `contracts/openapi/platform.yaml`. Tidak ada file OpenAPI yang diedit bersama oleh beberapa modul (PRD-000 FR-23).
- Rujukan ke kontrak lain memakai URI `erp:`: `erp:common#/…` selalu boleh; `erp:<modul>#/…` hanya ke modul yang ada di `dependencies` manifest.
- Kepatuhan terhadap pedoman ini diperiksa otomatis oleh linter (`erp contracts:lint`, ruleset `contracts/spectral.yaml`) di CI.
- Setiap endpoint **WAJIB** ada di kontrak modulnya sebelum di-merge, lengkap dengan: permission (`x-erp-permission: inventory.item.update`), field filterable/sortable (`x-erp-filterable`, `x-erp-sortable`), field group setiap field (`x-erp-field-group`), contoh request/respons, dan semua kode error yang mungkin.
- Skema bersama (amplop `data`, `error`, `page`, tipe desimal, UUID, timestamp) didefinisikan sekali di `contracts/openapi/common.yaml` dan dirujuk lewat `$ref: 'erp:common#/components/schemas/…'`.
- Route yang didaftarkan lewat `Registrar.Handle` dan operasi di kontrak harus cocok satu-satu (method, path, permission); ketidakcocokan menggagalkan CI.
- Test kontrak memastikan respons nyata sesuai skema OpenAPI-nya.

## 14. Evolusi & versioning

**Prinsip: API tidak pernah merusak klien yang sudah ada.** Path tidak memakai nomor versi (`/api/inventory/...`, bukan `/api/v1/...`), sehingga satu-satunya cara berevolusi adalah **menambah, bukan mengubah**.

### 14.1. Perubahan yang diizinkan

- Menambah endpoint, aksi, atau resource baru.
- Menambah field pada respons.
- Menambah parameter query/field request yang **opsional** (perilaku tanpa parameter itu tetap sama).
- Menambah kode error baru, atau nilai enum baru pada field yang didokumentasikan `x-erp-extensible-enum: true`.
- Melonggarkan validasi (mis. panjang maksimal dinaikkan).

### 14.2. Perubahan yang dilarang

- Menghapus atau mengganti nama endpoint, field, parameter, atau nilai enum.
- Mengubah tipe atau format field (mis. string → objek, desimal 2 digit → 4 digit tanpa pemberitahuan).
- Menjadikan field/parameter opsional menjadi wajib, atau mengetatkan validasi.
- Mengubah arti field, status code, atau perilaku endpoint yang sudah ada.

### 14.3. Pola bila struktur memang harus berubah: *expand → migrate → (opsional) contract*

1. **Expand**: tambahkan bentuk baru **berdampingan** dengan yang lama. Contoh: `address` (string) dipertahankan, ditambah `addressDetail` (objek). Server mengisi keduanya dan menerima keduanya saat menulis.
2. **Migrate**: frontend milik sendiri pindah ke bentuk baru. Bentuk lama ditandai `deprecated: true` di kontrak, dan respons endpoint terkait mengirim header `Deprecation`. Pemakaian bentuk lama dihitung lewat metrik OTel `erp.api.deprecated_usage` (per operasi, per klien).
3. **Contract** (opsional): bentuk lama **hanya boleh dihapus** bila (a) metrik menunjukkan tidak ada pemakaian selama minimal 6 bulan, (b) modul merilis versi **MAJOR**, dan (c) penghapusan diumumkan di changelog dengan header `Sunset` sejak minimal satu rilis sebelumnya. Bila salah satu tidak terpenuhi, bentuk lama dipertahankan.

Endpoint yang perilakunya harus berubah secara mendasar dibuat sebagai **endpoint baru** dengan nama berbeda (mis. aksi `post-v2` **tidak** boleh; gunakan nama yang mendeskripsikan perbedaannya, mis. `post-with-allocation`), dan endpoint lama tetap berfungsi.

### 14.4. Penegakan & kewajiban klien

- CI membandingkan kontrak setiap modul dengan kontrak di **tag rilis terakhir** modul itu (alat diff OpenAPI, mis. `oasdiff`). Perubahan yang terdeteksi breaking **menggagalkan CI**, kecuali rilis MAJOR yang memenuhi syarat 14.3 langkah 3 dan disertai label persetujuan eksplisit di PR.
- Klien **WAJIB** mengabaikan field yang tidak dikenalnya (tolerant reader) dan **WAJIB** menangani nilai enum yang tidak dikenal pada field `x-erp-extensible-enum`.

## 15. Checklist endpoint baru

- [ ] Path mengikuti §2 (jamak, kebab-case, maks. satu tingkat nesting, aksi berupa kata kerja).
- [ ] Tepat satu permission `<resource>.<action>` dan terdaftar di manifest.
- [ ] Didefinisikan di `modules/<x>/openapi.yaml` dengan `x-erp-permission`, filterable/sortable, field group, contoh, dan semua error; `erp contracts:lint` lolos.
- [ ] `erp contracts:generate` dijalankan dan hasilnya ikut di-commit.
- [ ] Query daftar memakai `authz.Filter`; record tunggal memakai `authz.Check`; respons memakai `authz.MaskFields`; update memakai `authz.CheckWrite` (PRD-001 FR-115).
- [ ] Record di luar scope menghasilkan 404.
- [ ] Uang/kuantitas berupa string desimal; waktu UTC ISO 8601.
- [ ] `version` + `ETag` dikembalikan; `If-Match` didukung untuk `PATCH`/`DELETE`/aksi.
- [ ] `Idempotency-Key` didukung untuk `POST`.
- [ ] Transisi status tidak sah → `409 invalid_state`.
- [ ] Test kontrak (respons sesuai skema) dan test otorisasi (tanpa permission, di luar scope, field terlarang).

---

## Lampiran A — Contoh lengkap: penyesuaian stok

```http
# 1. Buat draft
POST /api/inventory/adjustments
Idempotency-Key: 6f1c…
{ "warehouseId": "…", "postingDate": "2026-10-21",
  "lines": [ { "itemId": "…", "quantity": "-2.000", "reason": "damaged" } ] }
→ 201  Location: /api/inventory/adjustments/0193…
{ "data": { "id": "0193…", "number": "ADJ-2026-00042", "status": "draft", "version": 1, … } }

# 2. Ubah draft
PATCH /api/inventory/adjustments/0193…
If-Match: "1"
{ "lines": [ { "itemId": "…", "quantity": "-3.000", "reason": "damaged" } ] }
→ 200 { "data": { …, "version": 2 } }

# 3. Submit
POST /api/inventory/adjustments/0193…/submit
If-Match: "2"
Idempotency-Key: 9a2e…
→ 200 { "data": { …, "status": "submitted", "version": 3 } }

# 4. Submit ulang (tidak sah)
POST /api/inventory/adjustments/0193…/submit
→ 409 { "error": { "code": "invalid_state", "message": "Dokumen sudah disubmit." } }
```

Permission yang terlibat: `inventory.adjustment.create`, `inventory.adjustment.update`, `inventory.adjustment.submit`.
