# Dokumen Arsitektur

| Dokumen | Status | Isi |
| --- | --- | --- |
| [api-guidelines.md](api-guidelines.md) | Accepted v1 | Bentuk wajib kontrak & endpoint API |
| [tenancy-seams.md](tenancy-seams.md) | Implementasi M1 | Seam single-tenant yang ada di kode |
| [offline-strategy.md](offline-strategy.md) | Draft desain | Mode offline, `OfflinePolicy`, outbox klien → PRD Offline & Sync |
| [tenancy-design.md](tenancy-design.md) | Draft desain | Target database-per-tenant, control DB, resolver, isolasi → PRD Tenancy |
| [module-integration.md](module-integration.md) | Draft desain | Kontrak antarmodul, sinkron vs event, outbox server → PRD Events & Jobs |
| [extensions-and-marketplace.md](extensions-and-marketplace.md) | Draft desain | Kelas ekstensi, pipeline paket, persyaratan marketplace → PRD Marketplace / ADR-008 |
| [transaction-data-standards.md](transaction-data-standards.md) | Draft v1 (review) | Standar transaksi & integritas data (ISO 8601/4217/15489, SoD, ledger, pembulatan, retensi) |
| [roadmap-and-risks.md](roadmap-and-risks.md) | Draft desain | Roadmap fase, urutan modul, entitas domain, fondasi UX, testing, versi & rilis, scaling, daftar ADR, risiko |

"Draft desain" berarti materi dari dokumen awal yang belum menjadi PRD; ia menjadi masukan saat PRD terkait ditulis, bukan spesifikasi yang mengikat.

## Pemetaan rujukan dokumen awal

PRD-000 dan PRD-001 masih memuat rujukan seperti "Blueprint §7" atau "arsitektur §2" sebagai asal-usul keputusan. Kedua dokumen awal (`Blueprint.md`, `arsitektur.md`) sudah dihapus dari repository; versi aslinya tetap ada di riwayat git (commit `c42d676`). Materinya kini berada di:

| Rujukan lama | Lokasi sekarang |
| --- | --- |
| Blueprint §1 (ringkasan keputusan), §2 (gambaran sistem) | PRD-000 §1; `tenancy-design.md` §2 |
| Blueprint §3 (database-per-tenant, ERD control DB) | `tenancy-design.md` §1–§3 |
| Blueprint §3.3 (ERD transaksi ERP) | `roadmap-and-risks.md` §2 |
| Blueprint §4 (PostgreSQL vs MySQL) | ADR-003 |
| Blueprint §5, §5.1 (struktur folder & aturan dependensi) | PRD-000 §4 |
| Blueprint §6 (kontrak modul, manifest, SemVer) | PRD-000 FR-05, FR-06, FR-18 |
| Blueprint §7 (lifecycle modul) | PRD-000 FR-19 |
| Blueprint §8 (marketplace) | `extensions-and-marketplace.md` |
| Blueprint §9 (implementasi multi-tenancy) | `tenancy-design.md` §4–§5; seam di `tenancy-seams.md` |
| Blueprint §10 (kontrak antarmodul) | `module-integration.md` |
| Blueprint §11 (frontend & design system) | PRD-000 FR-08/FR-09; `roadmap-and-risks.md` §3 |
| Blueprint §12 (testing) | PRD-000 FR-14; `roadmap-and-risks.md` §4; `tenancy-design.md` §6 |
| Blueprint §13 (upgrade & rilis hybrid) | PRD-000 FR-07/FR-19; `roadmap-and-risks.md` §5 |
| Blueprint §14–§17 (roadmap, scaling, ADR, risiko) | `roadmap-and-risks.md` §1, §6–§8 |
| arsitektur §1–§2 (gambaran & module registry) | PRD-000 FR-05, FR-09 |
| arsitektur §3 (route registration) | PRD-000 FR-08 |
| arsitektur §4 (permission engine) | PRD-001 §6 |
| arsitektur §5 (offline) | `offline-strategy.md` |
| arsitektur §6 (bootstrap manifest) | PRD-000 FR-04; `offline-strategy.md` §5 |
| arsitektur §7 (struktur proyek) | PRD-000 §4 |
| arsitektur §8 (build-time vs runtime plugin) | `extensions-and-marketplace.md` §1 |
| arsitektur §9 (urutan implementasi) | `roadmap-and-risks.md` §1 |
