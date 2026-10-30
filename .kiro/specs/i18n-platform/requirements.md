# Requirements Document

## Introduction

Platform ERPora membutuhkan satu sistem terjemahan yang dipakai backend (Go) dan frontend (React) dengan **satu sumber katalog**, namun **terpisah per modul**. Spec ini mendefinisikan format katalog, runtime di kedua sisi, API penyaji katalog, kunci terketik, dan alat pemeriksa. M3a (shell frontend) bergantung pada spec ini.

**Keputusan pemilik produk (29 Okt 2026):**
- Satu katalog bersama untuk FE dan BE, tetapi terpisah di setiap modul.
- Sintaks pesan: **ICU MessageFormat** (standar), bukan gaya Laravel (`:name`, `a|b`). Alasan: standar industri dengan plural CLDR lengkap dan alat penerjemah profesional; kekurangannya (kunci DS dan paket Laravel Lang perlu dikonversi, dukungan Go) diterima.

- Katalog **disajikan backend lewat API per namespace (opsi A)**, bukan di-bundle saat build (opsi B). Alasan: istilah/bahasa dapat dikustomisasi pemilik instalasi tanpa build ulang (Requirement 12).
- Pemilik instalasi dapat **menimpa (override) teks bawaan** tanpa mengubah kode (Requirement 12). Cakupan awal: API dan CLI; UI manajemen istilah menyusul di blok lain.

**Rujukan yang sudah dipelajari:** Laravel Lang (`Laravel-Lang/lang`, `publisher`) dan `laravel-react-i18n`. Konsep yang diambil: namespace per paket (`package::group.key`), nama field (`attributes`) dipakai bersama validasi dan form, pelacak kelengkapan per bahasa, fallback ke bahasa default. Yang tidak diambil: katalog tunggal tanpa pemisahan per modul, pemuatan saat build, dan keterikatan pada Vite.

**Kondisi awal (diverifikasi 29 Okt 2026):** belum ada i18n di backend maupun frontend. `user_preferences.locale` dan `timezone` sudah ada di PRD-001 (belum diimplementasi). `api-guidelines §8` menyebut `error.message` mengikuti `Accept-Language` atau preferensi pengguna, dan `code` adalah kontrak (klien tidak boleh mem-parse `message`).

**Risiko:** pustaka ICU MessageFormat untuk Go kecil dan kurang terpelihara (hasil pencarian GitHub: 2–14 bintang; salah satunya terakhir diubah 2018). Design harus memutuskan antara memakai pustaka yang dipilih atau subset parser buatan sendiri, dengan aturan plural dari `golang.org/x/text` `[TODO: verifikasi]`.

**Di luar spec ini (non-goals):** penyimpanan bahasa per pengguna (PRD-001, `user_preferences.locale`); template email dan notifikasi server (PRD Notifications memakai runtime ini); terjemahan data master (nama produk dua bahasa); format angka/tanggal/mata uang di luar yang disediakan ICU; tata letak RTL; UI manajemen istilah (API dan CLI-nya masuk spec ini) dan terjemahan mesin; migrasi massal kunci dari aplikasi Laravel lama.

## Glossary

- **Katalog**: kumpulan pesan terjemahan satu bahasa untuk satu namespace (satu berkas JSON).
- **Namespace**: identitas pemilik katalog: slug modul (mis. `example`), `ui` (komponen generik), atau `platform`.
- **Kunci**: nama pesan di dalam namespace, bertitik (mis. `note.title`); kunci lengkap ditulis `namespace::note.title`.
- **ICU MessageFormat**: sintaks pesan standar Unicode (`{name}`, `{n, plural, one {…} other {…}}`, `{x, select, …}`).
- **Bahasa default**: `id`; bahasa rujukan untuk kelengkapan dan fallback.
- **Locale aktif**: bahasa yang dipakai untuk satu request atau sesi.
- **Registri bahasa**: daftar bahasa yang didukung platform beserta bahasa fallback-nya (`internal/platform/i18n/locales.json`).
- **Fallback**: bahasa tujuan bila sebuah kunci tidak ditemukan di bahasa aktif.
- **Override**: teks pengganti yang disimpan di database oleh pemilik instalasi untuk satu (namespace, bahasa, kunci); menimpa teks katalog bawaan.

## Requirements

### Requirement 1: Katalog per modul dan namespace

**User Story:** Sebagai developer modul, saya ingin katalog terjemahan saya berada di dalam modul saya sendiri, sehingga modul tetap mandiri dan terpisah dari modul lain.

#### Acceptance Criteria

1. THE katalog modul SHALL berada di `modules/<name>/i18n/<locale>.json` dengan namespace sama dengan slug modul; komponen generik di `packages/ui/i18n/<locale>.json` (namespace `ui`) dan platform di `internal/platform/i18n/<locale>.json` (namespace `platform`).
2. THE modul SHALL hanya membaca dan menulis namespace-nya sendiri ditambah `ui` dan `platform`; mengakses katalog modul lain SHALL ditolak dan ditegakkan arch test (selaras D1–D9).
3. THE manifest modul SHALL mendeklarasikan `locales` (daftar bahasa yang disediakan) dan `defaultLocale`; WHEN katalog untuk bahasa yang dideklarasikan tidak ada, THE `module:check` SHALL gagal.
4. THE struktur berkas JSON SHALL berupa **objek bersarang** (diputuskan 29 Okt 2026); pesan adalah nilai string di daun, dan SHALL diakses dengan **notasi titik** (`note.title`, atau lengkap `example::note.title`) di Go, TypeScript, override, CLI, dan API. Struktur bersarang SHALL konsisten antar bahasa dalam satu namespace.
5. THE segmen kunci (nama properti JSON) SHALL boleh memuat titik (diputuskan 29 Okt 2026). THE runtime SHALL meratakan pohon menjadi peta kunci-lengkap→pesan dengan menggabungkan segmen memakai `.` (mis. `{"order": {"status.draft": "Draf"}}` menjadi `order.status.draft`), dan pencarian SHALL memakai peta itu tanpa penelusuran bertingkat atau backtracking.
6. THE kunci lengkap hasil perataan SHALL unik per namespace dan bahasa; WHEN dua jalur berbeda menghasilkan kunci lengkap yang sama (mis. `{"a.b": {"c": "x"}}` dan `{"a": {"b.c": "y"}}` keduanya menjadi `a.b.c`), THE `i18n:check` SHALL gagal dengan menyebut kedua jalur.
7. THE karakter segmen SHALL dibatasi ke `[A-Za-z0-9_.-]`; segmen SHALL tidak kosong, tidak diawali atau diakhiri titik, tidak memuat titik berurutan (`..`), dan tidak memuat `::` (pemisah namespace).
8. WHEN `make:module` dijalankan, THE generator SHALL membuat `i18n/id.json` dan `i18n/en.json` berisi kunci contoh bersarang.

### Requirement 2: Format pesan ICU MessageFormat

**User Story:** Sebagai penerjemah dan developer, saya ingin format pesan standar, sehingga plural dan pilihan bahasa ditangani dengan benar dan alat penerjemah dapat dipakai.

#### Acceptance Criteria

1. THE pesan SHALL ditulis dalam ICU MessageFormat dan SHALL mendukung minimal: argumen sederhana `{name}`, `plural` (kategori CLDR `zero|one|two|few|many|other`, `=N`, dan `#`), dan `select`.
2. THE aturan plural SHALL mengikuti CLDR untuk bahasa yang dipakai, sama di Go dan TypeScript.
3. WHEN argumen yang dibutuhkan pesan tidak diberikan, THE runtime SHALL tidak crash: ia SHALL mengembalikan pesan dengan argumen tak terisi ditampilkan apa adanya dan mencatat peringatan di mode pengembangan.
4. THE pesan SHALL tidak berisi HTML; teks kaya (tebal, tautan) SHALL ditangani lewat penanda tag bergaya ICU (mis. `<b>…</b>`) yang dirender komponen di sisi tampilan, bukan HTML dalam katalog (diputuskan 29 Okt 2026; sintaks rincinya di design).
5. THE batas kedalaman penyarangan dan ukuran pesan SHALL ditetapkan untuk mencegah pesan berbahaya atau salah ketik menghabiskan sumber daya.
6. THE pesan SHALL mendukung argumen dan plural saja; angka, tanggal, dan mata uang SHALL diformat di kode (`Intl` di frontend, `golang.org/x/text` di backend) memakai locale aktif dan diteruskan sebagai argumen teks (diputuskan 29 Okt 2026).

### Requirement 3: Penentuan locale dan fallback

**User Story:** Sebagai pengguna, saya ingin aplikasi memakai bahasa pilihan saya, sehingga semua teks konsisten.

#### Acceptance Criteria

1. THE locale aktif SHALL ditentukan dengan urutan: pilihan eksplisit pengguna (`user_preferences.locale` setelah PRD-001; sebelum itu preferensi dari frontend), lalu header `Accept-Language`, lalu `defaultLocale` sistem (`id`).
2. THE fallback SHALL berjenjang untuk setiap kunci: override untuk bahasa persis, katalog bawaan bahasa persis (`en-US`), lalu **bahasa fallback yang dikonfigurasi** untuk bahasa itu (Requirement 3.5), lalu bahasa default (`id`), lalu kunci itu sendiri; pada setiap tingkat urutannya override sebelum katalog bawaan.
3. WHEN pesan tidak ditemukan sampai kunci sendiri, THE runtime SHALL mengembalikan `namespace::kunci`, mencatat peringatan satu kali per (bahasa, namespace, kunci) di semua lingkungan (dibatasi), menghitungnya pada metrik `erp.i18n.missing` (atribut `locale` dan `namespace`, tanpa kunci), dan menjadikannya galat di mode pengujian (`StrictMissing`). Diputuskan 30 Okt 2026 (PRD-000 Q37).
4. THE nilai `Accept-Language` dan bahasa yang diminta SHALL divalidasi terhadap daftar bahasa yang didukung; nilai tak dikenal SHALL jatuh ke fallback tanpa error dan SHALL tidak mengalir ke path berkas.
5. THE platform SHALL memiliki registri bahasa (`internal/platform/i18n/locales.json`) berisi daftar bahasa yang didukung, dengan setiap entri memuat `code`, `name` (nama asli bahasa), dan `fallback` (kode bahasa tujuan fallback; opsional). Contoh: `{ "code": "ms", "fallback": "id" }`, `{ "code": "en-GB", "fallback": "en" }`.
6. THE registri SHALL divalidasi saat startup dan oleh `i18n:check`: `fallback` harus menunjuk bahasa yang terdaftar, rantai fallback tidak boleh bersiklus dan dibatasi kedalamannya, dan bahasa default tidak punya fallback.
7. WHEN fallback dipakai untuk sebuah kunci, THE runtime SHALL mencatat peringatan satu kali per (bahasa, namespace, kunci) di mode pengembangan dan pengujian, dan SHALL menghitungnya pada metrik `erp.i18n.fallback` (pemakaian runtime lewat `Resolve`) serta metrik terpisah untuk jumlah kunci yang jatuh ke fallback per pembangunan katalog (Q37).
8. THE modul SHALL hanya dapat menyediakan bahasa yang terdaftar di registri; modul yang mendeklarasikan bahasa tak terdaftar SHALL ditolak `module:check`.
9. THE fallback SHALL ditentukan **hanya dari registri di repo** (diputuskan 29 Okt 2026); tidak ada pengaturan fallback per instalasi atau lewat database.

### Requirement 4: Runtime backend (`sdk/i18n`)

**User Story:** Sebagai developer backend, saya ingin menerjemahkan pesan dari katalog modul saya, sehingga galat dan keluaran server berbahasa pengguna.

#### Acceptance Criteria

1. THE `sdk/i18n` SHALL menyediakan fungsi terjemah yang terikat ke namespace modul dan locale di `context.Context` (mis. `i18n.T(ctx, "note.title", args)`); modul tidak memegang katalog mentah.
2. THE katalog SHALL ditanam di binary (`go:embed`) per modul dan dimuat bersama registry modul; hanya katalog modul terinstal dan aktif yang tersedia.
3. THE middleware platform SHALL mengisi locale ke context pada setiap request (urutan di Requirement 3), sejalan dengan middleware tenant.
4. THE `error.message` di respons API SHALL dirender dalam locale aktif memakai katalog pemilik error itu; `code` SHALL tetap stabil dan menjadi kontrak (api-guidelines §8).
5. THE pesan validasi dan nama field SHALL memakai konvensi `fields.<resource>.<field>` di katalog modul sehingga label form (FE) dan pesan validasi (BE) memakai teks yang sama.
6. THE `sdk/i18n` SHALL tidak mengimpor `internal/` (D5/D9) dan SHALL aman dipakai bersamaan oleh banyak goroutine.

### Requirement 5: API penyaji katalog (opsi A, diputuskan 29 Okt 2026)

**User Story:** Sebagai frontend, saya ingin mengambil katalog dari backend per namespace, sehingga teks selalu cocok dengan versi binary dan modul yang aktif.

#### Acceptance Criteria

1. THE backend SHALL menyediakan `GET /api/platform/i18n/{locale}/{namespace}` yang mengembalikan katalog namespace itu dalam bahasa diminta (dengan fallback sesuai Requirement 3), dengan `ETag` dan dukungan `If-None-Match` (304).
2. WHEN namespace bukan `ui`/`platform` dan modulnya tidak terinstal atau tidak aktif, THE endpoint SHALL mengembalikan `404` (tidak membocorkan keberadaan modul nonaktif).
3. THE bootstrap SHALL memuat revisi katalog per namespace sehingga frontend tahu kapan perlu memuat ulang.
4. THE endpoint SHALL didokumentasikan di `contracts/openapi/platform.yaml` dan tipe TS-nya dibangkitkan ke `packages/platform-contracts`; `contracts:check` SHALL lolos.
5. THE respons SHALL hanya berisi pesan terjemahan (data non-rahasia); katalog SHALL tidak pernah memuat rahasia, data pribadi, atau data tenant.
6. THE endpoint SHALL memakai amplop dan aturan api-guidelines (kecuali bentuk respons katalog yang ditetapkan di design) dan dapat di-cache (`Cache-Control`) tanpa membocorkan antar-tenant.
7. THE respons SHALL sudah menggabungkan override (Requirement 12) di atas katalog bawaan, sehingga frontend menerima satu katalog final; `ETag` SHALL berubah bila katalog bawaan atau override namespace itu berubah.
8. Opsi B (katalog di-bundle saat build) ditolak karena tidak mendukung kustomisasi tanpa build ulang; ia tidak dikerjakan.

### Requirement 6: Runtime frontend

**User Story:** Sebagai developer frontend, saya ingin hook sederhana untuk menerjemahkan teks, sehingga komponen tidak peduli dari mana katalog berasal.

#### Acceptance Criteria

1. THE frontend SHALL menyediakan provider dan hook `useTranslation(namespace)` dengan `t(key, args?)`, `locale`, `setLocale`, dan `loading`; `t` menangani plural/select ICU, sehingga tidak perlu fungsi terpisah `tChoice`.
2. THE katalog SHALL dimuat malas per namespace saat modul pertama kali dipakai dan di-cache; WHEN bahasa berganti, THE katalog yang sudah dimuat SHALL dimuat ulang untuk bahasa baru tanpa reload halaman.
3. WHEN katalog sedang dimuat, THE UI SHALL tidak menampilkan kunci mentah; ia SHALL menampilkan keadaan memuat atau menahan render bagian itu.
4. THE komponen `@erpora/ui` SHALL membaca teksnya dari namespace `ui` dan tidak menanam string tampilan.
5. THE pilihan bahasa SHALL disimpan di `localStorage` (preferensi UI) dengan penanganan gagal, dan tag `<html lang>` SHALL ikut diperbarui.
6. THE runtime SHALL dapat dibuat tersedia untuk test komponen (provider uji dengan katalog tiruan).
7. [TODO: design] Letak paket runtime frontend (usulan: `@erpora/platform-web`) dan apakah memakai pustaka ICU JavaScript yang ada atau implementasi tipis.

### Requirement 7: Kunci terketik

**User Story:** Sebagai developer, saya ingin salah ketik kunci atau argumen terdeteksi saat typecheck, sehingga galat terjemahan tidak sampai ke pengguna.

#### Acceptance Criteria

1. THE generator (`erpora i18n:generate`, atau bagian dari `contracts:generate`) SHALL membangkitkan tipe TypeScript untuk kunci dan tipe argumennya dari katalog bahasa default tiap namespace.
2. WHEN kunci tidak ada atau argumen salah tipe/hilang, THE `bun run typecheck` SHALL gagal.
3. THE `--check` generator SHALL gagal bila hasil generate di repo tertinggal dari katalog.

### Requirement 8: Pemeriksa kelengkapan dan kualitas

**User Story:** Sebagai pemilik produk, saya ingin tahu bahasa mana yang belum lengkap, sehingga tidak ada teks yang jatuh ke bahasa lain tanpa disadari.

#### Acceptance Criteria

1. THE `erpora i18n:check` SHALL membedakan dua tingkat temuan (diputuskan 29 Okt 2026: proses build hanya memberi **peringatan** untuk ketidaksesuaian antar bahasa). **Peringatan** (build tidak gagal): kunci hilang atau berlebih antar bahasa (butir 3 dan 4) dan argumen/tipe argumen yang tidak sama antar bahasa untuk kunci yang sama. **Galat** (build gagal) (diputuskan 29 Okt 2026: galat struktural tetap menggagalkan build karena katalog tidak dapat dimuat atau maknanya tidak pasti): sintaks ICU tidak valid; JSON tidak valid; kunci bernamespace pemilik lain; pesan berisi HTML; berkas JSON memuat kunci ganda pada tingkat yang sama (dibaca dengan pembaca JSON yang tidak diam-diam membuang duplikat); dua jalur menghasilkan kunci lengkap yang sama setelah perataan (Requirement 1.6); atau segmen kunci melanggar karakter yang diizinkan (Requirement 1.7).
2. THE `erpora i18n:status` SHALL menampilkan persentase kelengkapan per bahasa per namespace.
3. THE `i18n:check` SHALL membandingkan kunci setiap bahasa terhadap bahasa default dalam tiap namespace dan melaporkan secara terperinci: kunci yang **hilang** di sebuah bahasa, kunci **berlebih** yang tidak ada di bahasa default, dan argumen ICU yang tidak cocok; laporan SHALL menyebut berkas, bahasa, dan kunci lengkap, dan tersedia dalam format teks dan `--json`.
4. THE bahasa SHALL dianggap **lengkap** secara bawaan: kunci hilang atau berlebih di bahasa lengkap SHALL menjadi peringatan di `i18n:check` biasa (mode build) dan menjadi kegagalan hanya bila `i18n:check --strict` dipakai. Sebuah bahasa dapat ditandai `partial: true` di registri sebagai pernyataan bahwa ia sengaja belum selesai; untuk bahasa `partial`, kunci hilang SHALL tidak dilaporkan sebagai peringatan (teks jatuh ke fallback) dan tidak menggagalkan `--strict`, sedangkan kunci berlebih dan argumen tak cocok tetap dilaporkan.
5. THE gate bawaan (`task build`, `task lint`, CI) SHALL memakai mode biasa (peringatan saja); `--strict` SHALL tersedia sebagai pilihan tetapi tidak dipakai gate bawaan.
6. THE exit code `i18n:check` SHALL membedakan kegagalan validasi (kunci/format) dari kegagalan prasyarat (registri/konfigurasi), agar mudah dipakai di skrip dan CI.
7. THE pengecekan kesesuaian SHALL juga berjalan terhadap katalog hasil override di CI untuk fixture uji, memastikan override tidak mengubah kesesuaian argumen (Requirement 12.3).
8. THE `erpora i18n:add <locale>` SHALL membuat kerangka katalog bahasa baru dari bahasa default untuk modul yang dipilih.
9. THE pemeriksaan i18n SHALL menjadi bagian dari gate lokal dan CI (`task lint` atau gate khusus; temuan sesuai butir 1 dan 5), dan SHALL diperiksa oleh `make:module`/`test:generator` untuk modul hasil generate.

### Requirement 9: Kesamaan perilaku Go dan TypeScript

**User Story:** Sebagai developer, saya ingin pesan yang sama menghasilkan teks yang sama di server dan di browser, sehingga tidak ada beda antara email/ekspor dan tampilan.

#### Acceptance Criteria

1. THE repo SHALL memiliki satu berkas fixture konformansi (pesan, bahasa, argumen → hasil yang diharapkan) yang dijalankan oleh test Go dan `bun test`.
2. THE fixture SHALL mencakup plural untuk `id` dan `en` (dan bahasa lain yang dideklarasikan modul), select, argumen hilang, karakter khusus/escape ICU, dan batas ukuran.
3. WHEN implementasi Go dan TS memberi hasil berbeda untuk fixture yang sama, THE test SHALL gagal.

### Requirement 10: Keamanan

**User Story:** Sebagai pemilik produk, saya ingin sistem terjemahan tidak membuka celah baru.

#### Acceptance Criteria

1. THE nilai argumen yang berasal dari pengguna SHALL diperlakukan sebagai teks biasa; hasil terjemahan di frontend SHALL dirender sebagai teks (React meng-escape) dan SHALL tidak melewati `dangerouslySetInnerHTML`.
2. THE parameter `{locale}` dan `{namespace}` di endpoint SHALL divalidasi terhadap daftar tertutup (tidak dipakai langsung sebagai path berkas; tidak ada path traversal).
3. THE katalog SHALL ditinjau sebagai kode (berada di repo, lewat PR) dan SHALL tidak dapat diubah lewat API pada fase ini.
4. THE batas ukuran katalog, kedalaman pesan, dan jumlah kunci SHALL ditegakkan untuk mencegah pemakaian memori berlebih.
5. THE pesan galat SHALL tidak membocorkan detail internal; teks terjemahan dari backend SHALL tidak berisi data sensitif (selaras security-guidelines).

### Requirement 11: Gate pengerjaan

**User Story:** Sebagai pemilik produk, saya ingin bukti bahwa i18n berfungsi dan tidak merusak yang ada.

#### Acceptance Criteria

1. THE `task lint arch check:gowork-off test`, `task contracts:check`, dan `task web:check` SHALL lolos; `task test:integration` SHALL lolos bila menyentuh bootstrap/modul.
2. THE test SHALL mencakup: resolusi locale dan fallback, penolakan namespace modul nonaktif, ETag/304, kesamaan Go↔TS (fixture), pemeriksa kelengkapan (kasus gagal dan lolos), dan arch test batas namespace.
3. THE modul referensi `example` dan `exampleaddon` SHALL memiliki katalog `id` dan `en`, dan satu pesan galat API mereka SHALL terbukti berbahasa sesuai `Accept-Language`.
4. THE dokumentasi `docs/module-sdk/i18n.md` SHALL menjelaskan format, konvensi kunci, dan alur kerja penerjemah.
5. THE perubahan di luar PRD SHALL dicatat sebagai keputusan di PRD-000 (Q34) dan, bila menyentuh kontrak, di api-guidelines (penambahan saja; tanpa breaking change).

### Requirement 12: Kustomisasi istilah (override)

**User Story:** Sebagai administrator instalasi, saya ingin mengganti teks tertentu (mis. "Pelanggan" menjadi "Klien") tanpa mengubah kode atau membangun ulang, sehingga istilah sesuai bahasa bisnis perusahaan saya.

#### Acceptance Criteria

1. THE platform SHALL menyimpan override di tabel platform dengan kolom `tenant_id` NOT NULL, `namespace`, `locale`, `key`, `message`, `updated_by`, `updated_at`, dengan kunci unik (tenant, namespace, locale, key); skema lewat migrasi goose.
2. THE API SHALL menyediakan baca, simpan, dan hapus override (`/api/platform/i18n/overrides`), dilindungi permission `platform.i18n.read` dan `platform.i18n.manage` (kontrak di `contracts/openapi/platform.yaml`, tipe TS dibangkitkan).
3. WHEN simpan override, THE backend SHALL memvalidasi: namespace dan kunci ada di katalog bawaan (override hanya untuk kunci yang sudah ada), sintaks ICU valid, argumen dan tipenya sama dengan pesan bawaan, tanpa HTML, dan batas ukuran; pelanggaran SHALL ditolak `400` dengan kode galat yang jelas.
4. THE override SHALL dibaca dengan scope tenant dan SHALL tidak pernah menembus tenant lain; tenant selalu dari context, bukan dari parameter.
5. THE CLI SHALL menyediakan `erpora i18n:override set|list|rm` sebagai antarmuka utama (seperti `erpora modules`), dengan konfirmasi untuk penghapusan.
6. WHEN override berubah, THE revisi namespace SHALL naik sehingga `ETag` katalog berubah dan frontend memuat ulang tanpa reload halaman.
7. THE setiap perubahan override SHALL dicatat ke audit (log terstruktur sampai audit writer PRD-001 tersedia).
8. THE endpoint mutasi SHALL memakai pengamanan sementara yang sama dengan FR-20.2 (aktif hanya bila `ERP_ENV=development` atau `ERP_MODULES_ADMIN_API=true`) sampai PRD-001 menyediakan authz sungguhan; di luar itu `404`.
9. THE override SHALL tidak menimpa pesan galat yang kodenya menjadi kontrak: `code` tetap stabil; hanya `message` yang berubah.
10. THE API dan CLI SHALL masuk spec ini; UI manajemen istilah dikerjakan di blok lain (diputuskan 29 Okt 2026).

## Keputusan atas pertanyaan terbuka (disetujui pemilik produk, 29 Okt 2026)

| # | Pertanyaan | Keputusan |
| - | ---------- | --------- |
| 1 | Katalog disajikan API atau di-bundle saat build | **A: disajikan API**, agar istilah dapat dikustomisasi (R5, R12) |
| 2 | Pustaka ICU Go vs subset parser sendiri | Dievaluasi di design; cadangan: subset parser + `golang.org/x/text/feature/plural` `[TODO: verifikasi]` |
| 3 | Struktur JSON | **Bersarang**, akses notasi titik; segmen boleh bertitik dengan perataan dan cek tabrakan (R1.4-R1.7) |
| 4 | Teks kaya (tebal/tautan) | Penanda tag ICU yang dirender komponen, tanpa HTML di katalog (R2.4) |
| 5 | Format angka/tanggal dalam pesan | Argumen dan plural saja; angka/tanggal diformat di kode lewat `Intl`/`golang.org/x/text` (R2.6) |
| 6 | Letak runtime frontend | `@erpora/platform-web` (R6.7) |
| 7 | Urutan terhadap M3a | Spec ini lebih dulu; M3a memakai runtime-nya |
| 8 | Cakupan override | API dan CLI di spec ini; UI manajemen istilah di blok lain (R12.10) |
| 9 | Fallback | Registri bahasa **di repo saja**; tidak ada fallback per instalasi (R3.5-R3.9) |
| 10 | Bahasa lengkap vs `partial`; severity | Ketidaksesuaian antar bahasa hanya **peringatan saat build**; `--strict` opsional dan tidak dipakai gate bawaan (R8.1, R8.4, R8.5) |
