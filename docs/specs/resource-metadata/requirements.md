# Requirements Document

## Introduction

Spec ini menetapkan **sistem metadata resource**: satu berkas deklaratif per resource sebagai **satu-satunya sumber kebenaran** atas bentuk data (field, tipe, batasan, aturan kondisi, relasi, accessor, label, penguncian dokumen). Dari berkas itu dibangkitkan dan diperiksa secara otomatis: skema kontrak OpenAPI, validasi sisi server (Go) dan sisi klien (TypeScript), metadata untuk antarmuka generik (form, tabel data, filter) yang dibangun di M3c dan M3d, selisih serta migrasi database, dan, pada tahap akhir, CRUD (entitas, repository, use case, handler) untuk resource sederhana. Resource yang rumit tetap boleh menulis logika sendiri tanpa kehilangan manfaat tahap-tahap sebelumnya.

Tujuan utamanya: **tidak ada kolom yang terlewat atau berbeda** antara database, backend, dan frontend; kolom baru cukup ditambahkan di metadata, lalu kontrak, validasi, antarmuka, dan migrasi mengikuti. Seluruh integrasi wajib aman secara bawaan (otorisasi, IDOR, mass assignment, kebocoran data), bukan ditambahkan kemudian.

**Sumber keputusan:** sesi brainstorming bersama pemilik produk, 1 Nov 2026 (butir keputusan di bagian "Keputusan pemilik produk"); PRD-000 (FR-22 generator, FR-23 kontrak, §4.2 aturan arsitektur, Q31, Q40); PRD-001 FR-110 sampai FR-119 (resource, field group, scope, SDK otorisasi, IDOR); `docs/architecture/api-guidelines.md` (§4 sampai §7, §11, §14); `docs/security/security-guidelines.md` (§2, §3, §6); `docs/architecture/transaction-data-standards.md` (§7, §15); ADR-0003 (PostgreSQL, GORM, goose), ADR-0004 (database per tenant), ADR-0010 (migrasi dan kompatibilitas upgrade).

**Kondisi awal (diverifikasi 1 Nov 2026):**
- Sumber kebenaran kontrak saat ini hanya `modules/<x>/openapi.yaml` dengan ekstensi `x-erp-*` (permission, klasifikasi, `x-erp-filterable`, `x-erp-sortable`, `x-erp-field-group`), dilint Spectral dan diperiksa tes kesesuaian; tipe TypeScript dibangkitkan `openapi-typescript`. Tidak ada penghubung otomatis antara kontrak, domain Go, skema database, dan form frontend.
- Domain, repository, dan handler ditulis tangan atau dari kerangka satu kali `make:*` (`entity`, `usecase`, `repository`, `handler`, `contract`, `migration`); handler hasil generate memakai hook yang menjawab `501` sampai disambungkan.
- `sdk/authz` baru memuat `AsSystem` dan `MaskFields`. `Require`, `Filter`, `Check`, `Fields`, dan `CheckWrite` (PRD-001 FR-115) belum ada; manifest belum punya `resources`/`field_groups` (PRD-001 FR-110).
- Tabel modul **tidak** punya kolom `tenant_id`: isolasi tenant memakai database per tenant (ADR-0004, `sdk/data`). Migrasi goose ditulis tangan per modul (`modules/<dir>/migrations`); `AutoMigrate` dilarang (D3b).
- Frontend belum punya pustaka form atau tabel data; `FormDialog`, `FormPage`, list view, dan FilterBuilder adalah M3c dan M3d.

**Tahap pengerjaan** (urutan disetujui pemilik produk 1 Nov 2026; tiap tahap dapat dirilis sendiri):

| Tahap | Isi | Exit gate |
| ----- | --- | --------- |
| **T1 Inti metadata** | Skema berkas, penyertaan/abstrak/timpaan, model kondisi, accessor, templat label, relasi (deklarasi dan validasi statis), integrasi manifest, pembangkit OpenAPI + DTO dan validasi Go + metadata TypeScript, pemeriksaan drift | `metadata:check` dan `metadata:generate --check` hijau untuk `example` dan modul fixture; paritas Go dan TS lulus korpus vektor bersama |
| **T2 Database** | Mesin selisih terhadap skema database hasil migrasi, pembangkit migrasi, aturan deprecated dan rename, pemicu immutabilitas | Selisih kosong untuk `example` dan modul fixture di CI; skenario migrasi aman dan perlu-keputusan teruji |
| **T3 Runtime frontend** | Paket metadata TypeScript (tipe, evaluator kondisi, validator, registri tipe field dengan editor form dan editor filter) yang dikonsumsi M3c dan M3d | Paritas Go dan TS; anggaran bundle terjaga; kontrak konsumsi M3c dan M3d terdokumentasi |
| **T4 Relasi dan CRUD bangkitan** | Kontrak lintas modul (resolusi label, pencarian), pemeriksaan ID referensi, field salinan dan penguncian, CRUD resource sederhana, kait dan jalur lepas | Tes IDOR pengguna A vs B lulus untuk setiap integrasi; **bergantung PRD-001 FR-110 sampai FR-115 dan FR-119** |

**Prasyarat (keputusan pemilik produk, 1 Nov 2026):** sistem enkripsi field dibangun lebih dahulu sebagai spec terpisah (`field-encryption`, end to end: manajemen kunci, enkripsi AES-256-GCM, rotasi, indeks buta) dan dipakai bersama PRD-001 (rahasia pengaturan, kunci tanda tangan, rahasia MFA). Pemetaan database field `highly_confidential` (T2) dan CRUD bangkitan (T4) bergantung padanya; T1 dan T3 tidak.

**Di luar spec ini (non-goals):**
- Komponen UI generik (list view, FilterBuilder, form) dan halamannya: M3c dan M3d memakai paket T3 sebagai konsumen. Spec ini hanya menjamin metadata dan kontrak konsumsinya.
- Field kustom yang ditambah pengguna lewat UI saat runtime, builder metadata, dan penyimpanan metadata di database (keputusan pemilik produk: metadata diubah developer lewat kode).
- Pohon filter bersarang (AND/OR) pada antarmuka HTTP dan pencarian lanjutan: menunggu perluasan aditif `api-guidelines.md` §5.1 (P-3, diputuskan 1 Nov 2026: spec lanjutan terpisah setelah M3d).
- Pemakaian field salinan untuk mengurutkan dan memfilter relasi master **lintas modul** (ditunda pemilik produk, 1 Nov 2026). Mekanisme salinan untuk dokumen transaksi **termasuk** spec ini (Requirement 14).
- Sinkronisasi master ke master antar-modul lewat event, mesin workflow atau approval, penomoran, ledger, maker-checker, periode akuntansi (tetap per resource; lihat Requirement 13.7), import, export, dan print.
- Bahasa ekspresi umum: ekspresi dibatasi pada tata bahasa tertutup (Requirement 5).

## Glossary

- **Berkas metadata**: berkas YAML per resource di `modules/<dir>/metadata/`, divalidasi JSON Schema `contracts/resource.schema.json`.
- **Resource**: sesuatu yang dilindungi dan disimpan, dengan kunci `<slug>.<nama>` (PRD-001 FR-110); satu berkas metadata `kind: resource` memetakan tepat satu resource dan satu tabel.
- **Metadata abstrak**: berkas `kind: abstract` tanpa tabel sendiri (mirip trait), hanya untuk disertakan; contoh bawaan: `audit`, `document`.
- **Penyertaan** (`include`): metadata menyertakan metadata lain (abstrak atau resource) lalu menerapkan **timpaan** bergaya tambal (`set`, `omit`, `add` per field). Diselesaikan saat pembangkitan; hasilnya metadata datar.
- **Tipe domain**: definisi satu field bernama yang dipakai ulang (`kind: fieldType`), mis. `email`, `npwp`, `rupiah`: tipe dasar, batasan, klasifikasi bawaan, format tampilan, dan petunjuk editor.
- **Blok**: metadata abstrak berisi beberapa field beserta tata letak (mis. alamat, kontak) yang dapat disertakan berkali-kali dalam satu resource dengan `prefix`.
- **Kondisi**: pohon deklaratif terbatas `{field, op, value|ref}` dengan `all`/`any`/`not`; satu model untuk aturan tampil/wajib/hanya-baca, penguncian, validasi lintas field, dan filter pada field relasi.
- **Accessor**: field turunan (seperti `Attribute` Eloquent) dengan tiga jenis: `expr` (ekspresi deklaratif), `stored` (kolom turunan database), `func` (fungsi Go bernama, hanya tampil).
- **Templat label**: aturan satu resource menyusun label ringkas dari beberapa field (dipakai relasi, dropdown, saran pencarian, breadcrumb).
- **Relasi**: `belongsTo`, `hasOne`, `hasMany`, `belongsToMany`, `morphTo`, `morphMany`, `morphToMany`; **relasi satu modul** (FK dan JOIN boleh) berbeda dari **relasi lintas modul** (hanya menyimpan ID, data lewat `contract/`).
- **Field salinan** (snapshot): field yang disalin dari record relasi ke dokumen, disegarkan selama dokumen belum terkunci, lalu dibekukan.
- **Terkunci**: status dokumen yang dinyatakan `lockWhen`; semua field menjadi hanya-baca dan database menolak `UPDATE`/`DELETE`.
- **Selisih**: perbedaan antara skema database hasil menjalankan semua migrasi dan skema yang diturunkan dari metadata.
- **Deprecated**: penanda field yang akan digantikan; tetap ada di kontrak dan database sampai tahap contract (api-guidelines §14.3).
- **Korpus vektor**: kumpulan kasus uji JSON bersama (`contracts/metadata-vectors/`) yang dijalankan evaluator Go dan TypeScript untuk membuktikan paritas.
- **Generator**, **Registry modul**, **Manifest**, **Principal**, **Desk**: sama dengan glossary spec lain dan PRD-000.

## Keputusan pemilik produk (1 Nov 2026)

| # | Keputusan | Sumber |
| - | --------- | ------ |
| K1 | Satu sumber kebenaran metadata per resource untuk database, backend, dan frontend; metadata diubah developer lewat kode, bukan builder runtime | Brainstorming |
| K2 | Opsi C: antarmuka (daftar, form, filter) dirender dari metadata; halaman rumit tetap boleh ditulis tangan lewat titik perluasan | Brainstorming |
| K3 | Migrasi dibangkitkan dari selisih metadata sebagai berkas goose yang ditinjau; larangan `AutoMigrate` runtime tetap berlaku | Brainstorming |
| K4 | CRUD, repository, dan entitas dibangkitkan hanya untuk resource sederhana; resource maju punya kait dan jalur lepas | Brainstorming |
| K5 | Metadata mendukung penyertaan dan timpaan, metadata abstrak, relasi bergaya Eloquent, kondisi tampil/wajib/hanya-baca, accessor, templat label, dan filter pada field relasi | Brainstorming |
| K6 | Relasi lintas modul menyimpan ID saja; label diambil lewat eager load dan templat label | Brainstorming |
| K7 | Dokumen transaksi menyalin field master; selama belum terkunci boleh disegarkan, setelah terkunci tidak boleh berubah lagi (faktur terbit tidak berubah) | Brainstorming |
| K8 | Aturan migrasi merusak: field yang akan diganti atau dihapus ditandai `deprecated`; rename berarti field baru dengan salinan data dari field lama saat migrasi | Brainstorming |
| K9 | Pemakaian field salinan untuk mengurutkan relasi master lintas modul **ditunda** | Brainstorming |
| K10 | Urutan blok: M3a, **sistem enkripsi field (spec `field-encryption`)**, **blok metadata (spec ini)**, M3c, M3d, M3b (menunggu seluruh M3d), M3e, M3f, M3g | Brainstorming dan keputusan pemilik produk (1 Nov 2026) |
| K11 | Spec wajib menutup celah keamanan, terutama IDOR, di setiap integrasi | Permintaan pemilik produk |
| K12 | Kolom dapat dipakai ulang di banyak form dalam dua bentuk: **tipe domain** (satu kolom bertipe khusus, mis. email, NPWP, rupiah) dan **blok** (beberapa kolom, mis. alamat), agar penulisan berulang minimal | Brainstorming (1 Nov 2026; cakupan "keduanya" disetujui pemilik produk) |
| K13 | Pencarian atas field `highly_confidential` hanya kecocokan utuh (tanpa `matches`/`like`), lewat indeks buta dan body `POST`, bukan URL | Pemilik produk, 1 Nov 2026 |
| K14 | Sistem enkripsi field dibangun lebih dahulu, end to end, sebagai spec terpisah (`field-encryption`) yang dipakai bersama PRD-001 | Pemilik produk, 1 Nov 2026 |
| K15 | Kunci dan identifier berkas metadata ditulis camelCase; generator mengonversinya ke konvensi tiap bahasa (SQL snake_case, Go PascalCase, URL kebab-case, dan seterusnya) | Pemilik produk, 1 Nov 2026 |
| K16 | Pertanyaan terbuka P-1 sampai P-8 diputuskan sesuai rekomendasi ("sisanya saya terima"); P-7 digantikan K14 dan pencarian eksak K13 | Pemilik produk, 1 Nov 2026 |

## Requirements

### Requirement 1: Berkas metadata dan skema

**User Story:** Sebagai developer modul, saya ingin menulis metadata resource dalam satu format yang divalidasi, sehingga kesalahan penulisan tertangkap sebelum kode dibangkitkan.

#### Acceptance Criteria

1. THE berkas metadata SHALL berada di `modules/<dir>/metadata/<nama>.resource.yaml` (`kind: resource`) atau `<nama>.abstract.yaml` (`kind: abstract`), berformat YAML yang divalidasi JSON Schema `contracts/resource.schema.json` (dibenamkan seperti `module.schema.json`) dengan `schemaVersion`.
2. THE pembaca SHALL memakai pemuat YAML aman: tanpa tag kustom, jumlah dan kedalaman alias dibatasi, ukuran berkas ≤ 256 KB, dan berkas yang melampaui batas ditolak dengan pesan jelas (security-guidelines §3.8).
3. THE skema SHALL membatasi ukuran: ≤ 200 field per resource, ≤ 32 node per kondisi dengan kedalaman ≤ 4, ≤ 16 relasi, ≤ 100 nilai per `in`/enum, dan panjang `pattern` ≤ 200 karakter.
4. THE semua **kunci** berkas metadata (mis. `maxLength`, `lockWhen`) dan semua **identifier** buatan developer (nama field, relasi, accessor, aturan, dan segmen nama resource pada `key`) SHALL berbentuk camelCase ketat `^[a-z][a-z0-9]*([A-Z][a-z0-9]*)*$` (tanpa huruf kapital berurutan: akronim ditulis `taxId`, bukan `taxID`), panjang ≤ 63, dan SHALL tidak mengandung kata kunci SQL tercadang. Nilai `enum` bukan kunci dan tetap snake_case (Requirement 2.11); kunci manifest (`module.json`, PRD-001) tetap snake_case karena milik manifest.
5. THE `metadata:check` SHALL melaporkan semua pelanggaran skema dan semantik (nama ganda, rujukan tak terselesaikan, siklus) dengan lokasi berkas dan baris, tanpa menulis apa pun.
6. THE berkas metadata SHALL tidak memuat fragmen SQL, regex di luar subset yang diizinkan (Requirement 2.5), URL, atau kode; semua nilai diperlakukan sebagai data.
7. THE nama hasil gabungan (tabel `<slug>_<nama>`, tabel pivot, nama indeks, foreign key, pemicu, dan kolom berawalan) SHALL tidak melebihi **63 byte** (batas identifier PostgreSQL) dan SHALL unik per skema; `metadata:check` SHALL menolak pelanggaran, bukan membiarkan PostgreSQL memotong diam-diam.
8. THE `metadata:generate` SHALL mengonversi kunci dan identifier camelCase secara deterministik dan tanpa ambiguitas ke konvensi tiap keluaran (K15): SQL (tabel, kolom, indeks, constraint, pemicu) snake_case (`customerName` menjadi `customer_name`; tabel berprefiks slug modul dan jamak, mis. `sales_invoice_lines`); Go (field struct) PascalCase dengan inisialisme baku (`customerId` menjadi `CustomerID`) dan tag JSON camelCase; JSON, OpenAPI, dan TypeScript camelCase tanpa perubahan (api-guidelines §4.1); segmen URL kebab-case jamak (`invoiceLines` menjadi `invoice-lines`; api-guidelines §2); parameter query camelCase (api-guidelines §5.1). Aturan pemisahan kata: batas huruf kecil ke huruf besar memulai kata baru, angka melekat pada kata sebelumnya (`address2Line` menjadi `address2_line`). `metadata:check` SHALL menolak nama yang bertabrakan setelah konversi di salah satu target, kata tercadang target (SQL, Go, TypeScript), nama hasil konversi yang melampaui batas butir 7, dan konversi bolak-balik yang tidak identik. Tabel konversi lengkap ada di `design.md` Komponen 13.

### Requirement 2: Field, tipe, dan batasan

**User Story:** Sebagai developer modul, saya ingin menyatakan tipe dan batasan field sekali, sehingga database, backend, dan frontend memakai aturan yang sama.

#### Acceptance Criteria

1. THE tipe field SHALL mencakup: `string` (opsi `minLength`, `maxLength`, `pattern`, `format`), `text`, `integer`, `decimal` (opsi `precision`, `scale`, dan untuk uang `currency`), `boolean`, `date`, `datetime` (UTC), `enum` (nilai tertutup atau `extensible`), `uuid`, tipe domain (Requirement 3.8), dan field relasi (Requirement 6).
2. THE `decimal` SHALL dikirim dan diterima sebagai **string desimal** dan disimpan sebagai `numeric(p,s)`; perbandingan dan agregat SHALL memakai aritmetika desimal, tidak pernah `float`/`Number` (api-guidelines §4.1; transaction-data-standards §6).
3. THE setiap field SHALL menyatakan: `nullable` (default `false`), `default`, `readOnly`, nama kunci label i18n, `fieldGroup` (default grup `default` resource), `classification` (`public|internal|confidential|highly_confidential`, default `internal`), dan penanda `filterable`, `sortable`, `groupable`, `aggregatable`, `unique`, dan `index` (semua default `false`; indeks komposit dinyatakan di tingkat resource lewat `indexes`). Indeks dan unique **tidak** diturunkan otomatis: hanya yang dideklarasikan, ditambah yang melekat pada primary key, unique, dan foreign key, dianggap ada; `metadata:check` memberi peringatan untuk foreign key tanpa indeks.
4. THE field berklasifikasi `highly_confidential` SHALL **tidak pernah** ditandai `sortable`, `groupable`, atau `aggregatable`; penanda `filterable` hanya sah untuk **kecocokan utuh** lewat indeks buta (Requirement 2.12 dan 2.13, K13), tidak pernah lewat URL (security-guidelines §2 dan §3.4, aturan linter `erp-no-sensitive-in-url`); `aggregatable` hanya sah untuk `integer` dan `decimal`.
5. THE `pattern` SHALL dibatasi ke subset RE2 yang berperilaku sama di Go dan di JavaScript (tanpa lookaround dan backreference) dan SHALL lolos uji vektor di kedua sisi; pola di luar subset ditolak saat `metadata:check`; `metadata:check` juga menolak kuantor bersarang dan alternatif ambigu di bawah kuantor (tinggi bintang ≤ 1), dan klien membatasi panjang input sebelum menjalankan regex (mesin JavaScript dapat melakukan backtracking eksponensial).
6. THE field bawaan resource (`id`, `version`, `createdAt`, `updatedAt`, `createdBy`, `updatedBy`, dan `status` serta `amendedFromId` untuk dokumen; api-guidelines §4.3) SHALL berasal dari metadata abstrak bawaan dan SHALL selalu `readOnly`: tidak pernah diikat dari input klien.
7. THE properti bernama sensitif (aturan `erp-sensitive-needs-classification`) SHALL otomatis diwajibkan berklasifikasi `highly_confidential`; `metadata:check` SHALL gagal bila tidak.
8. THE field SHALL dapat ditandai `deprecated` (Requirement 12) dan SHALL dapat memuat `description` untuk dokumentasi kontrak.
9. THE field berklasifikasi `highly_confidential` SHALL disimpan **terenkripsi** (AES-256-GCM, nonce acak per nilai, `ERP_SECRET_KEY`) atau hanya sebagai hash (security-guidelines §2, §3.4); SHALL tidak dipakai pada kolom `stored`, CHECK nilai, indeks biasa (selain indeks buta, butir 12), atau templat label; dan skema baca bangkitannya SHALL mengecualikan field itu (`writeOnly`) kecuali kontrak mengizinkan `x-erp-allow-highly-confidential` menurut aturan linter yang ada.
10. THE sistem enkripsi field SHALL dibangun lebih dahulu, end to end, sebagai spec terpisah `field-encryption` yang dipakai bersama PRD-001 (K14, keputusan pemilik produk 1 Nov 2026); sebelum primitifnya tersedia di `sdk`, `metadata:check` SHALL menolak `generate.crud` dan pemetaan database pada resource yang memuat field `highly_confidential` (tahap T1 dan T3 tidak bergantung padanya).
11. THE nilai `enum` SHALL cocok `^[a-z][a-z0-9_]{0,62}$` (api-guidelines §4.1); `default`, nilai kondisi, dan nilai literal lain SHALL dikanonisasi sesuai tipenya (pola desimal, tanggal, uuid) sebelum dipakai di keluaran apa pun. Nilai `enum` adalah data, bukan kunci metadata, dan tidak tunduk pada aturan camelCase Requirement 1.4.
12. THE field `highly_confidential` yang ditandai `filterable` atau `unique` SHALL memakai **indeks buta**: kolom pendamping `<kolom>_idx` berisi HMAC-SHA-256 atas nilai yang telah dikanonisasi menurut tipe dan formatnya (mis. NPWP hanya digit), dengan kunci khusus yang diturunkan (HKDF) dari kunci induk berlabel resource dan field; nilai asli tidak pernah dipakai untuk pencarian, dan `unique` pada field ini hanya lewat kolom itu.
13. THE pencarian atas field `highly_confidential` SHALL hanya **kecocokan utuh** (`eq`); operator `contains`, `like`, `matches`, `in`, rentang, awalan, urut, grup, dan agregat SHALL ditolak (`400 invalid_filter`). Pencarian itu SHALL **tidak** lewat URL melainkan lewat `POST /api/<modul>/<resources>/search` dengan body JSON (aksi koleksi; tambahan aditif api-guidelines §14.1); SHALL mensyaratkan permission aksi `search` dan akses grup field minimal `read` atas field itu (tanpa itu `403 field_forbidden`); SHALL tercatat di audit tanpa nilai; SHALL dibatasi rate limit yang lebih ketat per principal (pencarian eksak dapat menjadi oracle keberadaan nilai); dan hasilnya SHALL melalui `authz.Filter`.

### Requirement 3: Penyertaan, metadata abstrak, timpaan, tipe domain, dan blok

**User Story:** Sebagai developer modul, saya ingin memakai ulang bentuk data yang mirip tanpa menulis ulang, sehingga resource dengan tabel terpisah tetap konsisten.

#### Acceptance Criteria

1. THE metadata SHALL dapat `include` metadata abstrak maupun resource lain, dan SHALL dapat menerapkan timpaan per field: `set` (ubah atribut), `omit` (buang), `add` (tambah).
2. THE penyertaan SHALL diselesaikan saat pembangkitan (`metadata:generate` dan `metadata:check`); hasilnya metadata datar, dan setiap atribut dapat ditelusuri ke berkas asalnya (untuk pesan galat dan tinjauan).
3. THE resolusi SHALL menolak siklus penyertaan, nama field ganda yang tidak ditimpa eksplisit, timpaan terhadap field yang tidak ada, dan timpaan yang melanggar skema (mis. mengubah tipe field yang sudah dipakai relasi).
4. THE metadata `kind: abstract` SHALL tidak menghasilkan tabel, kontrak, atau kode sendiri; hanya resource yang menyertakannya.
5. THE sumber penyertaan SHALL dibatasi: modul sendiri, metadata abstrak bawaan SDK, atau modul lain **hanya** lewat dependensi manifest (D1, D8) dan hanya untuk metadata yang ditandai `export: true` oleh modul pemiliknya; `include.source` SHALL divalidasi sebagai kunci metadata yang diresolusi lewat registry modul, bukan sebagai jalur berkas.
6. THE timpaan SHALL tidak dapat menurunkan `classification`, melonggarkan `fieldGroup`, atau mencabut `readOnly` yang berasal dari metadata asal; perubahan semacam itu ditolak. **Tanpa pengecualian** untuk field bawaan (Requirement 2.6), `owner_field`, `scope_fields`, `amendedFromId`, dan field `sdk:document` (tidak dapat di-`omit` atau di-`set`); `lockWhen` hanya boleh diperketat (Requirement 13.1). Untuk field lain, pelonggaran hanya dengan alasan tertulis eksplisit yang tercatat di keluaran `metadata:check`.
7. THE timpaan SHALL bergaya tambal dan deklaratif; mekanisme parameter atau logika tidak termasuk (YAGNI; lihat design D-5).
8. THE **tipe domain** (`kind: fieldType`) SHALL mendefinisikan satu field yang dipakai ulang: tipe dasar, batasan, `classification` bawaan, format tampilan, petunjuk editor (kunci widget dan masker), operator filter yang sah, dan kunci label bawaan; resource memakainya lewat `type: <nama tipe domain>` dan boleh menimpa lewat `set`.
9. THE tipe domain bawaan SDK (`sdk:`) SHALL mencakup minimal `email`, `phone`, `percentage`, dan `money`; tipe domain dan blok modul lain SHALL hanya dapat dipakai lewat dependensi manifest dan `export: true` (butir 5).
10. THE **blok** (metadata abstrak berisi beberapa field, dengan tata letak: urutan, seksi, kolom) SHALL dapat disertakan lebih dari sekali dalam satu resource dengan `prefix` (mis. `billing`, `shipping`; digabung secara camelCase menjadi `billingStreet`) sehingga nama field, kolom database, dan properti kontrak tidak bentrok; hasil penambahan prefix SHALL deterministik dan lolos aturan identifier (Requirement 1.4).
11. THE timpaan atas field dari tipe domain atau blok SHALL tunduk pada butir 6 (tidak menurunkan klasifikasi, grup, atau `readOnly`); klasifikasi tipe domain menular ke setiap pemakainya (mis. tipe `npwp` berklasifikasi `highly_confidential` di semua resource pemakai).
12. THE perubahan pada tipe domain atau blok bersama SHALL diperlakukan sebagai perubahan atas **setiap** resource pemakainya: `metadata:check`, `contracts:breaking`, dan mesin selisih SHALL melaporkan dampaknya per resource; perubahan merusak mengikuti Requirement 11.3 dan Requirement 12; tipe domain atau blok yang diekspor lintas modul SHALL mengikuti SemVer modul pemiliknya (perubahan merusak butuh versi MAJOR).
13. THE metadata TypeScript SHALL membawa kunci widget dan format dari tipe domain; modul SHALL dapat mendaftarkan widget kustom di registri tipe field (Requirement 18.1) tanpa mengubah `@erpora/ui`.
14. THE tipe domain dan blok SHALL tidak menambah jalur eksekusi saat runtime: keduanya diselesaikan saat pembangkitan menjadi metadata datar (design D-4).

### Requirement 4: Model kondisi

**User Story:** Sebagai developer modul, saya ingin menyatakan kapan field tampil, wajib, hanya-baca, atau terkunci dengan satu model, sehingga server dan klien menafsirkannya sama.

#### Acceptance Criteria

1. THE kondisi SHALL berbentuk pohon `all` (AND), `any` (OR), `not`, dan daun `{field, op, value}` atau `{field, op, ref}`; operator: `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`, `contains`, `isnull`, selaras operator api-guidelines §5.1.
2. THE `ref` SHALL merujuk field lain pada resource yang sama; `metadata:check` SHALL menolak rujukan ke field yang tidak ada dan **siklus ketergantungan** antarkondisi.
3. THE kondisi SHALL dipakai untuk: `visibleWhen`, `requiredWhen`, `readonlyWhen`, `lockWhen` (resource), validasi lintas field (`rules`), dan filter pada field relasi (Requirement 6.7); tidak ada mekanisme logika lain di berkas metadata.
4. THE evaluator SHALL ada di Go (`sdk/metadata`) dan di TypeScript (`@erpora/platform-web`), keduanya SHALL menghasilkan hasil identik untuk korpus vektor bersama yang mencakup tipe desimal, `null`, perbandingan teks, dan nilai tepi; CI SHALL menjalankan korpus yang sama di kedua sisi (property test dua sisi).
5. THE evaluasi SHALL tidak melempar panik atau galat untuk input apa pun (tipe tak cocok mengembalikan `false` dengan alasan terukur) dan SHALL berbiaya linear terhadap jumlah node.
6. THE aturan yang merujuk field berklasifikasi `confidential` ke atas, atau field di luar grup `default`, SHALL otomatis bertanda `scope: server` (tidak disertakan di metadata frontend; lihat Requirement 18.3 dan design D-8); aturan lain default `scope: both`.
7. THE validasi sisi server SHALL selalu menegakkan semua aturan `required`, `rules`, dan penguncian, terlepas dari apa yang dievaluasi klien; evaluasi klien hanya untuk kenyamanan.

### Requirement 5: Accessor dan templat label

**User Story:** Sebagai developer modul, saya ingin field turunan dan label ringkas dideklarasikan di metadata, sehingga tampilan relasi dan daftar konsisten.

#### Acceptance Criteria

1. THE accessor SHALL dinyatakan jenisnya: `expr`, `stored`, atau `func`; field accessor SHALL selalu `readOnly` dan tidak pernah diikat dari input.
2. THE ekspresi (`expr` dan `stored`) SHALL berasal dari tata bahasa tertutup: gabung teks, `coalesce`, aritmetika desimal dengan **mode pembulatan dan skala eksplisit**, dan `if` memakai kondisi (Requirement 4); tanpa pemanggilan fungsi bebas, tanpa SQL mentah.
3. THE `stored` SHALL dibangkitkan menjadi kolom turunan database dari ekspresi yang diterjemahkan oleh penerjemah beranotasi (hanya fungsi database immutable dari daftar putih); hanya `stored` yang boleh `sortable`, `filterable`, atau `groupable`.
4. THE `func` SHALL menunjuk fungsi Go bernama yang didaftarkan modul (kait, Requirement 16.5); accessor `func` SHALL hanya tampil dan metadata SHALL menolak penandaan `sortable`/`filterable`/`groupable` padanya.
5. THE templat label (`labelTemplate`) SHALL memakai placeholder `{field}` hanya untuk field milik resource itu (termasuk accessor); teks tetap di antaranya terbatas pada tanda baca dan spasi, sedangkan kata apa pun SHALL berasal dari kunci i18n dengan argumen bernama sesuai field (aturan i18n proyek).
6. THE label SHALL dirender sebagai **teks**, bukan HTML; setiap field yang menjadi bagian templat label SHALL berada di grup `default` dan berklasifikasi paling tinggi `internal`, tanpa pengecualian (label dikembalikan kepada siapa pun yang boleh membaca record; Requirement 17).
7. THE templat label dan accessor `expr` SHALL memakai evaluator yang sama dengan Requirement 4.4 dan lulus korpus vektor paritas.
8. THE accessor SHALL mewarisi klasifikasi tertinggi dan grup field paling ketat dari semua field masukannya (mis. `marginPct` turunan `costPrice` tidak boleh lebih longgar daripada `costPrice`); accessor `func` SHALL menyatakan grup dan klasifikasi secara eksplisit.

### Requirement 6: Relasi

**User Story:** Sebagai developer modul, saya ingin menyatakan relasi antar resource dengan kosakata yang lazim, sehingga kolom, form, dan kontrak turunannya konsisten dan aman.

#### Acceptance Criteria

1. THE relasi SHALL mendukung `belongsTo`, `hasOne`, `hasMany`, `belongsToMany` (tabel pivot bangkitan, boleh punya atribut), dan `morphTo`/`morphMany`/`morphToMany`, dengan opsi `onDelete` (`restrict` default, `cascade`, `setNull`) dan penanda `required`.
2. THE relasi **satu modul** SHALL dibangkitkan sebagai foreign key database; relasi **lintas modul** SHALL **tidak** pernah menjadi foreign key atau JOIN: hanya kolom ID, dengan data diambil lewat `modules/<x>/contract` (D1, D8) dan hanya ke modul yang tercantum di `dependencies` manifest dan `require` go.mod.
3. THE `metadata:check` SHALL menolak relasi lintas modul ke modul di luar dependensi manifest, ke resource yang tidak ada, dan ke resource yang tidak menyediakan kontrak relasi (Requirement 17).
4. THE `morphTo` SHALL memakai **daftar putih** tipe target (kunci resource yang dideklarasikan eksplisit); kolom tipe menyimpan kunci resource, bukan nama tabel atau kelas, nilai di luar daftar putih ditolak, dan tidak ada foreign key database (integritas ditegakkan aplikasi; Requirement 15.6).
5. THE relasi `hasMany` yang ditandai `embedded` (baris dokumen) SHALL diterima dan diubah sebagai satu kesatuan bersama induknya (api-guidelines §11); id baris di body SHALL dibuktikan milik induk itu (Requirement 15.5).
6. THE label relasi pada tampilan SHALL berasal dari templat label resource seberang (Requirement 5.5), diambil lewat eager load (`expand`, api-guidelines §5.3, maksimal satu tingkat) tanpa N+1 (satu panggilan kontrak per modul per halaman data).
7. THE field relasi SHALL dapat menyatakan `filter` (kondisi, Requirement 4) yang membatasi record yang boleh dipilih, termasuk rujukan ke field lain di form (`ref`); `metadata:check` SHALL mendeteksi siklus rujukan dan SHALL menolak filter pada field seberang yang tidak `filterable`.
8. THE filter relasi di form SHALL diperlakukan sebagai kenyamanan klien: server SHALL memvalidasi ulang saat simpan bahwa record terpilih memenuhi filter dan boleh diakses (Requirement 15.2).
9. THE tampilan rujukan yang tidak boleh dibaca atau sudah dihapus SHALL jatuh ke label placeholder yang diterjemahkan ("tidak tersedia"), tidak menggagalkan seluruh daftar, dan tidak membedakan "tidak ada" dari "tidak boleh dilihat".
10. THE relasi **satu modul** SHALL tetap menegakkan otorisasi resource seberang: `expand`, urut atau filter lewat relasi, dan hitungan atau agregat anak (`hasMany`) SHALL menerapkan `authz.Filter(ctx, <resource seberang>, "read")` dan `authz.Fields` milik resource seberang; JOIN tidak pernah melewati scope atau grup field resource seberang.
11. THE hanya relasi `embedded` yang boleh ditulis lewat induk; `hasOne` dan `hasMany` non-embedded SHALL hanya diubah lewat endpoint resource-nya sendiri. Baris `embedded` SHALL diperlakukan sebagai resource sendiri dengan grup field sendiri: `authz.CheckWrite` dan `authz.MaskFields` diterapkan **per baris** menurut resource baris.

### Requirement 7: Integrasi manifest, permission, field group, dan klasifikasi

**User Story:** Sebagai pemilik produk, saya ingin metadata tidak membuat sumber kebenaran kedua untuk izin dan field group, sehingga tidak ada pendaftaran ganda yang menyimpang.

#### Acceptance Criteria

1. THE `resource.key` di metadata SHALL merujuk resource yang dideklarasikan di manifest (`resources[].key`, PRD-001 FR-110); sebelum PRD-001 diimplementasikan, SHALL cocok dengan prefiks capability yang ada (`<key>.<aksi>`).
2. THE keanggotaan field dalam field group SHALL dinyatakan di metadata (`fieldGroup` per field); definisi grup (kunci, label, klasifikasi, grup `default`) tetap di manifest (PRD-001 FR-110). `module:check` SHALL gagal bila field menyebut grup yang tidak ada di manifest atau manifest memuat daftar `fields` yang menyimpang dari metadata (P-1, diputuskan 1 Nov 2026: metadata menjadi sumber keanggotaan, `fields` di manifest opsional dan diperiksa kesamaannya, perubahan aditif atas PRD-001 FR-110). Sebelum manifest modul memuat `resources`, T1 SHALL tidak mewajibkan `resources`/`field_groups` di manifest: grup field divalidasi terhadap definisi di metadata sendiri (grup `default` implisit), dan pemeriksaan kesamaan dengan manifest aktif begitu manifest memuat `resources`.
3. THE klasifikasi data properti kontrak bangkitan SHALL berasal dari metadata, sehingga linter kontrak yang ada (`erp-sensitive-needs-classification`, `erp-no-highly-confidential-in-response`, dan sejenisnya) tetap menegakkan aturannya terhadap hasil bangkitan.
4. THE permission operasi bangkitan SHALL mengikuti `<resource>.<aksi>` dan SHALL terdaftar di manifest; `metadata:check` SHALL menolak resource tanpa aksi yang dibutuhkan operasi yang dibangkitkan.
5. THE `owner_field` dan `scope_fields` (PRD-001 FR-110/FR-111) SHALL dirujuk dari metadata ke field yang ada dan bertipe sesuai; rujukan rusak ditolak saat `metadata:check`.

### Requirement 8: Pembangkitan kontrak dan kode (T1)

**User Story:** Sebagai developer modul, saya ingin kontrak dan kode turunan dibangkitkan dan dijaga sinkron, sehingga kolom yang terlewat menjadi kegagalan build, bukan temuan review.

#### Acceptance Criteria

1. THE `metadata:generate [modul]` SHALL membangkitkan, per resource: skema OpenAPI (`components.schemas`: baca, buat, ubah) dengan batasan JSON Schema dan ekstensi `x-erp-*`; DTO Go beserta decoder ketat dan validator; metadata TypeScript `modules/<dir>/web/src/metadata/<nama>.meta.gen.ts`; dan konstanta serta aturan Go pada `internal/domain`.
2. THE hasil SHALL deterministik (urutan stabil, tanpa cap waktu), berkepala `Code generated by erpora metadata:generate. DO NOT EDIT.`, dan SHALL **tidak** diedit tangan; `metadata:generate --check` SHALL gagal bila ada selisih (pola yang sama dengan `contracts:generate --check` dan `i18n:generate --check`).
3. THE pembaruan `openapi.yaml` SHALL memakai suntingan terstruktur pada posisi node YAML dan menandai skema bangkitan (`x-erp-generated: true`); skema yang ditandai tidak boleh dimodifikasi di luar generator, dan `contracts:lint` serta `contracts:generate --check` tetap lulus.
4. THE kode bangkitan SHALL tunduk pada aturan arsitektur (D1 sampai D11, D3, D3b): domain tanpa I/O, GORM hanya di `infrastructure`; tes `archcheck` tidak diberi pengecualian baru untuk kode bangkitan.
5. THE modul tanpa `web/` SHALL tidak menghasilkan keluaran TypeScript; modul tanpa metadata SHALL tidak terpengaruh.
6. THE `metadata:generate` SHALL gagal tanpa menulis apa pun bila `metadata:check` gagal, atau bila berkas keluaran punya penanda `Code generated` yang hilang (berkas dimodifikasi tangan).

### Requirement 9: Validasi server yang otoritatif

**User Story:** Sebagai pemilik produk, saya ingin server menjadi satu-satunya otoritas atas validitas data, sehingga klien yang dimanipulasi tidak dapat menyimpan data tidak sah.

#### Acceptance Criteria

1. THE decoder bangkitan SHALL menolak body dengan field tak dikenal dan field `readOnly` (`422 validation_failed` dengan detail per field), sehingga mass assignment tertutup secara bawaan (security-guidelines §3.1).
2. THE validator SHALL menegakkan tipe, wajib, `nullable`, panjang, rentang, pola, enum, dan semua `rules` kondisi; pelanggaran struktur menghasilkan `422 validation_failed` dan pelanggaran aturan bisnis `422 business_rule_violated` (api-guidelines §7.3).
3. THE perubahan field yang tidak `write` bagi principal SHALL menghasilkan `403 field_forbidden` beserta daftar field (api-guidelines §7.3; PRD-001 FR-112), tidak pernah diabaikan diam-diam.
4. THE aturan `readonlyWhen` dan `lockWhen` SHALL ditegakkan di server; `PATCH` pada dokumen terkunci SHALL menghasilkan `409 invalid_state`.
5. THE batasan SHALL tidak pernah **ketat lebih** di klien daripada di server maupun kontrak (paritas); pengetatan batasan pada field yang sudah ada mengikuti api-guidelines §14.2.
6. THE galat validasi SHALL tidak membocorkan nilai field `confidential` ke atas pada pesan atau detail.
7. THE panjang input, kedalaman JSON, dan ukuran body SHALL dibatasi (`MaxBodyBytes` rute, batas kedalaman decoder) untuk mencegah penolakan layanan.
8. THE aturan yang merujuk field di luar grup `default` SHALL memicu peringatan `metadata:check` (galat validasi dapat menjadi oracle nilai field itu lewat percobaan berulang); risiko dicatat sebagai risiko yang diterima dengan rate limit pada endpoint tulis.

### Requirement 10: Konsistensi database dan mesin selisih (T2)

**User Story:** Sebagai developer modul, saya ingin skema database diperiksa terhadap metadata, sehingga kolom di database tidak menyimpang dari yang dijanjikan metadata.

#### Acceptance Criteria

1. THE `metadata:diff [modul]` SHALL menjalankan semua migrasi goose modul (dan dependensinya) pada database sementara (infrastruktur `pgtest` atau `ERP_TEST_DATABASE_URL`), membaca skemanya, dan membandingkannya dengan skema turunan metadata: tabel, kolom, tipe, nullability, default, CHECK (panjang, rentang, enum), unique, indeks, foreign key relasi satu modul, kolom turunan, dan pemicu. Indeks atau unique di database yang tidak dideklarasikan dihitung sebagai selisih (ketat); indeks yang sudah ada pada migrasi tulis tangan harus dideklarasikan saat modul pindah ke metadata.
2. THE mesin selisih SHALL sama untuk pemeriksa dan pembangkit: selisih kosong berarti lulus (gerbang CI), selisih tidak kosong dapat dituliskan sebagai migrasi (Requirement 11).
3. THE tes selisih kosong SHALL berjalan di CI untuk `example` (setelah dimigrasikan ke metadata) dan untuk modul fixture `testdata` (Requirement 20.5), dan SHALL gagal bila metadata dan migrasi menyimpang; `exampleaddon` tidak punya tabel sehingga tidak diperiksa.
4. THE pemetaan tipe SHALL tertulis dan diuji: `string(maxLength)` ke `varchar`/`text` dengan CHECK `length`, `decimal(p,s)` ke `numeric(p,s)`, `datetime` ke `timestamptz`, `date` ke `date`, `uuid` ke `uuid`, `enum` ke `text` dengan CHECK nilai (bukan tipe enum PostgreSQL, agar penambahan nilai aditif), `boolean` ke `boolean`; field `highly_confidential` ke `bytea` (ciphertext AES-256-GCM yang memuat versi kunci) ditambah kolom `<kolom>_idx bytea` (indeks buta) hanya bila `filterable` atau `unique` (Requirement 2.12).
5. THE tabel dan kolom SHALL **tanpa** kolom `tenant_id` (ADR-0004); `id` bertipe `uuid` kunci utama; field bawaan (`version`, `created_at`, `updated_at`, `created_by`, `updated_by`) berasal dari metadata abstrak bawaan.
6. THE nama tabel SHALL berprefiks slug modul dan berupa hasil konversi snake_case jamak (`<slug>_<nama>`; Requirement 1.8), sesuai konvensi penamaan generator.
7. THE mesin selisih SHALL hanya berjalan di lingkungan pengembang dan CI, tidak pernah di binary rilis atau terhadap database produksi.
8. THE migrasi modul pada database sementara SHALL dijalankan dengan role non-superuser pemilik database sementara itu (bukan DSN admin), sehingga migrasi tidak dapat memakai fasilitas superuser (mis. `COPY … PROGRAM`).

### Requirement 11: Pembangkit migrasi (T2)

**User Story:** Sebagai developer modul, saya ingin selisih metadata dituliskan menjadi migrasi goose yang aman dan dapat ditinjau, sehingga menambah kolom tidak butuh menulis SQL tangan.

#### Acceptance Criteria

1. THE `metadata:migrate <modul> <nama>` SHALL menulis berkas `migrations/NNNN_<nama>.sql` bernomor berikutnya (konvensi `make:migration`) berisi `Up` dan `Down`, dan SHALL **tidak** pernah menerapkannya.
2. THE perubahan **aman (expand)** SHALL dibangkitkan otomatis: tabel baru, kolom baru yang `nullable` atau berdefault, indeks baru, pelonggaran batasan, dan penambahan nilai enum `extensible`.
3. THE perubahan **perlu keputusan** (mengganti nama, mengubah tipe, memperkecil panjang atau rentang, `NOT NULL` pada kolom yang sudah ada, unique pada data yang ada, menghapus apa pun) SHALL hanya menghasilkan **kerangka** bertanda `TODO` yang menggagalkan `metadata:check` sampai dilengkapi dan ditinjau manusia, sesuai pola expand, migrate, contract (api-guidelines §14.3; ADR-0010).
4. THE backfill pada tabel yang menurut metadata diberi `large: true` SHALL dibangkitkan bertahap per batch dengan `NO TRANSACTION` (contoh dan peringatan pada migrasi `0002` modul `example`).
5. THE identifier dalam SQL bangkitan SHALL selalu di-quote dan hanya berasal dari nilai yang lolos Requirement 1.4; ekspresi `stored` dan CHECK SHALL diterjemahkan oleh penerjemah beranotasi dari tata bahasa tertutup, tidak menyalin teks dari berkas metadata (pencegahan injeksi SQL pada pembangkitan).
6. THE migrasi bangkitan SHALL lolos `module:check`, dan `make:migration` SHALL tetap tersedia untuk migrasi tulis tangan (data, backfill kompleks).
7. THE `Down` SHALL dibangkitkan hanya bila aman dan tidak menghilangkan data; migrasi yang tidak dapat dibalik SHALL ditandai eksplisit.

### Requirement 12: Aturan deprecated dan rename

**User Story:** Sebagai pemilik produk, saya ingin penggantian dan penghapusan kolom selalu melewati masa transisi, sehingga klien dan data lama tidak rusak.

#### Acceptance Criteria

1. THE field yang akan diganti atau dihapus SHALL ditandai `deprecated: {since, replacedBy?}` lebih dulu; menghapus field dari metadata tanpa tanda `deprecated` pada rilis sebelumnya SHALL ditolak `metadata:check` (pembanding: metadata pada tag rilis terakhir modul, seperti `contracts:breaking`; modul tanpa tag dikecualikan).
2. THE rename SHALL berupa field baru dengan `migrateFrom: <field lama>`; pembangkit SHALL menghasilkan kolom baru dan salinan data dari kolom lama pada migrasi (batch bila `large: true`), tanpa menghapus kolom lama.
3. THE selama masa transisi, DTO bangkitan SHALL menerima dan mengirim **kedua** bentuk (expand; api-guidelines §14.3 langkah 1), menandai bentuk lama `deprecated: true` di kontrak, dan server mengisi keduanya saat menulis.
4. THE kolom lama SHALL hanya boleh dihapus (tahap contract) bila syarat api-guidelines §14.3 langkah 3 terpenuhi (tidak dipakai 6 bulan, rilis MAJOR, `Sunset` diumumkan); `metadata:migrate` SHALL hanya menyiapkan kerangka penghapusan dan menolak membuatnya sebelum syarat dinyatakan terpenuhi di metadata (`removeAfter`).
5. THE `contracts:breaking` dan mesin selisih SHALL tetap menjadi pengaman lapis kedua terhadap penghapusan tak sengaja.
6. THE pasangan `migrateFrom` SHALL memiliki `fieldGroup` dan `classification` identik; `metadata:check` menolak pasangan yang berbeda. `authz.CheckWrite` dan `MaskFields` dihitung atas field kanonik (yang baru), dan permintaan yang membawa kedua bentuk dengan nilai berbeda SHALL ditolak `422 validation_failed`.

### Requirement 13: Dokumen transaksional: penguncian

**User Story:** Sebagai pemilik produk, saya ingin dokumen yang sudah terbit tidak dapat berubah, sehingga catatan transaksi tetap sah dan teraudit.

#### Acceptance Criteria

1. THE metadata abstrak bawaan `document` SHALL menyediakan `status` (`draft`, `submitted`, `cancelled`, ...), `amendedFromId`, dan `lockWhen` bawaan (`status` bukan `draft`); resource transaksional menyertakannya, boleh menambah status, dan boleh **memperketat** kondisi kunci (kondisi turunan digabung dengan OR terhadap kondisi bawaan, tidak menggantikannya); `omit` atau `set` atas field bawaan `sdk:document` ditolak.
2. THE dokumen terkunci SHALL ditegakkan **tiga lapis**: metadata menyatakan kapan terkunci; antarmuka dan server menjadikan semua field hanya-baca (`409 invalid_state`); database menolak `UPDATE` dan `DELETE` lewat pemicu yang dibangkitkan dari metadata abstrak `document` (transaction-data-standards §15 butir 2; security-guidelines §1 pertahanan berlapis).
3. THE koreksi dokumen terkunci SHALL hanya lewat alur `cancel` dan `amend` yang menghasilkan dokumen **baru** (api-guidelines §11); tidak ada jalur ubah langsung.
4. THE transisi status SHALL mengikuti api-guidelines §11 dan hanya transisi yang dinyatakan metadata yang sah; transisi tidak sah SHALL menghasilkan `409 invalid_state`.
5. THE tes immutabilitas (`UPDATE`/`DELETE` langsung ke dokumen terkunci ditolak database) SHALL ada untuk resource dokumen pada modul fixture.
6. THE pemicu SHALL dikenali mesin selisih (Requirement 10.1) sehingga penghapusan pemicu tak sengaja terdeteksi.
7. THE perkara yang tetap per resource dan tidak dibangkitkan SHALL tertulis jelas di `design.md` (penomoran, ledger, maker-checker, periode akuntansi, rekonsiliasi), beserta pemetaan checklist transaction-data-standards §15.
8. THE pemicu SHALL dievaluasi terhadap keadaan `OLD` dan mengizinkan `UPDATE` baris terkunci **hanya** bila (a) hanya kolom `status`, `version`, `updated_at`, dan `updated_by` yang berubah **dan** pasangan (status lama, status baru) ada di tabel transisi yang dibangkitkan dari metadata, atau (b) hanya kolom tujuan `migrateFrom` yang diisi, dari `NULL` menjadi nilai yang sama persis dengan kolom asalnya; `DELETE` baris terkunci SHALL selalu ditolak.
9. THE tidak ada mekanisme pengecualian lain (penanda sesi, `session_replication_role`, atau menonaktifkan pemicu) yang dapat dipakai oleh jalur aplikasi, karena deployment memakai satu role database (`docs/operations/deployment.md`) sehingga setiap pengecualian yang tersedia bagi migrasi juga tersedia bagi kode aplikasi.
10. THE migrasi tulis tangan yang harus mengubah baris terkunci di luar butir 8 SHALL menonaktifkan dan mengaktifkan kembali pemicu secara eksplisit di dalam migrasi itu (ditinjau manusia), dan `metadata:diff` SHALL memastikan pemicu aktif setelah semua migrasi selesai.
11. THE baris anak dokumen (`embedded`, tabel pivot `belongsToMany`, dan `hasOne`) SHALL dilindungi pemicu anak yang menolak `INSERT`, `UPDATE`, dan `DELETE` bila baris induknya terkunci.
12. THE `lockWhen` yang tidak dapat diterjemahkan ke SQL SHALL menggagalkan `metadata:check` untuk resource dokumen; tidak boleh turun diam-diam ke lapis server saja.
13. THE kode CRUD bangkitan SHALL memuat induk dengan `SELECT … FOR UPDATE` di dalam transaksi untuk `PATCH`, `DELETE`, aksi, dan `submit` (transaction-data-standards §2), sehingga tulis bersamaan dengan transisi ke terkunci tidak dapat lolos.

### Requirement 14: Field salinan (snapshot)

**User Story:** Sebagai pemilik produk, saya ingin dokumen menyimpan salinan data master saat terbit, sehingga perubahan master tidak mengubah dokumen yang sudah sah.

#### Acceptance Criteria

1. THE relasi `belongsTo` SHALL dapat menyatakan `snapshot` (daftar `from` field seberang ke `to` field lokal); field lokal itu SHALL `readOnly` bagi klien dan dibangkitkan sebagai kolom biasa.
2. THE salinan SHALL diisi saat relasi diatur atau diubah selama dokumen belum terkunci, dan SHALL diisi ulang dalam transaksi yang sama dengan transisi ke status terkunci (`submit`), sehingga dokumen yang terbit memuat nilai master pada saat terbit (transaction-data-standards §7).
3. THE setelah terkunci, salinan SHALL tidak pernah disegarkan lagi, dalam kondisi apa pun, termasuk perubahan master dan migrasi data.
4. THE penyalinan SHALL berjalan dengan **principal pemanggil**, bukan hak sistem: penyalinan SHALL **gagal** `422 business_rule_violated` (`code: snapshot_forbidden`) bila principal tidak boleh membaca **salah satu** field sumber yang dideklarasikan (tidak ada nilai basi atau sebagian, sehingga isi dokumen tidak bergantung pada siapa yang menyimpan atau menerbitkan); penyalinan dengan hak sistem hanya lewat `authz.AsSystem(ctx, reason)` yang dinyatakan eksplisit di metadata dengan alasan, tercatat di audit (PRD-001 FR-119; P-4, diputuskan 1 Nov 2026).
5. THE field tujuan salinan SHALL menyatakan `fieldGroup` dan `classification` secara eksplisit (tanpa nilai bawaan) dan SHALL tidak lebih longgar daripada field sumbernya; field sumber lintas modul SHALL dideklarasikan `snapshotSource: true` beserta grup dan klasifikasinya oleh modul pemilik, dan hanya field itu yang dilayani `ReadSnapshotFields`; `metadata:check` menolak salinan dari grup non-`default` sumber ke grup `default` tujuan tanpa alasan tertulis.
6. THE salinan SHALL tidak dipakai untuk mengurutkan atau memfilter relasi lintas modul di spec ini (ditunda, K9); kolom salinan boleh `filterable`/`sortable` sebagai kolom lokal biasa hanya bila dinyatakan eksplisit pada field lokalnya.
7. THE catatan cakupan: sinkronisasi master ke master dan penyegaran lintas modul lewat event di luar spec ini.

### Requirement 15: Penegakan otorisasi dan pencegahan IDOR pada setiap integrasi

**User Story:** Sebagai pemilik produk, saya ingin setiap titik masuk yang menerima ID dilindungi secara bawaan, sehingga tidak ada pengguna yang dapat membaca atau mengubah data di luar haknya.

#### Acceptance Criteria

1. THE setiap kode bangkitan yang memuat record SHALL memuatnya lewat `authz.Check`/`authz.Filter` (PRD-001 FR-115); `db.First(&x, id)` tanpa filter otorisasi SHALL dilarang dan SHALL gagal di `archcheck`/analyzer `go vet` kustom (PRD-001 FR-115 dan FR-119 butir 3).
2. THE ID referensi di body (field relasi, `morphTo`, id baris `hasMany`, id pivot `belongsToMany`) SHALL dibuktikan: principal boleh `read` record yang dirujuk **dan** record berada dalam scope-nya; bila tidak, `422 validation_failed` dengan `code: not_found` pada field itu, tanpa membedakan "tidak ada" dari "tidak boleh dilihat" (PRD-001 FR-119 butir 1).
3. THE filter relasi yang dinyatakan metadata SHALL divalidasi ulang di server saat simpan terhadap record terpilih (Requirement 6.8), dan SHALL tidak bergantung pada apa yang dikirim klien.
4. THE record di luar scope pada endpoint ber-ID SHALL menghasilkan `404 not_found` (api-guidelines §8).
5. THE id baris (`embedded hasMany`) SHALL dibuktikan milik induk dalam permintaan yang sama; id baris milik induk lain SHALL ditolak (`422`, `code: not_found`) dan tidak mengubah baris itu.
6. THE `morphTo`/`morphMany`/`morphToMany` SHALL memeriksa otorisasi terhadap **tipe dan record target yang sebenarnya**, memakai kontrak modul pemilik tipe itu; tipe di luar daftar putih ditolak; dan karena tidak ada foreign key, keberadaan target SHALL diperiksa aplikasi pada setiap tulis dan SHALL ada pemeriksa piatu (orphan checker) yang dapat dijalankan operator.
7. THE parameter kontrak dan DTO bangkitan SHALL tidak memuat `tenantId`, `userId`, atau penanda hak dari pemanggil; identitas hanya dari context (PRD-001 FR-119 butir 2).
8. THE penggunaan `authz.AsSystem` pada kode bangkitan SHALL dilarang kecuali dinyatakan eksplisit di metadata dengan alasan, hanya di `internal/application`, tercatat di audit, dan ditandai `// erp:system-call-allowed` pada kontrak yang menerimanya.
9. THE **setiap** endpoint dan metode kontrak bangkitan yang menerima ID SHALL disertai tes IDOR dengan dua fixture principal (A dan B) di scope berbeda yang menegaskan: B membaca, mengubah, menghapus, atau menjalankan aksi pada record A menghasilkan 404; B memakai ID record A sebagai referensi di body menghasilkan `422 not_found`; B memanggil kontrak dengan ID record A menghasilkan galat `not_found`; dan tes ini SHALL gagal bila pembatasan scope dihapus (bukan hijau karena stub).
10. THE resource tanpa tes IDOR yang dibangkitkan SHALL gagal `metadata:check` untuk tahap T4.
11. THE analyzer dan tes arsitektur yang ada (D1 sampai D11, FR-115) SHALL diperluas bila perlu agar kode bangkitan tidak dapat melewati helper otorisasi.
12. THE tes IDOR bangkitan SHALL berupa **integration test** (build tag `integration`, fixture principal A dan B lewat `testsupport`) dengan penanda `// erp:idor-test <resource>.<operasi>`; `metadata:check` SHALL memastikan penanda ada untuk setiap operasi pada matriks IDOR design. Ini bukan integration test repository (Q31 ★2): integration test repository tetap ditulis tangan.
13. THE `PATCH` pada resource ber-`scope_fields` atau `owner_field` SHALL memeriksa `authz.Check` terhadap keadaan **sebelum dan sesudah** perubahan; bila keadaan sesudah berada di luar scope tulis principal, hasilnya `422 validation_failed` dengan `code: not_found` pada field itu. Field bawaan, `owner_field`, `scope_fields`, `createdBy`, `status`, dan `amendedFromId` tidak dapat dilepas dari `readOnly` oleh timpaan mana pun (Requirement 3.6).
14. THE membaca anak `morphMany`/`morphToMany` SHALL mensyaratkan `authz.Check(read)` atas record induk target lewat kontrak modul pemilik tipe; daftar anak SHALL difilter per tipe dan ID target yang boleh dibaca.
15. THE urutan penegakan di handler bangkitan SHALL: (1) `authz.Check(read)` atas record (gagal: `404`), (2) `authz.Check(aksi)` (gagal: `403 permission_denied`), (3) `If-Match` (gagal: `412 version_mismatch`, dengan `error.details` melalui `MaskFields`), (4) validasi dan tulis; filter non-`filterable` SHALL menghasilkan `400 invalid_filter` dan field `none` bagi principal `403 field_forbidden` (api-guidelines §5.1, §8).

### Requirement 16: CRUD bangkitan dan titik perluasan (T4)

**User Story:** Sebagai developer modul, saya ingin resource sederhana mendapat CRUD otomatis dengan tetap bebas menambah logika, sehingga pengembangan lebih cepat tanpa mengunci fleksibilitas.

#### Acceptance Criteria

1. THE resource ber-penanda `generate.crud: true` SHALL mendapat entitas (`internal/domain`), repository dan model GORM (`internal/infrastructure`), use case standar (`internal/application`), handler dan DTO (`internal/presentation/http`), route, dan operasi OpenAPI yang selaras Requirement 7 dan 8.
2. THE handler bangkitan SHALL menegakkan seluruh daftar periksa api-guidelines §15: permission, `authz.Filter`/`Check`/`MaskFields`/`CheckWrite`, 404 di luar scope, `version` dan `ETag`, `If-Match`, `Idempotency-Key` pada `POST`, `409 invalid_state`, string desimal, dan waktu UTC.
3. THE daftar bangkitan SHALL membatasi `sort`, `filter`, dan `group` pada field yang ditandai metadata, menolak filter, urut, atau grup pada field `none` bagi principal dengan `403 field_forbidden` (api-guidelines §5.1; PRD-001 FR-112), menerapkan scope server **sebelum** filter klien, dan menghitung agregat desimal di server dengan aritmetika desimal. Pencarian teks bebas `q` SHALL hanya mencocokkan field yang boleh dibaca principal (dihitung per principal) dan meng-escape `%`, `_`, dan `\` pada pencocokan `ILIKE`.
4. THE kait SHALL tersedia sebagai antarmuka Go bertipe (sebelum/sesudah validasi, simpan, hapus, dan transisi) yang dipanggil kode bangkitan; resource maju SHALL dapat mengganti use case tertentu sambil tetap memakai metadata, DTO, dan validasi bangkitan.
5. THE accessor `func` dan aturan bisnis kompleks SHALL didaftarkan lewat kait; kode tulis tangan SHALL tidak berada di berkas bangkitan.
6. THE **jalur lepas**: menghapus penanda `generate.crud` SHALL menghentikan pembangkitan CRUD, dan `metadata:eject <resource>` SHALL menyalin kode bangkitan menjadi kode biasa (tanpa penanda `Code generated`) tanpa mengubah perilaku, sehingga tidak ada kunci mati.
7. THE kode bangkitan CRUD SHALL **tidak dihasilkan** sampai PRD-001 FR-110 sampai FR-115 dan FR-119 tersedia di `sdk/authz`; sebelum itu hanya tahap T1 sampai T3 yang aktif (P-2, diputuskan 1 Nov 2026).
8. THE integration test repository SHALL tetap ditulis lewat platform (`reference_modules_integration_test.go` sebagai acuan); generator tidak membuat integration test repository (pengecualian: tes IDOR, Requirement 15.12).
9. THE pelanggaran unique pada tulis SHALL menghasilkan `409 conflict` dengan pesan generik tanpa membocorkan record di luar scope; risiko oracle keberadaan lintas scope dicatat sebagai risiko yang diterima (opsi unik per dimensi scope ditunda).

### Requirement 17: Kontrak relasi lintas modul

**User Story:** Sebagai developer modul, saya ingin relasi lintas modul bekerja tanpa JOIN, sehingga batas modul tetap terjaga.

#### Acceptance Criteria

1. THE resource yang menjadi target relasi lintas modul SHALL menyediakan kontrak bangkitan di `modules/<x>/contract` dengan tiga operasi: **resolusi label** (kumpulan ID menghasilkan `{id, label}` dalam satu panggilan), **pencarian** (teks dan kondisi terbatas menghasilkan `{id, label}` berhalaman), dan **baca field salinan** (`ReadSnapshotFields`: untuk satu ID, mengembalikan hanya field yang dideklarasikan resource pemanggil sebagai sumber salinan dan boleh dibaca principal menurut grup field dan klasifikasi).
2. THE implementasi kontrak SHALL menegakkan otorisasi sendiri terhadap principal di context (`authz.Filter`/`Check`), tidak mempercayai pemanggil, dan SHALL tidak menerima `tenantId`/`userId` (PRD-001 FR-119 butir 2).
3. THE resolusi label SHALL mengabaikan ID di luar scope atau tidak boleh dibaca tanpa membedakan keduanya dari "tidak ada" pada hasil (hasil hanya memuat yang boleh dilihat).
4. THE pencarian SHALL hanya menerima kondisi pada field seberang yang `filterable` dan boleh dibaca principal (non-`filterable`: `400 invalid_filter`; field `none` bagi principal: `403 field_forbidden`); teks bebas `q` hanya mencocokkan field yang boleh dibaca principal; ukuran halaman dibatasi (≤ 200), panjang `q` ≤ 100 karakter, dan jumlah kondisi dibatasi (Requirement 1.3).
5. THE kontrak SHALL hanya menyertakan field yang diperlukan untuk label dan pemilihan; field `confidential` ke atas tidak pernah ikut kecuali disebut eksplisit. Pengecualian terbatas: `ReadSnapshotFields` hanya mengembalikan field sumber salinan yang dideklarasikan dan boleh dibaca principal; field yang tidak boleh dibaca tidak dikembalikan (Requirement 14.4).
6. THE endpoint HTTP untuk dropdown SHALL memakai konvensi yang ada (`fields=id,label`, `q=`, filter api-guidelines §5.1; `expand` untuk label di daftar) dan dokumentasi OpenAPI-nya dibangkitkan.
7. THE setiap metode kontrak bangkitan SHALL punya tes IDOR (Requirement 15.9) dan tes isolasi principal A vs B.
8. THE `ResolveLabels` SHALL menerima paling banyak 200 ID per panggilan dan `ReadSnapshotFields` hanya satu ID; pemeriksaan referensi untuk baris `embedded` SHALL dilakukan secara batch (bukan satu kueri per baris); jumlah baris `embedded` per dokumen dibatasi (bawaan 500); endpoint dan kontrak pencarian SHALL punya rate limit per principal (security-guidelines §3.10).

### Requirement 18: Paket metadata frontend (T3)

**User Story:** Sebagai developer frontend, saya ingin paket metadata bertipe yang dapat dikonsumsi M3c dan M3d, sehingga form, tabel, dan filter dirender dari satu sumber.

#### Acceptance Criteria

1. THE `@erpora/platform-web` SHALL mengekspor tipe metadata (resource, field, kondisi, relasi), evaluator kondisi, validator, dan **registri tipe field** yang memetakan tiap tipe ke daftar operator yang sah, editor nilai untuk form, dan editor nilai untuk filter.
2. THE `@erpora/ui` SHALL tetap daun (F4) dan tidak mengimpor paket ini; komponen perender generik dibuat di M3c dan M3d di atas keduanya.
3. THE metadata TypeScript SHALL memuat struktur setiap field (nama, tipe, batasan, grup, kunci label), termasuk field `highly_confidential`, agar form generik dapat merendernya bagi pengguna yang berhak; yang tidak disertakan hanya aturan `scope: server` dan nilai ambang yang merujuk field `highly_confidential` (Requirement 4.6); field tertentu dapat keluar sepenuhnya dengan `clientVisible: false` beserta alasan tertulis, dan halaman yang memakainya ditulis tangan; klien tidak pernah menyimpulkan akses grup field dari metadata, dan SHALL memperlakukan field yang tidak ada di respons sebagai "tidak tersedia", bukan "kosong" (api-guidelines §4.4).
4. THE metadata SHALL dimuat malas per resource (chunk terpisah) dan evaluator serta validator SHALL hemat bundle; entry awal SHALL tetap ≤ 200 KB gzip (anggaran NFR-04; saat ini sangat mepet).
5. THE teks UI dari metadata (label, pesan galat, bantuan) SHALL melalui i18n (`msg()`/kunci katalog); `dangerouslySetInnerHTML` tetap dilarang.
6. THE kontrak konsumsi M3c dan M3d SHALL didokumentasikan di `docs/module-sdk/metadata.md` (apa yang disediakan paket, titik perluasan, aturan evaluasi).
7. THE nilai `decimal` di klien SHALL tidak pernah dikonversi ke `Number` untuk perbandingan, penjumlahan, atau format; format hanya lewat utilitas desimal dan `Intl` dengan string.
8. THE chunk metadata disajikan sebagai aset SPA statis tanpa autentikasi; karena itu hanya memuat apa yang aman dibaca siapa pun (struktur dan batasan, tanpa aturan atau ambang `scope: server`), dan nama field serta label yang sensitif dapat dikecualikan dengan `clientVisible: false`.

### Requirement 19: Batas sumber daya, penyalahgunaan, dan pembangkitan aman

**User Story:** Sebagai pemilik produk, saya ingin sistem metadata tahan penyalahgunaan, sehingga input dan berkas yang berbahaya tidak menjatuhkan layanan atau perkakas.

#### Acceptance Criteria

1. THE batas Requirement 1.3 SHALL ditegakkan oleh skema dan oleh evaluator saat runtime (kedalaman, jumlah node, panjang, jumlah nilai).
2. THE regex `pattern` SHALL tidak menimbulkan ReDoS: di server memakai RE2 (waktu linear); di klien dibatasi subset dan panjang (Requirement 2.5).
3. THE pembangkit SHALL menolak berkas keluaran yang menunjuk di luar direktori modul (path traversal), tidak mengikuti tautan simbolik, dan menulis hanya ke direktori yang diizinkan.
4. THE metadata dari modul selain trust `builtin` (ADR-0006, ADR-0008) SHALL diperlakukan tidak tepercaya: tidak dibangkitkan otomatis di lingkungan produksi dan perubahannya masuk tinjauan. Selama semua modul berupa kode Go yang dikompilasi bersama, kontrol yang nyata adalah `metadata:generate --check`, analyzer authz, dan tinjauan; larangan `AsSystem` atau kait `func` pada modul tak tepercaya baru efektif bersama sandbox ADR-0008.
5. THE galat pembangkit dan pemeriksa SHALL tidak membocorkan isi berkas di luar modul.
6. THE agregat dan grup di server SHALL dibatasi kedalaman (≤ 4) dan jumlah kelompok per respons.
7. THE semua keluaran bangkitan SHALL memakai kutipan identifier dan nilai terparameter; tidak ada penyusunan SQL dengan penyambungan teks dari input klien.
8. THE literal pada keluaran SQL SHALL melewati satu fungsi escape literal yang diuji (DDL tidak dapat diparameterisasi); badan fungsi pemicu dibangkitkan dari templat tetap tanpa menyisipkan teks berkas; nilai `enum`, `default`, dan batas CHECK hanya dari nilai yang lolos Requirement 2.11.
9. THE literal pada keluaran Go SHALL hanya lewat `strconv.Quote`, dan pada keluaran TypeScript hanya lewat `JSON.stringify` (regex dibangun dengan `new RegExp` dari string JSON); `description` dan pesan tidak ditulis sebagai komentar mentah atau kode.
10. THE tes golden dengan masukan berbahaya (kutip, `$$`, `*/`, backtick, baris baru, nilai `enum` jahat) SHALL ada untuk ketiga keluaran (SQL, Go, TS) dan membuktikan bahwa keluaran tidak menjalankan teks masukan.

### Requirement 20: Perkakas, CI, dan Windows

**User Story:** Sebagai developer, saya ingin perkakas metadata ikut gerbang yang ada, sehingga tidak ada langkah manual yang terlupa.

#### Acceptance Criteria

1. THE perintah SHALL bernama: `metadata:check`, `metadata:generate [--check]`, `metadata:diff`, `metadata:migrate`, `metadata:eject`, dijalankan lewat `task erpora -- <command>` (flavor dev; tidak ada di binary rilis).
2. THE `metadata:check` dan `metadata:generate --check` SHALL masuk `task lint arch check:gowork-off test` dan `task web:check`; `metadata:diff` SHALL berjalan di job integrasi CI.
3. THE perintah Taskfile SHALL tidak memakai utilitas khusus Unix (`rm`, `cp`, `sed`) dan SHALL berjalan di Windows.
4. THE templat generator yang berubah SHALL diverifikasi `task test:generator`; `make:module` SHALL dapat menghasilkan modul dengan kerangka metadata bila diminta.
5. THE modul `example` SHALL dimigrasikan ke metadata tanpa perubahan perilaku API yang teramati (`contracts:breaking` bersih; field bawaan yang belum ada di kontrak sekarang, seperti `createdBy` dan `updatedBy`, dilepas dengan `omit`); THE harus ada modul **fixture** di `testdata` (tidak didaftarkan ke aplikasi) yang memuat dokumen, relasi satu modul dan lintas modul, `decimal`, `enum`, baris `embedded`, `morph`, tipe domain, dan blok sebagai objek uji exit gate T1 sampai T4; `exampleaddon` tidak punya tabel dan tidak memerlukan metadata.

### Requirement 21: Dokumentasi dan pencatatan keputusan

**User Story:** Sebagai reviewer, saya ingin keputusan tercatat, sehingga perilaku yang disengaja tidak muncul kembali sebagai temuan.

#### Acceptance Criteria

1. THE PRD-000 SHALL mencatat Q-number baru (Q41) berisi urutan blok, keberadaan blok metadata, ketergantungan M3b pada seluruh M3d, dan ruang lingkup spec ini; tabel §11 SHALL diperbarui.
2. THE ADR baru SHALL dibuat untuk keputusan arsitektur "metadata sebagai sumber kebenaran yang membangkitkan kontrak dan migrasi" (memperbarui ADR-0003 dan ADR-0010 tanpa membatalkan larangan `AutoMigrate`).
3. THE `docs/module-sdk/metadata.md` (panduan), `generators.md`, `contracts.md`, dan `frontend.md` SHALL diperbarui; aturan baru pada `CLAUDE.md` SHALL disalin identik ke `AGENTS.md` (`TestAgentsMDMatchesClaudeMD`).
4. THE catatan kesalahan koreksi: standar tabel "kolom `tenant_id`" yang sempat disebut saat brainstorming **tidak berlaku** karena ADR-0004 (database per tenant); dicatat agar tidak menjadi temuan.

### Requirement 22: Verifikasi dan kriteria selesai

**User Story:** Sebagai pemilik produk, saya ingin bukti bahwa sistem bekerja dan aman, sehingga blok berikutnya aman dibangun di atasnya.

#### Acceptance Criteria

1. THE setiap tahap SHALL lulus gerbang lokal proyek (`task lint arch check:gowork-off test`, `task test:integration` bila menyentuh DB, `task contracts:check`, `task web:check`) dan exit gate tahap pada tabel Tahap.
2. THE tes paritas dua sisi (korpus vektor dan property test) SHALL ada dan hijau sebelum T3 dinyatakan selesai.
3. THE tes keamanan SHALL mencakup: mass assignment (field tak dikenal dan `readOnly`), IDOR (Requirement 15.9), field terlarang (filter, urut, grup, tulis), injeksi identifier SQL pada pembangkit, batas sumber daya, dan YAML berbahaya.
4. THE review akhir tiap putaran SHALL mencakup review kesesuaian dan review keamanan (skill `security-audit`) atas perubahan putaran itu.
5. THE `task test:generator` SHALL lulus bila templat atau generator berubah.

## Keputusan atas pertanyaan terbuka (1 Nov 2026, disetujui pemilik produk)

Pemilik produk menerima rekomendasi pada P-1 sampai P-6 dan P-8 ("sisanya saya terima"), dan menjawab P-7 dengan K14 serta menambah K13 (pencarian eksak).

| # | Pertanyaan | Keputusan |
| - | ---------- | --------- |
| P-1 | Arah pembaruan manifest atas keanggotaan field pada field group (PRD-001 FR-110) | Metadata menjadi sumber keanggotaan; `fields` di manifest opsional dan diperiksa kesamaannya oleh `module:check`. Perubahan aditif atas PRD-001 FR-110 (dicatat pada FR-110 dan PRD-000 Q41). Requirement 7.2 sudah mengikuti keputusan ini |
| P-2 | Jadwal T4 terhadap PRD-001 | T4 menunggu PRD-001 FR-110 sampai FR-115 dan FR-119 (authz nyata); T4 juga bergantung pada spec `field-encryption` untuk resource ber-field `highly_confidential` |
| P-3 | Pohon filter bersarang pada HTTP | Spec lanjutan terpisah setelah M3d (perluasan aditif `api-guidelines.md` §5.1); spec ini hanya membangkitkan penanda `filterable`, `sortable`, `groupable` |
| P-4 | Penyalinan field | Dengan principal pemanggil; hak sistem hanya lewat `authz.AsSystem` yang dinyatakan eksplisit di metadata dengan alasan dan audit |
| P-5 | Nama field label pada `expand` | `label` ditambahkan secara aditif; `name` tetap bila resource punya field itu |
| P-6 | Nama blok pada PRD-000 §11 | "M3m", setelah sistem enkripsi dan sebelum M3c |
| P-7 | Primitif enkripsi `highly_confidential` | Digantikan K14: sistem enkripsi dibangun lebih dahulu, end to end, spec terpisah `field-encryption`, dipakai bersama PRD-001 |
| P-8 | Isi chunk metadata publik | Default menyertakan struktur (nama, tipe, batasan, grup); aturan dan ambang `scope: server` tidak terkirim; `clientVisible: false` untuk kasus sensitif |

## Diserahkan kepada pelaksana (tambahan)

- Format kunci resource bermulti-kata pada manifest dan permission: registri dapat mensyaratkan huruf kecil; pelaksana memverifikasi aturan registry (`capability_prefix`) dan menentukan konversinya (mis. `sales.invoiceLine` menjadi `sales.invoice_line`), lalu mencatat pemetaan itu di `design.md` Komponen 13.
- Isi tepat spec `field-encryption` (manajemen kunci, versi kunci, rotasi, bentuk ciphertext, indeks buta, API `sdk`) ditentukan dalam spec itu, bukan di sini; spec ini hanya menetapkan kebutuhan yang dibebankan padanya (Requirement 2.9, 2.10, 2.12, 2.13).
