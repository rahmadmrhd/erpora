# Developer tools: generator dan perintah dev

Panduan perintah `erpora` flavor dev untuk penulis modul (PRD-000 FR-22). Rancangan dan keputusan ada di PRD-000 FR-22 dan Q31;
panduan kontrak API ada di [`contracts.md`](contracts.md).

## 1. Flavor dev dan release

| Flavor | Cara membuat | Isi |
| --- | --- | --- |
| **dev** | `task erpora -- <command>` (`go run -tags devtools ./cmd/erpora`) atau `task build:dev` | Runtime (`serve`, `migrate`, `modules`, `inspect`, command modul) dan developer tools di bawah |
| **release** | `task build` | Runtime saja. Developer tools **tidak ada** di binary (`erpora make:module` menghasilkan `unknown command`) |

Binary dev menulis banner peringatan ke **stderr** di setiap command (stdout, termasuk `--json`, tetap bersih) dan
**menolak command yang menulis** terhadap database berpenanda `production` (bagian 6). `erpora version` menampilkan `flavor: dev|release`.
Kode developer tools berada di `internal/devtools/**`; satu-satunya titik masuknya `cmd/erpora/devtools_on.go` (aturan arch `DEVTOOLS`).

## 2. Daftar `make:*`

Semua command dijalankan dari dalam repo (`task erpora -- make:... ` atau binary dev). Contoh memakai modul `inventory` dan `stock-opname`.

| Command | Hasil |
| --- | --- |
| `make:module <name> [--frontend] [--metadata] [--depends k1,k2] [--publisher id]` | Modul baru turunan `example` di `modules/<dir>`: go.mod, `module.json`, `openapi.yaml`, empat lapisan Clean Architecture, migrasi, command `recount`, `DevSeeder`, dan test. Didaftarkan di `go.work`, `go.mod` root (`replace`), dan `internal/app/modules.go`. `--metadata` menambah kerangka metadata resource (bagian 10) |
| `make:migration <module> <name>` | `migrations/NNNN_<name>.sql` goose bernomor berikutnya (modul, atau `--platform control\|tenant`) |
| `make:entity <module> <Name>` | Entity + test di `internal/domain` |
| `make:usecase <module> <Name> [--query]` | Use case (command atau query) + test di `internal/application` |
| `make:repository <module> <Name>` | Port repository, model GORM, implementasi, mapping, dan test mapping |
| `make:handler <module> <Name> --route "METHOD /path" --permission <cap>` | Handler + test (endpoint ber-ID disertai kerangka test IDOR), route di `module.go`, capability di `module.json`, dan operasi + skema di `openapi.yaml` |
| `make:contract <module> <Name>` | Interface + DTO di `contract/` (tanpa impor `internal/`) |
| `make:command <module> <name>` | Command CLI modul (`<slug>:<name>`) dan pendaftarannya di `module.go` |

Contoh:

```sh
task erpora -- make:module inventory --frontend --depends acme.example
task erpora -- make:module stock-opname
task erpora -- make:handler inventory Approve --route "POST /items/{id}/approve" --permission inventory.items.approve
```

Setelah `make:module`, langkah lanjutan dijalankan otomatis: `go mod tidy` (modul baru), dan dengan `--frontend`: `contracts:generate`,
`bun install`, dan Biome. `make:handler` pada modul ber-`web/` membangkitkan ulang tipe TS kontrak modul itu. Modul hasil generate lolos
`task lint arch check:gowork-off test`, `module:check`, `contracts:lint`, dan `contracts:generate --check`, dan dapat di-install/enable
(dibuktikan `task test:generator`, bagian 8).

Hal yang perlu diketahui:

- Handler hasil generate memakai hook use case tingkat paket (`<camel>Func`, bawaan `default<Name>`) yang menjawab `501` sampai Anda menghubungkannya ke use case. Header `If-Match` (patch, delete, aksi) dan `Idempotency-Key` (create, aksi) diteruskan ke hook sebagai argumen; use case wajib memeriksanya (api-guidelines section 7).
- Test IDOR hasil generate (endpoint ber-ID) memakai stub principal palsu yang menimpa hook dan menegaskan `404` tanpa kebocoran. Karena stub itu tetap hijau walau pembatasan scope di use case sungguhan hilang, test tersebut **gagal dengan pesan keras begitu hook sudah disambungkan** (`<camel>Func` bukan lagi `default<Name>`): tulis ulang terhadap use case sungguhan di atas repository yang menolak record milik principal lain (dua principal A vs B; `authz.Check`/`authz.Filter` setelah PRD-001), lalu hapus pemeriksaannya. Penggantian hook global dengan DI penuh (`*Handlers` + port use case) belum dikerjakan; dicatat sebagai pekerjaan lanjutan.
- Tabel modul **tidak** punya kolom `tenant_id`; tenant ditangani lapisan `sdk/data` (ADR-004).
- Generator tidak menghasilkan integration test repository (keputusan ★2, bagian 9): tulis manual lewat platform, mengikuti `internal/app/reference_modules_integration_test.go`.

## 3. Aturan penamaan

Argumen `make:module` adalah slug kebab-case. Direktori, nama package Go, dan akhiran module path memakai slug **tanpa tanda hubung**.

| Slug | `stock-opname` |
| --- | --- |
| Direktori / package / module path | `modules/stockopname`, `stockopname`, `erpora/modules/stockopname` |
| Key manifest | `<publisher>.<slug>` = `acme.stock-opname` (`--publisher`, default `acme`) |
| Prefix permission, command, dan API | `stock-opname` (`stock-opname.items.read`, `stock-opname:recount`, `/api/stock-opname/...`) |
| Tabel dan tabel goose | `stock_opname_*`, `goose_module_stock_opname` |
| Package npm | `@erpora/module-stockopname` |

Slug yang dicadangkan registry ditolak. Bentrok direktori (`stockopname` vs `stock-opname`), slug, atau key ditolak **sebelum** ada file yang ditulis.

## 4. Penanda (marker)

Perubahan pada file yang sudah ada selalu terstruktur, tidak pernah lewat pencarian teks bebas: `go.work`/`go.mod` lewat `golang.org/x/mod/modfile`,
`module.json`/`package.json` lewat edit JSON yang mempertahankan urutan kunci, `openapi.yaml` lewat posisi node YAML, dan file Go/TS lewat penanda satu baris:

| Penanda | File |
| --- | --- |
| `// erp:generated:modules` | `internal/app/modules.go` |
| `// erp:generated:routes`, `// erp:generated:commands` | `modules/<dir>/module.go` |
| `// erp:generated:web-imports`, `// erp:generated:web-modules` | `web/src/app/modules.ts` |

Bila penanda tidak ditemukan, generator **berhenti tanpa menulis apa pun** dan mencetak baris yang harus Anda tambahkan manual. Artinya: file itu pernah
diubah sehingga penandanya hilang. Kembalikan baris penanda di tempat yang tepat (lihat `modules/example/module.go` sebagai acuan), lalu ulangi perintahnya.

## 5. `--force` dan `--dry-run`

- Generator **tidak pernah menimpa** file yang sudah ada: pesan "file sudah ada" menyebut path dan (bila berlaku) `--force`. `--force` hanya berlaku untuk `make:entity`, `make:usecase`, `make:repository`, `make:contract`, dan `make:migration`, dan hanya untuk file yang *dibuat*; file yang *diubah* (go.work, module.go, openapi.yaml, dst.) tidak terpengaruh. **`make:module`, `make:handler` (operasi sudah ada), dan `make:command` (nama sudah ada) menolak tanpa syarat**: `--force` ditolak dengan pesan jelas (exit 2), karena menimpa file milik developer menghilangkan pekerjaannya; hapus atau ganti nama target secara manual bila memang ingin membuat ulang (Q32).
- `--dry-run` mencetak `create|modify|overwrite <path>` beserta diff, **tanpa menulis apa pun** dan tanpa menjalankan langkah lanjutan. Seluruh validasi tetap berjalan.
- Seluruh rencana divalidasi sebelum file pertama ditulis; penulisan memakai berkas sementara + rename. Target atau direktori induknya yang berupa symlink (atau junction Windows) ditolak, sehingga generator tidak bisa menulis keluar dari root repo.
- **Kegagalan di tengah:** bila penulisan satu file gagal, file yang sudah ditulis pada perintah itu dikembalikan (file baru dihapus, isi lama dipulihkan) dan pesan error menyebut jumlahnya. **Kegagalan langkah lanjutan** (`go mod tidy`, `contracts:generate`, `bun install`, Biome) **tidak di-rollback**: file hasil generate tetap ada. Pemulihan: perbaiki penyebabnya (mis. jaringan atau Bun) lalu jalankan langkah yang gagal secara manual (`go mod tidy` di `modules/<dir>`, `task erpora -- contracts:generate <dir>`, `bun install`), atau batalkan semuanya dengan `git checkout -- .` dan `git clean -fd modules/<dir>` pada path yang dicetak generator (hanya bila belum ada pekerjaan lain yang belum di-commit di path itu).

## 6. `db:fresh`, `db:seed`, dan tiga lapis pengaman

| Command | Fungsi |
| --- | --- |
| `db:fresh [--seed] [--yes]` | Menghapus semua objek di schema aktif, `migrate up`, modul required, install + enable semua modul, lalu (opsional) seed |
| `db:seed [module]` | `SeedDev` setiap modul `enabled` yang mengimplementasi `DevSeeder`, dependensi lebih dulu; `module` (key, slug, atau nama direktori) membatasi ke satu modul yang harus `enabled` |

Pengaman (PRD-000 FR-22.5), semuanya sebelum ada perubahan data:

1. **Build:** developer tools tidak di-compile ke binary release.
2. **Data:** `ERP_ENV` harus `development` (diperiksa *sebelum* database dibuka, exit 3), **dan** penanda `platform_state.environment` harus `development`. Penanda ditulis sekali oleh `migrate up` dan tidak pernah diubah CLI. Database tanpa penanda ditolak dengan petunjuk `migrate up`.
3. **Interaksi:** `db:fresh` meminta Anda mengetik nama database (persis, sensitif huruf besar-kecil). Nama salah, kosong, atau EOF → dibatalkan (exit 1), data utuh. `--yes` melewati langkah ini tetapi tidak melewati lapis 2.

`db:fresh` tidak men-`DROP SCHEMA`, sehingga hak schema milik DBA tetap utuh; objek milik ekstensi tidak disentuh.

Aturan penanda untuk command tulis lain pada binary dev (`serve`, `migrate up`, `modules` mutasi, command modul): ditolak (exit 3) bila penanda `production`,
**atau** penanda belum ada dan `ERP_ENV=production` (agar `migrate up` biner dev tidak menginisialisasi DB production). Command read-only (`inspect`, `modules list|status|plan|history`, `migrate status`, `version`, `list`) tidak memeriksa penanda.

## 7. `module:check` dan `doctor`

- `module:check [name]`: per modul (atau satu modul: direktori, slug, atau key) memeriksa manifest (`SCHEMA`: JSON Schema + SemVer + constraint), `require` go.mod ↔ `dependencies` manifest dan impor (`D8`), versi go/toolchain seragam (`GOVERSION`), dan awalan capability = slug (`CAPABILITY_PREFIX`). Mencetak `<module> <rule> <detail>`; exit 1 bila ada temuan, `OK` bila bersih.
- `doctor`: tabel `NAMA STATUS DETAIL` untuk go (≥ toolchain di go.mod), bun (= `packageManager`), task (≥ 3.54.0), docker (hanya untuk `task db:up` dan test integrasi: `warn`), PostgreSQL (`ERP_DATABASE_URL`; password tidak pernah dicetak), dan `.env`. Exit 1 hanya bila ada `fail`.

## 8. Bukti dan CI

`task test:generator` (tag `generator,integration`, batas 45 menit; job CI `generator`) menyalin repo ke `<repo>/tmp/generator-*`, membuat `inventory` (`--frontend --depends acme.example`)
dan `stock-opname`, lalu menjalankan seluruh gate dan memasang modul ke PostgreSQL sungguhan lalu `serve`. Catatan: path repo **tidak boleh memuat spasi** (Taskfile `for` memecah path pada spasi).

## 9. Status keputusan ★ (PRD-000 Q31)

- **★1 Frontend.** `--frontend` membuat package `modules/<dir>/web` (`@erpora/module-<dir>`) dan mendaftarkannya di `web/package.json` dan `web/src/app/modules.ts`. `make:page` ditunda ke M3d (Q36 PK-5).

Isi yang dibuat `make:module` (sumber: `internal/devtools/templates/module`):

- **Selalu** (dengan atau tanpa `--frontend`):
  - `i18n/id.json` dan `i18n/en.json` berisi kunci `errors.bad_request`.
  - `module.go` dengan `//go:embed i18n/*.json` dan `Catalogs()`.
  - Bagian `"i18n": { "locales": ["id", "en"], "defaultLocale": "id" }` pada `module.json`.
- **Hanya dengan `--frontend`** (modul tanpa frontend tidak punya Desk: PRD-000 Q40, hanya modul ber-`web/` yang wajib `desk`):
  - Kunci contoh tambahan di `i18n/id.json` dan `i18n/en.json`: `desk.title`, `nav.items`, `page.items.title`, dan `page.items.description`.
  - Bagian `"desk"` pada `module.json`: `label` `desk.title`, `icon` `boxes`, dan satu menu `items` dengan `to` `/items` serta permission `<slug>.items.read` (aturan `DESK` di `module:check`).
  - Di `modules/<dir>/web/`:
    - `bunfig.toml` (preload `web/test/dom.ts` dan `setup.ts`), `package.json` (`@erpora/module-<dir>`), dan `tsconfig.json`.
    - `src/index.ts` (mengekspor `api`, `moduleInfo`, `frontendModule`), `src/info.ts`, dan `src/module.ts` (`defineModule` tanpa `navigation`; menu berasal dari `module.json`).
    - `src/routes.tsx` (route lazy `/<slug>/items`, tanpa rute indeks: indeks adalah Dashboard shell) dan `src/pages/items-page.tsx`.
    - `src/i18n/keys.gen.ts` (augmentasi kunci awal) dan `src/module.test.ts` (memastikan key, slug, dan versi sama dengan `module.json`; offline `online-only`; tanpa `navigation`; `assertDeskRoutes` atas menu `desk`; route anak dengan crumb; dan entry paket).
    - Generator menyisipkan `frontendModule` ke `web/src/app/modules.ts` lewat penanda (bagian 4).
- **Langkah lanjutan `--frontend`:** `contracts:generate <dir>` (membangkitkan `api/contract.gen.ts` dari `openapi.yaml`), `bun install`, dan `biome check --write`.

Konvensi isi halaman dan teks ada di [`frontend.md`](frontend.md) dan [`i18n.md`](i18n.md).
- **★2 Integration test repository.** `make:repository` membuat unit test mapping, bukan integration test (Q28 menghapus driver Postgres dari `go.mod` modul). Integration test repository ditulis manual; keputusan generik ditinjau di PRD-001.

## 10. Metadata resource: `metadata:check` dan `metadata:generate`

Sistem metadata resource (spec `docs/specs/resource-metadata/`, [ADR-0016](../adr/0016-metadata-sumber-kebenaran-pembangkit-kontrak-dan-migrasi.md)) menjadikan satu berkas YAML per resource sebagai sumber kebenaran bentuk datanya. Skema OpenAPI, DTO dan validasi Go, dan metadata TypeScript dibangkitkan darinya; bentuk berkas dan aturan evaluasinya ada di [`metadata.md`](metadata.md) (sisi frontend) dan skema `contracts/resource.schema.json`. Bagian ini hanya menjelaskan perintahnya. Perintah `metadata:*` hanya ada di flavor dev (tidak ada di binary rilis, sama seperti `make:*`).

| Perintah | Isi | Status |
| --- | --- | --- |
| `metadata:check` | Memuat `modules/*/metadata/**` dan `modules/*/module.json`, menyelesaikan penyertaan, timpaan, tipe domain, dan blok, lalu memeriksa skema, tipe dan batasan, kondisi, accessor, templat label, relasi, integrasi manifest, penamaan, dan aturan deprecated, salinan, dan penguncian. Hanya membaca. Exit 0 (peringatan boleh ada), 2 ada galat, 3 root repo tidak ditemukan | tersedia |
| `metadata:generate [modul...] [--check]` | Menjalankan `metadata:check` lalu membangkitkan keluaran di bawah. Argumen modul (nama direktori, mis. `sales`) membatasi cakupan; kosong berarti semua modul bermetadata. `--check` hanya membandingkan dan tidak menulis. Exit 0, 2 (galat check, drift, edit tangan, bentrok, masukan tidak sah), 3, 4 (penulisan gagal) | tersedia |
| `make:module <name> --metadata` | Kerangka metadata pada modul baru: lihat di bawah | tersedia |
| `metadata:diff`, `metadata:migrate` | Selisih metadata terhadap skema database hasil migrasi dan pembangkit migrasi goose (T2) | menyusul |
| `metadata:eject` | Menyalin keluaran bangkitan menjadi kode biasa (jalan keluar tanpa kunci mati) | menyusul |

Task: `task metadata:check`, `task metadata:generate` (`-- <modul>` membatasi), dan `task metadata:generate:check`. Kedua yang bersifat memeriksa ikut `task lint`, dan `task metadata:generate:check` ikut `task web:check` karena berkas `*.meta.gen.ts` harus sinkron sebelum typecheck dan lint. Modul tanpa `metadata/` tidak terpengaruh.

### Keluaran per modul

Semua jalur relatif `modules/<dir>/`; `<nama>` adalah bagian nama pada kunci resource (`sales.invoiceLine` menjadi `invoice_line` untuk Go dan `invoiceLine` untuk TS).

| Berkas | Isi |
| --- | --- |
| `internal/domain/<nama>_meta.gen.go` | Konstanta kunci, nama field, dan nilai enum; `<Nama>Spec` (field, aturan lintas field, penguncian; AST kondisi tertanam lewat konstruktor `sdk/metadata`). Domain tanpa I/O |
| `internal/domain/meta_runtime.gen.go` | Validator domain bersama satu modul (tipe, wajib, nullable, panjang, rentang, pola, enum, desimal, `requiredWhen`, `readonlyWhen`, `rules`) |
| `internal/presentation/http/<nama>_dto.gen.go` | `<Nama>Create` dan `<Nama>Update` (hanya field yang boleh ditulis), `Decode<Nama>Create/Update`, dan `Validate` |
| `internal/presentation/http/meta_runtime.gen.go` | Pembacaan body lewat `httpx.DecodeJSON` dan pemetaan pelanggaran ke amplop galat |
| `openapi.yaml` | Skema `<Nama>`, `<Nama>Create`, `<Nama>Update` pada `components.schemas`, bertanda `x-erp-generated: true` (lihat [`contracts.md`](contracts.md) §9) |
| `web/src/metadata/<nama>.meta.gen.ts` | `resourceMeta` untuk antarmuka generik; hanya bila modul punya `web/package.json` (lihat [`frontend.md`](frontend.md)) |

Perilaku DTO bangkitan (api-guidelines §7.3, §8): body dibaca lewat `httpx.DecodeJSON` (batas ukuran, kedalaman, panjang array, kunci ganda, tipe skalar); field tak dikenal menghasilkan `422 validation_failed` dengan `details[].code = unknown_field`, field hanya-baca `read_only`. `Validate` menghasilkan `422 validation_failed` untuk pelanggaran struktur atau `422 business_rule_violated` (kode aturan snake_case pada `details[].code`, pesan dari katalog modul). Pesan galat tidak pernah memuat nilai field. Kode struktur yang stabil: `required`, `null_not_allowed`, `read_only`, `invalid_type`, `too_short`, `too_long`, `pattern_mismatch`, `invalid_format`, `invalid_enum_value`, `invalid_decimal`, `decimal_scale_exceeded`, `decimal_precision_exceeded`, `below_minimum`, `above_maximum`, `invalid_uuid`, `invalid_date`, `invalid_datetime`, `invalid_characters`, `conflicting_deprecated_field`. Validasi sisi server tetap otoritas; keluaran tahap ini tidak membaca atau menulis data (CRUD, repository, dan otorisasi per record adalah T4, menyusul).

### Penanda `Code generated` dan aturan tulis

- Setiap berkas `.gen.go` dan `.gen.ts` berkepala `// Code generated by erpora metadata:generate. DO NOT EDIT.`; jangan diedit tangan. Keluaran deterministik (urutan stabil, tanpa cap waktu).
- Bila `metadata:check` bergalat, atau berkas keluaran yang sudah ada tidak berkepala itu (diubah tangan atau milik orang lain), `metadata:generate` gagal **sebelum menulis apa pun**; berkas bernama sama tanpa penanda tidak pernah ditimpa. `--check` gagal bila ada keluaran yang berbeda, belum ada, atau yatim.
- Berkas bangkitan yang tidak lagi punya resource (yatim, masih berkepala) dihapus pada mode tulis dan dilaporkan `basi` pada `--check`. Skema `x-erp-generated` yang yatim dibuang dari `openapi.yaml`; skema tulis tangan bernama sama menghentikan pembangkitan.
- Penulisan hanya di bawah `modules/<dir>/`, tidak mengikuti symlink atau junction, atomik dengan rollback; galat tidak membocorkan isi di luar modul. Literal di keluaran melewati satu fungsi kutip per bahasa (`strconv.Quote`, pengodean JSON, dan node YAML); golden masukan berbahaya ada di tes pembangkit.
- Bagian yang sengaja tidak dipancarkan ke klien (aturan `scope: server`, kondisi yang merujuk field `confidential` ke atas atau di luar grup default, field `clientVisible: false`, ekspresi accessor `stored`) dicetak sebagai `tidak dipancarkan ke klien: ...`.
- **Jalan keluar:** `metadata:eject` (menyusul) menyalin keluaran menjadi kode biasa. Sampai tersedia, resource yang perlu ditulis tangan dikeluarkan dengan memindahkan berkas ke nama tanpa `.gen`, menghapus baris kepala, dan menghapus resource dari metadata; berkas tanpa kepala tidak pernah disentuh generator.

## 11. Dokumen transaksional (metadata)

Resource transaksional menyertakan `sdk:document` (field `status`, `amendedFromId`, dan `lockWhen` bawaan "status bukan draft"). Pemetaan checklist `transaction-data-standards.md` §15 terhadap sistem metadata (design Komponen 9):

| Butir §15 | Dicakup sistem metadata | Tetap per resource (tidak dibangkitkan) |
| --- | --- | --- |
| 1 Status dan transisi | `sdk:document`; transisi dinyatakan metadata. `lockWhen` tersedia di `<Nama>Spec.Locked` (tahap ini) | Status khusus dan aturan transisi |
| 2 Immutabilitas dan ledger append-only | Pemicu database pengunci dokumen (T2, menyusul) | Ledger append-only |
| 3 Satu transaksi data, ledger, nomor, idempotensi, outbox, audit | Data, idempotensi, dan audit pada CRUD bangkitan (T4, menyusul) | Ledger, nomor, outbox |
| 4 Konkurensi (`version`, `If-Match`) | CRUD bangkitan (T4, menyusul) | Kunci baris berurutan |
| 5 Referensi sumber unik, `Idempotency-Key` | Idempotensi pada `POST` (T4, menyusul) | Referensi sumber ledger |
| 6 Seri penomoran | Tidak | Ya |
| 7 Uang dan kuantitas desimal | Ya: tipe `decimal` (string desimal, perbandingan desimal di Go dan TS) | Titik dan metode pembulatan |
| 8 Snapshot data referensi | Salinan field (T4, menyusul) | Pemilihan field yang disalin |
| 9 Periode akuntansi dan tanggal | Tidak | Ya |
| 10 Maker-checker | Tidak | Ya |
| 11 Rekonsiliasi saldo | Tidak | Ya |
| 12 Retensi dan penghapusan | Tidak | Ya |
| 13 Tes properti, konkurensi, idempotensi, immutabilitas | Tes validasi dan paritas; tes immutabilitas dan idempotensi bangkitan menyusul (T2, T4) | Properti dan konkurensi domain |

Perkara yang **selalu tetap per resource dan wajib ditulis tangan serta dicakup spec resource itu**: penomoran, ledger append-only, maker-checker (pemisahan tugas), periode akuntansi, dan rekonsiliasi saldo (juga retensi dan pembulatan domain). Sistem metadata tidak membangkitkan dan tidak menggantikannya.

### Kerangka metadata pada `make:module --metadata`

Tanpa `--metadata` keluaran `make:module` tidak berubah. Dengan flag itu modul baru juga memuat (spec `resource-metadata`, task 18.3):

- `metadata/item.resource.yaml`: resource `<slug>.item` dengan satu field `title` (`string` 1 sampai 200, `sortable`, `filterable`) dan `labelTemplate: "{title}"`. Field audit (`id`, `version`, `createdAt`, `updatedAt`, `createdBy`, `updatedBy`) ikut otomatis dari `sdk:audit`.
- Capability `<slug>.item.read` di `module.json` (awalan kunci resource yang diminta `metadata:check` sebelum manifest memakai `resources`), di samping `<slug>.items.read` bawaan.
- Skema `Item` pada `openapi.yaml` bertanda `x-erp-generated: true`, sehingga digantikan pembangkit di tempatnya.
- Langkah akhir `metadata:generate <dir>` (sebelum `contracts:generate` bila `--frontend`): membangkitkan skema `ItemCreate` dan `ItemUpdate`, `internal/domain/item_meta.gen.go`, `internal/presentation/http/item_dto.gen.go` beserta runtime per modul, dan (dengan `--frontend`) `web/src/metadata/item.meta.gen.ts`. Hasilnya langsung hijau untuk `metadata:check`, `metadata:generate --check`, dan `module:check`. Bila `metadata:check` bergalat (di modul mana pun pada repo), langkah ini gagal dan modul sudah tertulis; perbaiki lalu jalankan `task erpora -- metadata:generate <dir>`.

Kerangka ini hanya titik awal: tabel awal belum memuat `created_by`/`updated_by` dan handler belum mengirimnya (tambahkan migrasi dan handler, atau lihat selisihnya dengan `metadata:diff`, menyusul penuh di T2).
