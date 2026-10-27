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
| `make:module <name> [--frontend] [--depends k1,k2] [--publisher id]` | Modul baru turunan `example` di `modules/<dir>`: go.mod, `module.json`, `openapi.yaml`, empat lapisan Clean Architecture, migrasi, command `recount`, `DevSeeder`, dan test. Didaftarkan di `go.work`, `go.mod` root (`replace`), dan `internal/app/modules.go` |
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

- Handler hasil generate memakai hook use case tingkat paket (`<camel>Func`) yang secara bawaan menjawab `501` sampai Anda menghubungkannya ke use case.
- Test IDOR hasil generate memakai principal tiruan dan menegaskan `404` tanpa kebocoran. Upgrade ke `authz.Check`/`authz.Filter` saat PRD-001 menyediakannya.
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

- Generator **tidak pernah menimpa** file yang sudah ada: pesan "file sudah ada" menyebut path dan `--force`. `--force` hanya berlaku untuk file yang *dibuat*; file yang *diubah* (go.work, module.go, openapi.yaml, dst.) tidak terpengaruh.
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

- **★1 Frontend minimal.** `--frontend` membuat package `modules/<dir>/web` (`package.json`, `tsconfig.json`, `src/index.ts`, `api/contract.gen.ts`) dan mendaftarkannya di `web/package.json` dan `web/src/app/modules.ts`. `make:page` ditunda ke M3 bersama shell dan router.
- **★2 Integration test repository.** `make:repository` membuat unit test mapping, bukan integration test (Q28 menghapus driver Postgres dari `go.mod` modul). Integration test repository ditulis manual; keputusan generik ditinjau di PRD-001.
