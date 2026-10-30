# ADR-006: Module manifest, SDK, dan semantic versioning

- **Status:** Accepted
- **Tanggal:** 2026-10-22
- **Sumber:** PRD-000 FR-05 (kontrak modul & registry), FR-06 (manifest schema), FR-18 (dependensi & kompatibilitas), Q15 (struktur Go module), Q24 (path module); keputusan rencana M2a (`docs/superpowers/plans/2026-10-22-m2a-module-contract-registry.md`)

## Konteks

Setiap modul memiliki manifest `module.json` dan dikompilasi sebagai Go module sendiri (ADR-001). Kontrak antara platform, SDK publik (`sdk/`), dan modul harus stabil, dapat divalidasi, dan memiliki aturan versi yang jelas.

Pada M1, `module.json` minimal sudah ada di `modules/example/`, dan `SDKVersion` ada di `sdk/module`. Pada M2a, Module Registry (`internal/platform/modules/registry`) memvalidasi setiap manifest dan merakit katalog saat startup, sehingga aturan dalam ADR ini sudah dapat diverifikasi di kode.

Kendala yang bekerja:

- Platform masih pre-1.0 (`0.1.0`), sehingga constraint SemVer harus mengikuti aturan 0.x.
- Belum ada modul pihak ketiga. Trust, signature, dan sandboxing ditunda ke fase 06 (ADR-008).
- Module path masih memakai path bare `erpora/...` (ADR-001, Q24).

## Opsi yang dipertimbangkan

### A. Sumber deklarasi capability

1. **Deklarasi di kode** (mis. annotation per handler). Sulit diaudit tanpa menjalankan kode modul.
2. **Manifest sebagai satu-satunya sumber deklarasi.** Permission setiap `Handle` harus ada di `capabilities` `module.json`. Dapat diaudit statis dan divalidasi registry sebelum server berjalan.

### B. Cara modul mendaftarkan route

1. **Modul menerima router** (`RegisterRoutes(mux *http.ServeMux)`, seperti contoh arsitektur awal). Modul terikat pada `ServeMux`, dan platform tidak bisa menjaga prefix namespace.
2. **Registrar diimplementasi platform.** Modul memanggil `Handle`, `Public`, dan `Command`. Platform memasang route di `/api/{slug}{path}`. Platform bisa mengganti router tanpa breaking change pada modul.

### C. Format schema manifest

1. **Schema longgar.** Field salah ketik lolos diam-diam.
2. **JSON Schema 2020-12 dengan `additionalProperties: false`.** Perluasan hanya boleh aditif.

### D. Pustaka SemVer

Pustaka `github.com/Masterminds/semver/v3` dipakai untuk parsing versi (`StrictNewVersion`) dan constraint (`NewConstraint`), dibungkus di `internal/platform/modules/compatibility`. Parser versi ditulis sendiri ditolak karena aturan constraint 0.x dan caret perlu diuji terhadap implementasi yang sudah teruji.

## Keputusan

1. **Manifest adalah satu-satunya sumber deklarasi capability.**
   - `module.json` harus lolos `contracts/module.schema.json` (JSON Schema 2020-12). Schema memakai `additionalProperties: false` di objek tingkat atas dan objek bersarang.
   - `schema_version` tetap `"1"`. Perluasan schema hanya boleh aditif.
   - Field marketplace (`artifact`, `checksum`, `signature`, `requested_permissions`, `license`) dan field PRD-001 (`required`, `resources`, `scope_dimensions`, `default_roles`, `settings`) sudah dicadangkan sebagai opsional.

2. **Modul tidak menerima router.** Modul mendaftarkan route lewat `module.Registrar` (`sdk/module/registrar.go`):
   - `Handle(method, path, permission, h, opts...)` wajib memakai permission yang ada di `capabilities` manifest modul. Pelanggaran menghasilkan `permission_undeclared` atau `permission_missing`.
   - `Public(method, path, h, opts...)` untuk route tanpa permission. Route ini tetap tunduk pada status modul.
   - `opts ...RouteOption` (M2c, Q26 butir 13, PRD-000 Q30): `module.MaxBodyBytes(n)` dan `module.Classify(c)`. Parameter variadik ditambahkan agar pemanggil modul tidak berubah; opsi tidak valid menjadi problem registry `invalid_route_option`. `SDKVersion` tetap `0.1.0` karena SDK belum pernah di-tag (Q24); aturan 0.x ADR ini mulai berlaku pada tag pertama.
   - `Command(c)` untuk perintah CLI. Nama harus cocok `^[a-z][a-z0-9-]*$`, `Run` wajib diisi, dan nama tidak boleh ganda dalam satu modul. Nama yang tercatat berbentuk `<slug>:<name>`.
   - `path` harus diawali `/`, sudah bersih (`path.Clean`), dan tidak memuat host, query, fragmen, atau spasi. Method yang diizinkan: GET, POST, PUT, PATCH, DELETE.
   - Penyimpangan dari contoh `RegisterRoutes` disetujui dan dicatat di PRD-000 FR-05.

3. **Versi platform dan SDK dikunci sebagai konstanta.**
   - `buildinfo.PlatformVersion = "0.1.0"` (`internal/platform/buildinfo`) dicocokkan dengan `compatibility.platform`.
   - `module.SDKVersion = "0.1.0"` (`sdk/module/version.go`) dicocokkan dengan `compatibility.sdk`.
   - `buildinfo.Version` dari `-ldflags` hanya label rilis (bisa `dev`) dan tidak dipakai untuk pencocokan SemVer.
   - Tag rilis root `vX.Y.Z` harus sama dengan `PlatformVersion` (`internal/platform/buildinfo`, dibaca lewat `go/parser`). Diperiksa oleh `erpora release:verify-tag` yang dijalankan `.github/workflows/release.yml` pada tag `v*` (M2d, Q32 K-6). Versi kontrak platform (`info.version` di `contracts/openapi/platform.yaml`, dipakai baseline dan approval `erpora contracts:breaking`) wajib sama dengan `PlatformVersion`; test `TestContracts_PlatformContractVersionEqualsPlatformVersion` menjaganya.

4. **Constraint 0.x selama pre-1.0.** Modul referensi memakai `compatibility.platform: "^0.1"` dan `compatibility.sdk: "^0.1"`, bukan `^1.0` seperti tabel FR-15. Versi modul referensi tetap `1.0.0`. `acme.example-addon` bergantung pada `acme.example ^1.0` sesuai FR-15.

5. **Aturan versi multi Go module.**
   - Setiap modul adalah Go module sendiri (`erpora/modules/<name>`, ADR-001). Tag rilis berbentuk `modules/<name>/vX.Y.Z`.
   - `version` di `module.json` harus sama dengan `X.Y.Z[-prerelease]` pada tag. Diperiksa oleh `erpora release:verify-tag <tag>` yang dijalankan workflow `.github/workflows/release.yml` pada tag `modules/*/v*` (M2d, PRD-000 Q31). Tag SDK `sdk/vX.Y.Z` dibandingkan dengan `SDKVersion` (`sdk/module/version.go`, dibaca lewat `go/parser`).
   - Modul menyerahkan kontrak OpenAPI-nya lewat interface opsional SDK `module.ContractSource` (`OpenAPI() []byte`, `openapi.yaml` di-embed). Ini **aditif** (tidak ada perubahan pada `Module`), sehingga `SDKVersion` tetap `0.1.0`.
   - Modul dengan MAJOR ≥ 2 memakai path `.../v2` sesuai aturan Go modules. Aturan ini belum dibutuhkan di fase ini.

6. **Dependensi hanya dari `dependencies`.** Resolver (`internal/platform/modules/resolver`) membangun graf dari `dependencies` (field `optional` membedakan wajib dan opsional), mendeteksi siklus, dan menghasilkan urutan topologis deterministik. Dependensi wajib yang hilang atau tidak cocok menghasilkan `missing_dependency` atau `version_mismatch` dan menggagalkan `Build`. Ketidakcocokan platform atau SDK tidak menggagalkan `Build`, tetapi dicatat di `Graph().Incompatible()`.
   - Field `provides` sudah ada di schema dan `module.Manifest`, tetapi resolver belum memakainya. Dependensi berbasis `provides` berada di luar cakupan FR-18 (lihat Pemicu evaluasi ulang).

7. **Slug tercadang ditolak registry.** Slug tercadang ditolak dengan `reserved_slug`. Daftar lengkap 25 slug (`ui` dan `sdk` ditambahkan 29 Okt 2026: `ui` menjadi namespace terjemahan bersama dan `sdk` bentrok dengan paket frontend `@erpora/module-sdk`, PRD-000 Q37; `platform`, `api`, `assets`, `settings`, `bootstrap`, `healthz`, `readyz`, `sw`, `make`, `db`, `module`, `modules`, `contracts`, `route`, `permission`, `migrate`, `serve`, `inspect`, `version`, `list`, `doctor`, `completion`, `help`) ada di `internal/platform/modules/registry/reserved.go` dan diuji per slug di `rules_test.go`. Slug `platform` juga dilaporkan `duplicate_slug` karena dipakai pseudo-modul `platform.core`.

8. **Trust hanya `builtin` di fase ini.** Schema menerima `builtin | verified | untrusted` agar format tidak berubah di fase 06. Validasi menolak selain `builtin` dengan kode `trust_not_supported`. Alasannya, jalur kepercayaan (verifikasi, signature, sandboxing) belum ada.

9. **Hook lifecycle didefinisikan di SDK, belum dijalankan runtime.** `sdk/module/module.go` mendefinisikan `MigrationSource`, `Installer`, `Upgrader`, `EnableHook`, `DisableHook`, `HealthChecker`, dan `DevSeeder`. Semuanya wajib idempoten. Pemanggilan hook oleh runtime menjadi cakupan M2b (keputusan rencana 6).

## Konsekuensi

- **Positif:**
  - Semua problem manifest, dependensi, dan route dikumpulkan dan dilaporkan sekaligus dengan nama modul dan field (`registry.Error`).
  - Katalog route dan permission dapat diaudit lewat `Registry.Routes()` dan perintah `erpora inspect routes`, `erpora inspect permissions`, serta `erpora inspect modules`.
  - Modul tidak bisa mendaftarkan route di luar namespace `/api/{slug}`.
  - Perluasan schema bersifat aditif, sehingga tidak ada breaking change API.
- **Negatif dan biaya:**
  - Constraint `^0.1` harus dinaikkan menjadi `^1.0` saat platform mencapai 1.0. Itu perubahan manifest semua modul referensi.
  - Module path bare `erpora/...` harus diganti ke vanity domain sebelum SDK dipublikasikan (ADR-001, Q24). Penggantian itu mengubah import, `go.mod`, dan `replace`.
- **Pekerjaan lanjutan wajib:**
  - M2b: pemanggilan hook lifecycle dan status modul per tenant (FR-19).
  - M2d ✅: pemeriksaan tag rilis modul, SDK, dan platform terhadap `version` manifest, `SDKVersion`, dan `PlatformVersion` (`release:verify-tag`).
  - Membuat `docs/module-sdk/versioning.md` yang disebut PRD-000 FR-05/Q15. Dokumen ini belum ada, dan aturan versinya sementara dirujuk ke ADR ini.

## Pemicu evaluasi ulang

- Modul pihak ketiga atau marketplace mulai dibuka (fase 06). Tinjau `trust`, `signature`, `checksum`, dan sandboxing (ADR-008).
- Resolver perlu dependensi berbasis `provides` (FR-18).
- Platform mencapai 1.0, atau SDK mencapai 1.0 sehingga constraint `^0.1` perlu diganti.
- Module path dipindah ke vanity domain (ADR-001, Q24).
- Modul pertama dengan MAJOR ≥ 2 dirilis, sehingga aturan path `/v2` harus diuji.
