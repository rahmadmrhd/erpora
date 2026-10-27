# M2d — Developer Tools & Kontrak API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Menutup M2. Pekerjaan rutin modul dikerjakan oleh developer tools `erpora` flavor dev: `make:*`, `db:*`, `module:check`, `doctor`, dan `contracts:*`. Hasilnya langsung lolos semua gate, dan developer tools tidak pernah ikut ke binary release. Setiap modul punya kontrak OpenAPI sendiri. Kontrak itu harus cocok dengan route terdaftar (method, path, permission, klasifikasi), lolos linter Spectral yang menegakkan api-guidelines dan klasifikasi data, dibangkitkan menjadi tipe TS, dan dijaga dari breaking change dengan `oasdiff`. Penanda lingkungan database melindungi data production dari binary dev. Rencana ini ditutup dengan verifikasi exit gate M2 (PRD-000 §11).

**Architecture:** Developer tools berada di `internal/devtools/**` (cobra, template `text/template` ter-embed). Satu-satunya titik masuknya adalah `cmd/erpora/devtools_on.go` (`//go:build devtools`), dan aturan arch baru `DEVTOOLS` menegakkannya. Kontrak dimodelkan oleh paket platform baru `internal/platform/apicontract`. Paket ini mengurai YAML OpenAPI 3.1, menyelesaikan rujukan `erp:<doc>#/…`, mendaftar operasi, menghitung klasifikasi respons secara rekursif, membundel, dan menggabungkan kontrak dengan namespace skema. Modul menyerahkan kontraknya lewat interface opsional SDK `module.ContractSource` (`OpenAPI() []byte`, file `openapi.yaml` di-embed), dan registry menyimpannya di `Entry.Contract`. Test kesesuaian di `internal/app` mencocokkan `Registry.Routes()` dengan operasi kontrak. Linter memakai Spectral (paket npm terpin di `web/package.json`) yang dijalankan Bun lewat `web/contracts-lint.ts`, dengan ruleset `contracts/spectral.yaml` dan fungsi kustom TS di `contracts/spectral-functions/`. Generator TS (`openapi-typescript`, `json-schema-to-typescript`) dijalankan lewat `web/contracts-generate.ts`. Deteksi breaking change memakai library Go `oasdiff` terhadap kontrak terbundel dari tag rilis terakhir. Guard penanda lingkungan (`app.GuardDevWrite`) hanya membaca `platform_state.environment` dan hanya aktif di flavor dev.

**Tech Stack:** Go 1.27.3 · cobra · `text/template` + `embed` · `go.yaml.in/yaml/v3` (Node API) · `golang.org/x/mod/modfile` + `golang.org/x/mod/semver` · `golang.org/x/tools/imports` + `go/ast/astutil` · `github.com/santhosh-tekuri/jsonschema/v6` · `github.com/oasdiff/oasdiff` + `github.com/getkin/kin-openapi` (hanya untuk diff) · OpenTelemetry metric API · Bun 1.4.2 · `@stoplight/spectral-*` · `openapi-typescript` · `json-schema-to-typescript` · Biome · testcontainers-go · GitHub Actions

**Spec:** `docs/prd/PRD-000-inisialisasi-project.md` FR-03 (`inspect config` + penanda), FR-06 AC (tipe TS), FR-12, FR-13, FR-14 (gate kontrak + workflow tag rilis), FR-22 (22.1–22.5 + AC), FR-23 (23.1–23.4 + AC), FR-24.2 AC (linter klasifikasi), §4 dan §4.2, §11 exit gate M2, Q16, Q19–Q24, Q26–Q29, Q30 (keputusan M2c); `docs/architecture/api-guidelines.md` §1–§4, §6–§8, §12–§15; `docs/security/security-guidelines.md` §2, §3.1, §7; ADR-006; pembagian M2: `docs/superpowers/plans/2026-10-22-m2-overview.md` (baris M2d); sisa dari "Di luar cakupan" rencana M2b dan M2c.

---

## Keputusan rencana (perlu disetujui pada review rencana)

> **Status (27 Okt 2026): kelima butir ★ disetujui owner** sesuai rekomendasi masing-masing; dicatat di PRD-000 Q31 beserta keputusan saat eksekusi dan deviasi.

Butir bertanda ★ **(perlu konfirmasi owner)** menyangkut ambiguitas atau kontradiksi spec, atau perilaku di luar teks spec. Butir lain adalah pilihan implementasi yang mengikat rencana ini.

1. **★ Generator frontend di M2d dibuat minimal; `make:page` ditunda ke M3 (perlu konfirmasi owner).** FR-22 AC dan exit gate M2 memakai `make:module … --frontend`, padahal shell, registry TS, `defineModule`, dan TanStack Router baru dibangun di M3 (FR-08/FR-09, overview M2). M2d mengerjakan hal berikut:
   - `--frontend` membuat package `modules/<dir>/web` (`@erpora/module-<dir>`: `package.json`, `tsconfig.json`, `src/index.ts` yang mengekspor `moduleInfo` dan `api`, serta `api/contract.gen.ts`).
   - Package itu didaftarkan di `web/package.json` dan di file baru `web/src/app/modules.ts`. File ini memakai dua penanda, `// erp:generated:web-imports` dan `// erp:generated:web-modules`, dan berisi array modul frontend yang belum dipakai shell.
   - `make:page` belum didaftarkan. Command itu dibuat di M3 bersama router.

   Alternatifnya, `--frontend` juga ditunda ke M3. Akibatnya AC FR-22 tidak terpenuhi di exit gate M2. Rekomendasi: setujui versi minimal.
2. **★ `make:repository` dan "integration test" vs Q28 (perlu konfirmasi owner).** FR-22.2 meminta generator membuat integration test repository, sedangkan Q28 butir 3 menghapus `gorm.io/driver/postgres` dari `go.mod` modul agar driver itu tidak tersalin ke template. Dari dalam modul, integration test tidak bisa membuka DB tanpa driver. Dari root (`internal/app`), test tidak bisa mengimpor `modules/<x>/internal/**`. Pilihan:
   - (A) Integration test ber-tag `integration` di dalam modul, dengan driver sebagai dependensi modul. Ini membatalkan Q28 untuk modul hasil generate.
   - (B) Generator membuat unit test mapping model ↔ entity, dan integration test repository ditulis manual lewat platform, mengikuti pola `internal/app/reference_modules_integration_test.go`.
   - (C) Helper test DB baru di SDK, yang menambah dependensi driver ke `sdk`.

   Rekomendasi: **(B)** di M2d. Keputusan integration test generik ditinjau di PRD-001, saat modul `core` pertama membutuhkannya.
3. **★ Ekstensi `x-erp-singleton: true` untuk resource tunggal (perlu konfirmasi owner).** api-guidelines §2 mewajibkan koleksi jamak. Namun route yang sudah ada adalah resource tunggal: `GET /api/example/ping`, `GET /api/example-addon/summary`, dan `GET /api/bootstrap`. Ada juga sub-koleksi platform `GET /api/platform/modules/{key}/history`, yang path-nya ditetapkan FR-20.2/Q29 dan berupa kata benda tak terhitung. Linter mewajibkan segmen koleksi terakhir berakhiran `s`, kecuali path item yang menyatakan `x-erp-singleton: true`. Untuk `history`, ekstensi yang sama berarti "nama tetap, tidak dijamakkan". Ekstensi ini ditambahkan aditif ke api-guidelines §2 dan §13 di Task 18. Alternatifnya adalah mengganti nama route (`/pings`, `/summaries`), yang tidak wajar dan mengubah API yang sudah didokumentasikan. Rekomendasi: setujui ekstensi.
4. **★ Binary dev menolak command tulis pada DB tanpa penanda bila `ERP_ENV=production` (perlu konfirmasi owner).** FR-22.5 hanya menyebut DB *berpenanda* `production`. Namun pada DB kosong, `migrate up` dari binary dev dengan `ERP_ENV=production` justru *menulis* penanda `production`, sehingga binary dev yang seharusnya ditolak malah menginisialisasi DB production. Aturan guard (Task 2): flavor dev menolak bila penanda = `production`, **atau** penanda belum ada dan `ERP_ENV=production`. Binary release tidak terpengaruh. Rekomendasi: setujui.
5. **★ Cakupan metrik `erp.api.deprecated_usage` (perlu konfirmasi owner).** FR-23.3 meminta metrik "untuk setiap operasi/field yang ditandai `deprecated`", dan api-guidelines §14.3 menyebut "per operasi, per klien". Yang dikerjakan M2d:
   - operasi `deprecated: true`, dengan header `Deprecation: true` dan counter;
   - parameter query `deprecated: true` yang benar-benar dikirim klien.

   Yang ditunda:
   - **properti body request** deprecated, karena middleware harus mengurai body dua kali. Penegakannya ditempatkan di `DecodeJSON` bila kelak ada konsumen.
   - atribut **klien**, karena identitas klien (principal/PAT) baru ada di PRD-001.

   Properti respons tidak dapat diukur pemakaiannya di server. Rekomendasi: setujui.
6. **Tool terpin (NFR-02, NFR-06).**
   - Paket npm Spectral (`@stoplight/spectral-core`, `-functions`, `-rulesets`, `-parsers`, `-ref-resolver`), `openapi-typescript`, `json-schema-to-typescript`, dan `yaml` menjadi `devDependencies` `web/package.json` dengan **versi exact** (tanpa `^`, `~`, atau `latest`) dan terkunci di `bun.lock`. Ini mengikuti pola `@biomejs/biome` dan `dependency-cruiser`. Skrip runner berada di `web/` seperti `deps.ts`/`sbom.ts`, dan tidak pernah memakai `bun x`/`npx`.
   - `oasdiff` dan `kin-openapi` adalah library Go di `go.mod` root (terkunci `go.sum`). Keduanya hanya diimpor `internal/devtools/**`, sehingga tidak masuk binary release.
   - `goimports` dijalankan in-process (`golang.org/x/tools/imports`), bukan binary.
   - Biome dipanggil dari instalasi lokal.
   - Taskfile tidak memakai utilitas Unix.
   - Test `TestTooling_PinnedExactVersions` (Task 6) menolak rentang versi di `web/package.json`.
7. **Model kontrak di Go ditulis sendiri** (`internal/platform/apicontract`, `go.yaml.in/yaml/v3`), bukan kin-openapi. Alasannya: kontrak memakai OpenAPI 3.1 dan skema URI `erp:` yang tidak dikenal kin-openapi, sedangkan kebutuhannya (operasi, ekstensi `x-erp-*`, walk skema, bundel) kecil. kin-openapi hanya memuat **hasil bundel** sebagai masukan `oasdiff`. YAML dengan anchor/alias ditolak.
8. **Spectral dijalankan secara programatik oleh Bun.** `web/contracts-lint.ts` membaca `contracts/spectral.yaml`, lalu memetakan nama fungsi ke fungsi bawaan `@stoplight/spectral-functions` atau ke fungsi kustom `contracts/spectral-functions/index.ts`. Ruleset `spectral:oas` (`recommended`) diturunkan, dengan resolver kustom untuk skema `erp:` yang memetakan `erp:common`, `erp:platform`, dan `erp:<slug>` ke file kontraknya. Konteks manifest (slug, dependensi) dikirim Go lewat file katalog JSON. Pendekatan ini menghindari bundler ruleset (rollup), yang tidak terpin dan rapuh di Windows.
9. **Kesesuaian route ↔ kontrak** (FR-23.2):
   - Modul dicocokkan **persis** pada method, path penuh `/api/{slug}/…`, `x-erp-permission`, dan `x-erp-public`.
   - `platform.core` dicocokkan lewat **kesetaraan dispatch**: path contoh setiap operasi didispatch `http.ServeMux` yang dibangun dari route terdaftar, sehingga route gabungan `GET /api/platform/modules/{key}/{sub}` (Q29) melayani operasi `…/{key}/history` dan `…/operations/{id}`. Pengecualian ini hanya berlaku untuk `platform.core` dan sudah tercatat di Q29.
   - `GET /api/bootstrap` (route `httpserver`, bukan registry) didaftarkan ke pemeriksaan sebagai route platform publik.
10. **Klasifikasi route = klasifikasi tertinggi respons di kontrak.** Ini menuntaskan keputusan 3 M2c. Klasifikasi kontrak dihitung secara rekursif atas skema respons 2xx (properti bersarang, `items`, `allOf`/`oneOf`/`anyOf`, `$ref` lintas dokumen, aman terhadap siklus). Properti tanpa anotasi dianggap `internal`, dan `RouteInfo.Classification == ""` juga dianggap `internal`. Ketidaksamaan di kedua arah gagal: route lebih rendah berarti `no-store` hilang, sedangkan route lebih tinggi berarti kontrak tidak lagi menjadi sumber kebenaran.
11. **`x-erp-permission` diperiksa terhadap `capabilities` manifest.** `resources` belum dirinci sampai PRD-001, dan saat itu `capabilities` diturunkan dari `resources` (FR-06), sehingga pemeriksaan ini tetap benar. Format permission: `^<slug>\.[a-z][a-z0-9-]*\.[a-z][a-z0-9_]*$` untuk modul, dan `platform.<resource>.<action>` untuk platform.
12. **Breaking change** (FR-23.3, api-guidelines §14):
    - Perubahan berlevel `oasdiff` **WARN ke atas** menggagalkan CI, karena §14.2 melarang menghapus field apa pun, termasuk field respons opsional yang oleh `oasdiff` dinilai WARN.
    - Baseline adalah tag semver tertinggi `modules/<dir>/vX.Y.Z` (prerelease diabaikan; root: `vX.Y.Z`). Kontrak baseline dibundel dari pohon tag yang sama, termasuk `erp:common` dan dependensinya.
    - Modul tanpa tag dikecualikan.
    - Perubahan breaking diizinkan bila PR berlabel `api-breaking-approved` (env `ERP_API_BREAKING_APPROVED=true`) **dan** major `version` manifest lebih besar daripada major tag baseline. Syarat "bukti tidak dipakai 6 bulan" diperiksa reviewer, bukan alat.
13. **Penamaan modul hasil generate.** Argumen `make:module` adalah slug kebab-case (`stock-opname`). Direktori, nama package Go, dan akhiran module path memakai slug **tanpa `-`** (`stockopname`), mengikuti preseden `exampleaddon`. Turunan lainnya:
    - key: `<publisher>.<slug>` (default publisher `acme`, flag `--publisher`);
    - prefix permission/command/API: slug;
    - tabel dan goose: `stock_opname_*` dan `goose_module_stock_opname`;
    - npm: `@erpora/module-stockopname` (F2).

    Slug yang dicadangkan registry ditolak. Bentrok direktori (`stockopname` vs `stock-opname`), slug, atau key ditolak sebelum ada file yang ditulis.
14. **Perubahan file yang sudah ada selalu terstruktur** (FR-22.3), tidak pernah lewat pencarian teks bebas:
    - `go.work` dan `go.mod` root lewat `golang.org/x/mod/modfile`;
    - file Go lewat penanda eksplisit (`// erp:generated:modules`, `// erp:generated:routes`, `// erp:generated:commands`), dengan impor ditambah oleh `astutil.AddImport`;
    - `module.json` dan `package.json` lewat edit JSON yang mempertahankan urutan kunci;
    - `openapi.yaml` lewat `yaml.Node`;
    - `web/src/app/modules.ts` lewat penanda.

    Bila penanda tidak ditemukan, generator berhenti tanpa menulis apa pun dan mencetak baris yang harus ditambahkan manual. Modul `example` dan `exampleaddon` menerima penanda `routes`/`commands` karena keduanya adalah sumber template.
15. **Kerangka `make:module` diturunkan dari `example`.** Isinya satu resource contoh `items`:
    - migrasi `0001_create_<snake>_items.sql`;
    - lapisan domain, application, infrastructure, dan presentation beserta test-nya;
    - `GET /items` berpagination;
    - command `recount`, `DevSeeder`, dan `openapi.yaml`.

    Dengan kerangka ini, modul baru langsung dapat di-install/enable, dan seluruh lapisan Clean Architecture punya contoh yang lolos gate. `--depends` menambah dependensi manifest (`^<major>.<minor>` dari versi dependensi), `require` + `replace` di `go.mod` modul, dan file `deps.go` berisi blank import `erpora/modules/<dep>/contract` dengan komentar. Blank import ini membuat D8 dan `go mod tidy` stabil sampai kontrak benar-benar dipakai. Dependensi frontend `@erpora/module-<dep>` hanya ditambahkan bila dependensi itu punya `web/` (Q27).
16. **Guard command tulis.** Guard hanya membaca (`SELECT` penanda) dan menolak dengan `ExitError{Code: 3}` (kelas exit "prasyarat tidak terpenuhi", sama dengan command modul yang tidak `enabled`). Guard dipasang di `serve`, `migrate up`, mutasi `modules` (`install|enable|disable|uninstall|upgrade|retry`), command modul, dan `db:*`. Command read-only (`inspect`, `modules list|status|plan|history`, `migrate status`, `version`, `list`) tidak memeriksa penanda.
17. **`db:fresh`** menghapus semua objek (tabel, view, sequence, tipe enum/domain, fungsi) di schema aktif DB tunggal (single-tenant: control = tenant default) dalam satu transaksi. Skema tidak di-`DROP SCHEMA`, agar hak schema milik DBA tetap utuh. Urutan langkahnya: cek `ERP_ENV` **sebelum** membuka DB → baca penanda → konfirmasi mengetik `current_database()` (atau `--yes`) → drop → `migrate up` → install + enable semua modul → seed (opsional).
18. **File hasil generate di-commit, kecuali `contracts/dist/`** (sudah di-`.gitignore`, sesuai FR-23.4 "artifact build"). Drift diperiksa dengan `erpora contracts:generate --check`, yang membangkitkan file di memori, membandingkannya dengan disk, lalu exit 1 sambil mendaftar file basi. Pemeriksaan ini lebih presisi daripada `git diff` dan juga berjalan di direktori sementara. File `*.gen.ts` dikecualikan dari lint Biome (format tetap diperiksa).
19. **Client fetch bertipe berada di `@erpora/platform-contracts`** (`createClient<Paths>()`), karena F2 hanya mengizinkan frontend modul mengimpor paket `@erpora/*` tertentu, sehingga library pihak ketiga seperti `openapi-fetch` tidak boleh diimpor dari modul.
20. **Validasi tag rilis (FR-14)** memakai command dev `erpora release:verify-tag <tag>`, sebuah tambahan aditif pada tabel FR-22.2. Workflow `.github/workflows/release.yml` (tag `modules/*/v*`, `sdk/v*`) menjalankannya. `SDKVersion` dibaca dari `sdk/module/version.go` lewat `go/parser`.
21. **Banner dev** ditulis ke **stderr** di setiap command, sehingga keluaran `--json` dan `completion` tetap bersih.
22. **Test yang butuh Bun** (lint, generate) di-*skip* bila `bun` tidak ada, **kecuali** `ERP_REQUIRE_BUN=1`, yang membuatnya gagal. CI selalu memasang env ini.

## Global Constraints

- Go module path: root `erpora`, SDK `erpora/sdk`, modul `erpora/modules/<dir>`. Semua `go.mod` (termasuk hasil generate): `go 1.27`, `toolchain go1.27.3`. Modul hasil generate: `replace erpora/sdk => ../../sdk`, tanpa `gorm.io/driver/*`.
- `internal/devtools/**` hanya boleh diimpor oleh `cmd/erpora/devtools_on.go` (aturan arch `DEVTOOLS`). `go list -deps ./cmd/erpora` tanpa tag tidak boleh memuat `erpora/internal/devtools`.
- Exit code CLI: `0` sukses; `1` error umum, temuan lint/check, dan breaking change; `2` `ValidationError` konfigurasi/argumen; `3` prasyarat tidak terpenuhi (modul tidak `enabled`, penanda lingkungan menolak). Kode lifecycle FR-20.1 tetap.
- Penanda lingkungan: `platform_state.environment` ∈ {`development`, `production`}, ditulis sekali oleh `migrate up` (`ON CONFLICT DO NOTHING`), dan tidak pernah diubah oleh CLI.
- Kontrak: `contracts/openapi/common.yaml` (hanya komponen), `contracts/openapi/platform.yaml`, `modules/<dir>/openapi.yaml`. Semua `openapi: 3.1.0`. Rujukan lintas dokumen hanya `erp:<common|platform|slug>#/<pointer>`; path file relatif dilarang. Ekstensi yang dikenal: `x-erp-permission`, `x-erp-public`, `x-erp-classification` (`public|internal|confidential|highly_confidential`), `x-erp-singleton`, `x-erp-allow-put`, `x-erp-filterable`, `x-erp-sortable`, `x-erp-field-group`, `x-erp-extensible-enum`.
- Namespace skema pada spec gabungan: `<slug>.<NamaSkema>` (`common` dan `platform` tidak diberi namespace).
- Header file hasil generate. TS: `// Dibangkitkan oleh \`erpora contracts:generate\` dari <sumber>. JANGAN DIEDIT.` JSON: kunci `"x-erp-generated": "erpora contracts:generate; jangan diedit"`. Go/YAML hasil `make:*` tidak diberi header, karena file itu milik developer.
- Penanda generator (satu baris, persis): `// erp:generated:modules` (`internal/app/modules.go`), `// erp:generated:routes` dan `// erp:generated:commands` (`modules/<dir>/module.go`), `// erp:generated:web-imports` dan `// erp:generated:web-modules` (`web/src/app/modules.ts`).
- Template generator: `internal/devtools/templates/**`, di-embed, `text/template` dengan `Option("missingkey=error")`. Generator **tidak pernah** menimpa file tanpa `--force`. `--dry-run` tidak menulis apa pun dan tidak menjalankan post-step.
- ID rule Spectral berawalan `erp-`. Setiap pesan rule menyebut sumbernya (`api-guidelines §N`, `PRD-000 FR-23.1`, atau `security-guidelines §2`). Semua rule `erp-*` berseverity `error`.
- Versi tool: paket npm baru exact di `web/package.json` + `bun.lock`; library Go baru di `go.mod` root. Dilarang `bun x`, `npx`, dan `go run <pkg>@<ver>` di Taskfile/CI/kode.
- Taskfile: tanpa `rm`/`cp`/`sed`/`grep`/`mv`. Operasi file ditulis di Go/Bun. Windows: `-race` dilewati.
- Tidak ada breaking change API HTTP. Perubahan kontrak di M2d hanya menambah dokumentasi (contoh, kode error, klasifikasi) dan komponen `common` baru.
- TDD; satu commit per task; pesan konvensional diakhiri baris kosong + `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Gate sebelum commit: `task lint arch check:gowork-off test`, ditambah `task test:integration` untuk task yang menyentuh DB (Task 2, 5, 15) dan `task contracts:check` sejak Task 7. Setelah mengubah dependensi Go jalankan `task tidy`; setelah mengubah `package.json` jalankan `bun install` dan commit `bun.lock`.

## File Structure (M2d)

```
cmd/erpora/{devtools_on.go, devtools_off.go, root.go, release_flavor_test.go, devtools_flavor_test.go}                       # Task 1
internal/devtools/{devtools.go, doc.go}; internal/devtools/commands/{commands.go, groups.go}                                  # Task 1
internal/archcheck/{graph.go, load.go, rules.go, rules_test.go}; internal/archcheck/testdata/devtools_import/**                # Task 1
internal/platform/migrate/{state.go, state_integration_test.go}; internal/app/{envguard.go, envguard_test.go, app.go, command.go};
  cmd/erpora/{migrate.go, modules.go, inspect.go}; tests/integration/envmarker_test.go                                        # Task 2
sdk/module/{module.go, contract_test.go}; contracts/embed.go; internal/platform/modules/registry/{registry.go, errors.go, contract_test.go};
  internal/platform/apicontract/{doc.go, parse.go, ref.go, set.go, operations.go, classify.go, bundle.go, merge.go, conformance.go, *_test.go,
  testdata/**}; go.mod                                                                                                         # Task 3
modules/example/{openapi.yaml, module.go, module_test.go}; modules/exampleaddon/{openapi.yaml, module.go, module_test.go};
  contracts/openapi/{common.yaml, platform.yaml}; internal/app/{registry.go, contract_conformance_test.go}                    # Task 4
internal/platform/apicontract/{validate.go, validate_test.go}; internal/app/contract_response_integration_test.go             # Task 5
contracts/spectral.yaml; contracts/spectral-functions/{index.ts, paths.ts, naming.ts, envelope.ts, operations.ts, refs.ts,
  classification.ts}; web/{contracts-lint.ts, package.json, tsconfig.json}; bun.lock;
  internal/devtools/contracttools/{repo.go, bun.go, lint.go, lint_test.go, tooling_test.go, testdata/lint/**};
  internal/devtools/commands/contracts.go; contracts/openapi/platform.yaml; contracts/README.md                               # Task 6
web/contracts-generate.ts; packages/platform-contracts/{package.json, tsconfig.json, src/{index.ts, client.ts, common.gen.ts,
  platform.gen.ts}}; packages/module-sdk/{package.json, tsconfig.json, src/{index.ts, manifest.gen.ts, manifest.check.ts}};
  internal/devtools/contracttools/{generate.go, generate_test.go}; biome.json; Taskfile.yml                                   # Task 7
internal/devtools/contracttools/{breaking.go, git.go, breaking_test.go}; go.mod                                              # Task 8
internal/platform/modules/registry/{registry.go, deprecation_test.go}; internal/platform/modules/runtime/{deprecation.go,
  deprecation_test.go, mount.go}; internal/app/assemble.go                                                                   # Task 9
internal/devtools/gen/{plan.go, apply.go, marker.go, format.go, diff.go, names.go, render.go, golden.go, *_test.go};
  internal/devtools/templates/{embed.go, migration/**}; internal/devtools/commands/make.go                                   # Task 10
internal/devtools/gen/{module.go, module_test.go, testdata/repo/**, testdata/golden/module_*/**};
  internal/devtools/templates/module/**; web/src/app/modules.ts; web/tsconfig.json                                           # Task 11
internal/devtools/gen/{layers.go, layers_test.go, testdata/golden/{entity,usecase,repository,contract,command}/**};
  internal/devtools/templates/{entity,usecase,repository,contract,command}/**                                              # Task 12
internal/devtools/gen/{handler.go, handler_test.go, openapi_edit.go, testdata/golden/handler_*/**};
  internal/devtools/templates/handler/**; modules/example/module.go, modules/exampleaddon/module.go (penanda)               # Task 13
internal/devtools/modcheck/{check.go, check_test.go}; internal/devtools/doctor/{doctor.go, doctor_test.go};
  internal/devtools/commands/{module.go, doctor.go}                                                                          # Task 14
internal/devtools/dbtools/{fresh.go, seed.go, drop.go, confirm.go, *_test.go}; internal/app/{seed.go, seed_integration_test.go};
  internal/devtools/commands/db.go; tests/integration/dbtools_test.go                                                        # Task 15
tests/generator/{make_module_test.go, copy.go, README.md}; Taskfile.yml                                                      # Task 16
internal/devtools/release/{verify.go, verify_test.go}; internal/devtools/commands/release.go;
  .github/workflows/{ci.yml, release.yml}; Taskfile.yml                                                                     # Task 17
docs/module-sdk/{generators.md, contracts.md, README.md, creating-a-module.md}; docs/architecture/api-guidelines.md;
  docs/security/security-guidelines.md; docs/operations/deployment.md; docs/prd/PRD-000-inisialisasi-project.md (Q31, FR-22.2,
  FR-23.3); docs/adr/0006-module-manifest-sdk-semver.md; CLAUDE.md; README.md; contracts/README.md;
  packages/*/README.md; docs/superpowers/plans/2026-10-22-m2-overview.md                                                    # Task 18
(tanpa file baru; verifikasi)                                                                                                # Task 19
```

## Review Focus

1. **`make:module` dengan slug berstrip** (`stock-opname`). Hasil yang diharapkan:
   - direktori dan package `stockopname`;
   - key `acme.stock-opname`;
   - permission `stock-opname.items.read`;
   - route `/api/stock-opname/items`;
   - tabel `stock_opname_items` dan `goose_module_stock_opname`;
   - npm `@erpora/module-stockopname`;
   - command `stock-opname:recount`.

   `make:module stockopname` setelahnya ditolak sebelum ada file yang ditulis. Test: `TestParseModuleName_Table` (T10), `TestMakeModule_Golden_Hyphenated` dan `TestMakeModule_RejectsCollisions` (T11), `TestMakeModule_PassesAllGates` (T16).
2. **Idempotensi dan proteksi timpa generator**:
   - target yang sudah ada menggagalkan seluruh rencana sebelum file pertama ditulis;
   - `--force` hanya menimpa file `create`;
   - penanda hilang menghentikan generator tanpa menulis apa pun;
   - baris registrasi tidak tersisip dua kali;
   - `--dry-run` tidak mengubah checksum repo.

   Test: `TestApply_ExistsFailsBeforeAnyWrite`, `TestApply_DryRunWritesNothing`, `TestInsertAtMarker_Table` (T10), `TestMakeHandler_ExistingOperationRejected` (T13), `TestMakeModule_PassesAllGates` (checksum, T16).
3. **Developer tools bocor ke binary release**: `go list -deps` tanpa tag, `make:module` menghasilkan `unknown command`, `version` menampilkan `flavor: release`, banner tidak muncul, aturan `DEVTOOLS` berlaku per file, dan pemeriksaan dijalankan terhadap artifact CI. Test: `TestReleaseFlavor_NoDevtoolsPackages`, `TestReleaseFlavor_UnknownDevCommands`, `TestDevtoolsRule_OnlyEntryFile` (T1), langkah smoke CI (T17).
4. **Command tulis binary dev terhadap DB berpenanda `production`**: `serve`, `migrate up`, `modules enable`, `example:recount`, dan `db:fresh` ditolak dengan exit 3 tanpa perubahan data. `inspect routes` dan `modules list` tetap berjalan. `db:fresh` dengan `ERP_ENV=production` ditolak sebelum DB dibuka. Binary release tidak terpengaruh. Test: `TestDecideDevWrite_Table` (T2), `TestDevBinary_ProductionMarker` (T2), `TestDBFresh_RefusedWithoutTouchingDB` (T15).
5. **Drift route ↔ kontrak**:
   - route tanpa operasi, operasi tanpa route, permission, dan `x-erp-public` berbeda;
   - permission tidak ada di `capabilities`;
   - klasifikasi berbeda, termasuk properti bersarang dan `$ref` lintas modul;
   - route gabungan platform `GET /modules/{key}/{sub}` yang melayani dua operasi kontrak;
   - pencocokan dispatch tidak berlaku untuk modul.

   Test: `TestCheckConformance_Table`, `TestResponseClassification_Nested` (T3), `TestContracts_RoutesMatchOperations`, `TestContracts_ClassificationMatchesRoute` (T4).

---

### Task 1: Kerangka devtools, flavor, banner, aturan `DEVTOOLS`, dan bukti binary release

**Files:**
- Create: `internal/devtools/devtools.go`, `internal/devtools/commands/{commands.go, groups.go}`, `cmd/erpora/release_flavor_test.go`, `cmd/erpora/devtools_flavor_test.go`, `internal/archcheck/testdata/devtools_import/**` (fixture: paket `erpora/cmd/erpora` dengan `devtools_on.go` yang mengimpor devtools (boleh) dan `bad.go` yang mengimpor devtools (dilanggar), serta paket `erpora/internal/platform/x` yang mengimpor devtools (dilanggar))
- Modify: `internal/devtools/doc.go`, `cmd/erpora/{devtools_on.go, devtools_off.go, root.go}`, `internal/archcheck/{graph.go, load.go, rules.go, rules_test.go}`, `Taskfile.yml` (task `test`: tambah `go test -tags devtools ./cmd/erpora/...`)

**Interfaces:**
- Produces:
  ```go
  // internal/devtools
  const Banner = "PERINGATAN: binary erpora flavor dev (developer tools aktif); jangan dipakai di production."
  type Options struct {
      EnvFile func() string // nilai flag --env-file saat command berjalan
  }
  // Register: buildinfo.SetFlavor("dev"); menambah grup "make" (Make), "db" (Database), "contracts" (Kontrak),
  // "devtools" (Developer tools); membungkus root.PersistentPreRunE sehingga Banner + "\n" ditulis ke
  // cmd.ErrOrStderr() sebelum setiap command (pre-run lama tetap dipanggil); lalu commands.Register(root, opts).
  func Register(root *cobra.Command, opts Options)

  // internal/devtools/commands
  const (GroupMake = "make"; GroupDB = "db"; GroupContracts = "contracts"; GroupModule = "module"; GroupDevtools = "devtools")
  func Register(root *cobra.Command, opts Options) // Task 1: belum ada command; task berikut menambah
  type Options struct{ EnvFile func() string }

  // cmd/erpora
  func registerDevtools(root *cobra.Command, opts *rootOpts) // devtools_on: devtools.Register(root, devtools.Options{EnvFile: func() string { return opts.envFile }}); devtools_off: no-op

  // internal/archcheck
  type Package struct { /* …lama… */ FileImports map[string][]string } // nama file (relatif Dir) → impor; diisi parser.ImportsOnly untuk semua GoFiles semua varian build
  // Aturan "DEVTOOLS": impor erpora/internal/devtools[/...] hanya boleh dari paket erpora/internal/devtools/** sendiri atau dari
  // erpora/cmd/erpora file devtools_on.go. Detail: "<file>: impor <path> hanya boleh dari cmd/erpora/devtools_on.go (PRD-000 FR-22.1)".
  ```

- [ ] **Step 1: Tulis test yang gagal**
  - `release_flavor_test.go` (`//go:build !devtools`):
    - `TestReleaseFlavor_NoDevtoolsPackages`: `exec.Command("go", "list", "-deps", "erpora/cmd/erpora")` (tanpa `-tags`) → tidak ada baris berawalan `erpora/internal/devtools`. Test juga memastikan daftar tidak kosong dan memuat `erpora/internal/app`.
    - `TestReleaseFlavor_UnknownDevCommands`: untuk `make:module x`, `make:handler`, `db:fresh`, `db:seed`, `contracts:lint`, `contracts:generate`, `module:check`, `doctor`, `release:verify-tag v1`, `newRootCmd(out, errOut)` + `Execute` menghasilkan error berisi `unknown command` dan `exitCode(err) == 1`.
    - `TestReleaseFlavor_VersionNoBanner`: `version` → stdout memuat `flavor: release`, stderr kosong.
  - `devtools_flavor_test.go` (`//go:build devtools`): `TestDevFlavor_BannerAndVersion`: `version` → stderr diawali `devtools.Banner`, stdout memuat `flavor: dev`; `TestDevFlavor_ListGroups`: tabel `wantDevCommands` (map grup → nama command) dan `erpora list` memuat judul grup `Runtime:`, `Inspect:`, `Modul:`. Setiap task yang menambah command menambah barisnya ke tabel ini.
  - `rules_test.go`: `TestDevtoolsRule_OnlyEntryFile` memuat fixture `testdata/devtools_import` → tepat 2 pelanggaran `DEVTOOLS` (`bad.go` di `cmd/erpora` dan paket platform), sedangkan `devtools_on.go` tidak dilanggar. `TestCheck_CleanRepoHasNoViolations` tetap hijau.
- [ ] **Step 2: Jalankan** `go test ./cmd/erpora/... ./internal/archcheck/...` dan `go test -tags devtools ./cmd/erpora/...` → FAIL (banner, aturan `DEVTOOLS`, dan `FileImports` belum ada).
- [ ] **Step 3: Implementasi** sesuai Interfaces. `devtools_on.go` kini hanya memanggil `devtools.Register`, dan `buildinfo.SetFlavor` pindah ke dalam `Register`.
- [ ] **Step 4: Jalankan** kedua perintah di Step 2 dan `task arch` → PASS. Bukti negatif manual: tambah sementara `import _ "erpora/internal/devtools"` di `cmd/erpora/serve.go` → `task arch` gagal dengan `DEVTOOLS`, lalu kembalikan perubahan itu.
- [ ] **Step 5: Commit** `feat(devtools): devtools entry point, dev banner, and DEVTOOLS arch rule with release-flavor proof`

### Task 2: Penanda lingkungan, guard command tulis binary dev, dan `inspect config`

**Files:**
- Create: `internal/app/envguard.go`, `internal/app/envguard_test.go`, `internal/platform/migrate/state_integration_test.go` (bila belum ada test `Environment`), `tests/integration/envmarker_test.go`
- Modify: `internal/platform/migrate/state.go`, `internal/app/{app.go, command.go}`, `cmd/erpora/{migrate.go, modules.go, inspect.go}`, `tests/integration/serve_test.go` (`TestMain` juga membangun binary release `erpBinRelease` tanpa tag)

**Interfaces:**
- Produces:
  ```go
  // internal/platform/migrate
  // Environment membaca platform_state.environment (read-only). ok=false bila tabel platform_state belum ada
  // (to_regclass IS NULL) atau barisnya belum ditulis. Nilai selain development|production → error.
  func Environment(ctx context.Context, db *database.DB) (env config.Env, ok bool, err error)

  // internal/app
  var ErrProductionDatabase = errors.New("database berpenanda production (atau belum berpenanda dengan ERP_ENV=production): binary dev menolak command yang menulis; pakai binary release")
  var ErrNotDevelopment = errors.New("db:fresh/db:seed hanya untuk ERP_ENV=development dan database berpenanda development")
  // decideDevWrite murni (tanpa I/O): flavor != "dev" → nil; marker production → ErrProductionDatabase;
  // !hasMarker && env == production → ErrProductionDatabase (keputusan ★4); selain itu nil.
  func decideDevWrite(flavor string, marker config.Env, hasMarker bool, env config.Env) error
  // GuardDevWrite: Environment(ctx, db) lalu decideDevWrite(buildinfo.Flavor(), …); penolakan dibungkus &ExitError{Code: 3}.
  func GuardDevWrite(ctx context.Context, db *database.DB, cfg config.Config) error
  // DestructiveEnvAllowed: cfg.Env != development → &ExitError{Code: 3, Err: ErrNotDevelopment}. Dipanggil SEBELUM DB dibuka.
  func DestructiveEnvAllowed(cfg config.Config) error
  // GuardDestructive: !hasMarker || marker != development → &ExitError{Code: 3, Err: ErrNotDevelopment} (pesan menyarankan `erpora migrate up`).
  func GuardDestructive(ctx context.Context, db *database.DB, cfg config.Config) error
  ```
- Titik pasang `GuardDevWrite` (keputusan 16):
  - `app.Run`: setelah DB dibuka, sebelum reconcile.
  - `migrate up`: setelah `openDB`, sebelum `UpWithTenant`.
  - `cmd/erpora/modules.go`: `opCmd` dan `retryCmd` sebelum `Execute`. `modulesBackend` memperoleh method `GuardWrite(ctx context.Context) error`.
  - `app.RunModuleCommand`: setelah DB dibuka, sebelum pemeriksaan status.
- `inspect config` menambah kunci `database.environment`. Nilainya hasil `Environment` dengan timeout 3 s, atau `unknown (<alasan singkat tanpa DSN>)` bila DSN kosong/DB tak terjangkau. Perintah ini tetap read-only dan selalu exit 0.

- [ ] **Step 1: Tulis test yang gagal**
  - `envguard_test.go`:
    - `TestDecideDevWrite_Table` mencakup flavor `release` × semua kombinasi → nil; dev + marker `production` (env apa pun) → `ErrProductionDatabase`; dev + marker `development` + env `production` → nil (marker yang menentukan); dev + tanpa marker + env `production` → `ErrProductionDatabase`; dev + tanpa marker + env `development` → nil.
    - `TestDestructiveEnvAllowed`.
  - `state_integration_test.go` `TestEnvironment_States`: DB kosong → `ok=false`; setelah `Up(…, development)` → `development, true`; nilai `staging` yang di-`UPDATE` manual → error.
  - `tests/integration/envmarker_test.go` `TestDevBinary_ProductionMarker` (Review Focus 4):
    1. `erpBinRelease migrate up` dengan `ERP_ENV=production` dan `ERP_AUTHZ_STUB=deny` → penanda `production`.
    2. Binary dev dengan `ERP_ENV=development`: `serve`, `migrate up`, `modules enable acme.example`, dan `example:recount` masing-masing exit 3 dengan keluaran memuat `berpenanda production`. Jumlah baris `module_installations` dan `module_operations` tidak berubah.
    3. `inspect routes` dan `modules list` → exit 0.
    4. `erpBinRelease modules install acme.example` → exit 0 (binary release tidak terpengaruh).
  - `TestDevBinary_UnmarkedProductionRefused`: DB kosong, binary dev, `ERP_ENV=production`, `migrate up` → exit 3 dan `to_regclass('platform_state') IS NULL`.
  - `TestInspectConfig_ShowsDatabaseEnvironment`: setelah migrasi development → `database.environment=development`. Tanpa DB (`ERP_DATABASE_URL` ke port tertutup) → `database.environment=unknown (` dan exit 0.
- [ ] **Step 2: Jalankan** `go test ./internal/app/...` dan `task test:integration` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces.
- [ ] **Step 4: Jalankan** → PASS; `TestCLI_*` M2b tetap hijau (memakai `ERP_ENV=development`).
- [ ] **Step 5: Commit** `feat(cli): environment marker guard refuses dev-binary writes to production databases`

### Task 3: Kontrak di SDK/registry dan paket `apicontract`

**Files:**
- Create: `internal/platform/apicontract/{doc.go, parse.go, ref.go, set.go, operations.go, classify.go, bundle.go, merge.go, conformance.go}` + `*_test.go`, `internal/platform/apicontract/testdata/{common.yaml, platform.yaml, alpha.yaml, beta.yaml, cyclic.yaml, anchors.yaml}`, `internal/platform/modules/registry/contract_test.go`
- Modify: `sdk/module/module.go`, `sdk/module/contract_test.go`, `contracts/embed.go`, `internal/platform/modules/registry/{registry.go, errors.go}`, `go.mod` (`go.yaml.in/yaml/v3` menjadi require langsung)

**Interfaces:**
- Produces:
  ```go
  // sdk/module (aditif; SDKVersion tetap 0.1.0)
  // ContractSource diimplementasi modul yang membawa kontrak API-nya (PRD-000 FR-23.1): isi modules/<dir>/openapi.yaml yang di-embed.
  type ContractSource interface{ OpenAPI() []byte }

  // contracts
  var CommonOpenAPI, PlatformOpenAPI []byte // //go:embed openapi/common.yaml, openapi/platform.yaml

  // registry
  type Entry struct { Manifest module.Manifest; Module module.Module; Contract []byte } // platform.core: Options.PlatformContract
  type Options struct { /* …lama… */ PlatformContract []byte }
  // Problem baru "invalid_contract": OpenAPI() tidak lolos apicontract.Parse (dikumpulkan bersama problem lain, FR-05 aturan 7).

  // internal/platform/apicontract
  type Doc struct {
      Name string         // "common" | "platform" | slug modul
      Spec map[string]any // YAML → tipe JSON (map[string]any, []any, string, float64, int64, bool, nil)
  }
  // Parse menolak: YAML tidak valid, anchor/alias, kunci non-string, `openapi` bukan "3.1.x", path di luar "/api/".
  func Parse(name string, data []byte) (*Doc, error)
  type Ref struct{ Doc, Pointer string }
  // ParseRef: "#/a/b" → {from, "/a/b"}; "erp:<name>#/a/b" → {name, "/a/b"}; bentuk lain (path relatif, http) → error.
  func ParseRef(from, ref string) (Ref, error)
  func (d *Doc) ExternalRefs() []string // nama doc unik yang dirujuk lewat erp:, terurut

  type Set struct{ /* docs by name */ }
  func NewSet(docs ...*Doc) (*Set, error)                 // nama ganda → error
  func (s *Set) Doc(name string) (*Doc, bool)
  func (s *Set) Resolve(from string, ref string) (doc string, node any, err error) // $ref tak ditemukan → error berisi from+ref

  type Operation struct {
      Doc, Method, Path, OperationID, Permission string // Method huruf besar
      Public, Deprecated bool
      DeprecatedQueryParams []string
      Node map[string]any
  }
  func (d *Doc) Operations() []Operation // urut Path, lalu Method

  // ResponseClassification: klasifikasi tertinggi x-erp-classification atas skema respons 2xx operasi (rekursif:
  // properties, items, additionalProperties, allOf/oneOf/anyOf, $ref lokal & erp:, aman siklus). Tanpa anotasi → ClassInternal.
  // Nilai x-erp-classification tidak valid → error.
  func (s *Set) ResponseClassification(op Operation) (module.Classification, error)

  // Bundle: dokumen mandiri untuk satu doc; setiap $ref erp:<x>#/components/<kind>/<N> disalin ke components/<kind>/"<x>.<N>"
  // (common → "common.<N>") dan dirujuk lokal; $ref lokal tetap. Deterministik.
  func (s *Set) Bundle(name string) (map[string]any, error)
  // Merge: common + platform + modul (urutan order) → satu spec; komponen modul diberi namespace "<slug>.<N>" (rujukan ikut
  // ditulis ulang); path/operationId bentrok → error menyebut kedua doc.
  func (s *Set) Merge(order []string) (map[string]any, error)

  // conformance.go
  type RouteSpec struct{ Module, Method, Pattern, Permission string; Public bool; Classification module.Classification }
  type Mismatch struct{ Kind, Module, Method, Path, Detail string }
  // Kind: missing_operation | missing_route | permission_mismatch | public_mismatch | undeclared_permission | classification_mismatch | missing_contract
  type ConformanceInput struct {
      Set          *Set
      DocOf        map[string]string   // key modul → nama doc ("platform.core" → "platform")
      Routes       []RouteSpec
      Capabilities map[string][]string // key modul → capabilities manifest (platform.core: admin.Capabilities)
      DispatchEquivalent map[string]bool // hanya {"platform.core": true} (keputusan 9)
  }
  func CheckConformance(in ConformanceInput) []Mismatch // terurut (Module, Path, Method, Kind)
  ```

- [ ] **Step 1: Tulis test yang gagal**
  - `parse_test.go` `TestParse_Rejects`: anchor/alias, `openapi: 3.0.3`, path `/v1/x`, dan kunci integer → error. Kontrak valid → `Operations()` urut, `Permission`/`Public`/`Deprecated` terbaca.
  - `ref_test.go` `TestParseRef_Table`: `#/x`, `erp:common#/components/schemas/Uuid`, `erp:example-addon#/a`, `../x.yaml#/a` (error), `https://x/y#/a` (error), `erp:#/a` (error).
  - `classify_test.go` `TestResponseClassification_Nested` (Review Focus 5): properti `confidential` di `data.items[].customer.address` → `confidential`; `$ref` ke `erp:beta#/…/Secret` (`highly_confidential`) → `highly_confidential`; skema siklik → selesai tanpa overflow; tanpa anotasi → `internal`; nilai `secret` → error.
  - `merge_test.go` `TestMerge_SameSchemaNameTwoModules` (AC FR-23): `alpha` dan `beta` sama-sama punya `Item` → spec gabungan memuat `alpha.Item` dan `beta.Item`, dan rujukan di path masing-masing menunjuk skema bernamespace. `TestMerge_PathConflict` → error. `TestBundle_InlinesErpRefs`: hasil `Bundle("beta")` tidak memuat string `erp:`.
  - `conformance_test.go` `TestCheckConformance_Table`: route tanpa operasi → `missing_operation`; operasi tanpa route → `missing_route`; permission beda → `permission_mismatch`; `Public` beda → `public_mismatch`; permission tidak ada di capabilities → `undeclared_permission`; route `""` vs kontrak `confidential` → `classification_mismatch`; route `confidential` vs kontrak tanpa anotasi → `classification_mismatch`; platform `GET /api/platform/modules/{key}/{sub}` dengan operasi `…/{key}/history` dan `…/operations/{id}` → tidak ada mismatch; pola gabungan yang sama untuk modul `alpha` (tanpa `DispatchEquivalent`) → `missing_route` dan `missing_operation`.
  - `registry/contract_test.go` `TestBuild_StoresContracts`: modul `ContractSource` → `Entry.Contract` terisi, modul tanpa → nil, `platform.core` → `PlatformContract`; YAML rusak → `*registry.Error` berisi problem `invalid_contract`.
  - `sdk/module/contract_test.go`: `fakeModule` mengimplementasi `ContractSource`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/apicontract/... ./internal/platform/modules/registry/... ./sdk/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces. Path contoh untuk dispatch: setiap `{param}` diganti nilai `example` parameter itu, atau `x` bila tidak ada. `http.NewServeMux` dibangun dari `Routes` modul bersangkutan, lalu `mux.Handler(req)` dipakai untuk mendapatkan pola. `go mod tidy` root.
- [ ] **Step 4: Jalankan** → PASS; `task lint arch check:gowork-off test` hijau.
- [ ] **Step 5: Commit** `feat(contracts): OpenAPI contract model with erp: refs, classification, bundling, merge, and route conformance`

### Task 4: Kontrak modul referensi dan test kesesuaian route ↔ kontrak (FR-23.2)

**Files:**
- Create: `modules/example/openapi.yaml`, `modules/exampleaddon/openapi.yaml`, `internal/app/contract_conformance_test.go`
- Modify:
  - `modules/example/{module.go, module_test.go}` (`//go:embed openapi.yaml`, `OpenAPI()`, penanda `// erp:generated:routes` dan `// erp:generated:commands` di `Register`);
  - `modules/exampleaddon/{module.go, module_test.go}` (sama);
  - `contracts/openapi/common.yaml`: `Decimal` (string, pattern `^-?[0-9]+(\.[0-9]+)?$`), `Date` (`format: date`), parameter `Cursor` dan `Limit` (maks. 1000), skema `CursorPage`;
  - `contracts/openapi/platform.yaml`: `GET /api/bootstrap` dengan `x-erp-singleton: true`;
  - `internal/app/registry.go` (`PlatformContract: contracts.PlatformOpenAPI`).

**Interfaces:**
- Consumes: `apicontract.CheckConformance`, `registry.Registry.{Entries, Routes}`, `admin.Capabilities`.
- Produces (isi kontrak):
  - `example`:
    - `GET /api/example/ping` (`x-erp-singleton`, `example.ping.read`, `{data:{pong:boolean}}`);
    - `GET /api/example/notes` (`example.notes.read`, parameter `erp:common` `Page`/`PageSize`, `{data: Note[], page}`, `400`/`403`);
    - `components.schemas.Note` (`id` Uuid, `title` dengan `x-erp-classification: internal`, `version` int64, `createdAt`/`updatedAt` Timestamp);
    - `components.schemas.NoteCount` (`type: integer, format: int64`; deskripsi "API publik untuk modul lain").
  - `exampleaddon`: `GET /api/example-addon/summary` (`x-erp-singleton`, `example-addon.summary.read`, `{data:{noteCount: {$ref: 'erp:example#/components/schemas/NoteCount'}}}`).
  - Setiap operasi punya `operationId`, contoh respons, dan kode error, agar lolos linter Task 6.
- Test (`internal/app`):
  ```go
  func TestContracts_EveryEntryHasContract(t *testing.T)       // semua Entry (termasuk platform.core) punya Contract; mismatch "missing_contract" bila tidak
  func TestContracts_RoutesMatchOperations(t *testing.T)       // CheckConformance atas BuildRegistry() + GET /api/bootstrap publik; nol mismatch
  func TestContracts_PermissionsDeclaredInManifest(t *testing.T)
  func TestContracts_ClassificationMatchesRoute(t *testing.T)  // klasifikasi kontrak == RouteInfo.Classification ("" = internal)
  func TestContracts_ErpRefsOnlyToDependencies(t *testing.T)   // Doc.ExternalRefs() ⊆ {common} ∪ slug dependencies manifest (FR-23.1, juga ditegakkan linter)
  ```

- [ ] **Step 1: Tulis test yang gagal** sesuai daftar di atas. Selain itu, `module_test.go` kedua modul memeriksa `OpenAPI()` tidak kosong dan lolos `apicontract.Parse`. Untuk membuktikan AC "Menambah `Registrar.Handle` tanpa operasi di kontrak menggagalkan test", subtest `TestContracts_RoutesMatchOperations/extra_route` memakai `compiledModulesFn` pengganti yang membungkus `example` dan menambah `Handle(GET, "/extra", …)` → tepat satu `missing_operation`. Subtest `/extra_operation` menambah operasi pada salinan kontrak → `missing_route`.
- [ ] **Step 2: Jalankan** `go test ./internal/app/... ./modules/...` → FAIL (kontrak modul belum ada).
- [ ] **Step 3: Implementasi** kontrak, embed, `OpenAPI()`, dan penanda. Isi `platform.yaml` disesuaikan hanya bila test menemukan mismatch nyata (catat di laporan task).
- [ ] **Step 4: Jalankan** → PASS; `task lint arch check:gowork-off test` hijau.
- [ ] **Step 5: Commit** `feat(modules): per-module OpenAPI contracts with route, permission, and classification conformance tests`

### Task 5: Test kontrak respons (integration, FR-23.2)

**Files:**
- Create: `internal/platform/apicontract/{validate.go, validate_test.go}`, `internal/app/contract_response_integration_test.go`
- Modify: —

**Interfaces:**
- Produces:
  ```go
  // ResponseSchema mengompilasi skema respons op untuk status (fallback "default"; tidak ada → error) dari Bundle(op.Doc)
  // dengan santhosh-tekuri/jsonschema/v6 (draft 2020-12); kata kunci x-erp-* diabaikan; format uuid/date-time ditegakkan.
  func (s *Set) ResponseSchema(op Operation, status int) (*jsonschema.Schema, error)
  ```

- [ ] **Step 1: Tulis test yang gagal**
  - `validate_test.go` `TestResponseSchema_Validates`: body `{"data":[{"id":"…","title":"a","version":1,…}],"page":{…}}` lolos; `version: "1"` → error; field `page` hilang → error; status `418` tanpa `default` → error.
  - `contract_response_integration_test.go` (`//go:build integration`) `TestContractResponses_ReferenceModules`: harness `api_integration_test.go` memasang dan meng-enable kedua modul. Untuk setiap operasi di kontrak `example` dan `exampleaddon`, request ke path contoh harus menghasilkan status yang terdokumentasi, dan body harus lolos `ResponseSchema`. Kasus tambahan: `GET /api/example/notes?page=0` → `400` yang lolos skema `Error`; dengan modul `disabled` → `403`, body lolos skema `erp:common` `Error`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/apicontract/...` dan `task test:integration` → FAIL.
- [ ] **Step 3: Implementasi** `ResponseSchema` (loader `jsonschema` dengan dokumen bundel in-memory, tanpa akses jaringan atau file).
- [ ] **Step 4: Jalankan** → PASS.
- [ ] **Step 5: Commit** `test(contracts): validate reference module responses against their OpenAPI schemas`

### Task 6: Linter Spectral dan `erpora contracts:lint` (FR-23.3, FR-24.2 AC linter)

**Files:**
- Create: `contracts/spectral.yaml`, `contracts/spectral-functions/{index.ts, paths.ts, naming.ts, envelope.ts, operations.ts, refs.ts, classification.ts}`, `web/contracts-lint.ts`, `internal/devtools/contracttools/{repo.go, bun.go, lint.go, lint_test.go, tooling_test.go}`, `internal/devtools/contracttools/testdata/lint/<kasus>/{contracts/openapi/common.yaml, modules/<x>/module.json, modules/<x>/openapi.yaml}`, `internal/devtools/commands/contracts.go`
- Modify: `web/package.json` (devDependencies exact), `web/tsconfig.json` (include `contracts-lint.ts`, `../contracts/spectral-functions/**/*.ts`), `bun.lock`, `contracts/openapi/platform.yaml` (contoh, kode error, `operationId`), `contracts/README.md`, `cmd/erpora/devtools_flavor_test.go` (`contracts:lint`)

**Interfaces:**
- Produces:
  ```go
  // internal/devtools/contracttools
  type ModuleInfo struct {
      Dir, Key, Slug, Version string
      Dependencies, Capabilities []string // Dependencies: key modul (tanpa platform.core)
      HasWeb bool                         // modules/<dir>/web/package.json ada
  }
  type Repo struct{ Root string; Set *apicontract.Set; Modules []ModuleInfo }
  // LoadRepo membaca contracts/openapi/{common,platform}.yaml, modules/*/module.json, modules/*/openapi.yaml (modul tanpa
  // openapi.yaml tetap tercatat; Set tidak memuatnya). Urutan modul: Dir.
  func LoadRepo(root string) (*Repo, error)
  func (r *Repo) Module(arg string) (ModuleInfo, bool) // cocok Dir, Slug, atau Key

  type BunRunner interface{ Run(ctx context.Context, dir string, args ...string) ([]byte, error) } // stdout; stderr diteruskan
  func FindBun() (BunRunner, error) // exec.LookPath("bun"); error: "bun tidak ditemukan; jalankan `erpora doctor`"
  type LintResult struct{ Target, File, Rule, Path, Message string; Line int }
  // Lint menulis katalog JSON ke file sementara: {root, targets:[{name, file, slug, dependencies:[slug]}], docs:{name: file}},
  // lalu menjalankan `bun web/contracts-lint.ts --catalog <file>`; stdout = JSON []LintResult. only "" = common+platform+semua modul.
  func Lint(ctx context.Context, repo *Repo, only string, bun BunRunner) ([]LintResult, error)
  ```
  CLI `erpora contracts:lint [module]` (grup `contracts`): mencetak `<file>:<line> <rule> <message>` per temuan, exit 1 bila ada temuan, exit 2 bila argumen modul tidak dikenal.
  ```ts
  // web/contracts-lint.ts: membaca --catalog; memuat contracts/spectral.yaml dengan paket `yaml`; extends [[spectral:oas, recommended]]
  // dari @stoplight/spectral-rulesets; memetakan then.function ke @stoplight/spectral-functions atau ke ekspor
  // contracts/spectral-functions/index.ts; resolver @stoplight/spectral-ref-resolver dengan resolver skema "erp" (erp:<name> → docs[name]);
  // menyisipkan konteks target ke dokumen sebagai `x-erp-lint-context: {slug, dependencies}` sebelum lint; mencetak JSON hasil; exit 0 (Go yang menilai); exit 2 bila runner gagal.
  ```
- Rule `contracts/spectral.yaml` (semua `error`). Pesan menyebut sumber dalam tanda kurung:

  | Rule | Memeriksa | Sumber |
  | --- | --- | --- |
  | `erp-path-module-prefix` | path modul diawali `/api/{slug}/`, path platform `/api/platform/` atau `/api/bootstrap` | api-guidelines §2, FR-23.1 |
  | `erp-path-kebab-case` | segmen statis `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`, parameter `{camelCase}` | §2 |
  | `erp-path-no-trailing-slash` | path tidak diakhiri `/` | §2 |
  | `erp-path-no-crud-verb` | segmen bukan `get|list|create|update|delete|remove|add|edit|fetch|save|new` (juga sebagai awalan camelCase, mis. `getItems`) | §2 |
  | `erp-path-collection-plural` | segmen koleksi terakhir berakhiran `s`, kecuali path item ber-`x-erp-singleton: true` atau segmen aksi setelah `{id}` dengan method `POST` | §2 |
  | `erp-path-max-nesting` | paling banyak satu sub-koleksi setelah `{id}` | §2 |
  | `erp-operation-permission` | ada `x-erp-permission` cocok `^<slug>\.[a-z][a-z0-9-]*\.[a-z][a-z0-9_]*$`, atau `x-erp-public: true` | §1, §13 |
  | `erp-operation-id` | `operationId` ada, camelCase, unik dalam doc | §13 |
  | `erp-property-camel-case` | nama properti skema camelCase | §4.1 |
  | `erp-enum-snake-case` | nilai enum string snake_case | §4.1 |
  | `erp-decimal-not-number` | properti `type: number`, atau properti bernama `/(amount|total|subtotal|price|cost|qty|quantity|balance|tax|discount|rate)$/i` yang tidak `$ref` `Decimal` | §4.1 |
  | `erp-success-envelope` | skema respons 2xx JSON punya properti wajib `data` (kecuali `204`) | §4.2 |
  | `erp-error-standard` | respons 4xx/5xx `$ref` ke `erp:common#/components/responses/*` atau skema `erp:common#/components/schemas/Error` | §8 |
  | `erp-collection-pagination` | `GET` koleksi (path tanpa `{…}` di akhir, `data` array) memakai `Page`+`PageSize` atau `Cursor`+`Limit` | §6 |
  | `erp-post-idempotency-key` | `POST` punya parameter header `Idempotency-Key` | §7.2 |
  | `erp-if-match` | `PATCH`, `DELETE`, dan aksi `POST …/{id}/<aksi>` punya `If-Match` | §7.1 |
  | `erp-no-put` | `put` hanya bila operasi ber-`x-erp-allow-put: true` | §3 |
  | `erp-ref-dependency` | `$ref: erp:<x>` hanya `common` atau slug dependensi manifest (modul), atau `common` (platform) | FR-23.1 |
  | `erp-examples` | setiap respons 2xx dan `requestBody` JSON punya `example`/`examples` | §13 |
  | `erp-error-codes-documented` | ada ≥ 1 respons 4xx; operasi non-publik punya `403`; operasi dengan parameter path punya `404`; operasi dengan `requestBody` punya `400` dan `422` | §13 |
  | `erp-classification-valid` | `x-erp-classification` ∈ empat kelas | security-guidelines §2 |
  | `erp-no-highly-confidential-in-response` | properti `highly_confidential` (rekursif, ikut `$ref`) di skema respons | FR-24.2, security-guidelines §2, §3.4 |
  | `erp-no-sensitive-in-url` | parameter `in: path|query` yang bernama sensitif atau berklasifikasi `highly_confidential` | FR-24.2 |
  | `erp-sensitive-needs-classification` | properti/parameter bernama `/^(password|passwd|secret|token|accessToken|refreshToken|apiKey|privateKey|nik|npwp|bankAccount|salary)$/i` wajib ber-`x-erp-classification` minimal `confidential`; `password|secret|token|apiKey|privateKey|accessToken|refreshToken` minimal `highly_confidential` | FR-24.2, security-guidelines §2 |

  Rule `spectral:oas` yang boleh dimatikan hanya `info-contact`, `info-license`, `oas3-api-servers`, `operation-tags`, dan `openapi-tags`. Untuk `common.yaml`, `oas3-unused-component` dimatikan lewat `overrides`.

- [ ] **Step 1: Tulis test yang gagal**
  - `tooling_test.go` `TestTooling_PinnedExactVersions`: semua `dependencies`/`devDependencies` di `web/package.json` dan `packages/*/package.json` cocok `^\d+\.\d+\.\d+$` atau `workspace:*`.
  - `lint_test.go` (helper `requireBun(t)`: skip bila `bun` tidak ada, kecuali `ERP_REQUIRE_BUN=1` → `t.Fatal`):
    - `TestLint_RepositoryContractsClean` (AC FR-23: kontrak `example` dan `exampleaddon` lolos, termasuk `erp:example#/…`): `LoadRepo("../../..")` → nol temuan.
    - `TestLint_Fixtures` dengan tabel kasus → rule dan potongan pesan yang diharapkan:
      - `ref_not_dependency` (`example` merujuk `erp:exampleaddon#/…`) → `erp-ref-dependency`, `FR-23.1`;
      - `number_total` (`total: {type: number}`) → `erp-decimal-not-number`, `api-guidelines §4.1`;
      - `crud_verb_path` (`/api/example/getItems`) → `erp-path-no-crud-verb` dan `erp-path-kebab-case`, `api-guidelines §2`;
      - `missing_permission` → `erp-operation-permission`, `api-guidelines §1`;
      - `password_response` (`password` di respons) → `erp-no-highly-confidential-in-response` atau `erp-sensitive-needs-classification`;
      - `token_query` (`?token=`) → `erp-no-sensitive-in-url`;
      - `put_without_flag` → `erp-no-put`;
      - `missing_envelope` → `erp-success-envelope`;
      - `collection_without_page` → `erp-collection-pagination`;
      - `post_without_idempotency` → `erp-post-idempotency-key`;
      - `singular_without_flag` (`/api/example/report` tanpa `x-erp-singleton`) → `erp-path-collection-plural`.

      Setiap kasus hanya memicu rule yang diharapkan (tidak ada temuan lain).
  - `devtools_flavor_test.go`: `contracts:lint` ada di grup `Kontrak:`.
- [ ] **Step 2: Jalankan** `ERP_REQUIRE_BUN=1 go test ./internal/devtools/contracttools/...` → FAIL.
- [ ] **Step 3: Implementasi.**
  1. **Spike dulu:** pastikan `@stoplight/spectral-core` dan resolver berjalan di Bun 1.4.2 pada Windows dan Linux untuk satu kontrak dengan `$ref: erp:common#/…`. Bila tidak berjalan, hentikan task dan laporkan sebelum menulis rule.
  2. Tambah devDependencies exact, `bun install`.
  3. Tulis ruleset, fungsi kustom, dan runner.
  4. Rapikan `platform.yaml` sampai `TestLint_RepositoryContractsClean` lolos.
- [ ] **Step 4: Jalankan** `ERP_REQUIRE_BUN=1 go test ./internal/devtools/...`, `task web:check`, `task erpora -- contracts:lint` → PASS, exit 0.
- [ ] **Step 5: Commit** `feat(devtools): Spectral contract linter with api-guidelines and data-classification rules`

### Task 7: `erpora contracts:generate`, paket TS, dan pemeriksaan drift (FR-23.4, FR-06 AC)

**Files:**
- Create: `web/contracts-generate.ts`, `packages/platform-contracts/{package.json, tsconfig.json, src/index.ts, src/client.ts}`, `packages/module-sdk/{package.json, tsconfig.json, src/index.ts, src/manifest.check.ts}`, `internal/devtools/contracttools/{generate.go, generate_test.go}`
- Generated (di-commit): `packages/platform-contracts/src/{common.gen.ts, platform.gen.ts}`, `packages/module-sdk/src/manifest.gen.ts`
- Modify: `web/package.json`, `bun.lock`, `biome.json` (override `**/*.gen.ts`: linter off, formatter on), `Taskfile.yml`, `internal/devtools/commands/contracts.go`, `packages/{module-sdk,platform-contracts}/README.md`, `cmd/erpora/devtools_flavor_test.go`
- Taskfile:
  - `contracts:lint` → `go run -tags devtools ./cmd/erpora contracts:lint`;
  - `contracts:generate`;
  - `contracts:check` (`contracts:lint`, `contracts:generate --check`, `contracts:breaking`; `contracts:breaking` ditambahkan di Task 8);
  - `web:check` → `bun run --filter '*' typecheck` dan `bun run --filter '*' lint`.

**Interfaces:**
- Produces:
  ```go
  type GenFile struct{ Path string; Content []byte } // Path relatif root, slash
  // Generate tanpa menulis: (1) packages/platform-contracts/src/{common,platform}.gen.ts dari Bundle("common"/"platform");
  // (2) modules/<dir>/web/api/contract.gen.ts untuk modul HasWeb (Bundle(slug) + `export const client = createClient<paths>()`);
  // (3) packages/module-sdk/src/manifest.gen.ts dari contracts/module.schema.json; (4) contracts/dist/openapi.json dari Merge.
  // Semua TS diformat Biome lokal; urutan GenFile terurut Path; deterministik.
  func Generate(ctx context.Context, repo *Repo, only string, bun BunRunner) ([]GenFile, error)
  func WriteGenerated(root string, files []GenFile) error
  // CheckGenerated: path yang isinya beda/tidak ada, ditambah *.gen.ts yatim (modul tanpa web/kontrak); contracts/dist/** dilewati.
  func CheckGenerated(root string, files []GenFile) (stale []string, err error)
  ```
  CLI: `erpora contracts:generate [module] [--check]`. Dengan `--check`, file basi dicetak per baris dengan petunjuk `jalankan erpora contracts:generate`, lalu exit 1.
  ```ts
  // packages/platform-contracts/src/client.ts
  export interface ApiError { code: string; message: string; details?: unknown; requestId?: string; traceId?: string }
  export type ApiResult<T> = { ok: true; status: number; data: T; page?: unknown } | { ok: false; status: number; error: ApiError };
  export interface ClientOptions { baseUrl?: string; fetch?: typeof fetch; headers?: () => Record<string, string> }
  export function createClient<Paths>(opts?: ClientOptions): TypedClient<Paths>;
  // TypedClient<Paths>: get/post/patch/delete<P extends PathsWith<Paths, M>>(path: P, init?: { params?: …; body?: …; headers?: … })
  //   → Promise<ApiResult<SuccessData<Paths[P][M]>>>; path parameter diganti dari params.path (encodeURIComponent); credentials "same-origin".
  // packages/module-sdk/src/index.ts: export type { ModuleManifest, … } from "./manifest.gen";
  ```

- [ ] **Step 1: Tulis test yang gagal**
  - `generate_test.go` (`requireBun`):
    - `TestGenerate_Deterministic`: dua kali `Generate` atas repo nyata → byte identik; setiap `.gen.ts` diawali header; `contracts/dist/openapi.json` memuat `"x-erp-generated"`, `example.Note`, dan `example-addon` (namespace).
    - `TestCheckGenerated_DetectsStale` (AC "Mengubah kontrak tanpa `contracts:generate` menggagalkan CI"): salin repo minimal ke `t.TempDir()`, tulis hasil generate, ubah deskripsi operasi di `platform.yaml` → `stale == ["packages/platform-contracts/src/platform.gen.ts"]`; setelah generate ulang → kosong.
    - `TestGenerate_ModuleWithWeb`: fixture modul ber-`web/package.json` → `modules/<dir>/web/api/contract.gen.ts` memuat `createClient` dan path `/api/<slug>/…`.
  - `packages/module-sdk/src/manifest.check.ts`: objek literal bertipe `ModuleManifest` yang meniru manifest `example`, plus `// @ts-expect-error` untuk `trust: "community"` dan `slug: 1`. Pemeriksaannya berlangsung lewat `typecheck`.
- [ ] **Step 2: Jalankan** `ERP_REQUIRE_BUN=1 go test ./internal/devtools/contracttools/...` dan `task web:check` → FAIL.
- [ ] **Step 3: Implementasi.** Spike `json-schema-to-typescript` terhadap schema 2020-12. Bila `unevaluatedProperties`/`$defs` tidak didukung, praproses schema di runner (catat di laporan task). Lalu `task erpora -- contracts:generate` dan commit hasilnya.
- [ ] **Step 4: Jalankan** `task contracts:check` (lint + `--check`), `task web:check`, dan `task lint arch check:gowork-off test` → PASS.
- [ ] **Step 5: Commit** `feat(devtools): contracts:generate for platform, module, and manifest TS types with drift check`

### Task 8: Deteksi breaking change `erpora contracts:breaking` (oasdiff)

**Files:**
- Create: `internal/devtools/contracttools/{breaking.go, git.go, breaking_test.go}`
- Modify: `go.mod`/`go.sum` (`github.com/oasdiff/oasdiff`, `github.com/getkin/kin-openapi`: versi rilis terbaru saat eksekusi, dicatat exact), `internal/devtools/commands/contracts.go`, `Taskfile.yml` (`contracts:check` + `contracts:breaking`), `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  type GitReader interface {
      Tags(ctx context.Context, pattern string) ([]string, error) // `git tag --list <pattern>`
      Show(ctx context.Context, rev, path string) ([]byte, error)  // `git show <rev>:<path>`; tidak ada → ErrNotExist
  }
  func ExecGit(root string) GitReader
  type Change struct{ ID, Level, Method, Path, Text string } // Level: ERR | WARN (INFO tidak dilaporkan)
  type BreakingResult struct {
      Target, BaseTag string
      Changes []Change
      Skipped string // "belum pernah dirilis" bila tidak ada tag
      Approved bool  // label + MAJOR (keputusan 12)
  }
  type BreakingOptions struct{ Only string; Approved bool } // Approved dari env ERP_API_BREAKING_APPROVED=true
  // LatestReleaseTag: semver tertinggi (golang.org/x/mod/semver) dari "<prefix>v*", prerelease diabaikan; "" bila tidak ada.
  func LatestReleaseTag(tags []string, prefix string) string
  // CheckBreaking: untuk platform (prefix "v") dan setiap modul (prefix "modules/<dir>/"): bundel kontrak HEAD dan kontrak
  // baseline (LoadRepo atas fs.FS yang membaca blob tag via GitReader), lalu oasdiff checker; gagal bila ada Change dan !Approved.
  func CheckBreaking(ctx context.Context, repo *Repo, git GitReader, opts BreakingOptions) ([]BreakingResult, error)
  func Failed(rs []BreakingResult) bool
  ```
  CLI `erpora contracts:breaking [module]` mencetak per target `skip|ok|breaking|approved` beserta perubahannya, lalu exit 1 bila `Failed`.

- [ ] **Step 1: Tulis test yang gagal** — `breaking_test.go` membuat repo git sementara (`git init`, user lokal, commit fixture berisi `contracts/openapi/common.yaml`, `modules/example/{module.json, openapi.yaml}`, lalu tag `modules/example/v1.0.0`):
  - `TestCheckBreaking_RemovedFieldFails` (AC FR-23): hapus `title` dari `Note` → satu target `breaking` dengan ≥ 1 `Change`, dan `Failed == true`.
  - `TestCheckBreaking_OptionalFieldPasses`: tambah properti respons opsional `pinned` → tanpa `Change`.
  - `TestCheckBreaking_UnreleasedSkipped`: modul `exampleaddon` tanpa tag → `Skipped == "belum pernah dirilis"`.
  - `TestCheckBreaking_ApprovedMajor`: hapus field + `Approved` + manifest `2.0.0` → `Approved == true` dan `Failed == false`. Dengan manifest `1.1.0` → tetap gagal.
  - `TestCheckBreaking_BaselineResolvesErpRefsAtTag`: `common.yaml` di tag berbeda dengan HEAD, tetapi baseline memakai versi tag (tidak salah positif).
  - `TestLatestReleaseTag`: `v1.10.0` > `v1.9.0`; `v2.0.0-rc.1` diabaikan; prefix modul lain diabaikan.
- [ ] **Step 2: Jalankan** `go test ./internal/devtools/contracttools/ -run Breaking` → FAIL.
- [ ] **Step 3: Implementasi.** **Spike:** pastikan kin-openapi memuat hasil `Bundle` berversi `openapi: 3.1.0`. Bila tidak bisa, input diff ditulis dengan `openapi: 3.0.3`. Kontrak saat ini tidak memakai kata kunci khusus 3.1 (`type` array, `const`), dan rule Spectral `erp-oas30-diffable` ditambahkan untuk menolaknya. Catat pilihan itu di laporan task dan di Q31.
- [ ] **Step 4: Jalankan** test, lalu `task erpora -- contracts:breaking` di repo nyata → semua target `skip` (belum ada tag), exit 0.
- [ ] **Step 5: Commit** `feat(devtools): oasdiff breaking-change gate against last release tag with MAJOR+label exception`

### Task 9: Metrik `erp.api.deprecated_usage` dan header `Deprecation` (FR-23.3, keputusan ★5)

**Files:**
- Create: `internal/platform/modules/registry/deprecation_test.go`, `internal/platform/modules/runtime/{deprecation.go, deprecation_test.go}`
- Modify: `internal/platform/modules/registry/registry.go`, `internal/platform/modules/runtime/mount.go`, `internal/app/assemble.go`

**Interfaces:**
- Produces:
  ```go
  // registry.RouteInfo (aditif): diisi Build dari kontrak Entry (operasi dengan method+path persis sama; platform.core lewat dispatch)
  Deprecated       bool
  DeprecatedParams []string // parameter query deprecated
  // runtime
  func WithMeter(m metric.Meter) MountOption // tanpa meter: middleware deprecation hanya memasang header
  // deprecationMW: rt.Deprecated → w.Header().Set("Deprecation", "true") + counter.Add(1, erp.module.key, http.route=rt.Pattern,
  // erp.api.deprecated.kind="operation"); untuk setiap p ∈ rt.DeprecatedParams yang ada di r.URL.Query() → Add(1, …kind="parameter",
  // erp.api.deprecated.name=p). Dipasang setelah permission, sebelum batas request.
  func deprecationMW(rt registry.RouteInfo, c metric.Int64Counter, next http.Handler) http.Handler
  ```
  Instrumen: `erp.api.deprecated_usage`, `Int64Counter`, unit `{request}`. `assemble` meneruskan `prov.Meter` lewat `WithMeter`.

- [ ] **Step 1: Tulis test yang gagal**
  - `TestBuild_ContractDeprecationMapped`: kontrak fixture dengan operasi `deprecated: true` dan parameter query `legacy` deprecated → `RouteInfo` sesuai; route lain tidak.
  - `deprecation_test.go` (`sdkmetric.NewManualReader`):
    - `TestMount_DeprecatedOperationCountsAndHeader` → header `Deprecation: true` dan satu titik data dengan atribut `kind=operation`;
    - `TestMount_DeprecatedQueryParamCounted` (`?legacy=1`) → `kind=parameter, name=legacy`; tanpa parameter → tidak ada titik data;
    - `TestMount_NonDeprecatedNoMetric`;
    - `TestMount_DeprecatedAfterPermission`: permission ditolak → tidak dihitung.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/modules/...` → FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Jalankan** → PASS; `TestMount_*` lama tetap hijau.
- [ ] **Step 5: Commit** `feat(modules): count deprecated operation and query-parameter usage from contracts`

### Task 10: Mesin generator dan `make:migration`

**Files:**
- Create: `internal/devtools/gen/{plan.go, apply.go, marker.go, format.go, diff.go, names.go, render.go, golden.go, module_ref.go, migration.go}` + `*_test.go`, `internal/devtools/gen/testdata/repo/**` (fixture repo kecil: `go.work`, `go.mod`, `internal/app/modules.go` dengan penanda, `web/package.json`, `web/src/app/modules.ts`, `modules/example/{go.mod, module.json, module.go, migrations/0001_x.sql, migrations/0002_y.sql}`, `modules/exampleaddon/{module.json (strategy none), module.go}`), `internal/devtools/templates/{embed.go, migration/{module.sql.tmpl, platform.sql.tmpl}}`, `internal/devtools/commands/make.go`
- Modify: `go.mod` (`golang.org/x/tools` dan `golang.org/x/mod` sebagai require langsung bila belum; `github.com/pmezard/go-difflib v1.0.0` untuk diff), `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  type OpKind int
  const (OpCreate OpKind = iota; OpModify)
  type FileOp struct{ Path string; Kind OpKind; Content []byte } // Path relatif root (slash); OpModify berisi isi baru utuh
  type PostStep struct{ Name string; Run func(ctx context.Context, root string) error }
  type Plan struct{ Ops []FileOp; Post []PostStep }
  type Options struct{ Root string; Force, DryRun bool; Out io.Writer }
  var (ErrExists = errors.New("file sudah ada"); ErrMarkerNotFound = errors.New("penanda tidak ditemukan"); ErrAmbiguousMarker = errors.New("penanda muncul lebih dari sekali"))
  // Apply: (1) validasi seluruh Ops (OpCreate & file ada & !Force → ErrExists menyebut path dan --force; OpModify & file tidak ada → error;
  // path keluar root → error) SEBELUM menulis apa pun; (2) DryRun: cetak "create <path>"/"modify <path>" + UnifiedDiff, tanpa menulis,
  // tanpa Post; (3) tulis via file sementara + rename, buat direktori; (4) Post berurutan; cetak ringkasan.
  func Apply(ctx context.Context, p Plan, o Options) error
  // InsertAtMarker menyisipkan line (indentasi = indentasi baris penanda) tepat di atas baris yang berisi persis marker (setelah trim).
  // line identik sudah ada → src tak berubah. Penanda tidak ada → ErrMarkerNotFound dengan pesan "tambahkan `<line>` secara manual di <path>".
  func InsertAtMarker(path string, src []byte, marker, line string) ([]byte, error)
  func AddGoImport(path string, src []byte, name, importPath string) ([]byte, error) // go/parser + astutil.AddNamedImport + format
  func FormatGo(path string, src []byte) ([]byte, error)                             // imports.Process (goimports in-process)
  func UnifiedDiff(path string, before, after []byte) string
  func Render(name string, data any) ([]byte, error) // template ter-embed, missingkey=error

  type ModuleName struct{ Slug, Dir, Key, Snake, Camel, Pascal, NPM, GoPath string }
  // ParseModuleName: slug ^[a-z][a-z0-9]*(-[a-z0-9]+)*$, panjang 2..40, bukan slug cadangan registry; publisher ^[a-z][a-z0-9]*(-[a-z0-9]+)*$.
  func ParseModuleName(arg, publisher string) (ModuleName, error)
  type TypeName struct{ Pascal, Camel, Snake, Kebab, PluralKebab, PluralSnake string }
  func ParseTypeName(arg string) (TypeName, error) // ^[A-Z][A-Za-z0-9]{0,63}$; plural: +s / y→ies / s|x|z|ch|sh→es
  func ParseSnakeName(arg string) (string, error)  // ^[a-z][a-z0-9_]{0,62}$
  type ModuleRef struct{ Dir, Slug, Key, Version, GoPath string; Strategy string; HasWeb bool }
  func ResolveModule(root, arg string) (ModuleRef, error) // arg = Dir, slug, atau key; tidak ditemukan → error berisi daftar modul
  func NextMigrationNumber(fsys fs.FS) (int, error)       // max prefix ^\d{4}_ + 1; kosong → 1
  func MakeMigration(root, module, name string, platform string) (Plan, error) // platform "" | "control" | "tenant"
  ```
  CLI `erpora make:migration <module> <name> [--platform control|tenant] [--force] [--dry-run]` (grup `make`; dengan `--platform`, argumen `<module>` ditiadakan). Modul ber-`migrations.strategy: none` → error dengan instruksi mengubah strategy dan menambah `Migrations()`.

- [ ] **Step 1: Tulis test yang gagal**
  - `apply_test.go`:
    - `TestApply_ExistsFailsBeforeAnyWrite` (Review Focus 2): rencana `a.go` (baru) + `b.go` (sudah ada) → `ErrExists` dan `a.go` tidak tercipta.
    - `TestApply_ForceOverwritesCreatesOnly`.
    - `TestApply_DryRunWritesNothing`: checksum SHA-256 seluruh `t.TempDir()` sebelum dan sesudah sama; keluaran memuat `create a.go`, `modify go.work`, dan baris diff `+`; Post tidak dipanggil.
    - `TestApply_PathEscapeRejected` (`../x`).
  - `marker_test.go` `TestInsertAtMarker_Table`: sisip dengan indentasi tab; baris sudah ada → tak berubah; penanda hilang → `ErrMarkerNotFound` dan pesan memuat baris manual; penanda ganda → `ErrAmbiguousMarker`. `TestAddGoImport`.
  - `names_test.go`:
    - `TestParseModuleName_Table` (Review Focus 1): `inventory`, `stock-opname` (`Dir stockopname`, `Snake stock_opname`, `NPM @erpora/module-stockopname`, `Key acme.stock-opname`); ditolak: `Inventory`, `stock_opname`, `-x`, `x-`, `a--b`, `1abc`, `platform`, `x` (terlalu pendek).
    - `TestParseTypeName_Table` (`Item`→`items`, `Category`→`categories`, `Box`→`boxes`, `StockOpname`→`stock-opnames`/`stock_opnames`).
  - `migration_test.go`: `TestNextMigrationNumber`; `TestMakeMigration_Golden` (modul `example` → `modules/example/migrations/0003_add_tags.sql`, isi golden dengan `-- +goose Up`/`-- +goose Down`); `--platform control` → `migrations/control/0003_<name>.sql`; `exampleaddon` (strategy `none`) → error berisi `migrations.strategy`.
  - `golden.go`: helper `AssertGolden(t, dir string, ops []FileOp)` dengan flag `-update`; diff ditampilkan saat gagal.
- [ ] **Step 2: Jalankan** `go test ./internal/devtools/gen/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces.
- [ ] **Step 4: Jalankan** → PASS; `task lint arch check:gowork-off test` hijau.
- [ ] **Step 5: Commit** `feat(devtools): generator engine with overwrite protection, dry-run diff, markers, and make:migration`

### Task 11: `make:module`

**Files:**
- Create: `internal/devtools/gen/{module.go, module_test.go}`, `internal/devtools/gen/testdata/golden/{module_inventory, module_stock_opname}/**`, `internal/devtools/templates/module/**`, `web/src/app/modules.ts`
- Modify: `internal/devtools/commands/make.go`, `web/tsconfig.json` (bila perlu), `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  type ModuleOptions struct {
      Name, Publisher string // Publisher default "acme"
      Frontend bool
      Depends []string       // key modul, mis. "acme.example"; platform.core selalu ditambahkan
  }
  // MakeModule membangun Plan (keputusan 13–15). Validasi sebelum Plan: nama, bentrok Dir/slug/key dengan modul yang ada,
  // dependensi ada di repo. Ops: semua file template (OpCreate) + OpModify go.work (modfile.ParseWork/AddUse),
  // go.mod root (AddRequire v0.0.0 + AddReplace ./modules/<dir>), internal/app/modules.go (AddGoImport + InsertAtMarker
  // "// erp:generated:modules" ← "<dir>.New(),"), dan bila Frontend: web/package.json (dependencies "@erpora/module-<dir>":
  // "workspace:*", urutan kunci dipertahankan) + web/src/app/modules.ts (dua penanda).
  // Post (urut): contracts:generate untuk modul baru (bila Frontend), `bun install`, Biome format file TS baru.
  func MakeModule(root string, o ModuleOptions) (Plan, error)
  ```
  File template (`internal/devtools/templates/module/`, `{{.Dir}}`/`{{.Snake}}` di nama file):
  - `go.mod`, `module.json` (`schema_version 1`, `version 0.1.0`, `trust builtin`, `compatibility {platform ^0.1, sdk ^0.1}`, capabilities `<slug>.items.read`, `migrations versioned`, `offline online-only`);
  - `module.go` (embed `module.json`, `openapi.yaml`, `migrations/*.sql`; `Register` dengan `GET /items` + command `recount` + penanda `routes`/`commands`; `Install`, `SeedDev`, `Health`; `OpenAPI()`);
  - `module_test.go` (`moduletest.Recorder`: route, permission, dan command terdaftar sesuai manifest; `OpenAPI()` tidak kosong dan diawali `openapi: 3.1.0`. Modul tidak boleh mengimpor `internal/platform/apicontract` (D9), sehingga kesesuaian penuh diperiksa oleh test di `internal/app`);
  - `deps.go` (hanya bila `--depends`);
  - `openapi.yaml` (lolos semua rule Task 6);
  - `README.md`;
  - `contract/doc.go`;
  - `internal/domain/item.go` + test;
  - `internal/application/{list_items.go, list_items_test.go, seed.go}`;
  - `internal/infrastructure/items_repo.go`;
  - `internal/presentation/http/{handlers.go, handlers_test.go}`;
  - `migrations/0001_create_<snake>_items.sql` (kolom `id uuid`, `tenant_id uuid not null`, `title`, `version`, `created_at`, `updated_at`; indeks `tenant_id`);
  - bila `--frontend`: `web/{package.json, tsconfig.json, src/index.ts}` (`export const moduleInfo = { key, slug } as const; export * as api from "../api/contract.gen";`).

  `web/src/app/modules.ts` (ditulis sekali di task ini):
  ```ts
  // Daftar modul frontend. Baris baru disisipkan `erpora make:module --frontend` di atas penanda. Dipakai shell mulai M3 (FR-09).
  // erp:generated:web-imports
  export const frontendModules = [
    // erp:generated:web-modules
  ] as const;
  ```

- [ ] **Step 1: Tulis test yang gagal** (`module_test.go`, fixture `testdata/repo` disalin ke `t.TempDir()`):
  - `TestMakeModule_Golden` (`inventory --frontend --depends acme.example`): semua file `create` + diff `modify` sama dengan golden `module_inventory`.
  - `TestMakeModule_Golden_Hyphenated` (Review Focus 1, `stock-opname`): golden memuat `module erpora/modules/stockopname`, `"slug": "stock-opname"`, `"/api/stock-opname/items"`, `stock_opname_items`, `@erpora/module-stockopname`, dan `stockopname.New(),`.
  - `TestMakeModule_RejectsCollisions`: `stockopname` setelah `stock-opname` diterapkan → `ErrExists` atau error bentrok `Dir`; slug `example` → bentrok slug; `--depends acme.missing` → error; slug cadangan `platform` → error. Untuk semua kasus, checksum direktori tidak berubah.
  - `TestMakeModule_MarkerMissing`: hapus penanda dari `internal/app/modules.go` → `ErrMarkerNotFound` dan tidak ada file baru.
  - `TestMakeModule_ApplyTwiceWithoutForce` → kali kedua `ErrExists`.
  - `TestMakeModule_TemplateMatchesExampleShape`: daftar file template memuat padanan setiap file non-notes `modules/example` (`module.go`, `module_test.go`, `go.mod`, `module.json`, `openapi.yaml`, `contract/`, keempat lapisan, `migrations/`). Test ini mencegah template tertinggal dari sumbernya.
  - `devtools_flavor_test.go`: `make:module`, `make:migration`.
- [ ] **Step 2: Jalankan** `go test ./internal/devtools/gen/...` → FAIL.
- [ ] **Step 3: Implementasi** template (turunkan dari `modules/example`, ganti `notes` → `items`) dan `MakeModule`, lalu daftarkan command `make:module <name> [--frontend] [--depends k1,k2] [--publisher id] [--force] [--dry-run]`. Kelulusan gate penuh atas hasilnya dibuktikan di Task 16.
- [ ] **Step 4: Jalankan** → PASS; `task lint arch check:gowork-off test` hijau.
- [ ] **Step 5: Commit** `feat(devtools): make:module scaffolds a gate-ready module derived from example`

### Task 12: `make:entity`, `make:usecase`, `make:contract`, `make:command`, `make:repository`

**Files:**
- Create: `internal/devtools/gen/{layers.go, layers_test.go}`, `internal/devtools/templates/{entity, usecase, contract, command, repository}/**`, `internal/devtools/gen/testdata/golden/{entity, usecase_command, usecase_query, contract, command, repository}/**`
- Modify: `internal/devtools/commands/make.go`, `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  func MakeEntity(root, module, name string) (Plan, error)              // internal/domain/<snake>.go + <snake>_test.go
  func MakeUsecase(root, module, name string, query bool) (Plan, error) // internal/application/<snake>.go + _test.go
  //   command: type <Name>Command struct; type <Name>Handler struct; func New<Name>Handler(…) *<Name>Handler; Handle(ctx, <Name>Command) error
  //   query:   type <Name>Query struct; type <Name>Result struct; type <Name>Handler; Handle(ctx, <Name>Query) (<Name>Result, error)
  func MakeContract(root, module, name string) (Plan, error)            // contract/<snake>.go: interface <Name> + <Name>DTO; tanpa impor internal/
  func MakeCommand(root, module, name string) (Plan, error)
  //   internal/presentation/cli/<snake>.go: func <Pascal>Command() module.Command (Name = name, Run menulis ke inv.Out) + <snake>_test.go;
  //   module.go: AddGoImport cli + InsertAtMarker "// erp:generated:commands" ← "r.Command(cli.<Pascal>Command())".
  //   name kebab ^[a-z][a-z0-9-]*$; bentrok dengan Name command yang sudah ada → error. Nama yang ada dibaca dari module.go dan
  //   internal/presentation/cli/*.go lewat go/ast (literal `Name: "<x>"` dalam composite literal module.Command).
  func MakeRepository(root, module, name string) (Plan, error)
  //   internal/domain/<snake>_repository.go (interface <Name>Repository: Get(ctx, id) (<Name>, error); List(ctx, page, size) ([]<Name>, int64, error); Save(ctx, *<Name>) error)
  //   internal/infrastructure/<snake>_model.go (struct <Name>Model, TableName() "<slugSnake>_<pluralSnake>", tag gorm eksplisit, TenantID)
  //   internal/infrastructure/<snake>_repo.go (data.DB(ctx); WHERE tenant_id dari data provider; ErrNotFound domain)
  //   internal/infrastructure/<snake>_mapping.go + <snake>_mapping_test.go (unit, tanpa DB) — keputusan ★2 opsi (B)
  //   Entity <Name> harus sudah ada (make:entity), bila belum → error dengan instruksi.
  ```
  CLI: `make:entity <module> <Name>`, `make:usecase <module> <Name> [--query]`, `make:contract <module> <Name>`, `make:command <module> <name>`, `make:repository <module> <Name>`. Semuanya menerima `--force` dan `--dry-run`.

- [ ] **Step 1: Tulis test yang gagal** — golden per generator atas modul fixture hasil `MakeModule` (`inventory`) di `t.TempDir()`. Test lain:
  - `TestMakeRepository_RequiresEntity`;
  - `TestMakeCommand_DuplicateNameRejected` (`recount` sudah ada);
  - `TestMakeContract_NoInternalImports` (berkas hasil `go/parser`, tanpa impor berisi `/internal/`);
  - `TestLayers_GeneratedCodeCompilesAndPassesArch`: terapkan kelima generator ke salinan fixture yang memuat `sdk` asli (lewat `replace` absolut ke `sdk/` repo), lalu `go build ./...` dan `go vet ./...` (`GOWORK=off`, `GOFLAGS=-mod=mod`, `GOPROXY=off`) di modul itu. Hasilnya juga dimuat dengan `archcheck.Check` dan tidak boleh ada pelanggaran D1/D3/D3b/D4.
  - `devtools_flavor_test.go`: kelima command.
- [ ] **Step 2: Jalankan** `go test ./internal/devtools/gen/...` → FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Jalankan** → PASS.
- [ ] **Step 5: Commit** `feat(devtools): make:entity, make:usecase, make:contract, make:command, make:repository generators`

### Task 13: `make:handler` (route + kontrak + capability)

**Files:**
- Create: `internal/devtools/gen/{handler.go, openapi_edit.go, json_edit.go, handler_test.go}`, `internal/devtools/templates/handler/{get_item.go.tmpl, get_collection.go.tmpl, post_create.go.tmpl, post_action.go.tmpl, patch.go.tmpl, delete.go.tmpl, handler_test.go.tmpl, operation.yaml.tmpl}`, `internal/devtools/gen/testdata/golden/{handler_get_item, handler_post_action, handler_collection_hyphenated}/**`
- Modify: `internal/devtools/commands/make.go`, `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  type HandlerOptions struct {
      Module, Name string // Name PascalCase
      Route string        // "METHOD /path" relatif modul, mis. "GET /items/{id}"
      Permission string   // "<slug>.<resource>.<action>"
  }
  // MakeHandler: validasi method ∈ GET|POST|PATCH|DELETE (PUT ditolak, §3), path lolos aturan registry (validRoutePath) dan pola api-guidelines §2
  // (kebab-case, jamak atau aksi), permission berprefiks slug; operasi (method, /api/<slug><path>) belum ada di openapi.yaml → bila ada,
  // error sebelum Plan. Ops: handler + test (OpCreate); module.go (AddGoImport httpapi bila belum + InsertAtMarker "// erp:generated:routes"
  // ← `r.Handle(http.Method<M>, "<path>", "<perm>", httpapi.<Name>())`); module.json (capability ditambahkan bila belum, urutan dipertahankan);
  // openapi.yaml (operasi baru via yaml.Node; path item dibuat bila belum; parameter path {x} → Uuid; komentar & urutan dipertahankan).
  func MakeHandler(root string, o HandlerOptions) (Plan, error)
  func AddOperation(src []byte, path, method string, op *yaml.Node) ([]byte, error)
  func AppendJSONArrayString(src []byte, key, value string) ([]byte, error) // order-preserving; nilai sudah ada → tak berubah
  ```
  Template handler memakai `httpx.WriteData`/`WritePage`/`WriteError`, `httpx.DecodeJSON[<Name>Request]` untuk `POST`/`PATCH`, `201` + `Location` untuk create, dan `204` untuk delete. Template operasi sudah memenuhi rule Task 6:
  - semua operasi: `operationId`, contoh, `403`, `404` bila ada parameter path;
  - `POST`: `Idempotency-Key`, `400`/`422`;
  - aksi, `PATCH`, `DELETE`: `If-Match`;
  - `GET` koleksi: `Page`/`PageSize`;
  - properti contoh ber-`x-erp-classification: internal`.

- [ ] **Step 1: Tulis test yang gagal**
  - Golden `GET /items/{id}`, `POST /items/{id}/approve`, dan `GET /stock-counts` pada modul `stock-opname`.
  - `TestMakeHandler_ExistingOperationRejected` (Review Focus 2): `GET /items` sudah ada → error dan checksum tak berubah.
  - `TestMakeHandler_PermissionPrefix` (`inventory.items.read` pada modul `stock-opname` → error).
  - `TestMakeHandler_PutRejected`.
  - `TestMakeHandler_ResultConforms`: setelah `Apply` pada salinan fixture, modul dibangun ulang lewat `go run` program kecil di `testdata` yang memanggil `Register(moduletest.Recorder)` dan mencetak route JSON. Route itu diperiksa dengan `apicontract.CheckConformance` terhadap `openapi.yaml` hasil edit dan capabilities `module.json` → nol mismatch.
  - `TestMakeHandler_ResultLints` (`requireBun`): `contracttools.Lint` atas salinan → nol temuan.
  - `TestAddOperation_PreservesComments`.
- [ ] **Step 2: Jalankan** → FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Jalankan** `ERP_REQUIRE_BUN=1 go test ./internal/devtools/...` → PASS.
- [ ] **Step 5: Commit** `feat(devtools): make:handler registers route, contract operation, and capability together`

### Task 14: `module:check` dan `doctor`

**Files:**
- Create: `internal/devtools/modcheck/{check.go, check_test.go}`, `internal/devtools/doctor/{doctor.go, doctor_test.go}`, `internal/devtools/commands/{module.go, doctor.go}`
- Modify: `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  // modcheck
  type Finding struct{ Module, Rule, Detail string } // Rule: SCHEMA | D8 | CAPABILITY_PREFIX | GOVERSION
  // Check: untuk setiap modules/*/module.json (atau satu modul `only`, dicocokkan Dir/slug/key): manifest.Validate (schema + aturan);
  // archcheck.Load(root) + Check difilter D8 & GOVERSION untuk modul itu; setiap capability berawalan "<slug>.".
  func Check(root, only string) ([]Finding, error)

  // doctor
  type Status string // "ok" | "warn" | "fail"
  type Result struct{ Name string; Status Status; Detail string }
  type Probe interface {
      LookPath(name string) (string, error)
      Output(ctx context.Context, name string, args ...string) (string, error)
      FileExists(path string) bool
  }
  type Inputs struct{ Root, EnvFile string; DSN string; Ping func(ctx context.Context, dsn string) error }
  // Run: go (≥ toolchain go.mod → ok; lebih rendah → fail), bun (== packageManager package.json → ok; beda → warn; tidak ada → fail),
  // task (≥ 3.54.0 → ok; tidak ada → fail), docker (`docker info` sukses → ok; tidak ada/daemon mati → warn: hanya untuk task db:up & integration),
  // postgres (DSN kosong → warn; Ping gagal → fail, detail tanpa password via config.RedactDSN), .env (ada → ok; tidak ada → warn + saran .env.example).
  func Run(ctx context.Context, in Inputs, p Probe) []Result
  func Failed(rs []Result) bool
  ```
  CLI: `erpora module:check [name]` (grup `module`; mencetak `<module> <rule> <detail>`, exit 1 bila ada temuan, `OK` + exit 0 bila bersih); `erpora doctor` (grup `devtools`; tabel `NAMA STATUS DETAIL`, exit 1 bila ada `fail`).

- [ ] **Step 1: Tulis test yang gagal**
  - `check_test.go`: `TestCheck_RepositoryClean` (`../../..` → nol temuan untuk `example` dan `exampleaddon`); `TestCheck_Fixtures` (salinan fixture: `version: "1.0"` → `SCHEMA`; capability `other.x.read` → `CAPABILITY_PREFIX`; `require` tanpa dependensi manifest → `D8`; `go 1.26` → `GOVERSION`); `only` tidak dikenal → error.
  - `doctor_test.go` `TestRun_Table` dengan `fakeProbe`, mencakup kombinasi ok/warn/fail di atas serta detail yang tidak memuat password DSN.
  - `devtools_flavor_test.go`: `module:check` (grup `Modul:`) dan `doctor`.
- [ ] **Step 2: Jalankan** `go test ./internal/devtools/...` → FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Jalankan** → PASS; `task erpora -- module:check` → `OK`; `task erpora -- doctor` dijalankan manual dan hasilnya dicatat di laporan task.
- [ ] **Step 5: Commit** `feat(devtools): module:check and doctor`

### Task 15: `db:fresh` dan `db:seed` (FR-22.2, FR-22.5 lapis 2–3)

**Files:**
- Create: `internal/devtools/dbtools/{fresh.go, seed.go, drop.go, confirm.go, confirm_test.go}`, `internal/devtools/commands/db.go`, `internal/app/{seed.go, seed_integration_test.go}`, `tests/integration/dbtools_test.go`
- Modify: `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  // internal/app
  // RunDevSeeders memanggil SeedDev setiap modul berstatus enabled yang mengimplementasi module.DevSeeder, dalam urutan topologis
  // (resolver), lewat runtime.ModuleContext dengan authz.AsSystem(ctx, "cli:db:seed"). only != "" → hanya modul itu (Dir/slug/key);
  // modul only tidak enabled → &ExitError{Code: 3}. Mengembalikan key yang di-seed.
  func RunDevSeeders(ctx context.Context, cfg config.Config, log *slog.Logger, only string) ([]string, error)

  // internal/devtools/dbtools
  type FreshOptions struct{ Seed, Yes bool; In io.Reader; Out io.Writer }
  // Fresh: app.DestructiveEnvAllowed(cfg) SEBELUM membuka DB → buka DB → app.GuardDestructive → Confirm (kecuali Yes) →
  // DropAll → migrate.UpWithTenant → app.EnsureRequiredModules → lifecycle install+enable semua modul (--all --with-deps --enable,
  // actor cli:<os-user>) → bila Seed: app.RunDevSeeders.
  func Fresh(ctx context.Context, cfg config.Config, log *slog.Logger, o FreshOptions) error
  // Confirm meminta pengguna mengetik nama database (SELECT current_database()); beda/kosong/EOF → ErrAborted (exit 1), tanpa perubahan.
  func Confirm(in io.Reader, out io.Writer, dbName string) error
  // DropAll: satu transaksi; DROP … CASCADE untuk view, tabel, sequence, fungsi, dan tipe enum/domain di current_schema() (keputusan 17).
  func DropAll(ctx context.Context, db *database.DB) error
  // Seed: DestructiveEnvAllowed → buka DB → GuardDestructive → app.RunDevSeeders.
  func Seed(ctx context.Context, cfg config.Config, log *slog.Logger, only string, out io.Writer) error
  var ErrAborted = errors.New("dibatalkan: nama database tidak cocok")
  ```
  CLI (grup `db`): `erpora db:fresh [--seed] [--yes]` dan `erpora db:seed [module]`.

- [ ] **Step 1: Tulis test yang gagal**
  - `confirm_test.go` `TestConfirm_Table`: `erp\n` → nil; `ERP\n`, `\n`, EOF → `ErrAborted`.
  - `seed_integration_test.go`: `TestRunDevSeeders_TopologicalEnabledOnly` (example enabled, exampleaddon enabled → `["acme.example"]` karena hanya `example` yang `DevSeeder`; jumlah notes bertambah; panggilan kedua idempoten); `TestRunDevSeeders_OnlyDisabledExit3`.
  - `tests/integration/dbtools_test.go` (binary dev):
    - `TestDBFresh_RefusedWithoutTouchingDB` (Review Focus 4, AC FR-22): (a) `ERP_ENV=production` dengan `ERP_DATABASE_URL` ke port tertutup → exit 3 dengan pesan `ErrNotDevelopment`, bukan error koneksi. Ini membuktikan penolakan terjadi sebelum DB dibuka. (b) DB berpenanda `production` (dibuat dengan `erpBinRelease migrate up`, `ERP_ENV=production`) dengan `ERP_ENV=development --yes` → exit 3, dan jumlah tabel `public` serta baris `platform_state` tidak berubah.
    - `TestDBFresh_ConfirmAndRebuild`: DB development berisi notes tambahan. Stdin `salah\n` → exit 1 dan data tetap. Stdin `<dbname>\n` → exit 0, kedua modul `enabled`, notes kembali ke seed install (3). Dengan `--seed --yes` → notes > 3.
    - `TestDBSeed_RequiresDevelopmentMarker`: DB tanpa migrasi → exit 3 dengan pesan memuat `migrate up`.
  - `devtools_flavor_test.go`: `db:fresh` dan `db:seed` (grup `Database:`).
- [ ] **Step 2: Jalankan** `task test:integration` → FAIL.
- [ ] **Step 3: Implementasi.**
- [ ] **Step 4: Jalankan** → PASS.
- [ ] **Step 5: Commit** `feat(devtools): db:fresh and db:seed guarded by environment marker and typed confirmation`

### Task 16: Bukti `make:module` lolos semua gate (FR-22 AC, exit gate M2)

**Files:**
- Create: `tests/generator/{make_module_test.go, copy.go, README.md}` (`//go:build generator`)
- Modify: `Taskfile.yml` (task `test:generator`: `go test -tags generator -timeout 45m ./tests/generator/...`; ditambahkan ke `task ci`)

**Interfaces:**
- Produces:
  ```go
  // copyRepo menyalin root ke dst, melewati .git, node_modules, bin, tmp, .superpowers, contracts/dist, dan isi web/dist kecuali .gitkeep.
  func copyRepo(t *testing.T, root, dst string)
  func treeChecksum(t *testing.T, dir string) string // SHA-256 atas (path relatif, isi) terurut, melewati node_modules
  func run(t *testing.T, dir string, env []string, name string, args ...string) string // gagal → t.Fatalf dengan keluaran lengkap
  ```

- [ ] **Step 1: Tulis test** `TestMakeModule_PassesAllGates`:
  1. `copyRepo` ke `t.TempDir()`, `bun install --frozen-lockfile`, lalu `go build -tags devtools -o <tmp>/erpora ./cmd/erpora`.
  2. `sum0 := treeChecksum`. `erpora make:module inventory --frontend --depends acme.example --dry-run` → exit 0, keluaran memuat `create modules/inventory/go.mod` dan `modify go.work`. `treeChecksum == sum0` (AC `--dry-run`).
  3. `erpora make:module inventory --frontend --depends acme.example` → exit 0. Ulangi tanpa `--force` → exit ≠ 0 dengan pesan memuat `sudah ada`, dan checksum tidak berubah dari sebelum perintah ulang (Review Focus 2).
  4. `erpora make:module stock-opname` → exit 0 (Review Focus 1). Lalu `erpora make:handler inventory Approve --route "POST /items/{id}/approve" --permission inventory.items.approve`.
  5. Build ulang binary dev. Gate berikut harus exit 0:
     - `task lint`, `task arch`, `task check:gowork-off`, `task test`;
     - `erpora module:check inventory`, `erpora module:check stock-opname`;
     - `erpora contracts:lint`, `erpora contracts:generate --check`;
     - `bun install --frozen-lockfile`, `bun run --filter '*' typecheck`, `bun run --filter '*' lint`, `bun run --filter @erpora/web deps`.
  6. Postgres (`pgtest`, testcontainers):
     - `erpora migrate up`, lalu `erpora modules install acme.inventory --with-deps --enable` dan `erpora modules install acme.stock-opname --enable` → exit 0;
     - `erpora modules status acme.inventory --json` → status `enabled`;
     - `erpora inventory:recount` → exit 0 dan keluaran memuat `items:`;
     - `erpora serve` (port acak), lalu `GET /api/stock-opname/items` → 200 dengan amplop `data` + `page`.
- [ ] **Step 2: Jalankan** `task test:generator` → FAIL bila ada template yang tidak lolos gate. Perbaiki template (Task 11–13) sampai PASS, dan catat setiap perbaikan di laporan task.
- [ ] **Step 3: Jalankan** `task test:generator` → PASS (catat durasi; target < 15 menit di CI).
- [ ] **Step 4: Commit** `test(devtools): prove make:module output passes every quality gate and installs`

### Task 17: CI, workflow tag rilis, dan `release:verify-tag` (FR-14, FR-22 AC release)

**Files:**
- Create: `internal/devtools/release/{verify.go, verify_test.go}`, `internal/devtools/commands/release.go`, `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml`, `Taskfile.yml`, `cmd/erpora/devtools_flavor_test.go`

**Interfaces:**
- Produces:
  ```go
  // VerifyTag: "modules/<dir>/vX.Y.Z[-pre]" → modules/<dir>/go.mod ada dengan path erpora/modules/<dir> dan module.json version == "X.Y.Z[-pre]";
  // "sdk/vX.Y.Z[-pre]" → SDKVersion di sdk/module/version.go (dibaca go/parser, const string) sama; tag lain/semver tidak valid → error.
  func VerifyTag(root, tag string) error
  ```
  CLI `erpora release:verify-tag <tag>` (grup `devtools`; exit 1 bila tidak cocok, pesan menyebut kedua nilai).
- Perubahan CI (semua action tetap di-pin SHA):
  - `go-checks`: tambah `setup-bun` + `bun install --frozen-lockfile`, env `ERP_REQUIRE_BUN: "1"`. `task test` kini juga menjalankan `go test -tags devtools ./cmd/erpora/...`.
  - job baru `contracts`:
    - `actions/checkout` dengan `fetch-depth: 0` (tag tersedia), lalu setup Go, Bun, Task, dan `bun install --frozen-lockfile`;
    - env `ERP_API_BREAKING_APPROVED: ${{ contains(github.event.pull_request.labels.*.name, 'api-breaking-approved') }}`;
    - `task contracts:check`;
    - `task erpora -- contracts:generate` lalu upload artifact `openapi` (`contracts/dist/openapi.json`).
  - job baru `generator`: setup Go, Bun, Task, golangci-lint (terpin, `install-mode: goinstall`), lalu `task test:generator` (Docker tersedia di `ubuntu-latest`).
  - `build.needs` ditambah `contracts` dan `generator`.
  - `smoke-linux` dan `smoke-windows`: langkah "Bukti binary release tanpa developer tools". `./erpora version` memuat `flavor: release`. `./erpora make:module x` gagal dengan keluaran memuat `unknown command`. `./erpora doctor` juga gagal dengan `unknown command`. Skrip memakai bash/pwsh di workflow, bukan Taskfile.
- `release.yml`: `on: push: tags: ['modules/*/v*', 'sdk/v*']`, `permissions: contents: read`; checkout, setup Go dan Task, lalu `task erpora -- release:verify-tag "${GITHUB_REF_NAME}"`.

- [ ] **Step 1: Tulis test yang gagal** — `verify_test.go` `TestVerifyTag_Table`:
  - `modules/example/v1.0.0` → nil (manifest `1.0.0`);
  - `modules/example/v1.0.1` → error yang memuat `1.0.0`;
  - `modules/exampleaddon/v1.0.0` → nil;
  - `modules/missing/v1.0.0` → error;
  - `sdk/v0.1.0` → nil;
  - `sdk/v0.2.0` → error;
  - `v1.0.0` dan `modules/example/1.0.0` → error format;
  - prerelease `modules/example/v1.1.0-rc.1` pada fixture ber-manifest `1.1.0-rc.1` → nil.

  `devtools_flavor_test.go`: `release:verify-tag`.
- [ ] **Step 2: Jalankan** → FAIL.
- [ ] **Step 3: Implementasi** `VerifyTag`, command, workflow, dan Taskfile.
- [ ] **Step 4: Jalankan** `go test ./internal/devtools/release/...` → PASS. Validasi YAML workflow dengan `actionlint`, bila tersedia lokal; bila tidak, review manual dan catat di laporan task. `task erpora -- release:verify-tag modules/example/v1.0.0` → exit 0.
- [ ] **Step 5: Commit** `ci: contract gates, generator gate, release-binary proof, and release tag validation workflow`

### Task 18: Dokumentasi dan penyelarasan spec

**Files:**
- Create: `docs/module-sdk/generators.md`, `docs/module-sdk/contracts.md`
- Modify:
  - `docs/module-sdk/{README.md, creating-a-module.md}`: §8 kini diawali `erpora make:module`; penyalinan manual menjadi lampiran.
  - `docs/architecture/api-guidelines.md`: §2 `x-erp-singleton` (★3); §13 daftar ekstensi `x-erp-*`, ruleset `contracts/spectral.yaml`, `x-erp-classification`; §14.4 `oasdiff` level WARN+ dan label `api-breaking-approved`.
  - `docs/security/security-guidelines.md` §7: linter klasifikasi dan kesesuaian route ↔ kontrak → **M2d ✅**.
  - `docs/operations/deployment.md`: binary release tanpa developer tools, penanda lingkungan, dan larangan memakai binary dev di production.
  - `docs/prd/PRD-000-inisialisasi-project.md`:
    - baris **Q31** (setelah Q30 M2c; bila Q31 sudah terpakai, pakai nomor berikutnya) memuat jawaban owner atas ★1–★5, keputusan 6–22 yang mengikat, dan hasil spike Task 6/7/8;
    - tabel FR-22.2 ditambah `contracts:breaking` dan `release:verify-tag`, ditandai aditif;
    - catatan FR-23.3 tentang cakupan metrik deprecated.
  - `docs/adr/0006-module-manifest-sdk-semver.md`: `ContractSource` aditif, tag `modules/<dir>/vX.Y.Z` dan `sdk/vX.Y.Z` divalidasi workflow.
  - `CLAUDE.md` (bagian Perintah dan API & kontrak: `task contracts:check`, `task test:generator`, `erpora make:*`; hapus "tersedia mulai M2").
  - `README.md` (perintah developer).
  - `contracts/README.md`, `packages/{module-sdk,platform-contracts}/README.md`.
  - `docs/superpowers/plans/2026-10-22-m2-overview.md`: catatan M2d selesai dan sisa untuk M3.

**Interfaces:** tidak ada (dokumentasi).

`generators.md` memuat:
1. Flavor dev vs release, beserta banner.
2. Daftar `make:*` dengan contoh nyata (`inventory`, `stock-opname`).
3. Aturan penamaan (keputusan 13).
4. Penanda dan arti pesan "penanda tidak ditemukan".
5. `--force`/`--dry-run`.
6. `db:fresh`/`db:seed` dan ketiga lapis pengaman.
7. `module:check` dan `doctor`.
8. Status ★1/★2.

`contracts.md` memuat:
1. Kepemilikan kontrak dan `erp:` refs.
2. Ekstensi `x-erp-*`.
3. Tabel rule linter → pasal.
4. Klasifikasi: anotasi, aturan nama sensitif, dan kesesuaian dengan `module.Classify`.
5. Test kesesuaian dan test respons.
6. `contracts:generate` dan `--check`.
7. Breaking change, label, dan tag rilis.
8. Metrik deprecated.

- [ ] **Step 1:** Tulis dokumen.
- [ ] **Step 2:** Pastikan setiap identifier, command, dan nama task yang disebut ada (`git grep -n <nama>` per nama; `task --list`; `task erpora -- list`), dan semua tautan relatif valid.
- [ ] **Step 3: Commit** `docs: document M2d developer tools and API contracts; record PRD-000 Q31`

### Task 19: Exit gate M2

**Files:** tidak ada perubahan kode. Bila ada butir yang gagal, perbaikannya dikerjakan di task terkait dan dicatat di ledger.

Exit gate PRD-000 §11 M2, AC FR-22/FR-23/FR-24.2 (bagian linter), dan item overview M2d:

| Kriteria | Bukti (test/perintah) |
| --- | --- |
| **§11: Semua AC FR-18/FR-19 lulus di integration test** | `task test:integration`: tabel "Verifikasi akhir M2b" (`TestService_*`, `TestExample_*`, `TestCLI_*`, `TestAssemble_*`) dan `internal/platform/modules/resolver` (`TestResolve_*`), semuanya hijau di branch ini |
| **§11: Kedua modul referensi dikelola penuh lewat CLI** | `TestCLI_ManageBothModules`, `TestCLI_ExitCodes`, `TestCLI_CrashAfterMigrateThenRetry`, `TestCLI_ConcurrentEnableTwoProcesses` |
| **§11: `make:module` menghasilkan modul yang lolos semua gate** | `TestMakeModule_PassesAllGates` (T16), job CI `generator` |
| **§11: Binary release terbukti tanpa developer tools** | `TestReleaseFlavor_NoDevtoolsPackages`, `TestReleaseFlavor_UnknownDevCommands`, `TestDevtoolsRule_OnlyEntryFile` (T1); langkah smoke CI terhadap artifact (T17) |
| FR-22 AC: golden test setiap `make:*` | `TestMakeMigration_Golden` (T10), `TestMakeModule_Golden*` (T11), golden T12, golden T13 |
| FR-22 AC: `--dry-run` tidak mengubah file (checksum repo) | `TestApply_DryRunWritesNothing` (T10), langkah 2 `TestMakeModule_PassesAllGates` |
| FR-22 AC: `db:fresh` ditolak (`ERP_ENV=production` / penanda `production`) | `TestDBFresh_RefusedWithoutTouchingDB` (T15) |
| FR-22 AC: binary dev vs DB `production` (`serve`, `migrate up`, `modules enable` ditolak; `inspect routes` jalan) | `TestDevBinary_ProductionMarker` (T2) |
| FR-22 AC: `example:recount` enabled/disabled | `TestRunModuleCommand_EnabledRuns`, `TestRunModuleCommand_NotEnabledExit3` (M2b) |
| FR-22 AC: `erpora list` dikelompokkan | `TestDevFlavor_ListGroups` (T1, dilengkapi T6–T17) |
| FR-22.1: `version` menampilkan flavor, banner dev | `TestReleaseFlavor_VersionNoBanner`, `TestDevFlavor_BannerAndVersion` (T1) |
| FR-03: `inspect config` menampilkan penanda | `TestInspectConfig_ShowsDatabaseEnvironment` (T2) |
| FR-23 AC: kontrak `example`/`exampleaddon` lolos; `erp:example` dari addon lolos | `TestLint_RepositoryContractsClean` (T6) |
| FR-23 AC: `erp:exampleaddon` dari `example` ditolak | `TestLint_Fixtures/ref_not_dependency` (T6) |
| FR-23 AC: `total: number`, `/getItems`, operasi tanpa permission ditolak dengan pasal | `TestLint_Fixtures` (T6) |
| FR-23 AC: `Handle` tanpa operasi (atau sebaliknya) gagal | `TestContracts_RoutesMatchOperations/extra_*` (T4), `TestCheckConformance_Table` (T3) |
| FR-23 AC: dua modul `Item` digabung tanpa bentrok | `TestMerge_SameSchemaNameTwoModules` (T3) |
| FR-23 AC: kontrak berubah tanpa generate → CI gagal | `TestCheckGenerated_DetectsStale` (T7), `task contracts:check` |
| FR-23 AC: hapus field dari kontrak bertag gagal; field opsional lolos | `TestCheckBreaking_RemovedFieldFails`, `TestCheckBreaking_OptionalFieldPasses` (T8) |
| FR-23.2: test kontrak respons | `TestContractResponses_ReferenceModules` (T5) |
| FR-23.3: metrik deprecated | `TestMount_Deprecated*` (T9) |
| FR-24.2 AC: linter menolak `password` di respons dan token di query | `TestLint_Fixtures/password_response`, `/token_query` (T6) |
| M2c ★3: klasifikasi route = kontrak (bersarang) | `TestContracts_ClassificationMatchesRoute` (T4), `TestResponseClassification_Nested` (T3) |
| FR-06 AC: tipe TS `packages/module-sdk` dari schema | `manifest.gen.ts` + `manifest.check.ts` (typecheck) + `contracts:generate --check` (T7) |
| FR-14: workflow validasi tag rilis | `TestVerifyTag_Table` (T17), `.github/workflows/release.yml` |
| FR-14: gate kontrak di CI | job `contracts` (T17) |

- [ ] **Step 1:** `task ci` exit 0. Urutannya: `lint`, `arch`, `check:gowork-off`, `test`, `test:integration`, `vuln`, `web:check`, `contracts:check`, `test:generator`, `build`. Catat durasi setiap task.
- [ ] **Step 2:** `go list -deps ./cmd/erpora` (tanpa tag) → tidak memuat `internal/devtools`. `task build` lalu `bin/erpora make:module x` → `unknown command`, dan `bin/erpora version` → `flavor: release`.
- [ ] **Step 3: Manual (PowerShell, Windows):**
  1. `task db:up`, `task erpora -- doctor` → tanpa `fail`.
  2. `task erpora -- db:fresh --seed`, ketik nama DB → kedua modul `enabled`.
  3. `task erpora -- make:module demo --frontend --dry-run` → daftar file, dan `git status` tetap bersih.
  4. `task erpora -- contracts:lint` → exit 0.
  5. `task erpora -- serve`, lalu `curl -s localhost:8080/api/example/notes` → 200.
- [ ] **Step 4:** Tabel di atas disalin ke ledger `.superpowers/sdd/<tanggal>-m2d/progress.md`, setiap baris berstatus ✅ beserta hash commit. Butir ★ yang belum dijawab owner ditandai ⏳ dan tidak diklaim selesai.
- [ ] **Step 5:** Tidak ada push atau PR tanpa permintaan user. Bila semua hijau, laporkan bahwa M2 siap untuk review menyeluruh branch (satu review akhir, lalu satu gelombang perbaikan).

#### Hasil verifikasi exit gate M2 (27 Okt 2026, Windows, branch `feat/m2d-devtools`)

**Step 1 (`ERP_REQUIRE_BUN=1`, setiap task dijalankan terpisah; semuanya exit 0):** `lint` 42 dtk, `arch` 103 dtk, `check:gowork-off` 23 dtk, `test` 82 dtk, `test:integration` 173 dtk, `test:generator` 521 dtk, `vuln` 53 dtk, `web:check` 8 dtk, `contracts:check` 11 dtk, `build` 11 dtk. `contracts:check` ditambahkan ke `task ci` (sebelumnya tidak ada).

**Step 2:** `go list -deps ./cmd/erpora` (tanpa tag) memuat 0 paket `erpora/internal/devtools`; `bin/erpora make:module x` → `unknown command "make:module"`; `bin/erpora version` → `flavor: release`.

**Step 3 (manual, PostgreSQL dari `task db:up`, tanpa file .env permanen):** (1) `task db:up` lalu `task erpora -- doctor`: go/bun/task/docker/postgres `ok`, `.env` `warn` (tidak ada), tanpa `fail` ✅. (2) `task dev:setup` (migrate up + `modules install --all --enable`): kedua modul referensi `enabled` ✅. `db:fresh --seed` dengan mengetik nama DB **tidak dijalankan manual** (aksi destruktif ditolak oleh pengaman sesi); perilakunya dibuktikan `TestDBFresh_ConfirmAndRebuild` dan `TestDBFresh_RefusedWithoutTouchingDB` ⏳ (manual menunggu owner). (3) `task erpora -- make:module demo --frontend --dry-run`: daftar file tercetak, `git status` tetap bersih ✅. (4) `task erpora -- contracts:lint` → `kontrak OK`, exit 0 ✅. (5) `task erpora -- serve --env-file .env` lalu `curl /api/example/notes` → 200 dengan amplop `data` ✅. Lingkungan dibersihkan (`task db:down`, `.env` sementara dihapus).

| Kriteria | Bukti | Commit | Status |
| --- | --- | --- | --- |
| §11: AC FR-18/FR-19 lulus di integration test | `task test:integration` hijau (`TestService_*`, `TestExample_*`, `TestCLI_*`, `TestAssemble_*`, `TestResolve_*`) | M2b | ✅ |
| §11: kedua modul referensi dikelola penuh lewat CLI | `TestCLI_ManageBothModules`, `TestCLI_ExitCodes`, `TestCLI_CrashAfterMigrateThenRetry`, `TestCLI_ConcurrentEnableTwoProcesses` | M2b | ✅ |
| §11: `make:module` menghasilkan modul yang lolos semua gate | `TestMakeModule_PassesAllGates` (12m20s lokal; 521 dtk di `task ci`), job CI `generator` | `7d9c0ca` | ✅ lokal Windows; ⏳ job CI `generator` di GitHub belum pernah berjalan |
| §11: binary release tanpa developer tools | `TestReleaseFlavor_NoDevtoolsPackages`, `TestReleaseFlavor_UnknownDevCommands`, `TestDevtoolsRule_OnlyEntryFile`; Step 2; langkah smoke CI | `f2abbb5`, `bb5a980` | ✅ test lokal dan Step 2; ⏳ langkah smoke CI terhadap artifact belum pernah berjalan di GitHub |
| FR-22 AC: golden test setiap `make:*` | `TestMakeMigration_Golden`, `TestMakeModule_Golden*`, golden T12 dan T13 (`TestMakeHandler_Golden`) | `ed2dac6`, `69f58ae`, `b7fbbb8`, `f092cb0` | ✅ |
| FR-22 AC: `--dry-run` tidak mengubah file | `TestApply_DryRunWritesNothing`; langkah 2 `TestMakeModule_PassesAllGates`; Step 3 (3) | `ed2dac6`, `7d9c0ca` | ✅ |
| FR-22 AC: `db:fresh` ditolak (`ERP_ENV=production` / penanda `production`) | `TestDBFresh_RefusedWithoutTouchingDB` (dibuktikan juga lewat mutasi) | `af1fec7` | ✅ |
| FR-22 AC: binary dev vs DB `production` | `TestDevBinary_ProductionMarker` | `6a1c395` | ✅ |
| FR-22 AC: `example:recount` enabled/disabled | `TestRunModuleCommand_EnabledRuns`, `TestRunModuleCommand_NotEnabledExit3` | M2b | ✅ |
| FR-22 AC: `erpora list` dikelompokkan | `TestDevFlavor_ListGroups` | `f2abbb5`..`bb5a980` | ✅ |
| FR-22.1: `version` menampilkan flavor, banner dev | `TestReleaseFlavor_VersionNoBanner`, `TestDevFlavor_BannerAndVersion` | `f2abbb5` | ✅ |
| FR-03: `inspect config` menampilkan penanda | `TestInspectConfig_ShowsDatabaseEnvironment` | `6a1c395` | ✅ |
| FR-23 AC: kontrak `example`/`exampleaddon` lolos; `erp:example` dari addon lolos | `TestLint_RepositoryContractsClean`; Step 3 (4) | `ca6e312` | ✅ |
| FR-23 AC: `erp:exampleaddon` dari `example` ditolak | `TestLint_Fixtures/ref_not_dependency` | `ca6e312` | ✅ |
| FR-23 AC: `total: number`, `/getItems`, operasi tanpa permission ditolak dengan pasal | `TestLint_Fixtures` | `ca6e312` | ✅ |
| FR-23 AC: `Handle` tanpa operasi (atau sebaliknya) gagal | `TestContracts_RoutesMatchOperations/extra_*`, `TestCheckConformance_Table` | `292dee8`, `4e94c67` | ✅ |
| FR-23 AC: dua modul `Item` digabung tanpa bentrok | `TestMerge_SameSchemaNameTwoModules` | `4e94c67` | ✅ |
| FR-23 AC: kontrak berubah tanpa generate → CI gagal | `TestCheckGenerated_DetectsStale`, `task contracts:check`, job CI `contracts` | `9c8b041`, `bb5a980` | ✅ lokal (`TestCheckGenerated_DetectsStale`, `task contracts:check`); ⏳ job CI `contracts` belum pernah berjalan di GitHub |
| FR-23 AC: hapus field dari kontrak bertag gagal; field opsional lolos | `TestCheckBreaking_RemovedFieldFails`, `TestCheckBreaking_OptionalFieldPasses` | `83c5020` | ✅ |
| FR-23.2: test kontrak respons | `TestContractResponses_ReferenceModules` | `1206787` | ✅ |
| FR-23.3: metrik deprecated | `TestMount_Deprecated*` (operasi dan parameter query; properti body dan klien ditunda, Q31 ★5) | `d73e2b4` | ✅ (cakupan sesuai ★5) |
| FR-24.2 AC: linter menolak `password` di respons dan token di query | `TestLint_Fixtures/highly_confidential_in_response` (penolakan `highly_confidential` di respons), `/password_response` (wajib klasifikasi), `/new_password_needs_classification`, `/token_query`, `/reset_token_query` | `ca6e312` | ✅ |
| M2c ★3: klasifikasi route = kontrak (bersarang) | `TestContracts_ClassificationMatchesRoute`, `TestResponseClassification_Nested` | `292dee8`, `4e94c67` | ✅ |
| FR-06 AC: tipe TS `packages/module-sdk` dari schema | `manifest.gen.ts` + `manifest.check.ts` (typecheck) + `contracts:generate --check` | `9c8b041` | ✅ |
| FR-14: workflow validasi tag rilis | `TestVerifyTag_Table`, `.github/workflows/release.yml`, `task release:verify-tag` | `bb5a980` | ⏳ test lokal hijau (`TestVerifyTag_Table`); workflow belum pernah berjalan di GitHub (sintaks YAML diperiksa dengan parser) |
| FR-14: gate kontrak di CI | job `contracts` di `ci.yml` | `bb5a980` | ⏳ job `contracts` belum pernah berjalan di GitHub; `task contracts:check` hijau lokal |
| Butir ★1–★5 | Disetujui owner 27 Okt 2026, dicatat di PRD-000 Q31 | `c49ed88` | ✅ |
| Manual: `db:fresh --seed` dengan ketik nama DB | Tidak dijalankan manual (pengaman sesi); dibuktikan test integrasi | — | ⏳ |

---

## Di luar cakupan M2d (sengaja)

- `make:page`, shell/registry TS (`defineModule`, TanStack Router), frontend `example`/`exampleaddon`, `/settings/modules`, E2E Playwright, gate `bun test` → **M3** (FR-08–FR-10, FR-15 frontend, FR-20.3, Q23). Lihat ★1.
- Integration test repository hasil generate → menunggu ★2 (rekomendasi: PRD-001).
- Metrik deprecated untuk properti body request dan atribut klien → ★5 (PRD-001 untuk principal).
- Hook pre-commit git otomatis. FR-23.3 menyebutnya opsional; `task contracts:check` sudah dapat dipakai sebagai pre-commit manual.
- Penerbitan dokumentasi API (portal/UI) dari `contracts/dist/openapi.json` → PRD terkait integrator. M2d hanya menghasilkan artifact CI.
- Vanity domain untuk SDK dan modul pihak ketiga (Q24), serta penandatanganan artifact → fase marketplace.
- Task `dev:setup` (FR-13, cakupan M1) tidak ditambahkan di sini. Temuan ini dilaporkan terpisah bila memang belum ada.
