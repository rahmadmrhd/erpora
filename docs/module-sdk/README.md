# Module SDK

Dokumentasi SDK modul ERPora untuk penulis modul. SDK ada di `sdk/` (Go module `erpora/sdk`), dan kontrak utamanya di paket `erpora/sdk/module`.

## Isi

| Dokumen | Isi |
| --- | --- |
| [`creating-a-module.md`](creating-a-module.md) | Cara membuat modul baru: struktur folder, `module.json`, `Register`, hook lifecycle, kontrak antarmodul, dan langkah menyalin `example`. |
| [ADR-006](../adr/0006-module-manifest-sdk-semver.md) | Keputusan arsitektur: manifest sebagai sumber capability, `Registrar`, SemVer, dan aturan versi multi Go module. |
| [ADR-002](../adr/0002-clean-architecture-and-dependency-rules.md) | Aturan dependensi D1–D11 yang wajib dipatuhi modul. |
| [`contracts/module.schema.json`](../../contracts/module.schema.json) | JSON Schema untuk `module.json`. |

## Kontrak utama

- `module.Module`: `Manifest()` dan `Register(r Registrar)`.
- `module.Registrar`: `Handle`, `Public`, dan `Command`. Modul tidak menerima router.
- Hook lifecycle opsional, semuanya wajib idempoten: `MigrationSource`, `Installer`, `Upgrader`, `EnableHook`, `DisableHook`, `HealthChecker`, dan `DevSeeder`.

Definisi lengkap ada di `sdk/module/module.go` dan `sdk/module/registrar.go`.

## Status

Module Registry (M2a) sudah memvalidasi manifest, dependensi, route, dan command saat startup. Pemanggilan hook lifecycle oleh runtime dan status modul per tenant menjadi cakupan M2b. Dokumen `versioning.md` yang disebut PRD-000 belum dibuat, dan aturan versinya sementara ada di ADR-006.

Panduan frontend (`@erpora/module-sdk`) belum ditulis.
