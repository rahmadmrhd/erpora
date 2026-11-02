# Implementation Plan: Metadata Resource (satu sumber kebenaran)

## Overview

Implementasi mengikuti empat tahap spec dan tiap tahap berakhir pada checkpoint beserta review akhir putaran. **T1 Inti** (grup 1 sampai 9): skema dan pemuat aman, penamaan camelCase, `sdk/metadata` (evaluator Go dan TS dengan korpus vektor bersama), resolver (penyertaan, timpaan, abstrak, tipe domain, blok), `metadata:check`, pembangkit OpenAPI, DTO dan validator Go, metadata TypeScript, modul fixture, dan migrasi `example`. **T2 Database** (10 sampai 15): mesin selisih, pembangkit migrasi, aturan deprecated dan rename, pemicu immutabilitas dokumen. **T3 Frontend** (16 dan 17): paket metadata `@erpora/platform-web` untuk M3c dan M3d. **Dokumentasi** (18). **T4 Relasi dan CRUD** (19 sampai 25): **diblokir sampai PRD-001 FR-110 sampai FR-115 dan FR-119 tersedia di `sdk/authz`** (D-12, Requirement 16.7).

Pola yang dipakai ulang: `contracts/embed.go` dan `module.schema.json` (skema dibenamkan), `contracttools` (`--check`, penyuntikan YAML terstruktur, `git.go` untuk pembanding tag rilis), `i18ntools` (pola `generate --check`), `internal/devtools/commands` (grup perintah), `pgtest` dan `ERP_TEST_DATABASE_URL` (integrasi), `modcheck` (`module:check`), dan golden generator. Yang **tidak** berubah: amplop API, mode single-tenant, larangan `AutoMigrate`, aturan arsitektur D1 sampai D11 dan D3b (tanpa pengecualian baru untuk kode bangkitan), `@erpora/ui` sebagai daun (F4), dan keluaran tulis tangan modul yang sudah ada.

**Penafsiran yang dicatat (perancang tasks, untuk dikonfirmasi saat review rencana ini):** tabel Tahap menuntut paritas Go dan TS pada exit gate T1, sehingga **evaluator dan desimal TypeScript beserta pelari korpus vektor dibangun di T1** (grup 3), sedangkan T3 melengkapi sisanya (validator, registri tipe field, pemuat malas, anggaran bundle). Ini tidak menambah perilaku di luar spec.

Keputusan K1 sampai K18, P-1 sampai P-8, dan D-1 sampai D-35 sudah disetujui pemilik produk (1 Nov 2026); K19 sampai K24 (jawaban konfirmasi 1 dan 2 Nov 2026: `example` aditif, `math/big`, linter desimal nullable, `sdk:audit` otomatis, lifecycle status baku, dan `statusAxes`) beserta D-36 sampai D-38 juga disetujui pemilik produk; K25 sampai K34 dan D-39 sampai D-46 (konfirmasi 2 Nov 2026: regex identifier, kata tercadang per target, `reason` pelonggaran, `unmanagedColumns` berobjek, `lockWhen` hanya dokumen, penamaan status baku, pemicu TRUNCATE, `Down` hanya-development, siklus `transaction-data-standards.md`, modul tanpa tag) juga disetujui pemilik produk. Pengaman K18: `Down` tidak dapat dibalik untuk tahap contract disetujui (K32); penjaga data pada migrasi penghapusan masih usulan perancang yang menunggu konfirmasi. Konsep `unmanagedColumns` (K17) ditegakkan penuh sesuai K28 (entri `{name, classification: highly_confidential, reason}`). Penyimpangan lain dari spek **menunggu konfirmasi**; jangan memutuskan sendiri (lihat instruksi global "Menangani temuan review").

## Tasks

- [x] 1. Prasyarat dan keputusan pelaksana (Kelas beban: 1.1 Haiku; 1.2 Sonnet)
  - [x] 1.1 Verifikasi baseline
    - Pastikan branch bersih dan basis (`main`) hijau: `task lint arch check:gowork-off test`, `task contracts:check`, `task web:check`. Catat di "Catatan hasil": jumlah tes Go dan frontend, ukuran JS awal gzip (anggaran 200 KB), versi Go dan Bun. Bila ada yang merah sebelum perubahan, **berhenti dan laporkan**.
    - _Requirements: 22.1_
  - [x] 1.2 Putuskan dan catat keputusan pelaksana
    - Pilih dan catat (di "Catatan hasil" dan `design.md` bagian "Diserahkan kepada pelaksana"): pustaka YAML aman (batas alias dan kedalaman), pustaka desimal Go (untuk `sdk/metadata`) dan TypeScript, pustaka property test (`rapid` atau `testing/quick`; `fast-check`), mekanisme property test dua sisi, lokasi modul fixture (di bawah `testdata`, tidak didaftarkan ke `internal/app`), dan **format kunci resource bermulti-kata** pada manifest dan permission (verifikasi aturan `capability_prefix` registry; mis. `sales.invoiceLine` menjadi `sales.invoice_line`) beserta pemetaannya di `design.md` Komponen 13.
    - Dependensi baru wajib dipin, lolos `go mod verify`/`bun audit`, dan dicatat lisensi serta alasannya; pustaka yang menambah dependensi besar ditanyakan dulu ke pemilik produk.
    - _Requirements: 1.2, 1.8, 4.4, 7.1_

- [x] 2. Skema, pemuat aman, dan penamaan (Kelas beban: Sonnet)
  - [x] 2.1 Tulis test skema dan pemuat (merah dulu)
    - Tabel uji pelanggaran skema (`kind`, `schemaVersion`, kunci tak dikenal, tipe tak sah) dan batas ukuran: > 200 field, > 32 node atau kedalaman > 4 pada kondisi, > 16 relasi, > 100 nilai `in`/enum, `pattern` > 200 karakter. YAML berbahaya: alias bomb, kedalaman berlebih, berkas > 256 KB, tag kustom. Berkas tak boleh memuat fragmen SQL, URL, atau kode (nilai hanya data). `metadata:check` melaporkan semua pelanggaran dengan berkas dan baris tanpa menulis apa pun.
    - **Validates: Requirements 1.1, 1.2, 1.3, 1.5, 1.6, 19.1**
  - [x] 2.2 Implementasi skema `resource.schema.json` dan pemuat
    - `contracts/resource.schema.json` (dibenamkan lewat `contracts/embed.go` seperti `module.schema.json`, dengan `schemaVersion`); pemuat di `internal/devtools/metadatatools/load` (build tag `devtools`) memakai pustaka dari 1.2, menolak tag kustom, membatasi alias, kedalaman, dan ukuran sebelum parse, dan memetakan galat ke berkas dan baris.
    - _Requirements: 1.1, 1.2, 1.3, 1.5, 1.6, 19.1_
  - [x] 2.3 Tulis test penamaan (merah dulu)
    - Tes properti dan tabel: konversi camelCase ke snake_case, PascalCase (inisialisme `ID`, `URL`, `UUID`), kebab-case jamak, dan kembali identik (bolak-balik); angka melekat pada kata sebelumnya (`address2Line` menjadi `address2_line`); tolak huruf kapital berurutan (`taxID`), kata kunci SQL tercadang, kata tercadang Go dan TypeScript, tabrakan setelah konversi, nama hasil gabungan (tabel `<slug>_<nama>`, pivot, indeks, FK, pemicu, kolom berawalan blok) > 63 byte, dan ketunggalan per skema.
    - **Validates: Requirements 1.4, 1.7, 1.8**
  - [x] 2.4 Implementasi paket penamaan
    - `metadatatools/naming`: fungsi murni untuk validasi dan konversi per target (tabel konversi `design.md` Komponen 13); aturan jamak untuk nama tabel ditulis eksplisit dan diuji. Kasus yang tidak tertangani aturan **jangan** ditambal dengan opsi baru tanpa konfirmasi.
    - _Requirements: 1.4, 1.7, 1.8, 10.6_

- [x] 3. `sdk/metadata`: kondisi, ekspresi, desimal, dan paritas Go dan TS (Kelas beban: Sonnet)
  - [x] 3.1 Tulis korpus vektor dan test evaluator (merah dulu)
    - `contracts/metadata-vectors/*.json`: kondisi (semua operator `eq`..`isnull`, `all`/`any`/`not`, `ref`), tipe desimal (perbandingan tanpa float), `null`, perbandingan teks, nilai tepi, tipe tak cocok mengembalikan `false` dengan alasan; ekspresi (gabung teks, `coalesce`, aritmetika desimal dengan skala dan mode pembulatan eksplisit, `if`); templat label; vektor pola regex subset. Test Go dan TS membaca korpus yang sama.
    - **Validates: Requirements 2.2, 2.5, 4.1, 4.4, 5.2, 5.7**
  - [x] 3.2 Implementasi `sdk/metadata` (Go)
    - AST kondisi bertipe, evaluator total fungsi (tanpa panik, biaya linear, alasan terukur), aritmetika desimal, galat bertipe, batas kedalaman, jumlah node, dan panjang saat runtime. Paket daun: tidak mengimpor platform (D-2; periksa `archcheck`). Pelari korpus Go.
    - _Requirements: 2.2, 4.1, 4.5, 5.7, 19.1_
  - [x] 3.3 Validator subset `pattern`
    - `sdk/metadata`: terima hanya subset RE2 yang berperilaku sama di Go dan JavaScript (tanpa lookaround dan backreference), tolak kuantor bersarang dan alternatif ambigu di bawah kuantor (tinggi bintang ≤ 1), batasi panjang ≤ 200; runtime server memakai `regexp` RE2.
    - _Requirements: 2.5, 19.2_
  - [x] 3.4 Write property tests evaluator Go (tidak panik dan linear)
    - **Property: evaluator tidak melempar panik atau galat untuk input apa pun dan berbiaya linear terhadap jumlah node**
    - Input acak (tipe campur, `null`, desimal besar, pohon di batas) menghasilkan `false` atau hasil sah dalam batas waktu; pohon melampaui batas ditolak, bukan dieksekusi.
    - **Validates: Requirements 4.5, 19.1**
  - [x] 3.5 Implementasi evaluator dan desimal TypeScript beserta pelari korpus
    - `packages/platform-web/src/metadata/` (hanya evaluator kondisi, ekspresi, desimal berskala tanpa `Number`, pelari korpus; sisanya di grup 16). Hasil harus identik dengan Go untuk seluruh korpus 3.1; entry awal tetap ≤ 200 KB gzip (modul dimuat malas).
    - _Requirements: 2.2, 4.4, 5.7, 18.7_
  - [x] 3.6 Write property tests paritas dua sisi
    - **Property: untuk masukan acak berseed, evaluator Go dan TS menghasilkan hasil identik**
    - Mekanisme sesuai 1.2 (mis. Go menulis korpus acak berseed yang dibaca dua sisi); mencakup desimal, `null`, perbandingan teks, dan ekspresi.
    - **Validates: Requirements 4.4, 22.2**

- [x] 4. Resolver: penyertaan, timpaan, abstrak, tipe domain, blok (Kelas beban: Sonnet; 4.3 Haiku untuk berkas YAML bawaan setelah spesifikasi tipe jelas)
  - [x] 4.1 Tulis test resolver (merah dulu)
    - Tabel dan tes properti: `include` modul sendiri, `sdk:` bawaan, dan modul dependensi hanya bila `export: true` dan terdaftar di manifest; `include.source` diperlakukan sebagai kunci registry, **bukan** jalur berkas (percobaan `../` ditolak); timpaan `set`/`omit`/`add`; tolak siklus penyertaan, nama field ganda tanpa timpaan eksplisit, timpaan terhadap field yang tidak ada, timpaan yang melanggar skema; timpaan tidak boleh menurunkan `classification`, melonggarkan `fieldGroup`, atau mencabut `readOnly`, dan **tanpa pengecualian** untuk field bawaan, `owner_field`, `scope_fields`, `amendedFromId`, dan field `sdk:document` (tidak dapat di-`omit`/`set`); `lockWhen` hanya boleh diperketat (OR); `prefix` blok menghasilkan identifier camelCase deterministik (`billingStreet`); klasifikasi tipe domain menular; abstrak tidak menghasilkan tabel, kontrak, atau kode; jejak asal per atribut.
    - **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.10, 3.11, 13.1**
  - [x] 4.2 Implementasi resolver
    - `metadatatools/resolve`: hasil metadata datar dengan jejak asal (untuk pesan galat dan tinjauan); resolusi penuh saat `metadata:generate`/`check`; tidak ada jalur eksekusi runtime tambahan untuk tipe domain atau blok (D-4, D-5: tambal deklaratif tanpa parameter atau logika).
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.10, 3.11, 3.14, 13.1_
  - [x] 4.3 Metadata bawaan SDK
    - Berkas bawaan yang dibenamkan: abstrak `audit` dan `document` (field bawaan `id`, `version`, `createdAt`, `updatedAt`, `createdBy`, `updatedBy`, `status`, `amendedFromId`, semuanya `readOnly`; `createdBy` dan `updatedBy` nullable; `createdAt` dan `updatedAt` dengan `DEFAULT now()` implisit, K22; `status` dengan lifecycle baku `draft`, `deleted`, `pending_approval`, `accepted`, `rejected`, `cancelled`, `done` beserta tabel transisinya, K23; `lockWhen` bawaan `status` bukan `draft`) serta tipe domain `email`, `phone`, `percentage`, `money` (tipe dasar, batasan, klasifikasi, format, kunci widget, operator, kunci label).
    - _Requirements: 2.6, 3.8, 3.9, 13.1, 13.4_
  - [x] 4.4 Pelaporan dampak tipe domain dan blok bersama
    - `metadata:check` menghitung indeks balik pemakai dan melaporkan dampak per resource (termasuk lintas modul untuk ekspor); perubahan bersama diperlakukan sebagai perubahan atas setiap pemakai; ekspor lintas modul tunduk SemVer modul pemilik (perubahan merusak butuh MAJOR). Test: ubah tipe `email` mengubah laporan semua pemakai.
    - _Requirements: 3.12_
  - [x] 4.5 Resolver: `sdk:audit` otomatis dan `statusAxes` (K22, K24)
    - Test lalu implementasi di `metadatatools/resolve` dan skema `resource.schema.json`: setiap resource (tabel biasa maupun dokumen) otomatis menyertakan `sdk:audit` tanpa `include` eksplisit, dan `include: sdk:audit` eksplisit tetap sah tanpa nama ganda; `createdBy` dan `updatedBy` nullable, `createdAt` dan `updatedAt` membawa penanda `DEFAULT now()` implisit untuk model skema (tanpa nilai bawaan dinamis di berkas metadata); kunci `statusAxes` (nilai, `initial`, `transitions`) diterima hanya pada resource yang menyertakan `sdk:document`, diselesaikan menjadi field enum `readOnly` bukan-nullable berurutan stabil, dengan tabel transisi sumbu pada model datar (jejak asal), dan tidak diwarisi lewat `include` seperti atribut tingkat resource lain.
    - _Requirements: 2.6, 13.14, 13.15, 13.17_

- [x] 5. Validasi semantik `metadata:check` (Kelas beban: Sonnet)
  - [x] 5.1 Tulis test validasi field (merah dulu)
    - Tipe dan batasan (`string`, `text`, `integer`, `decimal(p,s)`, `boolean`, `date`, `datetime`, `enum`, `uuid`, tipe domain), `default` dan nilai literal dikanonisasi, nilai `enum` `^[a-z][a-z0-9_]{0,62}$`, `aggregatable` hanya `integer`/`decimal`, FK tanpa indeks memberi peringatan, indeks hanya yang dideklarasikan; field `highly_confidential` **ditolak** dengan pesan merujuk pengembangan berikutnya, termasuk dari tipe domain, blok, accessor, dan properti bernama sensitif; `unmanagedColumns` hanya untuk kolom `highly_confidential`, tidak muncul di DTO, kontrak, metadata klien, atau selisih, dan resource pemakainya tanpa `generate.crud`; field bawaan selalu `readOnly`; `deprecated` dan `description` sah.
    - **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.7, 2.8, 2.9, 2.11**
  - [x] 5.2 Implementasi validasi field, tipe, dan klasifikasi
    - Pemeriksa di `metadatatools/check` memakai model datar dari 4.2 dan paket penamaan 2.4; pesan memuat berkas dan baris. `unmanagedColumns` berobjek `{name, classification, reason}` sesuai K28.
    - _Requirements: 2.1, 2.3, 2.4, 2.7, 2.8, 2.9, 2.11_
  - [x] 5.3 Tulis test kondisi, accessor, dan templat label (merah dulu)
    - Rujukan `ref` ke field tak ada dan **siklus** antarkondisi ditolak; `visibleWhen`/`requiredWhen`/`readonlyWhen`/`lockWhen`/`rules`/filter relasi memakai satu model kondisi tanpa mekanisme logika lain; aturan yang merujuk `confidential` ke atas atau grup non-`default` otomatis `scope: server`; accessor `expr`/`stored`/`func` (readOnly, tidak diikat dari input; tata bahasa tertutup, aritmetika desimal dengan skala dan pembulatan eksplisit; hanya `stored` boleh `sortable`/`filterable`/`groupable`; `func` menyatakan grup dan klasifikasi eksplisit; accessor mewarisi klasifikasi tertinggi dan grup paling ketat); `labelTemplate` memakai `{field}` milik resource, teks tetap hanya tanda baca dan spasi, field di grup `default` berklasifikasi ≤ `internal` tanpa pengecualian; aturan yang merujuk field di luar grup `default` memicu peringatan.
    - **Validates: Requirements 4.2, 4.3, 4.6, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.8, 9.8**
  - [x] 5.4 Implementasi validasi kondisi, accessor, dan label
    - Pemeriksa statis untuk 5.3 (penerjemahan ekspresi `stored` ke SQL baru di 10.3; di sini hanya validasi tata bahasa dan daftar putih fungsi immutable). Evaluator yang sama dengan grup 3 dipakai untuk templat label dan `expr`.
    - _Requirements: 4.2, 4.3, 4.6, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 9.8_
  - [x] 5.5 Tulis test relasi dan integrasi manifest (merah dulu)
    - Relasi `belongsTo`/`hasOne`/`hasMany`/`belongsToMany`/`morph*` dengan `onDelete` dan `required`; lintas modul hanya ID dan hanya ke dependensi manifest + `require` go.mod, ditolak bila resource tidak ada atau tanpa kontrak relasi; `morphTo` memakai daftar putih tipe (kunci resource); `embedded` hanya pada `hasMany`; `filter` relasi: siklus `ref` dan field seberang non-`filterable` ditolak; `resource.key` cocok dengan manifest (sebelum PRD-001: prefiks capability `<key>.<aksi>`); keanggotaan `fieldGroup` divalidasi terhadap definisi manifest atau metadata sendiri (grup `default` implisit), `module:check` gagal bila manifest memuat `fields` yang menyimpang (P-1); permission operasi `<resource>.<aksi>` terdaftar; `owner_field`/`scope_fields` merujuk field ada dan bertipe sesuai.
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.4, 6.5, 6.7, 7.1, 7.2, 7.3, 7.4, 7.5**
  - [x] 5.6 Implementasi validasi relasi dan manifest
    - `metadatatools/check` plus perluasan `internal/devtools/modcheck` (`module:check`) untuk kesamaan `fields`; klasifikasi properti bangkitan tetap ditegakkan linter kontrak yang ada (`erp-sensitive-needs-classification`, `erp-no-highly-confidential-in-response`). T1 **tidak** mewajibkan `resources`/`field_groups` di manifest (D-24).
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5, 6.7, 7.1, 7.2, 7.3, 7.4, 7.5_
  - [x] 5.7 Validasi statis deprecated, salinan, penguncian, dan `generate`
    - Test lalu implementasi: pasangan `migrateFrom` memiliki `fieldGroup` dan `classification` identik; bentuk deklarasi `snapshot`/`snapshotSource` valid, field tujuan menyatakan `fieldGroup` dan `classification` eksplisit dan tidak lebih longgar dari sumber, salinan dari grup non-`default` ke `default` ditolak tanpa alasan tertulis; kolom salinan hanya `filterable`/`sortable` bila dinyatakan eksplisit; `lockWhen` untuk resource dokumen harus dapat diterjemahkan ke SQL (kolom milik sendiri, `eq`/`ne`/`in`/`isnull`) bila tidak, `metadata:check` gagal; `generate.crud` tidak boleh bersama `unmanagedColumns`.
    - **Validates: Requirements 12.6, 13.12, 13.21, 14.1, 14.5, 14.6**
  - [x] 5.8 Validasi statis `statusAxes` (K24)
    - Test lalu implementasi di `metadatatools/check`: nama sumbu camelCase sah, tidak bertabrakan dengan field lain atau sumbu lain setelah konversi nama dan bukan `status`; `values` snake_case (`^[a-z][a-z0-9_]{0,62}$`) dan unik; `initial` termasuk `values`; setiap `from` dan `to` termasuk `values`, tanpa pasangan ganda; sumbu tidak mendefinisikan ulang atau memperluas lifecycle `status` dan hanya pada resource ber-`sdk:document`; transisi mundur atau siklik sah; sumbu tidak dapat dilepas dari `readOnly`.
    - **Validates: Requirements 13.14, 13.15, 13.18**

- [x] 6. Pembangkit T1: OpenAPI, Go, TypeScript (Kelas beban: Sonnet; templat TS 6.4 Sonnet)
  - [x] 6.1 Tulis test golden dan drift (merah dulu)
    - Golden per jenis keluaran untuk modul fixture: skema OpenAPI (`Invoice`, `InvoiceCreate`, `InvoiceUpdate`), DTO dan decoder Go, konstanta dan aturan domain, metadata TS; keluaran deterministik (urutan stabil, tanpa cap waktu) dan berkepala `Code generated by erpora metadata:generate. DO NOT EDIT.`; `--check` gagal bila ada selisih atau berkas dimodifikasi tangan; modul tanpa `web/` tanpa keluaran TS; modul tanpa metadata tidak terpengaruh; golden masukan berbahaya (kutip, `$$`, `*/`, backtick, baris baru, nilai `enum` jahat) membuktikan keluaran Go dan TS tidak menjalankan teks masukan.
    - **Validates: Requirements 8.1, 8.2, 8.5, 8.6, 19.9, 19.10**
  - [x] 6.2 Templat Go: domain, DTO, decoder, dan validator
    - `internal/devtools/templates/resource/`: `modules/<dir>/internal/domain/<nama>_meta.gen.go` (konstanta, aturan; tanpa I/O) dan `internal/presentation/http/<nama>_dto.gen.go`. Decoder `DisallowUnknownFields` menolak field tak dikenal dan `readOnly` (`422 validation_failed` rinci per field), validator menegakkan tipe, wajib, `nullable`, panjang, rentang, pola, enum, dan `rules` (`422 validation_failed` struktur; `422 business_rule_violated` aturan bisnis), batas `MaxBodyBytes` dan kedalaman JSON, galat tanpa nilai field `confidential` ke atas, AST kondisi tertanam sebagai data bertipe (D-2), batasan tidak pernah lebih ketat di klien daripada server atau kontrak.
    - _Requirements: 4.7, 8.1, 8.4, 9.1, 9.2, 9.5, 9.6, 9.7, 13.17_
  - [x] 6.3 Templat OpenAPI dan penyuntikan terstruktur
    - Tulis `components.schemas` ke `modules/<dir>/openapi.yaml` lewat suntingan node YAML, dengan `x-erp-generated: true`, `x-erp-field-group`, `x-erp-classification`, `x-erp-filterable`, `x-erp-sortable`, `x-erp-decimal`, `x-erp-extensible-enum`, `deprecated`; `decimal` sebagai `string` dengan pola; `contracts:lint` dan `contracts:generate --check` tetap lulus; skema bertanda tidak boleh diubah di luar generator.
    - _Requirements: 8.1, 8.3_
  - [x] 6.4 Templat metadata TypeScript
    - `modules/<dir>/web/src/metadata/<nama>.meta.gen.ts`: struktur field (nama, tipe, batasan, grup, kunci label), kunci widget dan format dari tipe domain, aturan `scope: both` saja; aturan dan ambang `scope: server` dan field `clientVisible: false` (dengan alasan tertulis) tidak disertakan; literal hanya lewat `JSON.stringify`, regex lewat `new RegExp` dari string JSON; chunk dimuat malas.
    - _Requirements: 3.13, 4.6, 8.1, 18.3, 18.8, 19.9_
  - [x] 6.5 Pagar keamanan pembangkit
    - Tolak jalur keluaran di luar direktori modul, tidak mengikuti symlink, tulis hanya ke direktori yang diizinkan; `metadata:generate` gagal tanpa menulis apa pun bila `check` gagal atau penanda `Code generated` di berkas keluaran hilang; galat tidak membocorkan isi berkas di luar modul; identifier selalu di-quote, nilai terparameter; tidak ada penyusunan SQL dari input klien; `archcheck` tanpa pengecualian baru untuk kode bangkitan; metadata modul non-`builtin` diperlakukan tidak tepercaya (tidak dibangkitkan otomatis di produksi).
    - _Requirements: 8.4, 8.6, 19.3, 19.4, 19.5, 19.7, 19.9_

- [x] 7. Perintah dan gerbang CI T1 (Kelas beban: Sonnet; 7.2 Haiku)
  - [x] 7.1 Perintah `metadata:check` dan `metadata:generate [--check]`
    - Daftarkan di `internal/devtools/commands` (build tag `devtools`; `task erpora -- <command>`). Test: binary rilis **tidak** memiliki perintah `metadata:*` (matriks IDOR baris 13); `metadata:generate` tidak pernah berjalan terhadap database.
    - **Validates: Requirements 20.1, 8.6**
    - _Requirements: 8.2, 20.1_
  - [x] 7.2 Taskfile dan gerbang
    - `metadata:check` dan `metadata:generate --check` masuk `task lint arch check:gowork-off test` dan `task web:check`; tanpa `rm`/`cp`/`sed`; berjalan di Windows (uji pada mesin ini).
    - _Requirements: 20.2, 20.3_

- [x] 8. Modul fixture dan migrasi `example` ke metadata (Kelas beban: Sonnet)
  - [x] 8.1 Modul fixture di `testdata`
    - Satu atau dua modul fixture (lokasi dari 1.2; **tidak** didaftarkan ke aplikasi; lintas modul butuh dua modul dengan dependensi manifest) memuat: dokumen (`sdk:document`), relasi satu modul dan lintas modul, `decimal`, `enum`, baris `embedded`, `morph`, tipe domain, dan blok (beserta `prefix`), serta (K24) satu resource dokumen bersumbu status (`statusAxes`: penagihan dan pengiriman dengan transisi mundur, mis. retur pengiriman). Dipakai exit gate T1 sampai T4. Metadata saja pada T1; migrasi dan kode data di tahap berikutnya.
    - _Requirements: 20.5, 13.19_
  - [x] 8.2 Migrasi `example` ke metadata
    - Pindahkan `example` ke metadata dengan `sdk:audit` utuh (K19, 1 Nov 2026: tanpa `omit`); tambahkan `createdBy` dan `updatedBy` secara **aditif** ke kontrak dan implementasi `example` (migrasi expand kolom nullable bila perlu, domain, repository, handler) tanpa perubahan breaking: `task contracts:check` (termasuk `contracts:breaking`) dan tes modul tetap lulus. Bila berkas bangkitan bertabrakan nama dengan kode tulis tangan `example` dan penyelesaiannya mengubah perilaku atau menyimpang dari spek, **tanyakan dulu**. `exampleaddon` tidak punya tabel dan tidak dimigrasikan.
    - _Requirements: 8.3, 20.5_

- [x] 9. Checkpoint T1 — exit gate dan review (Kelas beban: 9.1 Sonnet; 9.2 agent independen model lebih tinggi)
  - [x] 9.1 Gerbang T1
    - Jalankan `task lint arch check:gowork-off test`, `task contracts:check`, `task web:check`; `metadata:check` dan `metadata:generate --check` hijau untuk `example` dan fixture; paritas Go dan TS lulus korpus; catat jumlah tes dan ukuran JS awal terhadap baseline 1.1. Ensure all tests pass, ask the user if questions arise.
    - _Requirements: 22.1, 22.2_
  - [x] 9.2 Review akhir putaran T1
    - Reviewer independen (agent baru, tanpa context implementer; diberi diff dan spek tanpa kesimpulan implementer): (a) kesesuaian dengan spec/PRD dan kebenaran/regresi; (b) `security-audit` terfokus pada perubahan putaran (YAML berbahaya, injeksi literal pada keluaran Go/TS, path traversal pembangkit, mass assignment pada DTO, kebocoran lewat metadata klien). Temuan sesuai spek dikerjakan implementer yang sama lewat `SendMessage`; yang di luar spek menunggu konfirmasi pemilik produk dan dicatat pada spec terkait (bukan hanya di samping). Setelah perbaikan, satu review atas diff perbaikan saja.
    - _Requirements: 22.3, 22.4_

- [x] 10. Mesin selisih database (T2) (Kelas beban: Sonnet)
  - [x] 10.1 Spike alat selisih (**berhenti untuk konfirmasi bila memilih alat pihak ketiga**)
    - Evaluasi Atlas atau alat serupa terhadap implementasi sendiri untuk subset yang dimodelkan: lisensi, dukungan format goose, ketergantungan, dan kecocokan dengan role non-superuser. Catat hasil di "Catatan hasil" dan `design.md` D-6. Dependensi baru atau alat eksternal hanya setelah disetujui pemilik produk.
    - _Requirements: 10.1_
  - [x] 10.2 Tulis test model skema, penerjemah SQL, dan escape literal (merah dulu)
    - Pemetaan tipe tertulis dan teruji (`string(maxLength)` ke `varchar`/`text` + CHECK `length`, `decimal(p,s)` ke `numeric(p,s)`, `datetime` ke `timestamptz`, `date`, `uuid`, `enum` ke `text` + CHECK nilai, `boolean`, `stored` ke `GENERATED ALWAYS AS (...) STORED` dari daftar putih fungsi immutable); tabel bernama `<slug>_<nama>` jamak snake_case tanpa `tenant_id`; `id` `uuid` kunci utama; satu fungsi escape literal SQL yang diuji; golden masukan berbahaya SQL (kutip, `$$`, nilai `enum` jahat); badan pemicu dari templat tetap tanpa menyisipkan teks berkas.
    - **Validates: Requirements 5.3, 10.4, 10.5, 10.6, 19.7, 19.8, 19.10**
  - [x] 10.3 Implementasi model skema turunan dan penerjemah beranotasi
    - `metadatatools/dbmodel` dan `metadatatools/sqlgen`: identifier selalu di-quote; ekspresi `stored`, CHECK, dan kondisi `lockWhen` diterjemahkan dari tata bahasa tertutup, tidak menyalin teks berkas; satu fungsi escape literal dipakai semua pembangkit SQL.
    - _Requirements: 5.3, 10.4, 10.5, 10.6, 19.7, 19.8_
  - [x] 10.4 Tulis test integrasi mesin selisih (merah dulu)
    - Integrasi (`pgtest`, `ERP_TEST_DATABASE_URL`): selisih kosong untuk modul referensi; selisih pada tabel, kolom, tipe, nullability, default, CHECK, unique, indeks, FK relasi satu modul, kolom turunan, pemicu; indeks atau unique yang tidak dideklarasikan dihitung selisih; kolom `unmanagedColumns` dikecualikan; migrasi dijalankan oleh role non-superuser (mis. `COPY … PROGRAM` gagal). Jangan jalankan wadah uji di laptop: pakai VPS lewat tunnel SSH; bila server kewalahan, `-p 2`.
    - **Validates: Requirements 10.1, 10.2, 10.7, 10.8, 13.6**
  - [x] 10.5 Implementasi `metadata:diff`
    - Jalankan semua migrasi goose modul dan dependensinya pada database sementara berole non-superuser, baca katalog, bandingkan dengan model turunan; mesin yang sama dipakai pemeriksa dan pembangkit; hanya berjalan di lingkungan pengembang dan CI, tidak pernah di binary rilis atau terhadap database produksi.
    - _Requirements: 10.1, 10.2, 10.7, 10.8_

- [x] 11. Pembangkit migrasi (T2) (Kelas beban: Sonnet)
  - [x] 11.1 Tulis test pembangkit migrasi (merah dulu)
    - Perubahan **aman** (tabel baru, kolom nullable atau berdefault, indeks baru, pelonggaran batasan, nilai enum `extensible`) dihasilkan otomatis; perubahan **perlu keputusan** (rename, ubah tipe, perkecil panjang atau rentang, `NOT NULL` pada kolom ada, unique pada data ada, hapus apa pun) hanya menghasilkan kerangka bertanda `TODO` yang menggagalkan `metadata:check`; tabel `large: true` memakai backfill per batch `NO TRANSACTION`; `Down` hanya bila aman, yang tidak dapat dibalik ditandai eksplisit; berkas `NNNN_<nama>.sql` bernomor berikutnya dan tidak pernah diterapkan; hasil lolos `module:check`; `make:migration` tetap tersedia.
    - **Validates: Requirements 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 11.7**
  - [x] 11.2 Implementasi `metadata:migrate <modul> <nama>`
    - Mesin selisih 10.5 sebagai masukan; penomoran dan format mengikuti konvensi `make:migration`; semua SQL lewat 10.3.
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 11.7_

- [x] 12. Aturan deprecated, rename, dan penghapusan (T2) (Kelas beban: Sonnet)
  - [x] 12.1 Tulis test aturan deprecated dan rename (merah dulu)
    - Menghapus field tanpa tanda `deprecated` pada rilis sebelumnya ditolak (pembanding: metadata pada tag rilis terakhir modul, seperti `contracts:breaking`; modul tanpa tag dikecualikan); `migrateFrom` menghasilkan kolom baru plus salinan data (batch bila `large`) tanpa menghapus kolom lama; DTO menerima dan mengirim kedua bentuk dan kontrak menandai bentuk lama `deprecated: true`; kedua bentuk dengan nilai berbeda `422 validation_failed`; `removeAfter` harus versi MAJOR berikutnya atau sesudahnya, dan `metadata:migrate` menolak kerangka penghapusan sebelum `removeAfter` dinyatakan; migrasi penghapusan memuat penjaga (gagal bila ada baris dengan kolom lama terisi dan kolom pengganti kosong) dan `Down` ditandai tidak dapat dibalik; integrasi: lompatan beberapa versi menjalankan semua migrasi berurutan dengan hasil sama seperti upgrade bertahap, pada fixture. Pengaman K18: `Down` mengikuti K32; penjaga data pada migrasi penghapusan masih usulan perancang yang menunggu konfirmasi.
    - **Validates: Requirements 12.1, 12.2, 12.3, 12.4, 12.5, 12.6, 12.7, 12.8**
  - [x] 12.2 Implementasi pemeriksa deprecated dan `removeAfter`
    - `metadata:check`: pembanding tag rilis memakai helper git `contracttools`; validasi `deprecated: {since, replacedBy?}` dan `removeAfter`; `contracts:breaking` dan mesin selisih tetap pengaman lapis kedua.
    - _Requirements: 12.1, 12.4, 12.5_
  - [x] 12.3 Pembangkitan migrasi rename dan penghapusan
    - `metadata:migrate`: salinan data `migrateFrom` (batch bila `large`), kerangka penghapusan berpenjaga dan `Down` tidak dapat dibalik; catat di dokumen upgrade bahwa backup wajib sebelum migrasi MAJOR yang menghapus kolom.
    - _Requirements: 12.2, 12.4, 12.7, 12.8_
  - [x] 12.4 DTO dan kontrak dua bentuk
    - Pembangkit 6.2 dan 6.3 menerima serta mengirim bentuk lama dan baru selama masa transisi, server mengisi keduanya saat menulis, menandai bentuk lama `deprecated: true`, dan menolak nilai berbeda; field kanonik (baru) menjadi dasar `CheckWrite` dan `MaskFields` kelak (D-33).
    - _Requirements: 12.3, 12.6_

- [x] 13. Pemicu immutabilitas dokumen (T2) (Kelas beban: Sonnet)
  - [x] 13.1 Tulis test integrasi pemicu (merah dulu)
    - `UPDATE`/`DELETE` langsung pada dokumen terkunci ditolak database; pengecualian (a) hanya `status`/`version`/`updated_at`/`updated_by` berubah dengan pasangan status ada di tabel transisi, (b) hanya kolom tujuan `migrateFrom` diisi dari `NULL` menjadi salinan persis kolom asal; `DELETE` baris terkunci selalu ditolak; evaluasi pada `OLD`; pemicu anak menolak `INSERT`/`UPDATE`/`DELETE` untuk `embedded`, pivot, dan `hasOne` bila induk terkunci; tidak ada mekanisme pengecualian bagi jalur aplikasi (penanda sesi, `session_replication_role`, nonaktifkan pemicu); migrasi tulis tangan yang melampaui butir 8 menonaktifkan dan mengaktifkan kembali pemicu eksplisit, dan `metadata:diff` memastikan pemicu aktif di akhir; penghapusan pemicu tak sengaja terdeteksi mesin selisih.
    - Tambahan K23 dan K24: tabel transisi lifecycle bawaan (tujuh pasangan sah lolos, pasangan lain termasuk `submitted` ditolak); pemicu mengizinkan perubahan kolom sumbu status dengan pasangan sah (termasuk mundur) pada dokumen terkunci dan menolak pasangan tak sah, perubahan sumbu bersama kolom data lain, dan `DELETE`; kolom sumbu NOT NULL dengan default `initial` dan CHECK nilai.
    - **Validates: Requirements 13.2, 13.5, 13.6, 13.8, 13.9, 13.10, 13.11, 13.16, 13.19, 13.20**
  - [x] 13.2 Implementasi pembangkit pemicu
    - Templat SQL tetap untuk pemicu dokumen dan pemicu anak, tabel transisi lifecycle (K23) dan transisi sumbu (K24) dibangkitkan dari metadata, `lockWhen` diterjemahkan lewat 10.3; mesin selisih mengenali pemicu dan statusnya (aktif) serta kolom sumbu (NOT NULL, default `initial`, CHECK). Pengecualian lain tidak disediakan.
    - _Requirements: 13.2, 13.4, 13.5, 13.6, 13.8, 13.9, 13.10, 13.11, 13.14, 13.16_

- [x] 14. Migrasi nyata, CI, dan selisih kosong (T2) (Kelas beban: Sonnet; 14.2 Haiku)
  - [x] 14.1 Migrasi modul referensi dan fixture
    - Deklarasikan indeks dan unique yang sudah ada di migrasi tulis tangan `example` agar selisih kosong; hasilkan migrasi fixture dengan `metadata:migrate` (dokumen, pivot, `embedded`, pemicu); `metadata:diff` kosong untuk `example` dan fixture; `exampleaddon` tidak diperiksa (tanpa tabel).
    - _Requirements: 10.3, 20.5_
  - [x] 14.2 Job CI selisih
    - Tambah `metadata:diff` ke job integrasi CI (tes selisih kosong gagal bila metadata dan migrasi menyimpang) dan entri Taskfile lintas platform.
    - _Requirements: 10.3, 20.2, 20.3_

- [x] 15. Checkpoint T2 — exit gate dan review (Kelas beban: 15.1 Sonnet; 15.2 agent independen model lebih tinggi)
  - [x] 15.1 Gerbang T2
    - `task lint arch check:gowork-off test`, `task test:integration` (VPS lewat tunnel; `-p 2` bila perlu), `task contracts:check`; selisih kosong untuk `example` dan fixture; skenario aman dan perlu-keputusan teruji; catat hasil. Ensure all tests pass, ask the user if questions arise.
    - _Requirements: 22.1, 22.3_
  - [x] 15.2 Review akhir putaran T2
    - Reviewer independen: kesesuaian (selisih, migrasi, deprecated, pemicu) dan `security-audit` terfokus (injeksi SQL pada pembangkit, escape literal, role non-superuser, pemicu dan celah pengecualian, migrasi destruktif otomatis). Alur perbaikan seperti 9.2.
    - _Requirements: 22.3, 22.4_

- [x] 16. Paket metadata frontend (T3) (Kelas beban: Sonnet; 16.4 Haiku)
  - [x] 16.1 Tulis test paket frontend (merah dulu)
    - `bun test`: `validateRecord` (tipe, wajib, `nullable`, panjang, rentang, pola, enum, `rules`), registri tipe field (operator sah per tipe selaras api-guidelines §5.1, kontrak editor form dan editor filter), format desimal hanya lewat utilitas desimal dan `Intl` dengan string (tanpa `Number`), pemuat malas per resource, field `clientVisible: false` dan aturan `scope: server` tidak ada di metadata, field tidak ada di respons diperlakukan "tidak tersedia" bukan "kosong", teks UI lewat i18n.
    - **Validates: Requirements 18.1, 18.3, 18.5, 18.7, 18.8**
  - [x] 16.2 Implementasi paket
    - Lengkapi `packages/platform-web/src/metadata/`: tipe (`ResourceMeta`, `FieldMeta`, `Condition`, `RelationMeta`), `validateRecord`, registri tipe field, pemuat malas; `@erpora/ui` tetap daun (F4; `task web:check` bagian `deps`); komponen perender generik bukan bagian spec ini (M3c dan M3d).
    - _Requirements: 18.1, 18.2, 18.3, 18.5, 18.7_
  - [x] 16.3 Anggaran bundle dan chunk
    - Chunk metadata terpisah per resource; evaluator dan validator hemat bundle; entry awal ≤ 200 KB gzip (`web:check`, anggaran saat ini sangat mepet); laporkan ukuran terhadap baseline 1.1 dan **tanyakan** bila mendekati batas.
    - _Requirements: 18.4_
  - [x] 16.4 Dokumentasikan kontrak konsumsi M3c dan M3d
    - `docs/module-sdk/metadata.md`: apa yang disediakan paket, titik perluasan (registri tipe field, widget kustom modul tanpa mengubah `@erpora/ui`), aturan evaluasi, perlakuan `clientVisible` dan `scope: server`.
    - _Requirements: 3.13, 18.6_

- [x] 17. Checkpoint T3 — exit gate dan review (Kelas beban: 17.1 Sonnet; 17.2 agent independen model lebih tinggi)
  - [x] 17.1 Gerbang T3
    - `task web:check`, `bun test` dari root, `ERP_REQUIRE_BUN=1`; paritas Go dan TS hijau; anggaran bundle terjaga; kontrak konsumsi terdokumentasi. Ensure all tests pass, ask the user if questions arise.
    - _Requirements: 22.1, 22.2_
  - [x] 17.2 Review akhir putaran T3
    - Reviewer independen: kesesuaian dan `security-audit` terfokus (isi chunk publik, regex di klien/ReDoS, desimal, XSS pada label). Alur perbaikan seperti 9.2.
    - _Requirements: 22.3, 22.4_

- [x] 18. Dokumentasi dan pencatatan keputusan (Kelas beban: Haiku; 18.3 Sonnet)
  - [x] 18.1 ADR baru
    - `docs/adr/` (nomor berikutnya): "metadata sebagai sumber kebenaran yang membangkitkan kontrak dan migrasi"; memperbarui ADR-0003 dan ADR-0010 tanpa membatalkan larangan `AutoMigrate`; catat koreksi `tenant_id` (ADR-0004) agar tidak menjadi temuan; catat bahwa sistem enkripsi dan field `highly_confidential` adalah pengembangan berikutnya (K17) dan sinkronisasi master ke master di luar cakupan.
    - _Requirements: 2.10, 14.7, 21.2, 21.4_
  - [x] 18.2 Panduan modul
    - Perbarui `docs/module-sdk/metadata.md`, `generators.md`, `contracts.md`, `frontend.md`; `docs/security/security-guidelines.md` §7 (status penegakan); catat pemetaan checklist `transaction-data-standards.md` §15 dan perkara yang tetap per resource (penomoran, ledger, maker-checker, periode akuntansi, rekonsiliasi).
    - _Requirements: 13.7, 21.3_
  - [x] 18.3 Kerangka metadata pada `make:module`
    - `make:module` dapat menghasilkan modul dengan kerangka metadata bila diminta (templat generator); `modcheck` tetap lulus.
    - _Requirements: 20.4_
  - [x] 18.4 `CLAUDE.md`, `AGENTS.md`, dan PRD
    - Bila ada aturan baru, ubah `CLAUDE.md` dan `AGENTS.md` **identik** (`TestAgentsMDMatchesClaudeMD`); verifikasi PRD-000 Q41 dan tabel §11 sesuai kondisi akhir dan perbarui status spec.
    - _Requirements: 21.1, 21.3_
  - [x] 18.5 `task test:generator`
    - Jalankan `task test:generator` (±12 menit; Docker dan Bun, jalankan di VPS, bukan di laptop) karena templat dan generator berubah; catat hasil.
    - _Requirements: 20.4, 22.5_

- [ ] 19. T4: gerbang prasyarat dan analyzer (**DIBLOKIR sampai PRD-001 authz tersedia**) (Kelas beban: 19.1 Sonnet; 19.2 Sonnet)
  - [ ] 19.1 Verifikasi prasyarat PRD-001
    - Pastikan `sdk/authz` memuat `Require`, `Filter`, `Check`, `Fields`, `CheckWrite`, `MaskFields` dan manifest memuat `resources`/`field_groups` (PRD-001 FR-110 sampai FR-115, FR-119). **Bila belum, berhenti dan laporkan; jangan kerjakan 19.2 dan seterusnya** (D-12). Kode data tanpa authz nyata menanam celah IDOR di banyak resource.
    - _Requirements: 16.7_
    - Hasil verifikasi 2 Nov 2026: `sdk/authz` hanya memuat `AsSystem` dan `MaskFields`; `Require/Filter/Check/Fields/CheckWrite` dan manifest `resources`/`field_groups` (PRD-001 FR-110 sampai FR-115, FR-119) belum ada, sehingga T4 (grup 19 sampai 25) tidak dikerjakan (D-12, Req 16.7).
  - [ ] 19.2 Perluas analyzer dan `archcheck`
    - Kode bangkitan tidak dapat melewati helper otorisasi: `db.First(&x, id)` tanpa filter otorisasi gagal di `archcheck`/analyzer `go vet` kustom; `authz.AsSystem` hanya di `internal/application` dengan alasan eksplisit di metadata, teraudit, dan ditandai `// erp:system-call-allowed` pada kontrak yang menerimanya. Test: kode contoh yang melanggar gagal.
    - _Requirements: 15.1, 15.8, 15.11_

- [ ] 20. T4: kontrak relasi lintas modul (Kelas beban: Sonnet)
  - [ ] 20.1 Tulis test kontrak (merah dulu)
    - `ResolveLabels` (≤ 200 ID per panggilan; ID di luar scope dibuang tanpa dibedakan dari "tidak ada"), `Search` (hanya kondisi pada field seberang `filterable` dan boleh dibaca; non-`filterable` `400 invalid_filter`; field `none` `403 field_forbidden`; `q` hanya field terbaca; halaman ≤ 200, `q` ≤ 100 karakter, jumlah kondisi dibatasi; rate limit per principal), `ReadSnapshotFields` (satu ID; hanya field `snapshotSource` yang boleh dibaca principal); tanpa `tenantId`/`userId` pada parameter; field `confidential` ke atas tidak ikut kecuali eksplisit; IDOR A vs B untuk tiap metode.
    - **Validates: Requirements 15.7, 17.1, 17.2, 17.3, 17.4, 17.5, 17.7, 17.8**
  - [ ] 20.2 Implementasi pembangkit kontrak
    - `modules/<x>/contract/<nama>.gen.go` dengan tiga operasi (otorisasi di implementasi terhadap principal di context); endpoint dropdown memakai konvensi yang ada (`fields=id,label`, `q=`, `expand`), dokumentasi OpenAPI dibangkitkan; modul pemanggil hanya mengimpor kontrak dependensi manifest.
    - _Requirements: 6.2, 17.1, 17.2, 17.3, 17.4, 17.5, 17.6, 17.8_

- [ ] 21. T4: relasi dan pemeriksaan ID referensi (Kelas beban: Sonnet)
  - [ ] 21.1 Tulis test IDOR relasi (merah dulu)
    - Matriks baris 3 sampai 7, 10, 14, 16 sampai 18: ID `belongsTo` dibuktikan (dapat dibaca, dalam scope, memenuhi filter relasi yang divalidasi ulang server) `422 not_found`; id baris `embedded` milik induk lain ditolak dan baris tak berubah; pivot `belongsToMany` dua sisi; `morph*` daftar putih dan otorisasi lewat kontrak modul pemilik tipe; `expand` dan urut atau filter atau hitungan lewat relasi satu modul tetap menerapkan `authz.Filter`/`authz.Fields` resource seberang; hanya `embedded` ditulis lewat induk dengan `CheckWrite` dan `MaskFields` per baris; baca anak morph mensyaratkan `Check(read)` atas target; label placeholder "tidak tersedia" untuk rujukan tak terbaca atau terhapus; tidak ada N+1 (satu panggilan kontrak per modul per halaman; batch untuk `embedded`).
    - **Validates: Requirements 6.6, 6.8, 6.9, 6.10, 6.11, 15.2, 15.3, 15.5, 15.6, 15.14, 17.8**
  - [ ] 21.2 Implementasi pemeriksaan referensi dan repository relasi
    - Pemeriksaan ID di body, relasi satu modul via FK dan JOIN tetap melewati otorisasi seberang, eager load (`expand`, satu tingkat), jumlah baris `embedded` dibatasi (bawaan 500).
    - _Requirements: 6.5, 6.6, 6.8, 6.9, 6.10, 6.11, 15.2, 15.3, 15.5, 15.6, 15.14_
  - [ ] 21.3 Pemeriksa piatu (orphan checker) untuk morph
    - Perintah yang dapat dijalankan operator untuk menemukan target `morph*` yang hilang (integritas ditegakkan aplikasi pada setiap tulis karena tanpa FK).
    - _Requirements: 6.4, 15.6_

- [ ] 22. T4: salinan field dan penguncian di server (Kelas beban: Sonnet)
  - [ ] 22.1 Tulis test salinan dan penguncian (merah dulu)
    - Salinan diisi saat relasi diatur atau diubah selama belum terkunci dan diisi ulang dalam transaksi `submit`; beku setelah terkunci dalam kondisi apa pun (perubahan master, migrasi data); dengan principal pemanggil, gagal `422 business_rule_violated` (`code: snapshot_forbidden`) bila salah satu field sumber tak boleh dibaca (tanpa nilai basi atau sebagian); `authz.AsSystem` hanya eksplisit di metadata dengan alasan dan audit; `PATCH` dokumen terkunci `409 invalid_state`; transisi tidak sah `409 invalid_state`; koreksi hanya lewat `cancel`/`amend` yang menghasilkan dokumen baru; dua `submit` bersamaan dan `PATCH` bersamaan `submit` aman (`SELECT … FOR UPDATE`); ubah baris anak setelah `submit` ditolak.
    - Tambahan K23 dan K24: `submit` menghasilkan `accepted` atau `pending_approval` menurut kait (tidak ada status `submitted`); `amend` hanya dari `cancelled` atau `rejected` dan membuat dokumen baru `draft` dengan `amendedFromId`; sumbu status ditolak pada body `Create`/`Update` (hanya-baca), berubah hanya lewat aksi atau kait server dengan pasangan sah (`409 invalid_state` di luar pasangan), termasuk mundur.
    - **Validates: Requirements 9.4, 13.3, 13.4, 13.13, 13.15, 13.17, 13.19, 14.2, 14.3, 14.4, 14.5, 14.6**
  - [ ] 22.2 Implementasi salinan dan penegakan penguncian
    - Pengisian dan pengisian ulang salinan, penegakan `lockWhen` dan transisi di server, pemuatan induk dengan `SELECT … FOR UPDATE` dalam transaksi untuk `PATCH`, `DELETE`, aksi, dan `submit`; field tujuan `readOnly` bagi klien.
    - _Requirements: 9.4, 13.3, 13.4, 13.13, 13.15, 13.17, 14.1, 14.2, 14.3, 14.4, 14.5, 14.6_

- [ ] 23. T4: CRUD bangkitan, kait, dan jalur lepas (Kelas beban: Sonnet)
  - [ ] 23.1 Tulis test handler (merah dulu)
    - Urutan penegakan: `authz.Check(read)` (404 di luar scope), `authz.Check(aksi)` (`403 permission_denied`), `If-Match` (`412 version_mismatch`, `details` lewat `MaskFields`), lalu validasi dan tulis; `403 field_forbidden` pada tulis dan pada filter/urut/grup field `none`; filter non-`filterable` `400 invalid_filter`; scope server sebelum filter klien; agregat desimal di server dengan kedalaman grup ≤ 4 dan jumlah kelompok dibatasi; `q` hanya field terbaca (per principal) dengan escape `%`, `_`, `\`; `409 conflict` generik pada unique; `version`/`ETag`/`If-Match`, `Idempotency-Key` pada `POST`, string desimal, waktu UTC; `PATCH` pada resource ber-`scope_fields`/`owner_field` memeriksa keadaan sebelum **dan** sesudah; tulis lewat pasangan field deprecated hanya field kanonik; mass assignment tertutup.
    - **Validates: Requirements 9.3, 15.4, 15.13, 15.15, 16.2, 16.3, 16.9, 19.6**
  - [ ] 23.2 Implementasi pembangkit CRUD dan kait
    - Untuk `generate.crud: true`: entitas (`internal/domain`), repository dan model GORM (`internal/infrastructure`), use case (`internal/application`), handler dan DTO (`internal/presentation/http`), route, operasi OpenAPI; kait bertipe (sebelum/sesudah validasi, simpan, hapus, transisi; accessor `func` didaftarkan lewat kait); resource maju mengganti use case tertentu sambil memakai metadata, DTO, dan validasi bangkitan; kode tulis tangan tidak di berkas bangkitan; integration test repository tetap ditulis tangan (Q31 ★2).
    - _Requirements: 5.4, 15.4, 15.7, 15.13, 15.15, 16.1, 16.2, 16.3, 16.4, 16.5, 16.8, 16.9, 19.6_
  - [ ] 23.3 `metadata:eject` dan jalur lepas
    - Menghapus `generate.crud` menghentikan pembangkitan; `metadata:eject <resource>` menyalin kode bangkitan menjadi kode biasa (tanpa penanda `Code generated`) tanpa mengubah perilaku; test: perilaku identik sebelum dan sesudah eject.
    - _Requirements: 16.6_

- [ ] 24. T4: tes IDOR bangkitan (Kelas beban: Sonnet)
  - [ ] 24.1 Pembangkit tes IDOR dan pemeriksa penanda
    - Tulis test lalu implementasi: integration test (tag `integration`, fixture A dan B lewat `testsupport`) dengan penanda `// erp:idor-test <resource>.<operasi>` untuk setiap operasi matriks IDOR (baris 1 sampai 12 dan 14 sampai 21), `metadata:check` untuk T4 gagal untuk resource tanpa penanda; B membaca, mengubah, menghapus, atau menjalankan aksi pada record A menghasilkan 404, memakai ID record A di body `422 not_found`, memanggil kontrak `not_found`.
    - **Validates: Requirements 15.9, 15.10, 15.12, 17.7**
  - [ ] 24.2 Write test bahwa tes IDOR gagal bila scope dihapus
    - **Test: tes IDOR bangkitan gagal bila pembatasan scope dihapus (bukan hijau karena stub)**
    - Hasilkan handler dengan filter scope dibuang pada salinan sementara dan pastikan tes IDOR bangkitan gagal.
    - **Validates: Requirements 15.9, 22.3**

- [ ] 25. Checkpoint T4 — exit gate dan review (Kelas beban: 25.1 Sonnet; 25.2 agent independen model lebih tinggi)
  - [ ] 25.1 Gerbang T4
    - `task lint arch check:gowork-off test`, `task test:integration` (VPS lewat tunnel), `task contracts:check`; tes IDOR pengguna A vs B lulus untuk setiap integrasi; perbarui `docs/module-sdk/metadata.md` (T4) dan `security-guidelines.md` §7. Ensure all tests pass, ask the user if questions arise.
    - _Requirements: 15.9, 22.1, 22.3_
  - [ ] 25.2 Review akhir putaran T4
    - Reviewer independen: kesesuaian dan `security-audit` terfokus pada matriks IDOR 21 baris, otorisasi kontrak, mass assignment, kebocoran lewat `412`/galat validasi, balapan `submit`, penggunaan `AsSystem`. Alur perbaikan seperti 9.2.
    - _Requirements: 22.3, 22.4_

- [ ] 26. Final checkpoint - Ensure all tests pass
  - Jalankan seluruh gerbang lokal (`task lint arch check:gowork-off test`, `task test:integration`, `task contracts:check`, `task web:check`, `task test:generator`); perbarui status spec dan "Catatan hasil"; ringkas ke pemilik produk: yang dikerjakan, yang menunggu konfirmasi, yang sengaja tidak dikerjakan. Ensure all tests pass, ask the user if questions arise.
  - _Requirements: 22.1, 22.5_

## Catatan hasil

(Diisi pelaksana: baseline 1.1, keputusan 1.2, hasil spike 10.1, jumlah tes, ukuran bundle, dan keputusan pemilik produk yang timbul selama pengerjaan.)

### Baseline 1.1 (1 Nov 2026, cabang `feat/resource-metadata`)

Versi: Go 1.27 (`go1.27.3 windows/amd64`), Bun 1.4.2, TypeScript 6.0.3, golangci-lint v2.15.0 (terpin di Taskfile). Dijalankan di Windows (`-race` dilewati) tanpa test integrasi/Docker.

| Gerbang | Hasil |
| ------- | ----- |
| `task lint` | hijau: 0 issues pada root, `sdk`, `modules/example`, `modules/exampleaddon` (tanpa dan dengan tag `integration,devtools`) dan `tests/generator`; `i18n:check` 0 galat 0 peringatan |
| `task arch` | hijau (`tests/architecture` dan `internal/archcheck`) |
| `task check:gowork-off test` | hijau (`EXIT=0`) setelah diulang dengan `GOFLAGS=-p=2`; lihat catatan lingkungan di bawah |
| `task contracts:check` | hijau (`contracts:breaking`: seluruh modul `skip ... belum pernah dirilis`, sehingga pembanding tag rilis belum pernah dilatih oleh modul nyata; fixture grup 8 dan 12 harus menutupnya) |
| `task web:check` (`ERP_REQUIRE_BUN=1`) | hijau: typecheck, lint (Biome: 1 info, bukan galat), `i18n:generate --check`, `deps`, `bun test`, build |

Jumlah tes (snapshot; `go test -json`, per modul):

- Go: ±875 tes tingkat atas (root 666, `sdk` 130, `example` 25, `exampleaddon` 11, `cmd/erpora` dengan tag `devtools` 43), ±1979 termasuk subtest. Satu tes di-skip (root).
- Frontend (`bun test`): 1506 tes di 129 berkas: 1492 lulus, 14 di-skip, 0 gagal.
- JS awal: **198,5 KB dari 200,0 KB gzip (99,2 persen), satu berkas**; sisa ruang hanya ±1,5 KB. Ada 25 chunk lazy (terbesar 90,7 KB). Konsekuensi untuk grup 3 dan 16: evaluator, desimal, dan validator TypeScript **tidak boleh** ikut entry awal; ekspor lewat subpath terpisah (mis. `@erpora/platform-web/metadata`, pola `./testing`) dan dimuat malas, bukan dire-ekspor dari `src/index.ts`.
- Keamanan dependensi: `bun audit` bersih (394 paket); `govulncheck` pada `sdk` bersih; pada root 0 kerentanan yang terpanggil (2 pada modul `golang.org/x/net` v0.61.0, GO-2026-6860 dan GO-2026-6866, diperbaiki di v0.62.0; tidak dipanggil kode dan tidak terkait spec ini, ditandai untuk pemilik repo); `go mod verify` lulus.

Catatan lingkungan dan kegagalan sementara (bukan baseline merah):

- Eksekusi pertama `task check:gowork-off` gagal dengan `fatal error: out of memory` (`VirtualAlloc ... errno=1455`) saat mengompilasi `tests/security.test`, dan `govulncheck` root serta `gen` sempat gagal serupa (`Insufficient system resources`, linker `cannot allocate memory`). Penyebabnya kehabisan commit memori mesin karena build paralel beberapa agent; diulang dengan `-p=2` dan hijau. Ulangi dengan `-p 2` bila terjadi lagi.
- Pada snapshot hitung tes, paket `internal/devtools/metadatatools/naming` dan `sdk/metadata` gagal build karena tes baru (merah dulu, TDD grup 2 dan 3) tanpa implementasi: sementara. Tes `gen.TestMakeHandler_AllKindsCompileAndPassTests` gagal hanya saat memori habis; lulus pada proses `task test`.

### Keputusan 1.2 (pelaksana, 1 Nov 2026)

Dicatat juga di `design.md` bagian "Diserahkan kepada pelaksana" dan Komponen 13.

1. **Kunci resource bermulti-kata pada manifest dan permission** (aturan registry terverifikasi). Registry (`internal/platform/modules/registry`, `contracts/module.schema.json`) mensyaratkan setiap `capabilities` berpola `^[a-z][a-z0-9-]*(\.[a-z][a-z0-9_]*)+$` dan berawalan `<slug>.` (`CAPABILITY_PREFIX` di `modcheck`, `CodeCapabilityPrefix` di registry); segmen setelah slug hanya huruf kecil, angka, dan garis bawah, **tanpa huruf besar**. `field_groups[].key` berpola `^[a-z][a-z0-9_]*$`. Karena itu kunci resource `sales.invoiceLine` di berkas metadata **harus dikonversi** menjadi `sales.invoice_line` di manifest dan permission (`sales.invoice_line.read`); konversi memakai fungsi snake_case yang sama dengan kolom SQL (`metadatatools/naming`, bolak-balik identik; angka melekat: `address2Line` menjadi `address2_line`, tetap lolos pola). Slug modul tidak diubah (boleh berstrip, mis. `my-mod`; awalan tabel mengganti strip dengan garis bawah). Nama aksi kustom tetap `[a-z_]+` (PRD-001 FR-110). Daftar `fields` pada field group di manifest (P-1) memakai snake_case (kunci manifest tetap snake_case), dan `module:check` membandingkannya terhadap metadata setelah konversi yang sama. Kunci i18n tetap camelCase (`sales.invoiceLine.customerName`; pola `messageKey` mengizinkannya). Generator menulis pemetaan kunci metadata ke kunci manifest pada keluaran/laporan `metadata:check`, dan galat bila dua kunci bertabrakan setelah konversi.
2. **Pustaka YAML: `go.yaml.in/yaml/v3 v3.0.5`** (sudah dependensi langsung root; lisensi MIT dan Apache-2.0; dipakai `apicontract` dan `gen`). Pemuat memakai pola `apicontract.checkNode`: periksa ukuran berkas (> 256 KB ditolak) **sebelum** parse, parse ke `yaml.Node` (bukan `Decode` langsung), tolak berkas multi-dokumen, **batasi alias** sesuai Requirement 1.2 (paling banyak 16 alias per berkas dan perluasan alias paling banyak 10000 simpul tambahan; kunci gabungan `<<` ditolak; pelindung rasio alias bawaan pustaka tetap sebagai lapis kedua), tolak tag selain `!!str/!!int/!!float/!!bool/!!null/!!map/!!seq` (termasuk tag kustom, `!!merge`, `!!binary`, `!!timestamp`), kunci non-string, kedalaman melebihi batas (pustaka hanya membatasi 10000, jadi batas kita sendiri, mis. 32) dan jumlah node. Galat dipetakan ke berkas dan baris lewat `yaml.Node.Line`.
3. **Pustaka JSON Schema: `santhosh-tekuri/jsonschema/v6 v6.0.3`** (sudah dependensi langsung root; Apache-2.0; dipakai validasi manifest). Galat skema tidak memuat nomor baris, sehingga JSON pointer dipetakan ke `yaml.Node.Line`.
4. **Pustaka desimal Go: `github.com/shopspring/decimal v1.5.0`** (MIT, tanpa dependensi lain, rilis 5 Okt 2026). Verifikasi: sebelum spec ini hanya indirect pada root (lewat `gorm.io/plugin/opentelemetry` ke driver ClickHouse) dan **tidak ada di `sdk/go.mod`**; `sdk/metadata` menambahnya sebagai dependensi langsung SDK (perubahan `sdk/go.mod`/`go.sum` sudah ada di pohon kerja). Syarat pakai: jangan memakai `Div`/presisi pembagian bawaan (16 digit); semua aritmetika dengan skala dan mode pembulatan eksplisit, plus batas jumlah digit dan skala (Requirement 19.1). **Alternatif tanpa dependensi baru: `math/big` (`big.Int` tak berskala + skala), selaras dengan `BigInt` TypeScript.** Karena ini dependensi baru pada SDK yang dipublikasikan, pilihan ini ditandai untuk ditinjau pemilik produk saat review T1; bila ditolak, ganti ke `math/big` (permukaan API `sdk/metadata` tertutup sehingga penggantian terlokalisasi di `decimal.go`).
5. **Desimal TypeScript: buatan sendiri di atas `BigInt`** (target ES2023, tanpa `Number` dan tanpa dependensi), skala dan pembulatan eksplisit identik dengan Go, korpus vektor sebagai arbiter (grup 3).
6. **Pustaka property test**: Go **tanpa dependensi baru** memakai PRNG berseed stdlib (`math/rand`/`math/rand/v2`, seed tetap dan dapat dioverride lewat env, cetak seed dan kasus JSON saat gagal). `pgregory.net/rapid v1.2.0` **tidak dipakai**: berlisensi MPL-2.0 (copyleft tingkat berkas), belum ada di `go.mod` (hanya masuk `go.sum` lewat testcontainers), dan belum ada kebijakan lisensi tertulis di repo; pakai hanya setelah konfirmasi pemilik produk. TypeScript: **`fast-check 4.10.2`** (MIT; dependensi `pure-rand 8.4.2` MIT) sudah menjadi devDependency `platform-web`, `ui`, dan `module-sdk` serta tercatat di `bun.lock`, sehingga tidak ada dependensi baru; `bun audit` bersih.
7. **Mekanisme property test dua sisi**: korpus acak **berseed yang dikomit**. Go membangkitkan kasus acak (seed tetap) beserta hasil Go ke `contracts/metadata-vectors/random/*.json` (regenerasi hanya dengan flag `-update`; tes biasa gagal bila berkas tidak identik dengan hasil pembangkitan ulang berseed, sehingga drift terdeteksi), lalu pelari TS membaca berkas yang sama dan membandingkan hasil. Arah sebaliknya: tes `fast-check` dengan seed tetap membangkitkan kasus, mengevaluasi di TS, dan menulis ke berkas bersaudara hanya dengan `UPDATE_VECTORS=1`; tes Go membaca dan membandingkan. Tidak ada ketergantungan antarproses saat CI; `fast-check` juga dipakai untuk properti sisi TS (tanpa lempar dan linear).
8. **Lokasi modul fixture (D-18, Requirement 20.5): mini-repo `internal/devtools/metadatatools/testdata/repo/`**, mengikuti preseden `internal/devtools/gen/testdata/repo` (mini-repo dengan `go.mod`, `go.work`, `modules/*`, `migrations`; disalin ke direktori sementara per tes lewat `fixtureRepo`) dan `internal/archcheck/testdata/secmod`. Isi: dua modul Go sendiri, `modules/fixturecore` (master dengan `export: true`, mis. `customer`, tipe domain dan blok bersama) dan `modules/fixturesales` (dokumen `sdk:document` `invoice` dengan baris `embedded` `invoice_line`, relasi satu modul dan lintas modul ke `fixturecore` lewat dependensi manifest, `decimal`, `enum`, `morph`, tipe domain, blok berawalan), masing-masing dengan `module.json`, `go.mod`, `migrations/`, `i18n/`, dan `web/` seperlunya. Alasan: (a) Go dan alat web mengabaikan `testdata` (`./...`, golangci-lint, Biome `includes`, workspaces `modules/*/web`); (b) `archcheck`, `modcheck`, dan pemuat bertipe hanya meng-glob `modules/*/go.mod` dan `modules/*/module.json` dari root repo sehingga fixture tidak terpindai dan tidak terdaftar ke `internal/app` (D7); (c) tidak perlu `go:embed` (tidak bisa menembus direktori ber-`go.mod`) karena tes menyalin pohon ke direktori sementara. Dampak: **`go.work` root tidak berubah** (`GO_MODULES` dan seluruh gerbang `lint/build/test/vuln` tidak melihat fixture); modul fixture hanya `require erpora/sdk v0.0.0` dan `go.work` di salinan sementara dibuat oleh tes dengan `use` ke `sdk` asli (path absolut, tanpa spasi) bila perlu dikompilasi atau diuji (`go build`/`go test` sebagai subproses, pola tes `gen`); `metadata:check`, `module:check`, dan `archcheck` dijalankan terhadap **root salinan sementara** (semuanya sudah menerima parameter root), sehingga kepatuhan D1/D8/D9 fixture ikut dibuktikan tanpa pengecualian baru di `archcheck`. Tambahkan satu tes penjaga di `tests/architecture`: tidak ada `use` ke `testdata` di `go.work`, tidak ada impor path `testdata` dari kode non-tes, dan fixture tidak terdaftar di `internal/app`. Satu helper tes (mis. `metadatatools/internal/testrepo`) menemukan direktori lewat `runtime.Caller` dan menyalinnya, dipakai bersama paket `load`, `resolve`, `check`, generator, dan `dbmodel`. Fixture dipakai juga oleh integrasi T2 sampai T4 (`pgtest`) dengan salinan sementara yang sama.
9. **Dependensi lain**: tidak ada tambahan selain butir 4 (shopspring pada `sdk`). `go mod verify` lulus dan seluruh dependensi di atas bertipe MIT/Apache-2.0 (tidak ada GPL/AGPL/MPL yang masuk).

### Spike 10.1 dan keputusan T2 mesin selisih (pelaksana, 1 Nov 2026)

**Keputusan: implementasi sendiri untuk subset yang dimodelkan; tidak ada alat pihak ketiga (Atlas atau sejenis) dan tidak ada dependensi baru.** Alasan teknis: (1) PostgreSQL sendiri yang menormalkan hasil perbandingan: DDL bangkitan dari model diterapkan ke skema sementara `erpora_expected` pada database sementara yang sama, lalu skema hasil migrasi (`public`) dan skema diharapkan dibaca dengan kueri katalog yang sama (`format_type`, `pg_get_expr`, `pg_get_constraintdef`), sehingga penulisan berbeda yang setara (mis. `BETWEEN` versus `>=`/`<=`) tidak menjadi selisih tanpa kita menulis pengurai ekspresi SQL; (2) pembangkit migrasi (task 11) dan pemicu (task 13) memakai penerjemah dan DDL yang sama dengan pembanding, jadi satu mesin; (3) permukaannya kecil (introspeksi ±200 baris, pembanding ±250 baris) dan seluruhnya Go; (4) migrasi dijalankan lewat `internal/platform/migrate.ApplyModule` (goose) oleh role non-superuser buatan sendiri, sesuai Requirement 10.8, tanpa mengikat ke format direktori alat lain. Lisensi dan dukungan format goose pada Atlas **tidak diverifikasi** (tanpa akses daring pada spike ini); keputusan tidak bergantung padanya. Bila pemilik produk kelak menginginkan alat siap pakai, antarmuka `diff.Catalog` dan `diff.Compare` adalah titik penggantian.

Pemetaan yang ditetapkan (paket `dbmodel`, `sqlgen`, `diff`; keputusan kecil, dapat dibalik tanpa mengubah API):

- `string` dan `text` menjadi `text` dengan satu CHECK `length` (minLength/maxLength) per kolom, sejalan dengan migrasi tulis tangan `example_notes.title`; `integer` menjadi `integer` bila `minimum` dan `maximum` keduanya ada dan muat 32-bit, selain itu `bigint`, dengan satu CHECK rentang; `decimal` wajib `precision` dan `scale`; `enum` menjadi `text` + CHECK `IN`; `pattern` dan `format` tidak masuk database (semantik RE2 dan PostgreSQL berbeda).
- Perbandingan mengabaikan nama constraint dan indeks (migrasi tulis tangan sering bernama lain) tetapi membandingkan definisi; indeks atau unique yang tidak dideklarasikan, kolom berlebih, dan nilai bawaan berbeda adalah selisih; tabel berawalan slug yang tidak ada di model hanya catatan; pemicu di luar model hanya catatan; pemicu bangkitan yang hilang, nonaktif (`tgenabled` bukan `O`), atau berbadan lain adalah selisih (Requirement 13.10).
- Kolom turunan `stored`: hanya konstruk tata bahasa tertutup dan fungsi immutable (`round`, `trunc`, `abs`, `mod`, `COALESCE`, `NULLIF`, `CASE`); `concat` atas `date`/`datetime` ditolak (cast ke teks bukan immutable); pembagian dengan `NULLIF(pembagi, 0)` (evaluator: tak tentu; database: NULL); ketujuh mode pembulatan diterjemahkan dan terbukti paritas dengan `metadata.EvaluateExpr` pada 150 baris acak berseed (tes integrasi).
- Literal teks hanya lewat `sqlgen.Literal` (kutip digandakan, `E'...'` bila ada backslash, NUL dan UTF-8 tak sah ditolak); literal numerik hanya lewat `NumericLiteral` (string desimal kanonik); badan fungsi dibungkus `DollarQuote` dengan penanda yang dipilih agar tidak ada di badan; setiap pernyataan keluaran dibuktikan tetap satu pernyataan oleh pemecah pernyataan tingkat atas pada tes.
- Role migrasi: `CREATE ROLE erpora_mig_<hex> ... NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS` dengan hak `CREATE` pada database sementara dan skema `public` (bukan pemilik database; tidak membutuhkan superuser pada sisi admin); role dan objeknya dihapus (`DROP OWNED BY`, `DROP ROLE`) juga saat gagal. Terbukti: `COPY ... PROGRAM`, `pg_read_file`, `CREATE ROLE`, dan `SET session_replication_role` gagal dengan `permission denied`.

Hal yang menunggu konfirmasi atau keputusan perancang spesifikasi (belum dikerjakan secara diam-diam):

1. **Tabel transisi status** (Requirement 13.8 butir a): skema `resource.schema.json` belum punya kunci transisi, sehingga `Derive` belum membuat pemicu penguncian; templat pemicu sudah menerima `LockSpec.Transitions`. Perlu ditetapkan dari mana pasangan (lama, baru) berasal (task 13). **Ditutup 2 Nov 2026 (K23, K24):** pasangan lifecycle berasal dari metadata bawaan `sdk:document` (tujuh pasangan sah) dan transisi sumbu dari `statusAxes`; skema memuat kunci `statusAxes` (task 4.5).
2. **Nilai kolom `<relasi>_type` pada `morphTo`**: dipakai kunci resource bentuk manifest (`sales.invoice_line`); spek hanya menyebut "kunci resource".
3. **Penamaan tabel pivot `belongsToMany`**: `<slug>_<pemilik>_<relasi>` dengan kolom `<pemilik>_id` dan `<target>_id`; pivot antar resource yang sama dan `morphToMany` belum didukung (galat jelas).
4. **`hasOne`**: nama kolom FK seberang tidak ditebak; wajib ada tepat satu `belongsTo` balik pada resource seberang (UNIQUE ditambahkan pada kolom itu).
5. **Nilai bawaan `now()`** pada kolom `created_at`/`updated_at` migrasi `example` tidak dapat dinyatakan sebagai literal metadata, sehingga `metadata:diff` untuk `example` (task 14.1) melaporkan dua `column_default` (tes integrasi mendokumentasikannya). Perlu keputusan: nilai bawaan dinamis di metadata, atau pelonggaran perbandingan default. **Ditutup 2 Nov 2026 (K22):** `createdAt` dan `updatedAt` memakai `DEFAULT now()` implisit dari `sdk:audit` yang selalu disertakan; tidak ada nilai bawaan dinamis di berkas metadata.
6. **Task 8.2 versus resolver**: resolver menolak `omit`/`set` atas field `sdk:audit` (Requirement 3.6, tanpa pengecualian), sedangkan D-18 dan task 8.2 menyebut `omit` atas field bawaan (`createdBy`, `updatedBy`) untuk memigrasikan `example`. Perlu diselaraskan sebelum task 8.2. **Ditutup 1 Nov 2026 (K19):** `example` dimigrasikan tanpa `omit`; `createdBy` dan `updatedBy` ditambahkan aditif ke kontrak dan implementasi, nullable (K22).

### Pembangkit migrasi 11.1 dan 11.2 (pelaksana, 2 Nov 2026)

Paket `internal/devtools/metadatatools/migrate` dan perintah `metadata:migrate <modul> <nama> [--dry-run]` (berkas `commands/metadata_migrate.go`; Taskfile `metadata:migrate`). Alur: `metadata:diff` untuk satu modul, `migrate.Generate`, lalu tulis `modules/<dir>/migrations/NNNN_<nama>.sql` lewat `gen.Apply` (nomor dan nama mengikuti `make:migration`; tidak menimpa; tidak pernah menerapkan).

- **Otomatis (aman):** tabel baru (beserta indeks, FK, pemicu), kolom nullable atau berdefault, indeks baru, pelepasan NOT NULL, pelebaran CHECK panjang dan rentang, nilai enum `extensible` bertambah (hanya bentuk CHECK bangkitan yang dikenali; selain itu kerangka), perubahan nilai bawaan, pemicu bangkitan hilang atau nonaktif, dan constraint (CHECK, unique, FK) yang hanya menyentuh kolom yang ditambahkan pada migrasi yang sama.
- **Kerangka `TODO(metadata:migrate)` (seluruhnya komentar):** ubah tipe, ganti ekspresi kolom turunan, NOT NULL pada kolom ada atau kolom baru NOT NULL tanpa nilai bawaan, perkecil CHECK, CHECK atau unique atau FK baru pada kolom ada, hapus apa pun (kolom, constraint, indeks), kunci utama berbeda, dan pemicu bangkitan yang berubah bukan dari templat kita (nama tak dikenal pada model). Pemicu bangkitan yang berbeda dari templat (parameter metadata berubah, mis. pasangan `migrateFrom` baru) diganti otomatis dengan `CREATE OR REPLACE FUNCTION` dan `CREATE OR REPLACE TRIGGER` lalu `ENABLE ALWAYS`, tanpa `DROP` (lihat 12.3 dan putaran perbaikan T2). Tabel `large: true` menambah penanda `-- +goose NO TRANSACTION` dan templat backfill per batch (komentar); `sqlgen.BatchCopyBackfill` membangkitkan salinan per batch yang aktif (blok `DO` ber-`COMMIT`, terbukti lewat goose NO TRANSACTION) untuk task 12.3.
- **Down:** hanya pembalikan aman (menghapus yang ditambahkan migrasi yang sama). Bila ada langkah tidak dapat dibalik (pelonggaran batasan, nilai bawaan lama, status pemicu), Down ditandai `TIDAK DAPAT DIBALIK` dan memuat penjaga `RAISE EXCEPTION` yang menggagalkan `goose down`.
- **Pemeriksa TODO:** `migrate.FindTodos(fsys)` dan `migrate.HasTodo` disediakan untuk `metadata:check` (menggagalkan selama penanda ada); pemanggilannya di paket `check` belum dipasang (milik agent A).
- **K22:** `Derive` memberi `DEFAULT now()` pada `createdAt`/`updatedAt` (lewat `Field.DefaultNow`) dan `DEFAULT 1` pada `version` dari `sdk:audit` secara implisit (`version` default 1 ditetapkan di `Derive`, bukan di `audit.abstract.yaml`); `createdBy`/`updatedBy` nullable. `metadata:diff example` kosong.

### Pemicu immutabilitas dokumen 13.1 dan 13.2 (pelaksana, 2 Nov 2026)

`dbmodel.Derive` kini membuat `Trigger` dari metadata terselesaikan: untuk resource dokumen satu pemicu penguncian `<tabel>_lock_trg` (BEFORE UPDATE OR DELETE; `LockSpec` memuat `lockWhen`, tujuh pasangan lifecycle `Resolved.Lifecycle`, `Axes` dari `StatusAxes`, dan `MigratePairs` dari `migrateFrom`), dan pemicu anak `<tabel>_<kolomfk>_lock_trg` (BEFORE INSERT OR UPDATE OR DELETE) untuk baris `embedded`, pivot `belongsToMany`, dan tabel seberang `hasOne`. Badan fungsi dari templat tetap di `sqlgen/trigger.go`.

- Pemicu dokumen (OLD): `DELETE` baris terkunci selalu ditolak; `UPDATE` lolos hanya bila (a) hanya status, version, updated_at, updated_by berubah dengan pasangan lifecycle sah, (a2) hanya kolom sumbu status (plus version, updated_at, updated_by) berubah, sekurangnya satu sumbu berubah, dan tiap sumbu yang berubah dengan pasangan sah (termasuk mundur), atau (b) hanya kolom tujuan `migrateFrom` diisi dari NULL menjadi salinan persis (dan sekurangnya satu berubah). Dokumen `draft` bebas; kolom turunan dikecualikan dari perbandingan baris (pada BEFORE belum dihitung).
- Pemicu anak: induk dikunci `FOR SHARE` tanpa predikat kunci lalu diperiksa dengan pernyataan baru; pada READ COMMITTED predikat kunci saja tidak menunggu transaksi induk yang belum commit. Terbukti dengan tes balapan dua arah (INSERT anak versus transisi induk). Pada UPDATE yang memindahkan anak antar induk, kedua induk diperiksa. Tabel induk pada badan fungsi tidak dikualifikasi skema (badan identik di skema mana pun; dibandingkan mesin selisih).
- **Keputusan pelaksana, perlu konfirmasi pemilik produk:** pemicu dibuat `ENABLE ALWAYS TRIGGER` (`tgenabled = A`), bukan `ENABLE` biasa, agar `session_replication_role = replica` (Requirement 13.9) tidak melewatinya; mesin selisih menganggap pemicu yang tidak ALWAYS sebagai `trigger_disabled`, dan migrasi tulis tangan yang menonaktifkan pemicu wajib mengaktifkannya kembali dengan `ENABLE ALWAYS`. Keterbatasan yang tidak dapat ditutup di database: pemilik tabel (role aplikasi tunggal `erp`) tetap dapat menjalankan `ALTER TABLE ... DISABLE TRIGGER`; yang dijaga adalah tidak adanya jalur pengecualian yang disediakan sistem, dan penonaktifan terdeteksi `metadata:diff`.
- Perbaikan tambahan di `Derive`: `hasOne` dikerjakan setelah semua `belongsTo` (sebelumnya bergantung urutan kunci resource).

### Rename dan penghapusan kolom 12.3 (pelaksana, 2 Nov 2026)

`metadata:migrate`: field baru dengan `migrateFrom` menghasilkan `ADD COLUMN` plus salinan data otomatis (`sqlgen.CopyColumn`; per batch `NO TRANSACTION` lewat `BatchCopyBackfill` pada tabel `large: true`), tanpa menghapus kolom lama; kolom lama boleh sudah hilang dari metadata (rename dan hapus dalam satu migrasi: salin dulu). Pemicu bangkitan yang berubah (mis. pasangan `migrateFrom` baru) diganti otomatis sebelum salinan (lepas pemicu, `CREATE OR REPLACE FUNCTION`, buat ulang, `ENABLE ALWAYS`; Down tak dapat dibalik) agar baris terkunci ikut tersalin lewat pengecualian (b). Penghapusan kolom (temuan `column_extra`) tetap hanya kerangka `TODO(metadata:migrate)` tetapi memuat penjaga aktif (`sqlgen.GuardUnsyncedRows`: gagal bila ada baris dengan kolom lama terisi dan kolom pengganti, yaitu kolom dengan `migrateFrom` ke kolom itu, kosong), `DROP COLUMN` hanya komentar, catatan "BACKUP WAJIB", dan Down tidak dapat dibalik (penjaga `RAISE EXCEPTION`); constraint dan indeks yang ikut hilang bersama kolom tidak menjadi kerangka tersendiri. **Rancangan syarat `removeAfter`:** tidak menduplikasi git di `migrate`; `metadata:migrate` menjalankan `metadata:check` dengan pembanding tag rilis (`check.ExecGit`) lebih dulu dan berhenti bila penghapusan field melanggar deprecate-before-remove atau `removeAfter` (modul tanpa tag rilis dikecualikan, sama seperti D-15). Terbukti integrasi: lompatan versi 0001 ke 0003 sama dengan upgrade bertahap (skema dan data) dan cocok dengan metadata; penjaga menggagalkan dan membatalkan migrasi lalu lolos setelah disalin; Down ditolak; salinan per batch pada 250 baris; rename pada dokumen terkunci. Catatan upgrade: `docs/module-sdk/lifecycle.md` bagian migrasi.

### Putaran perbaikan T2 (pelaksana, 2 Nov 2026; hasil review independen)

- **I1 rename dan hapus dalam satu rilis:** `metadata:check` kini mengizinkan `migrateFrom` ke field yang SUDAH dihapus dari metadata bila baseline rilis terakhir (pembanding git yang sama, `check/release.go`) memuat field itu `deprecated`; baseline memuatnya tanpa deprecated atau tidak memuatnya adalah galat `migrate_from_unknown`; tanpa baseline (modul belum pernah dirilis, tanpa git) hanya peringatan `migrate_from_unbaselined`. Dengan begitu rilis MAJOR yang menghapus kolom lama tetap mempertahankan `migrateFrom` pada field baru sehingga `metadata:migrate` membangkitkan penjaga penghapusan. Pasangan `migrateFrom` pada pemicu dokumen tidak lagi mensyaratkan kolom asal ada: kolom asal dibaca dinamis (`to_jsonb(OLD) -> 'asal'`, kolom yang tidak ada dianggap null) sehingga fungsi pemicu tidak rusak setelah kolom lama dihapus. Dibuktikan tes ujung ke ujung yang melewati `check` dan CLI nyata (`TestMetadataMigrate_RemovingRenamedFieldKeepsGuard`) dan tes dokumen terkunci (`TestRename_LockedDocumentRemovalInOneMigration`).
- **I2 pemicu efektif:** mesin selisih membandingkan `pg_get_triggerdef` (WHEN, UPDATE OF, skema fungsi), `prosecdef`, `proconfig`, bahasa fungsi, dan menandai pemicu tulis tangan BEFORE ROW yang berjalan sesudah pemicu bangkitan dan dapat mengubah NEW sebagai selisih `trigger_after_lock` (bukan catatan).
- **M2** pemicu diganti dengan `CREATE OR REPLACE` tanpa `DROP`, `ADD COLUMN IF NOT EXISTS` pada berkas NO TRANSACTION. **M3** `div` kolom `stored` memakai pembagian eksak atas bilangan bulat (`div`/`mod`) dengan semua mode pembulatan (paritas acak termasuk kasus pembulatan ganda). **M4** introspeksi hanya tabel (`relkind r,p`) dan tahan nil. **M5** nama dari katalog database divalidasi (`^[a-z][a-z0-9_]{0,62}$`) sebelum masuk SQL aktif. **M6** `--timeout` (bawaan 10 menit) dan Ctrl-C/SIGTERM pada `metadata:diff` dan `metadata:migrate`; pembersihan role dan database sementara tetap berjalan (sesi role diputus lebih dulu). **M7** langkah CI terpisah `task test:metadata-cli` dengan env hanya pada langkah itu. **M8** kerangka rename ke kolom NOT NULL tanpa default dan kerangka penghapusan memuat langkah salin (komentar; per batch pada tabel large); `migrateFrom` yang ditambahkan pada kolom yang SUDAH ada tidak mengubah apa pun di database sehingga tidak ada selisih untuk dibangkitkan; salinannya diatur lewat kerangka penghapusan atau migrasi tulis tangan. **M9** `lockWhen` yang merujuk accessor `stored` (kolom turunan) atau memakai `ref` ditolak `metadata:check` (`lockwhen_not_sql`). **M10** `LOCK TABLE ... IN SHARE ROW EXCLUSIVE MODE` sebelum penjaga penghapusan (dilewati pada berkas NO TRANSACTION). **M11** pembanding memuat kolasi, identity, definisi indeks lengkap (opclass, DESC, NULLS, INCLUDE, NULLS NOT DISTINCT, predikat), dan indeks tidak valid (`index_invalid`). **M12** dokumen diselaraskan.
- `TRUNCATE` dan `Down` yang berisi `DROP` sudah diputuskan pemilik produk pada 2 Nov 2026 (K31 dan K32).

### Ringkasan pelaksanaan T1 sampai T3 dan gerbang (2 Nov 2026)

- Gerbang T1, T2, dan T3 hijau. T2: 1245 tes integrasi lewat tunnel SSH ke VPS. T3: `task web:check` exit 0, 4390 tes frontend, JS awal 198,5 KB dari 200 KB gzip. `task test:generator` hijau (669 detik). `metadata:check` dan `metadata:generate --check` hijau; `metadata:diff` kosong untuk `example` dan fixture.
- Tiga review akhir putaran independen (evaluator dan frontend, T1, T2) dilakukan; seluruh temuan yang sesuai spek sudah diperbaiki. Temuan di luar spek dicatat di daftar konfirmasi di bawah.
- Verifikasi browser: aplikasi penuh dengan modul `example` di basis data sementara di VPS lewat tunnel. Dashboard Desk dan halaman Notes tampil 23 catatan; API mengembalikan bentuk bangkitan dengan `createdBy` dan `updatedBy` bernilai null; tanpa galat konsol. Basis data sementara sudah dihapus.
- Keputusan pemilik produk K19 sampai K24 (jawaban konfirmasi 1 dan 2 Nov 2026): `example` aditif, `math/big`, linter desimal nullable, `sdk:audit` otomatis, lifecycle status baku, dan `statusAxes`. Penamaan status ditetapkan pada K30 (konfirmasi 2 Nov 2026).

### Konfirmasi pemilik produk atas temuan pelaksanaan (2 Nov 2026)

Disetujui dan dicatat di requirements (K25 sampai K34), design (D-39 sampai D-46), dan PRD-000 Q41 butir (15). Tindak lanjut kode dikerjakan pada putaran konfirmasi 2 Nov 2026.

1. Pola identifier Req 1.4: teks diselaraskan ke pola ketat (K25); akronim ditulis `taxId`.
2. Kata tercadang: dicek per target pada bentuk hasil konversi (K26); `type`, `order`, `user`, `group`, dst. sah.
3. Alasan include `set` (Req 3.6, 14.5): kunci `reason` wajib pada setiap include `set` (K27).
4. `unmanagedColumns` (Req 2.9): objek `{name, classification: highly_confidential, reason}` (K28).
5. `lockWhen` hanya pada resource dokumen konkret (K29).
6. Nama status baku `draft`, `deleted`, `pending_approval`, `accepted`, `rejected`, `cancelled`, `done` (K30; `need_approval` diganti `pending_approval`, dapat dibatalkan); pemicu `BEFORE TRUNCATE` (K31); `Down` pembalik expand hanya-development (K32).
7. `transaction-data-standards.md` §1 dan §2 serta `roadmap-and-risks.md` diselaraskan ke K23 (K33); `posted` dipetakan ke `accepted`.
8. Kunci resource selalu tunggal; tabel dan URL jamak turunan `naming.Plural`; capability tulis-tangan `example.notes.read` tidak diganti (K35; uji `TestResourceKeyMappingRule`).
9. Modul tanpa tag rilis dikecualikan dari syarat `deprecated`/`removeAfter`/MAJOR (K34).

### Masih menunggu konfirmasi pemilik produk

10. Keputusan kecil implementer (semantik tiga-nilai evaluator, `contains` hanya melipat ASCII, batas ekspresi, ambang `overlapping_repeat` 32, format desimal klien, operator filter per tipe, kunci morph bentuk manifest, runtime bangkitan disalin per modul bukan di SDK, pesan validasi platform belum ada di katalog i18n `platform`, label i18n bangkitan belum ada): belum ditinjau pemilik produk; rekomendasi: setujui dan catat sebagai keputusan desain.
11. `ENABLE ALWAYS TRIGGER` pada pemicu bangkitan (agar `session_replication_role = replica` tidak melewatinya) dan penjaga data pada migrasi penghapusan (K18): belum dikonfirmasi; rekomendasi: setujui keduanya.
12. T4 (grup 19 sampai 25) menunggu PRD-001 authz.

## Notes

- Setiap task mereferensi requirement spesifik untuk traceability; checkpoint memastikan validasi inkremental.
- **Alur sprint** (instruksi global): satu putaran per tahap = implementasi tanpa review per task, satu review akhir putaran (kesesuaian + `security-audit` terfokus pada perubahan putaran), lalu putaran perbaikan oleh implementer yang sama lewat `SendMessage`. Audit seluruh codebase hanya bila diminta eksplisit. Subagent tidak men-spawn subagent lain.
- **Kelas beban:** Haiku untuk dokumen dan pekerjaan mekanis berspesifikasi lengkap; Sonnet untuk logika inti, integrasi lintas file, tes, dan debugging; model lebih tinggi hanya untuk review akhir putaran. Satu agent per kelas beban yang terus dilanjutkan; bila putaran berikutnya tidak berhubungan, compact dulu atau spawn baru dengan brief lengkap.
- **TDD:** tulis test yang gagal dulu, lalu implementasi; satu commit per unit kerja yang bisa diuji, pesan konvensional (`feat(metadata): …`). Formatter dan linter proyek dijalankan setelah semua task selesai (gerbang `task lint`), bukan per task. Jangan push atau membuat PR tanpa diminta.
- **Lingkungan:** Windows (`-race` dilewati); Postgres uji lewat tunnel SSH ke VPS dan `ERP_TEST_DATABASE_URL`, **jangan** menjalankan wadah uji di laptop; `task test:generator` juga di VPS; set `ERP_REQUIRE_BUN=1`; Taskfile tanpa `rm`/`cp`/`sed`; path repo tanpa spasi.
- Hal di luar spek atau yang menyimpang dari PRD/ADR/api-guidelines ditanyakan dulu; keputusan yang disetujui dicatat pada spec terkait (requirements/design, PRD Q-number) dengan tanggal absolut dan pemilik produk sebagai penyetuju.
- Di luar spec ini: komponen UI generik M3c dan M3d, filter bersarang dan pencarian lanjutan, sistem enkripsi dan field `highly_confidential`, field kustom runtime, salinan untuk urut relasi master lintas modul (K9), sinkronisasi master ke master, workflow, penomoran, ledger, import, export, dan print.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["2.1", "2.3", "3.1"] },
    { "id": 3, "tasks": ["2.2", "2.4", "3.2", "3.5"] },
    { "id": 4, "tasks": ["3.3", "3.4", "3.6", "4.1", "5.1", "5.3", "5.5"] },
    { "id": 5, "tasks": ["4.2"] },
    { "id": 6, "tasks": ["4.3", "4.4", "4.5", "5.2", "5.4", "5.6", "8.1"] },
    { "id": 7, "tasks": ["5.7", "5.8", "6.1"] },
    { "id": 8, "tasks": ["6.2", "6.3", "6.4", "6.5"] },
    { "id": 9, "tasks": ["7.1", "8.2"] },
    { "id": 10, "tasks": ["7.2"] },
    { "id": 11, "tasks": ["9.1"] },
    { "id": 12, "tasks": ["9.2"] },
    { "id": 13, "tasks": ["10.1", "16.1"] },
    { "id": 14, "tasks": ["10.2", "16.2"] },
    { "id": 15, "tasks": ["10.3", "16.3", "16.4"] },
    { "id": 16, "tasks": ["10.4"] },
    { "id": 17, "tasks": ["10.5", "17.1"] },
    { "id": 18, "tasks": ["11.1", "12.1", "13.1", "17.2"] },
    { "id": 19, "tasks": ["11.2", "12.2", "13.2"] },
    { "id": 20, "tasks": ["12.3", "12.4"] },
    { "id": 21, "tasks": ["14.1"] },
    { "id": 22, "tasks": ["14.2"] },
    { "id": 23, "tasks": ["15.1"] },
    { "id": 24, "tasks": ["15.2"] },
    { "id": 25, "tasks": ["18.1", "18.2", "18.3", "18.4", "19.1"] },
    { "id": 26, "tasks": ["18.5", "19.2"] },
    { "id": 27, "tasks": ["20.1", "21.1", "22.1", "23.1"] },
    { "id": 28, "tasks": ["20.2", "21.2", "22.2"] },
    { "id": 29, "tasks": ["21.3", "23.2"] },
    { "id": 30, "tasks": ["23.3"] },
    { "id": 31, "tasks": ["24.1"] },
    { "id": 32, "tasks": ["24.2"] },
    { "id": 33, "tasks": ["25.1"] },
    { "id": 34, "tasks": ["25.2"] },
    { "id": 35, "tasks": ["26"] }
  ]
}
```
