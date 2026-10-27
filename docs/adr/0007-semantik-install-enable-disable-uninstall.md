# ADR-007: Semantik install, enable, disable, dan uninstall modul

- **Status:** Accepted (27 Oktober 2026, M2b)

## Konteks

Modul dapat diaktifkan dan dinonaktifkan pada runtime tanpa rebuild binary (PRD-000 FR-19). Operasi ini menyentuh dependensi, migrasi modul, data, dan status yang terlihat oleh pengguna, sehingga semantiknya harus pasti: apa yang terjadi saat operasi diulang, gagal di tengah, atau dijalankan bersamaan dari dua proses. Keputusan di sini diimplementasikan di `internal/platform/modules/{lifecycle,store,runtime,admin}` dan dicatat sebagai Q29 di PRD-000.

## Keputusan

### 1. Status tersimpan vs status turunan

- **Tersimpan** (`module_installations.status`): `installed`, `enabled`, `disabled`, `upgrade_pending`, `failed`, `uninstalled` (`verified` dicadangkan di CHECK). `available` bersifat virtual: modul terkompilasi tanpa baris.
- **Turunan** (dihitung `lifecycle.Effective`, tidak disimpan): `missing` (baris tanpa modul di binary) dan `incompatible`.
- `incompatible` berlaku **transitif**: modul yang tidak kompatibel dengan platform/SDK, atau yang dependensi wajib (transitif)-nya tidak kompatibel, dan status tersimpannya `installed|enabled|disabled|upgrade_pending`, dilaporkan `incompatible` (guard `503 module_unavailable`). `failed`, `uninstalled`, dan `available` tidak ditimpa. `disable` dan `uninstall` tetap diizinkan; `install`, `enable`, `upgrade` ditolak `module_incompatible`. Alasan: kode dependensi yang tidak kompatibel tetap dipanggil lewat `contract/`, jadi dipilih fail-closed.

### 2. Tabel transisi dan no-op

| Operasi | Dari | Ke | No-op bila dari |
| --- | --- | --- | --- |
| install | `available`, `uninstalled` | `installed` | `installed`, `enabled`, `disabled` |
| enable | `installed`, `disabled` | `enabled` | `enabled` |
| disable | `enabled` | `disabled` | `disabled` |
| upgrade | `upgrade_pending` | status sebelumnya (`previous_status`) | `installed`/`enabled`/`disabled` dengan versi sama |
| uninstall | `installed`, `disabled` | `uninstalled` | `uninstalled` |

Pasangan lain adalah `invalid_transition`. Operasi yang status targetnya sudah tercapai adalah **no-op sukses**: tanpa baris operasi, event, atau kenaikan revision. Ini membuat `install --all --enable` dapat diulang. Prasyarat (`Check`) dievaluasi setelah tabel transisi, sehingga no-op selalu menang.

### 3. Langkah per operasi

Setiap langkah dicatat ke `module_operations.steps` sebelum dan sesudah dijalankan (kunci deteksi `interrupted`) dan menjadi child span `module.<op>.<langkah>`.

| Operasi | Langkah |
| --- | --- |
| install | `resolve` → `compatibility` → `migrate` → `hook:install` |
| enable | `dependencies` → `hook:enable` → `health` (gagal → `hook:disable`, status tetap asal) |
| disable | `dependents` → `hook:disable` |
| upgrade | `resolve` → `compatibility` → `migrate` → `hook:upgrade` |
| uninstall | `dependents` |

Prasyarat: dependensi minimal (install) = `installed|enabled|disabled`; enable membutuhkan dependensi wajib `enabled`; **upgrade yang akan mengembalikan modul ke `enabled` (previous_status `enabled`) dan retry yang akan membuat modul `enabled` juga membutuhkan semua dependensi wajib `enabled` dan ditolak sebelum operasi dimulai (`dependency_not_ready`; tidak ada baris operasi, status tidak berubah)**; disable ditolak bila dependen wajib `enabled` **atau `upgrade_pending`/`failed` dengan previous_status `enabled`** (`dependents_active`) (Q32); uninstall ditolak bila ada dependen wajib berstatus selain `uninstalled`; upgrade memeriksa versi ter-install dependensi terhadap constraint manifest baru. Modul `required: true` menolak `disable`/`uninstall` (`module_required`). **`uninstall` tidak pernah menghapus tabel atau data**; install ulang menjalankan hanya migrasi yang belum diterapkan.

### 4. Kegagalan dan retry

- Kegagalan langkah yang mengubah data (`migrate`, `hook:install`, `hook:upgrade`) → modul `failed` dengan `previous_status` = status asal (untuk install dari nol dicatat sebagai teks `available`; untuk upgrade tetap status sebelum upgrade).
- Kegagalan `hook:enable`, `health`, `hook:disable`, atau prasyarat → status modul **tidak berubah**; operasi berstatus `failed` (kecuali penolakan prasyarat, yang tidak membuat baris operasi: hanya error dan log WARN).
- `retry` hanya untuk operasi `failed` yang menjadi `last_operation_id` modulnya dan bukan `reconcile`. Retry membuat baris operasi baru (nama operasi = asal), mencatat event `retried` dengan `details.retryOf`, dan melewati langkah yang sudah `succeeded` **kecuali `migrate`** (goose hanya menerapkan file yang belum diterapkan, sehingga migrasi ke-1 tidak diulang). Prasyarat diperiksa ulang **sebelum operasi dimulai** (termasuk `hook:enable`, yang tidak pernah dilewati saat retry karena efeknya dibatalkan kompensasi `hook:disable`).
- Migrasi dijalankan satu file per transaksi; file yang gagal di-rollback penuh dan dicatat `failed` di `module_migrations` (tenant DB).

### 5. Serialisasi: advisory lock

Setiap operasi lifecycle dan reconcile berjalan di bawah `pg_try_advisory_lock(hashtextextended('erp.module.lifecycle:' || tenant_id, 0))` pada satu koneksi khusus, dicoba ulang dengan backoff 50 ms → 500 ms sampai `DefaultLockTimeout` (5 s). Gagal → `operation_in_progress` (exit 3 / `409 request_in_progress`). Unlock memakai `context.WithoutCancel`; bila gagal, `pg_advisory_unlock_all()` dijalankan sebelum koneksi kembali ke pool. Reconcile `serve` yang kehabisan waktu hanya memuat snapshot (read-only) dengan log WARN.

### 6. Operasi terputus (`interrupted`)

Setelah memperoleh lock, setiap operasi dan reconcile menandai semua operasi `running` tenant itu `failed` dengan `error.code = interrupted` dan langkah yang sedang berjalan. Selama lock dipegang tidak ada operasi lain yang hidup, jadi baris `running` pasti sisa proses yang mati. Modul ikut `failed` hanya bila yang terputus adalah `install` atau `upgrade`; operasi itu dapat di-`retry`. Event `interrupted` memakai actor pelaku penandaan (`system:reconcile` atau actor operasi berikutnya); actor operasi asal disimpan di `details.originalActor` (Q32).

### 7. Reconcile saat startup

`serve` menjalankan `Reconcile` sebelum melayani request: versi binary > ter-install → `upgrade_pending` (event `reconciled`); versi binary < ter-install → operasi `reconcile` `failed` `downgrade_not_supported`, modul `failed`, `previous_status` disimpan, event `operation_failed`; binary kembali ke versi ter-install → status semula dipulihkan (`reconciled`, `version_restored`), baik dari `failed` (downgrade) maupun dari `upgrade_pending` (rollback binary; Q32); baris tanpa modul → `missing` + WARN. Reconcile tidak pernah memigrasi, meng-install, atau meng-enable; migrasi hanya lewat `migrate up` dan operasi lifecycle.

### 8. Revision dan propagasi

- `tenant_state.module_state_revision` naik pada **setiap commit operasi** (termasuk install, reconcile, `interrupted`, dan operasi gagal yang tidak mengubah status), karena status terlihat di `/api/bootstrap` dan ETag = revision harus berubah ketika `lastOperationId` berubah (Q32). Satu `Commit` atomik memuat installation, penutupan operasi, event (append-only, dijaga trigger), dan revision.
- Snapshot per proses memakai compare-and-set per revision (`Snapshot.Apply`). `StateNotifier` in-process memberi tahu subscriber; karena CLI adalah proses terpisah, `serve` juga **memeriksa revision DB setiap 2 detik** (`lifecycle.DefaultPollInterval`) dan mempublikasikan revision yang lebih baru ke notifier, yang memicu `Refresh`.

### 9. Modul `required`

Modul `required: true` (beserta dependensinya) di-install dan di-enable oleh `erpora migrate up` dengan actor `system:migrate`, idempoten (no-op bila sudah enabled).

### 10. Akses API dan actor

Mutasi lewat `/api/platform/modules` berjalan sinkron dengan batas tunggu 30 detik; lebih lama → `202` + `Location: …/operations/{id}` dan operasi berlanjut di server (`context.WithoutCancel`). Shutdown di tengah operasi menghasilkan `interrupted` yang dapat di-`retry`. Hanya aktif bila `ERP_ENV=development` atau `ERP_MODULES_ADMIN_API=true` (selain itu `404`). Actor: `cli:<os-user>`, `api:dev`, `system:reconcile`, `system:migrate`. Hook dan command modul memperoleh hak sistem teraudit lewat `authz.AsSystem(ctx, "module.lifecycle:<op>" | "cli:<slug>:<name>")`.

**Respons API (Q32):** mutasi wajib `Content-Type: application/json` (body kosong atau `{}` boleh; selain itu `415 unsupported_media_type`); `If-Match` dievaluasi di dalam advisory lock (RFC 9110: `*`, daftar, perbandingan kuat) sebelum simulasi; respons `200`/`202` memuat semua ID operasi rangkaian di `operationIds` (aditif; `Location` ke operasi pertama); `message` kegagalan `migration_failed` dan `internal_error` diringkas tanpa teks SQL/hook mentah (detail penuh hanya di log dengan `operation_id`/`trace_id`).

## Konsekuensi

- Operasi idempoten dan aman diulang dari skrip deployment; kegagalan sebagian selalu dapat dilanjutkan.
- Snapshot status bergantung pada polling ≤ 2 detik antarproses; perubahan lewat CLI terlihat di `serve` tanpa restart.
- `incompatible` fail-closed dapat mematikan modul yang dependensinya tidak kompatibel setelah upgrade platform; operator melihatnya di `modules list` dan dapat `disable`/`uninstall`.
- Platform memakai `authz.AsSystem` untuk hook dan command modul: pengecualian terhadap aturan "hanya dari `internal/application` modul" (PRD-001 FR-119); analyzer FR-115 wajib mengizinkan `internal/platform/modules/**` dan `internal/app`.

## Pemicu evaluasi ulang

- **Multi-instance** (lebih dari satu proses `serve`) → ganti poller dengan `LISTEN/NOTIFY` di balik `StateNotifier`.
- **PRD Jobs** → ganti jalur `202` dengan antrean job; `module_operations` tetap catatan status.
- **PRD-001** (principal nyata) → principal menggantikan actor `api:dev`/`cli:*`, `Idempotency-Key` disimpan 24 jam, dan analyzer `AsSystem` ditegakkan.
