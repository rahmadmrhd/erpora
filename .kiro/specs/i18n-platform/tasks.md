# Implementation Plan: i18n-platform

## Overview

Implementasi mengikuti tahap **I0–I7** di PRD-000 FR-25: spike mesin ICU, inti `sdk/i18n`, runtime katalog backend, API katalog, tooling, override, runtime frontend, lalu dokumentasi dan gate akhir. Pola yang dipakai ulang: `module.MigrationSource` (untuk `CatalogSource`), `contract.gen.ts` + `--check` (untuk `keys.gen.ts`), snapshot atomik (untuk `Service`), dan `admin.API` (untuk API override). Yang **tidak** berubah: amplop API dan kode error, mode single-tenant, lifecycle modul, dan seluruh aturan arsitektur D1–D11 kecuali satu penambahan allowlist D3b (Q35). Tidak ada UI manajemen istilah dan tidak ada migrasi data lama.

## Tasks

- [x] 1. Tahap I0 — Spike mesin ICU (kode spike dibuang, hanya hasil yang dicatat)
  - [x] 1.1 Spike mesin ICU Go: bandingkan `kaptinlin/messageformat-go/mf1` dan parser subset sendiri + `golang.org/x/text/feature/plural`
    - Kerjakan di direktori scratchpad (bukan di repo). Tulis 30 kasus awal (plural `id`/`en`, `select`, `=N`, `#`, kutip ICU `'{'`, argumen hilang, tag rich `<b>`, pesan sangat bersarang, pesan sangat panjang) dalam format `{ "locale", "message", "args", "expected" }`.
    - Jalankan keduanya terhadap kasus itu; catat: kasus yang lolos, perilaku argumen hilang, dukungan tag rich, ukuran dependensi transitif, dan apakah dapat dibatasi (`Limits`).
    - _Requirements: 2.1, 2.2, 2.3, 2.5_
  - [x] 1.2 Spike ukuran `intl-messageformat` di frontend
    - Bangun bundel kecil dengan `Bun.build` (minify + gzip) yang mengimpor `intl-messageformat` dan memformat satu pesan plural + rich; catat ukuran gzip dan bandingkan dengan anggaran NFR-04 (entry shell ≤ 200 KB gzip). Catat juga ukuran bila hanya `@formatjs/icu-messageformat-parser` yang dipakai.
    - _Requirements: 6.1, 6.7_
  - [x] 1.3 Catat keputusan spike
    - Perbarui `design.md` (D-2, D-3) dengan pilihan akhir, alasan, dan versi yang di-pin; tambahkan satu kalimat hasil spike ke PRD-000 Q35 (butir D-2/D-3). Bila mf1 lolos ≥ semua kasus inti, pilih mf1; selain itu parser sendiri.
    - Commit: `docs(spec): hasil spike I0 mesin ICU`
    - _Requirements: 2.1, 11.5_

- [x] 2. Tahap I1 — Inti `sdk/i18n` (modul Go `sdk`)
  - [x] 2.1 `sdk/i18n/limits.go` dan tipe `Issue`
    - `Limits{MaxMessageLen, MaxDepth, MaxOptions, MaxArgs, MaxKeys, MaxFileBytes, MaxKeyLen}` dengan `DefaultLimits()` (2000, 8, 20, 32, 5000, 1 MB, 200). `Issue{File, Locale, Key, Code, Detail}` dengan kode: `invalid_json`, `duplicate_key`, `flatten_collision`, `invalid_segment`, `invalid_icu`, `html_not_allowed`, `too_large`.
    - _Requirements: 2.5, 10.4_
  - [x] 2.2 Fixture konformansi dan harness uji Go (merah dulu)
    - Buat `sdk/i18n/testdata/conformance.json` dari kasus spike 1.1 (dibersihkan, ≥ 40 kasus: plural `id`/`en`, `select`, `=N`, `#`, escape/kutip, argumen hilang, rich, batas ukuran). Tulis `conformance_test.go` yang memuat fixture dan memanggil `Compile`+`Format` (gagal karena belum ada implementasi).
    - _Requirements: 9.1, 9.2_
  - [x] 2.3 Tulis test `ParseCatalog` (merah dulu)
    - Kasus: pohon valid diratakan (`note.title`, `note.status.draft`); segmen bertitik `{"a.b":{"c":"x"}}`; tabrakan `{"a.b":{"c":..}}` vs `{"a":{"b.c":..}}` → `flatten_collision` menyebut kedua jalur; kunci ganda pada tingkat sama → `duplicate_key`; karakter terlarang, segmen kosong, `..`, awalan/akhiran titik, `::` → `invalid_segment`; pesan berisi HTML (`<script>`) → `html_not_allowed`; JSON rusak; berkas > `MaxFileBytes`; kunci > `MaxKeys`.
    - **Validates: Requirements 1.4, 1.5, 1.6, 1.7, 8.1**
  - [x] 2.4 Implementasi `ParseCatalog(r io.Reader, lim Limits) (Flat, []Issue)`
    - `sdk/i18n/catalog.go` + `flatten.go`: baca dengan `json.Decoder.Token()` (pembaca token melaporkan kunci ganda yang dibuang `encoding/json` biasa), ratakan dengan `.`, deteksi tabrakan, validasi karakter segmen, deteksi HTML (`<` hanya sebagai tag rich bernama `[a-z][a-z0-9]*`), hormati `Limits`. Tidak ada I/O selain `r`.
    - _Requirements: 1.4, 1.5, 1.6, 1.7, 2.4, 10.4_
  - [x] 2.5 Implementasi `Compile`/`Message`/`Format` sesuai keputusan I0
    - `sdk/i18n/icu.go`: antarmuka `Message{Format(loc Locale, args Args) (string, error); Params() []Param}`, `Compile(src, lim)`, `Param{Name, Kind}`. Dukungan: argumen, `plural` (kategori CLDR, `=N`, `#`), `select`, tag rich (dilucuti pada `Format` teks polos), kutip ICU. `selectordinal` dan `{x, number}` ditolak. Argumen hilang → placeholder apa adanya (bukan panic). Hasil harus meloloskan fixture 2.2.
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 4.6_
  - [x] 2.6 `Locale`, konteks, dan `T`
    - `sdk/i18n/localizer.go`: `type Locale string`, `WithLocale`, `WithScope`, antarmuka `Resolver` kecil yang disuntik platform (`Resolve(ctx, ns, key, loc) (src string, ok bool)`), `T(ctx, key, args...)` (kunci relatif ke scope; `ui::`/`platform::` absolut; namespace modul lain ditolak → `ns::key` + peringatan). Aman untuk banyak goroutine, tidak pernah panik (recover mengembalikan kunci).
    - _Requirements: 4.1, 4.6_
  - [x] 2.7 Tulis test properti dan fuzz inti SDK
    - **P1 Perataan deterministik** (`testing/quick`: pohon acak tanpa tabrakan → setiap daun dapat dicari lewat kunci lengkap; dua jalur sama → selalu `flatten_collision`).
    - **P4 Format total dan terbatas** (`go test -fuzz` pada `Compile`/`Format`: tidak panik, selesai dalam batas waktu, input di atas batas ditolak oleh `Compile`).
    - **P12 Namespace tertutup** (`T` dengan scope `m` tidak pernah memberi pesan dari namespace modul lain).
    - **Validates: Requirements 1.6, 2.5, 4.1, 4.6, 10.4**
  - [x] 2.8 Naikkan `golang.org/x/text` di `sdk/go.mod` (indirect `v0.20.0` → versi root `v0.43.0` atau sesuai `go mod tidy`), jalankan `task check:gomod check:gowork-off`, dan (keputusan G9, 29 Okt 2026) `SDKVersion` tetap `0.1.0`: penambahan aditif SDK tidak menaikkan versi selama SDK belum di-tag (ADR-0006 butir 2 dan 5; preseden `ContractSource`), dan `^0.1` pada manifest modul akan putus bila naik ke 0.2.0.
    - _Requirements: 4.6_

- [x] 3. Checkpoint — inti SDK
  - Jalankan `task lint arch check:gowork-off test` dan pastikan `sdk/i18n` hijau (termasuk fuzz singkat). Tanyakan ke user bila ada keraguan.

- [x] 4. Tahap I2 — Runtime katalog backend (tanpa override)
  - [x] 4.1 Registri bahasa `internal/platform/i18n/locales.json` + loader + validasi (test dulu)
    - `locales.go`: struct `Registry{Default string; Locales []LocaleInfo{Code, Name, Fallback, Partial}}`, `Load()` dari embed, validasi: kode unik berpola `^[a-z]{2,3}(-[A-Z]{2})?$`, `fallback` menunjuk kode terdaftar, tanpa siklus, kedalaman ≤ 4, `default` tanpa `fallback`; fungsi `Chain(code) []string`. Isi awal: `id` (default), `en` (fallback `id`).
    - **Validates: Requirements 3.2, 3.5, 3.6, 3.9**
  - [x] 4.2 `module.CatalogSource` + properti manifest `i18n` + `module:check`
    - `sdk/module/module.go`: antarmuka `CatalogSource{Catalogs() fs.FS}`. `sdk/module/manifest.go`: struct `I18n{Locales []string; DefaultLocale string}` opsional. Perbarui `contracts/module.schema.json` (properti `i18n`, tetap `schema_version: "1"`), bangkitkan ulang `packages/module-sdk/src/manifest.gen.ts`, dan perluas pemeriksaan `module:check` (`internal/devtools/modcheck`): bahasa terdaftar di registri, `defaultLocale ∈ locales`, berkas `<locale>.json` ada, modul ber-`CatalogSource` wajib punya bagian `i18n`. Registry (`internal/platform/modules/registry`) memetakan modul → `CatalogSource`.
    - _Requirements: 1.1, 1.3, 3.8_
  - [x] 4.3 Katalog awal `platform` dan `ui`
    - `internal/platform/i18n/catalog/{id,en}.json` + `embed.go` (subpohon `errors.<code>` untuk kode api-guidelines §8, plus kunci shell awal), dan `packages/ui/i18n/{id,en}.json` + `packages/ui/i18n/embed.go` (paket `uii18n`; mengikuti pola `web/embed.go`). Kunci dimulai kecil; blok M3 menambahkannya.
    - _Requirements: 1.1, 4.4_
  - [x] 4.4 Tulis test `Service` (merah dulu)
    - Resolusi rantai fallback (override belum ada: bawaan bahasa persis → fallback registri → default → `ns::key`); namespace modul nonaktif → tidak tersedia; `ui`/`platform` selalu aktif; ETag identik untuk isi sama dan berbeda bila isi berbeda; peringatan fallback sekali per (bahasa, ns, kunci) di mode dev/test.
    - **Validates: Requirements 3.2, 3.3, 3.7, 5.2, 5.7**
  - [x] 4.5 Implementasi `internal/platform/i18n.Service` (snapshot atomik)
    - `service.go`, `resolve.go`: bangun snapshot immutable dari katalog `platform`, `ui`, dan tiap modul terinstal; parse dengan `sdk/i18n.ParseCatalog`; **tolak start** bila ada galat struktural (pesan menyebut berkas dan kunci); `Catalog(...) View`, `Resolve(...)`, `Revisions(...)`, ETag `W/"<sha256 16 byte>"` atas peta efektif kanonik; cache pesan terkompilasi; penyuntik `Resolver` ke `sdk/i18n`. Ketidaksesuaian antar bahasa hanya `slog.Warn`.
    - _Requirements: 3.2, 3.3, 3.7, 4.2, 5.7, 8.1_
  - [x] 4.6 Negosiasi bahasa dan middleware locale (test dulu)
    - `negotiate.go`: `Accept-Language` (maks 1 KB, lebih diabaikan) dicocokkan dengan `golang.org/x/text/language.Matcher` terhadap registri; hasil selalu anggota registri. Middleware `localeContext` di `internal/platform/httpserver/middleware.go` setelah `tenantContext`; `SetModule` juga memasang `i18n.WithScope(ctx, slug)`; hook lifecycle dan command memperoleh scope serupa dari platform.
    - **Validates: Requirements 3.1, 3.4, 4.3**
  - [x] 4.7 Integrasi `sdk/httpx.WriteError`
    - Tambahkan antarmuka kecil di `sdk/i18n` yang dibaca `WriteError` dari ctx; aturan `errors.<code>` di scope modul → `platform::errors.<code>` → `message` dari pemanggil. `code` dan `details` tidak berubah; 5xx tetap tanpa detail internal. Tanda tangan `WriteError` tidak berubah.
    - _Requirements: 4.4_
  - [x] 4.8 Rangkai di `internal/app/assemble.go`
    - Bangun `Service`, pasang middleware, simpan di `assembly`; startup gagal bila katalog struktural rusak. Pastikan arch test D-rules tetap hijau.
    - _Requirements: 4.2, 4.3_
  - [x] 4.9 Tulis test properti runtime backend
    - **P2 Rantai fallback berhingga dan deterministik**, **P3 Pencarian total**, **P7 ETag mengikuti isi**, **P9 Negosiasi tertutup** (fuzz string `Accept-Language`).
    - **Validates: Requirements 3.2, 3.3, 3.4, 5.7**

- [x] 5. Checkpoint — runtime backend
  - Jalankan `task lint arch check:gowork-off test`; pastikan test `Service`, negosiasi, dan `WriteError` hijau serta archcheck tidak melaporkan pelanggaran baru.

- [x] 6. Tahap I3 — API katalog
  - [x] 6.1 Kontrak OpenAPI dan pedoman API
    - `contracts/openapi/platform.yaml`: path `/api/platform/i18n/{locale}/{namespace}` (`x-erp-public`, `ETag`, 200/304/404), skema `I18nCatalogEnvelope`, dan bagian `i18n` pada skema bootstrap (additive). `docs/architecture/api-guidelines.md`: catatan §8 bahwa `error.message` terlokalisasi (kode tidak berubah) dan kode baru `invalid_i18n_override` di katalog (dipakai tahap I5).
    - _Requirements: 5.1, 5.4, 5.6_
  - [x] 6.2 Handler katalog dan pendaftaran route
    - Di `platform.core` (`internal/platform/i18n/admin` atau `internal/platform/modules/admin`): `GET /api/platform/i18n/{locale}/{namespace}` publik; validasi `locale` terhadap registri dan `namespace` terhadap `ui`/`platform`/modul aktif (selain itu `404 not_found` tanpa membedakan penyebab); `ETag`/`If-None-Match` → 304; `Cache-Control: private, no-cache`; respons `{ "data": { locale, namespace, revision, messages } }` dengan fallback sudah digabung.
    - _Requirements: 5.1, 5.2, 5.5, 5.6, 5.7_
  - [x] 6.3 Bootstrap memuat `i18n`
    - `internal/platform/bootstrap`: tambah `I18n{DefaultLocale, Locales[{Code,Name}], Revisions map[ns]string}` (additive, hanya namespace aktif); isi dari `Service` di `bootstrapFrom` (`assemble.go`); `ETag` bootstrap ikut berubah bila revisi katalog berubah.
    - _Requirements: 5.3_
  - [x] 6.4 Bangkitkan tipe TS dan perbarui klien
    - `task erpora -- contracts:generate` → `packages/platform-contracts`; perbarui `client.check.ts` bila perlu.
    - _Requirements: 5.4_
  - [x] 6.5 Tulis test API dan kesesuaian kontrak
    - Katalog `id`/`en`; modul nonaktif → 404; bahasa/namespace tak dikenal → 404 identik; `ETag` + `If-None-Match` → 304; bootstrap hanya memuat namespace aktif; test kesesuaian route ↔ kontrak (`internal/app/contract_conformance_test.go`) dan `task contracts:check` lolos.
    - **Validates: Requirements 5.1, 5.2, 5.3, 5.4, 5.6, 10.2**

- [x] 7. Checkpoint — API katalog
  - Jalankan `task lint arch check:gowork-off test contracts:check`; bila menyentuh DB jalankan `task test:integration` melalui PostgreSQL di VPS (tunnel + `ERP_TEST_DATABASE_URL`).

- [x] 8. Tahap I4 — Tooling pemeriksa, generator, dan templat modul
  - [x] 8.1 Tulis test `i18ntools` (merah dulu)
    - Kasus galat (JSON/ICU tidak valid, kunci ganda, tabrakan, karakter terlarang, HTML, namespace salah, registri/fallback tidak valid, modul mendeklarasikan bahasa tak terdaftar) → exit code 2; peringatan (hilang/berlebih/parameter tak cocok) → exit 0; `--strict` → peringatan menjadi galat; bahasa `partial` tidak melaporkan kunci hilang tetapi tetap melaporkan berlebih; `--json`; exit code 3 untuk kegagalan konfigurasi.
    - **Validates: Requirements 8.1, 8.3, 8.4, 8.5, 8.6**
  - [x] 8.2 Implementasi `internal/devtools/i18ntools` (`check`, `status`)
    - Muat katalog dari filesystem (bukan embed), registri, dan manifest; pakai `sdk/i18n.ParseCatalog`/`Compile` yang sama dengan runtime; laporan teks dan `--json` (file, bahasa, kunci lengkap); `status` menampilkan persentase kelengkapan per bahasa per namespace. Daftarkan `i18n:check` dan `i18n:status` di `internal/devtools/commands`.
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_
  - [x] 8.3 `i18n:add <locale> [module…]`
    - Buat kerangka katalog dari bahasa default (nilai kosong ditandai `TODO`) dan tambahkan ke registri bila belum ada; idempotent.
    - _Requirements: 8.8_
  - [x] 8.4 Generator kunci terketik `i18n:generate [--check]` (test dulu)
    - Dari katalog bahasa default tiap namespace tulis `keys.gen.ts` (antarmuka kunci → tipe argumen: `string | number` untuk argumen sederhana, `number` untuk `plural`, gabungan literal untuk `select`, tag rich) dengan augmentasi `declare module "@erpora/platform-web" { interface I18nCatalogs { … } }`; target: `modules/<x>/web/src/i18n/keys.gen.ts`, `packages/ui/src/i18n/keys.gen.ts`, `packages/platform-web/src/i18n/keys.platform.gen.ts`. `--check` gagal bila usang.
    - **Validates: Requirements 7.1, 7.3**
  - [x] 8.5 Task dan gate lokal
    - `Taskfile.yml`: `i18n:check` dan `i18n:generate` (tanpa `rm`/`cp`/`sed`); panggil `i18n:check` dari `task lint` (galat struktural menggagalkan, peringatan hanya dicetak) dan `i18n:generate -- --check` dari `task web:check`.
    - _Requirements: 8.9, 11.1_
  - [x] 8.6 Templat `make:module` dan modul referensi
    - Perbarui templat di `internal/devtools/templates` dan `internal/devtools/gen`: `i18n/id.json`, `en.json` (kunci contoh bersarang), `//go:embed i18n/*.json` + `Catalogs()`, bagian `i18n` di `module.json`, dan `web/src/i18n/keys.gen.ts`. Tambahkan katalog `id`/`en` ke `modules/example` dan `modules/exampleaddon` (termasuk satu `errors.<code>` yang dipakai API `example`); perbarui golden test generator. Jalankan `task test:generator` (lama, Docker + Bun; di VPS sesuai aturan proyek).
    - _Requirements: 1.8, 11.3_
  - [x] 8.7 Arch rule `I1` (test dulu)
    - `internal/archcheck`: berkas `modules/**` tidak boleh mengimpor `internal/platform/i18n` dan tidak boleh memuat literal `"<ns>::…"` bernamespace modul lain pada argumen `i18n.T` (AST). Tambahkan fixture pelanggaran yang terbukti gagal; catat aturan `I1` di PRD-000 §4.2.
    - **Validates: Requirements 1.2**

- [x] 9. Checkpoint — tooling
  - Jalankan `task lint arch check:gowork-off test web:check` dan pastikan `task erpora -- i18n:check` serta `i18n:generate --check` hijau pada repo.

- [x] 10. Tahap I5 — Override istilah
  - [x] 10.1 Migrasi `migrations/control/0003_i18n_overrides.sql` (test dulu)
    - Tabel `i18n_overrides` dan `i18n_revisions` sesuai `design.md` (kolom `tenant_id` NOT NULL, `CHECK` pola namespace/locale/key, panjang pesan 1–2000, kunci utama komposit); tambahkan ke test migrasi (naik, idempoten) di `tests/integration`.
    - **Validates: Requirements 12.1**
  - [x] 10.2 Store GORM + allowlist D3b
    - `internal/platform/i18n/store`: `List`, `Upsert` (satu transaksi dengan kenaikan `i18n_revisions` hanya bila nilai berubah), `Delete` (revisi naik hanya bila baris ada); semua query memfilter `tenant_id`. Perbarui `internal/archcheck` (allowlist D3b menambah `internal/platform/i18n/store`) dan tes aturan.
    - _Requirements: 12.1, 12.4, 12.6_
  - [x] 10.3 Tulis test `Service` override (merah dulu)
    - Override mengalahkan bawaan pada bahasa yang sama; override pada bahasa fallback; `PUT` dua kali tidak menaikkan revisi kedua kali (P8); `Delete` kunci tak ada tidak menaikkan revisi; validasi: kunci tak ada di katalog bawaan bahasa default → `unknown_key`, ICU tidak valid, `Params()` berbeda → `param_mismatch` (P10), HTML, terlalu panjang; isolasi tenant A vs B (P6); cache segar setelah mutasi dan setelah revisi DB berubah (TTL).
    - **Validates: Requirements 12.3, 12.4, 12.6**
  - [x] 10.4 Implementasi override di `Service`
    - `SetOverride`, `DeleteOverride`, cache `(tenant, ns, locale) → Flat` dengan pemeriksaan `i18n_revisions` (TTL 5 detik); katalog efektif menggabungkan override di atas bawaan sebelum fallback; log terstruktur `i18n_override_changed` (tenant, ns, locale, key, actor, trace_id); bila DB tak terjangkau, tetap sajikan bawaan dengan peringatan.
    - _Requirements: 3.2, 5.7, 12.3, 12.6, 12.7_
  - [x] 10.5 API override (kontrak dulu)
    - `contracts/openapi/platform.yaml`: `GET /api/platform/i18n/overrides` (`platform.i18n.read`), `PUT` dan `DELETE /api/platform/i18n/overrides/{namespace}/{locale}/{key}` (`platform.i18n.manage`); tambahkan `platform.i18n.read|manage` ke `admin.Capabilities`; kode `invalid_i18n_override` dengan `details[].code`. Handler: gerbang `ERP_ENV=development` atau `ERP_MODULES_ADMIN_API=true` (selain itu `404` sebelum permission), `Content-Type: application/json` wajib (`415`), batas body 8 KB, `If-Match` opsional (`412`), actor `api:dev`. Bangkitkan tipe TS.
    - _Requirements: 12.2, 12.3, 12.8, 12.9_
  - [x] 10.6 Test API override dan IDOR
    - Permission (stub authz deny → 403), mutasi mati di `production` → 404, `415`, `412`, validasi 400, `PUT` idempoten, `code` galat tetap stabil walau `message` ditimpa, `tenantId` di body ditolak; test IDOR tingkat store (tenant A vs B) untuk list/put/delete; `contracts:check` lolos.
    - **Validates: Requirements 12.2, 12.3, 12.4, 12.8, 12.9**
  - [x] 10.7 CLI produksi `erpora i18n override set|list|rm`
    - `cmd/erpora` (di samping `modules`): memakai `Service` langsung, konfirmasi untuk `rm` (`--yes` untuk skrip), exit code 2/3/4; test golden untuk `list` dan exit code.
    - _Requirements: 12.5_
  - [x] 10.8 Test integrasi alur override (PostgreSQL di VPS)
    - `PUT` → revisi naik → `GET` katalog berubah (ETag baru, ETag lama → 200) → `DELETE` → kembali ke bawaan; `error.message` modul `example` berbahasa sesuai `Accept-Language` dan tertimpa override sementara `code` tetap.
    - **Validates: Requirements 5.7, 11.2, 12.6**

- [x] 11. Checkpoint — override
  - Jalankan `task lint arch check:gowork-off test contracts:check test:integration`; tanyakan ke user bila ada temuan keamanan.

- [ ] 12. Tahap I6 — Runtime frontend (`@erpora/platform-web`)
  - [ ] 12.1 Kerangka paket `packages/platform-web`
    - Buat `package.json` (`@erpora/platform-web`, versi dependensi dipin, workspace `packages/*` sudah terdaftar), `tsconfig.json` (`strict` + `noUncheckedIndexedAccess`), `src/index.ts`, dan `src/i18n/`. Tambahkan dependensi `intl-messageformat` (versi dipin hasil spike 1.2) dan dev dependency `fast-check`; jalankan `bun install` agar `bun.lock` terkunci. Biome dan `dependency-cruiser` (F1–F3) tetap lolos. Koordinasikan dengan M3a: paket ini dipakai bersama.
    - _Requirements: 6.7, 11.1_
  - [ ] 12.2 Tulis test konformansi TS (merah dulu)
    - `src/i18n/conformance.test.ts` membaca `sdk/i18n/testdata/conformance.json` lewat path relatif repo dan membandingkan keluaran `format.ts` dengan hasil yang diharapkan.
    - **Validates: Requirements 9.1, 9.2, 9.3**
  - [ ] 12.3 `src/i18n/format.ts`
    - Bungkus `intl-messageformat`: `compile(locale, message)` dengan cache, argumen hilang tidak melempar, tag rich → fungsi `rich`, teks polos melucuti tag; angka/tanggal diformat di kode (`Intl`) dan diteruskan sebagai teks.
    - _Requirements: 2.1, 2.3, 2.4, 2.6_
  - [x] 12.4 `loader.ts` dan `provider.tsx`
    - `I18nProvider` + `useTranslation(ns)`: `useSuspenseQuery` dengan key `[tenantId, "i18n", locale, ns]`, `staleTime: Infinity`, kirim `If-None-Match`, muat ulang bila `bootstrap.i18n.revisions[ns]` berubah, tanpa kunci mentah selama memuat (Suspense). Bahasa pilihan: `localStorage["erpora.locale"]` dengan `try/catch`, divalidasi terhadap `bootstrap.i18n.locales`, lalu `navigator.languages`, lalu `defaultLocale`; perbarui `document.documentElement.lang`; ganti bahasa memuat ulang katalog tanpa reload halaman.
    - _Requirements: 6.1, 6.2, 6.3, 6.5_
  - [x] 12.5 Tipe `t`/`rich` dan helper uji
    - `I18nCatalogs` (diperluas lewat augmentasi `keys.gen.ts`), `t<K>(key, ...args)` terketik, `rich`, `createTestI18n({ ns: {...} })` untuk `bun test` tanpa jaringan; `@erpora/ui` memakai konteks tipis `UiLabels` yang diisi dari namespace `ui` (tanpa mengimpor `platform-web`).
    - _Requirements: 6.1, 6.4, 6.6, 7.1, 7.2_
  - [x] 12.6 Header `Accept-Language` pada klien API
    - `packages/platform-contracts/src/client.ts`: terima `getLocale()` dan sisipkan `Accept-Language` pada setiap request; `client.check.ts` tetap lolos.
    - _Requirements: 6.2, 4.4_
  - [x] 12.7 Test komponen runtime
    - `useTranslation`: Suspense tanpa kunci mentah, ganti bahasa, `rich`, validasi bahasa pilihan, kegagalan `localStorage`, bahasa tak terdaftar, `If-None-Match`/304 (ETag disimpan); uji tipe: kunci salah ketik atau argumen salah tipe gagal pada `tsc --noEmit` (fixture negatif).
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.5, 7.2**
  - [ ]* 12.8 Test properti `fast-check` (opsional)
    - Properti: `format` tidak melempar untuk pesan/argumen acak yang lolos kompilasi; hasil identik untuk panggilan berulang (cache aman); validasi bahasa pilihan selalu anggota daftar atau default.
    - **Validates: Requirements 2.3, 6.5, 9.3**
  - [ ] 12.9 Gate frontend
    - `bun run typecheck`, `bun run lint`, `bun test`, `bun run build`; periksa ukuran entry shell ≤ 200 KB gzip (NFR-04) dan `csp-check`; `i18n:generate --check`.
    - _Requirements: 11.1_

- [ ] 13. Checkpoint — runtime frontend
  - Jalankan `task web:check` dan `ERP_REQUIRE_BUN=1 task test`; tanyakan ke user bila anggaran bundle terlampaui (keputusan D-3 cadangan).

- [ ] 14. Tahap I7 — Dokumentasi dan gate akhir
  - [x] 14.1 `docs/module-sdk/i18n.md`
    - Tulis panduan: tata letak katalog dan namespace, sintaks ICU yang didukung (dan yang tidak), konvensi kunci (`fields.*`, `errors.*`, `status.*`), segmen bertitik dan aturan tabrakan, registri bahasa dan fallback, alur penerjemah (`i18n:add`, `check`, `status`), kustomisasi istilah (override), kunci terketik, dan catatan keamanan (tanpa HTML dan data sensitif). Tautkan dari `docs/module-sdk/README.md` dan perbarui `docs/module-sdk/generators.md`.
    - _Requirements: 11.4_
  - [x] 14.2 Selaraskan dokumen proyek
    - `CLAUDE.md` dan `AGENTS.md` (harus identik, `TestAgentsMDMatchesClaudeMD`): tambahkan butir i18n (katalog per modul, `task i18n:check`, jangan menanam string UI); perbarui `docs/architecture/api-guidelines.md` bila belum (task 6.1); tandai FR-25 selesai di PRD-000 dan perbarui `Requirement 12.5` di `requirements.md` agar sama dengan penamaan CLI final (`erpora i18n override`).
    - _Requirements: 11.4, 11.5_
  - [ ] 14.3 Gate akhir lengkap
    - Jalankan berurutan: `task lint arch check:gowork-off test`, `task contracts:check`, `task web:check`, `task test:integration` (PostgreSQL di VPS), dan `task test:generator`; verifikasi manual `Accept-Language: en` pada modul `example`, override lewat CLI, dan `i18n:check` pada repo. Catat hasil dan hal yang tidak terverifikasi.
    - _Requirements: 11.1, 11.2, 11.3_
  - [ ] 14.4 Review akhir independen
    - Satu review menyeluruh oleh agent baru (diff + spec, tanpa kesimpulan implementer), mencakup kesesuaian spec dan skill `security-audit` terfokus pada perubahan; putaran perbaikan sesuai aturan CLAUDE.md.
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5_

- [ ] 15. Final checkpoint — Ensure all tests pass
  - Pastikan semua gate hijau dan tidak ada temuan terbuka; laporkan terpisah yang dikerjakan, yang menunggu konfirmasi, dan yang sengaja tidak dikerjakan. M3a boleh dimulai setelah grup 12 selesai.

## Notes

- Setiap task mereferensi nomor requirement (`R.n`) pada `requirements.md`; grup 1–2 dan 4 menutup tahap I0–I2, grup 6 = I3, grup 8 = I4, grup 10 = I5, grup 12 = I6, grup 14 = I7 (PRD-000 FR-25.2).
- **TDD:** untuk setiap implementasi, test yang bersangkutan ditulis dan dijalankan merah lebih dulu (ditandai "test dulu"/"merah dulu"), lalu implementasi sampai hijau. Satu commit per sub-task dengan pesan konvensional (`feat(i18n): …`, `test:`, `docs:`).
- **Test yang butuh PostgreSQL** dijalankan di VPS lewat tunnel SSH (`ERP_TEST_DATABASE_URL`); jangan memulai container di laptop. `task test:generator` juga dijalankan di VPS.
- **Windows:** `-race` dilewati lokal; CI Linux tetap menjalankannya (hook global pada test harus memakai `atomic.Pointer`). Jangan menambah perintah Taskfile yang memakai utilitas Unix.
- **Kelas beban untuk delegasi:** Haiku untuk 4.3, 8.5, 14.1, 14.2 (mekanis/dokumen); Sonnet untuk sisanya (logika inti, integrasi lintas file, test); review akhir 14.4 oleh model tertinggi yang tersedia.
- **Spike 1.1–1.2 tidak masuk repo**; hanya hasilnya yang dicatat (1.3). Bila spike gagal memilih pustaka Go, task 2.5 mengimplementasikan parser sendiri (cadangan di `design.md`).
- **Di luar cakupan:** UI manajemen istilah, preferensi bahasa per pengguna (PRD-001), template email/notifikasi (PRD Notifications), terjemahan data master, dan pengetatan akses katalog (PRD-001).
- Keputusan D-1 sampai D-9 sudah disetujui (PRD-000 Q35); jangan mengerjakan perubahan di luar spek tanpa konfirmasi.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["1.3"] },
    { "id": 2, "tasks": ["2.1", "2.2", "2.8"] },
    { "id": 3, "tasks": ["2.3", "2.5", "2.6"] },
    { "id": 4, "tasks": ["2.4", "2.7"] },
    { "id": 5, "tasks": ["3"] },
    { "id": 6, "tasks": ["4.1", "4.2", "4.3"] },
    { "id": 7, "tasks": ["4.4", "4.6", "4.7"] },
    { "id": 8, "tasks": ["4.5"] },
    { "id": 9, "tasks": ["4.8", "4.9"] },
    { "id": 10, "tasks": ["5"] },
    { "id": 11, "tasks": ["6.1", "8.1", "8.7"] },
    { "id": 12, "tasks": ["6.2", "6.3", "8.2", "8.3"] },
    { "id": 13, "tasks": ["6.4", "6.5", "8.4", "8.5", "8.6"] },
    { "id": 14, "tasks": ["7", "9"] },
    { "id": 15, "tasks": ["10.1", "10.2", "10.3", "12.1"] },
    { "id": 16, "tasks": ["10.4", "12.2", "12.3"] },
    { "id": 17, "tasks": ["10.5", "12.4", "12.5", "12.6"] },
    { "id": 18, "tasks": ["10.6", "10.7", "10.8", "12.7", "12.8"] },
    { "id": 19, "tasks": ["11", "12.9"] },
    { "id": 20, "tasks": ["13"] },
    { "id": 21, "tasks": ["14.1", "14.2"] },
    { "id": 22, "tasks": ["14.3"] },
    { "id": 23, "tasks": ["14.4"] },
    { "id": 24, "tasks": ["15"] }
  ]
}
```
