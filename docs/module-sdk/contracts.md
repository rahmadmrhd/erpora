# Kontrak API modul (OpenAPI)

Panduan kontrak untuk penulis modul (PRD-000 FR-23). Bentuk wajib endpoint ada di
[`api-guidelines.md`](../architecture/api-guidelines.md); perintah pembuatnya ada di [`generators.md`](generators.md).

## 1. Kepemilikan kontrak dan rujukan `erp:`

| Pemilik | File | Isi |
| --- | --- | --- |
| Platform (bersama) | `contracts/openapi/common.yaml` | Hanya komponen: amplop, `Decimal`, `Uuid`, `Timestamp`, parameter/header standar, respons error |
| Platform | `contracts/openapi/platform.yaml` | `/api/bootstrap`, `/api/platform/*` |
| Setiap modul | `modules/<dir>/openapi.yaml` | Path di bawah `/api/<slug>/` saja; di-embed lewat `module.ContractSource` (`OpenAPI() []byte`) |

Semua kontrak memakai `openapi: 3.1.0`; YAML dengan anchor/alias ditolak. Rujukan lintas dokumen hanya `erp:<common|platform|slug>#/<pointer>`
(path file relatif dilarang). `erp:common` selalu boleh; `erp:<slug>` hanya ke modul yang ada di `dependencies` manifest (rule `erp-ref-dependency`).
Pada spec gabungan nama skema diberi namespace `<slug>.<NamaSkema>` (`common` dan `platform` tidak), sehingga dua modul boleh punya `Item`.

## 2. Ekstensi `x-erp-*`

| Ekstensi | Dipakai pada | Arti |
| --- | --- | --- |
| `x-erp-permission` | operasi | Permission `<slug>.<resource>.<action>` (platform: `platform.<resource>.<action>`); harus ada di `capabilities` manifest |
| `x-erp-public` | operasi | Route tanpa permission (`Registrar.Public`) |
| `x-erp-classification` | properti, parameter | `public`, `internal`, `confidential`, `highly_confidential` (security-guidelines §2). Tanpa anotasi dianggap `internal` |
| `x-erp-singleton` | path item | Resource tunggal atau nama tetap yang tidak dijamakkan (`/api/example/ping`, `…/modules/{key}/history`); api-guidelines §2 |
| `x-erp-allow-put` | operasi | Mengizinkan `PUT` (api-guidelines §3) |
| `x-erp-filterable`, `x-erp-sortable` | properti | Field yang boleh difilter/diurutkan (api-guidelines §5) |
| `x-erp-field-group` | properti | Field group untuk masking (security-guidelines §2) |
| `x-erp-extensible-enum` | enum | Nilai baru boleh ditambah tanpa breaking (api-guidelines §14.1) |

## 3. Linter: rule → pasal

`erpora contracts:lint [module]` menjalankan Spectral (terpin di `web/package.json`, dijalankan Bun; tanpa `bun x`/`npx`) dengan ruleset
`contracts/spectral.yaml` + fungsi kustom `contracts/spectral-functions/`. Seluruh rule `erp-*` berseverity `error` dan pesannya menyebut pasalnya.
Dasarnya `spectral:oas` (`recommended`), dengan `info-contact`, `info-license`, `oas3-api-servers`, `operation-tags`, `openapi-tags`, dan
`oas3-unused-component` dimatikan (yang terakhir karena komponen diekspor ke dokumen lain lewat `erp:`).

| Rule | Menegakkan | Pasal |
| --- | --- | --- |
| `erp-path-module-prefix` | Path modul `/api/<slug>/…`; platform `/api/platform/` atau `/api/bootstrap` | api-guidelines §2; FR-23.1 |
| `erp-path-kebab-case`, `erp-path-no-trailing-slash`, `erp-path-no-crud-verb` | Segmen kebab-case, parameter camelCase, tanpa `/`, tanpa kata kerja CRUD | §2 |
| `erp-path-collection-plural` | Koleksi terakhir jamak (kecuali `x-erp-singleton` atau aksi `POST`) | §2 |
| `erp-path-max-nesting` | Maksimal satu sub-koleksi setelah `{id}` | §2 |
| `erp-operation-permission` | `x-erp-permission` berformat benar atau `x-erp-public` | §1, §13 |
| `erp-operation-id` | `operationId` ada, camelCase, unik | §13 |
| `erp-property-camel-case`, `erp-enum-snake-case` | Nama properti dan nilai enum | §4.1 |
| `erp-decimal-not-number` | Uang/kuantitas string desimal, bukan `number` | §4.1 |
| `erp-success-envelope` | Respons sukses JSON berbentuk `{data}` | §4.2 |
| `erp-error-standard` | 4xx/5xx memakai komponen error `erp:common` | §8 |
| `erp-collection-pagination` | `GET` koleksi memakai `page`+`pageSize` atau `cursor`+`limit` | §6 |
| `erp-post-idempotency-key` | `POST` punya header `Idempotency-Key` | §7.2 |
| `erp-if-match` | `PATCH`, `DELETE`, dan aksi `POST` punya `If-Match` | §7.1 |
| `erp-no-put` | `PUT` hanya dengan `x-erp-allow-put` | §3 |
| `erp-ref-dependency` | `erp:<slug>` hanya ke dependensi manifest | FR-23.1 |
| `erp-examples` | Respons 2xx dan `requestBody` JSON punya contoh | §13 |
| `erp-error-codes-documented` | Operasi mendokumentasikan 4xx dan `403`; `404` bila ada parameter path; `400` dan `422` bila ada `requestBody`. **Operasi publik** (`x-erp-public`) tanpa parameter non-header dan tanpa `requestBody` tidak diwajibkan mendokumentasikan 4xx/403 (tidak ada jalur error klien); publik dengan masukan tetap wajib minimal satu 4xx | §13 |
| `erp-classification-valid` | `x-erp-classification` salah satu dari empat kelas | security-guidelines §2 |
| `erp-no-highly-confidential-in-response` | Respons tidak memuat properti `highly_confidential` | FR-24.2; security-guidelines §2, §3.4 |
| `erp-no-sensitive-in-url` | Data sensitif (mis. `token`, `password`) tidak lewat path atau query | FR-24.2 |
| `erp-sensitive-needs-classification` | Properti/parameter bernama sensitif (`password`, `secret`, `token`, `apiKey`, `nik`, `npwp`, `bankAccount`, `salary`, …) wajib punya klasifikasi memadai | FR-24.2; security-guidelines §2 |

## 4. Klasifikasi respons dan `module.Classify`

Klasifikasi respons sebuah operasi dihitung **rekursif** atas skema respons 2xx (properti bersarang, `items`, `allOf`/`oneOf`/`anyOf`, `$ref` lintas dokumen,
aman terhadap siklus) dan diambil yang tertinggi; properti tanpa anotasi dianggap `internal`. Test kesesuaian mensyaratkan klasifikasi route
(`module.Classify(c)` pada `Registrar.Handle`; kosong dianggap `internal`) **sama persis** dengan klasifikasi kontrak, di kedua arah: route lebih rendah berarti
`Cache-Control: no-store` hilang, route lebih tinggi berarti kontrak bukan lagi sumber kebenaran. Lihat [`security-primitives.md`](security-primitives.md).

## 5. Test kesesuaian dan test respons

- `internal/app/contract_conformance_test.go`: `TestContracts_RoutesMatchOperations` (method, path penuh, `x-erp-permission`, `x-erp-public`; `platform.core` dicocokkan lewat kesetaraan dispatch karena route gabungan `{key}/{sub}`, Q29), `TestContracts_PermissionsDeclaredInManifest`, `TestContracts_ClassificationMatchesRoute`, `TestContracts_ErpRefsOnlyToDependencies`, `TestContracts_EveryEntryHasContract`.
- `internal/app/contract_response_integration_test.go`: respons nyata modul referensi divalidasi terhadap skema kontraknya.
- Menambah `Handle` tanpa operasi (atau sebaliknya) menggagalkan test di atas; `make:handler` menjaga ketiganya (route, capability, operasi) sinkron.

## 6. `contracts:generate` dan `--check`

`erpora contracts:generate [module]` membangkitkan, lalu menulis:

- `packages/platform-contracts/src/{common,platform}.gen.ts` (tipe + client fetch bertipe `createClient<Paths>()`),
- `modules/<dir>/web/api/contract.gen.ts` untuk modul ber-`web/`,
- `packages/module-sdk/src/manifest.gen.ts` (dari `contracts/module.schema.json`),
- `contracts/dist/openapi.json` (spec gabungan; artifact build, tidak di-commit).

File hasil generate di-commit dan diberi header "JANGAN DIEDIT". `erpora contracts:generate --check` tidak menulis apa pun: ia membangkitkan di memori,
membandingkannya dengan disk, mencetak file yang basi, dan exit 1. `task contracts:check` = `contracts:lint` + `generate --check` + `contracts:breaking`.

## 7. Breaking change, label, dan tag rilis

`erpora contracts:breaking [module]` membandingkan kontrak terbundel HEAD dengan kontrak pada **tag rilis terakhir** (`oasdiff` sebagai library Go).

- Baseline: tag semver tertinggi non-prerelease. Platform: `vX.Y.Z`; modul: `modules/<dir>/vX.Y.Z`; SDK: `sdk/vX.Y.Z`. Kontrak baseline dibundel dari pohon tag yang sama (termasuk `erp:common` dan dependensi). Target tanpa tag dilewati.
- Perubahan berlevel `WARN` ke atas menggagalkan, karena api-guidelines §14.2 melarang menghapus field apa pun (termasuk field respons opsional yang `oasdiff` nilai WARN). Menambah field opsional lolos.
- Pengecualian hanya bila **keduanya**: PR berlabel `api-breaking-approved` (env `ERP_API_BREAKING_APPROVED=true` di CI; pada event `push` ke main CI mengaturnya `true`, karena perubahan itu sudah melewati review PR) **dan** major `version` manifest lebih besar daripada major tag baseline. Setelah breaking yang disetujui di-merge dan sebelum tag baru dibuat, PR berikutnya tanpa label tidak gagal: `ERP_API_BREAKING_BASE_REF` (CI: `origin/<base_ref>`) membuat perubahan yang sudah ada di cabang dasar dianggap disetujui, sedangkan perubahan breaking baru tetap butuh label. Syarat "tidak dipakai 6 bulan" (§14.3) diperiksa reviewer, bukan alat.
- Workflow `.github/workflows/release.yml` (tag `modules/*/v*`, `sdk/v*`) menjalankan `erpora release:verify-tag <tag>`: versi tag harus sama dengan `version` di `module.json` (dan `go.mod` modul harus ada dengan path `erpora/modules/<dir>`) atau dengan `SDKVersion` di `sdk/module/version.go`. Tag platform `vX.Y.Z` dipakai sebagai baseline; platform memakai `info.version` di `contracts/openapi/platform.yaml` sebagai versi yang disetujui.

## 8. Metrik deprecated

Operasi `deprecated: true` di kontrak: respons membawa header `Deprecation: true` dan counter OTel `erp.api.deprecated_usage` bertambah (atribut `erp.module.key`, `http.route`, `erp.api.deprecated.kind=operation`).
Parameter **query** `deprecated: true` yang benar-benar dikirim klien menambah counter yang sama dengan `kind=parameter` dan `erp.api.deprecated.name`. Dipasang setelah pemeriksaan permission,
sehingga request yang ditolak tidak dihitung. Belum dicakup (PRD-000 Q31 ★5): properti body request deprecated (menuntut parse body dua kali; ditegakkan di `DecodeJSON` bila ada konsumen),
atribut klien (identitas klien baru ada di PRD-001), dan properti respons (tidak terukur di server).
