# Module SDK

Dokumentasi SDK modul ERPora untuk penulis modul. SDK ada di `sdk/` (Go module `erpora/sdk`), dan kontrak utamanya di paket `erpora/sdk/module`.

## Isi

| Dokumen | Isi |
| --- | --- |
| [`creating-a-module.md`](creating-a-module.md) | Cara membuat modul baru: struktur folder, `module.json`, `Register`, hook lifecycle, kontrak antarmodul, dan pembuatan modul lewat `make:module` (penyalinan manual `example` sebagai lampiran). |
| [`lifecycle.md`](lifecycle.md) | Siklus hidup modul: urutan hook per operasi, idempotensi, konteks hook, aturan migrasi, upgrade/downgrade, `retry`, perilaku `--all`, dan cara menguji hook. |
| [`generators.md`](generators.md) | Developer tools `erpora` flavor dev: `make:*`, `db:fresh`/`db:seed`, `module:check`, `doctor`, penanda, `--dry-run`/`--force`, dan pengaman penanda lingkungan. |
| [`contracts.md`](contracts.md) | Kontrak OpenAPI per modul: rujukan `erp:`, ekstensi `x-erp-*`, rule linter, klasifikasi, `contracts:generate`, breaking change, dan tag rilis. |
| [`security-primitives.md`](security-primitives.md) | Primitif keamanan: `httpx` (respons, error, `DecodeJSON`), opsi route (`MaxBodyBytes`, `Classify`), `egress`, `export`, dan `statement_timeout`; aturan arch `EGRESS`/`DECODE`. |
| [`frontend.md`](frontend.md) | Panduan frontend modul: struktur `modules/<x>/web`, `defineModule`, route lazy dan guard, aksi halaman, teks lewat i18n, pemakaian `@erpora/ui`, uji komponen, dan anggaran bundle. |
| [ADR-007](../adr/0007-semantik-install-enable-disable-uninstall.md) | Semantik install, enable, disable, uninstall: tabel transisi, kegagalan, advisory lock, reconcile, revision. |
| [ADR-006](../adr/0006-module-manifest-sdk-semver.md) | Keputusan arsitektur: manifest sebagai sumber capability, `Registrar`, SemVer, dan aturan versi multi Go module. |
| [ADR-002](../adr/0002-clean-architecture-and-dependency-rules.md) | Aturan dependensi D1–D11 yang wajib dipatuhi modul. |
| [`contracts/module.schema.json`](../../contracts/module.schema.json) | JSON Schema untuk `module.json`. |

## Kontrak utama

- `module.Module`: `Manifest()` dan `Register(r Registrar)`.
- `module.Registrar`: `Handle`, `Public` (keduanya menerima opsi route variadik), dan `Command`. Modul tidak menerima router.
- Hook lifecycle opsional, semuanya wajib idempoten: `MigrationSource`, `Installer`, `Upgrader`, `EnableHook`, `DisableHook`, `HealthChecker`, dan `DevSeeder`.

Definisi lengkap ada di `sdk/module/module.go` dan `sdk/module/registrar.go`.

## Status

Module Registry (M2a) sudah memvalidasi manifest, dependensi, route, dan command saat startup. Lifecycle modul (M2b) sudah berjalan: hook dipanggil oleh runtime, status tersimpan per tenant, dan modul dikelola lewat CLI `erpora modules` dan API `/api/platform/modules` (lihat `lifecycle.md`). Dokumen `versioning.md` yang disebut PRD-000 belum dibuat, dan aturan versinya sementara ada di ADR-006.

Primitif keamanan M2c (`sdk/httpx`, `sdk/egress`, `sdk/export`, opsi route) tersedia; lihat `security-primitives.md`.

Developer tools dan kontrak API per modul (M2d) tersedia; lihat `generators.md` dan `contracts.md`.

Frontend (M3a) sedang dikerjakan: panduan modul frontend ada di [`frontend.md`](frontend.md). `make:page` ditunda ke M3d (PK-5, belum dikonfirmasi). Templat `make:module --frontend` belum selaras dengan panduan itu (lihat `generators.md`).
