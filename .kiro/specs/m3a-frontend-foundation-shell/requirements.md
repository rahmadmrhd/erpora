# Requirements Document

## Introduction

M3a adalah blok pertama M3 (frontend core, PRD-000 Q33). Ia membangun fondasi yang dipakai semua blok M3 berikutnya dan semua modul bisnis: toolchain React, paket `@erpora/ui` (shadcn terbaru yang disalin dan dimiliki repo), token dan tipografi ERP Design System, shell aplikasi, theme switcher, routing, registry modul frontend, notifikasi, menu avatar, dan halaman `/settings/modules`.

**Kondisi awal (diverifikasi 29 Okt 2026):** `web/` baru berisi `main.ts` yang membaca `GET /api/bootstrap`; belum ada React, Tailwind, `packages/ui`, TanStack, maupun `defineModule`. `packages/module-sdk` baru mengekspor tipe `ModuleManifest`; `packages/platform-contracts` sudah punya klien terketik untuk bootstrap dan API modul. Bootstrap mengembalikan `appVersion`, `tenantId`, `modules[]` (`key, slug, version, status, basePath, offline`), `permissions[]` (masih kosong), dan `revision` (juga sebagai `ETag`). Backend notifikasi dan identitas/pengguna belum ada (PRD-001).

**Pendekatan (Q33):** React + shadcn/ui terbaru mengikuti ERP Design System (Figtree + zinc, disesuaikan). Frappe UI hanya rujukan UX dan perilaku. Setiap komponen dikonfirmasi ke pemilik produk sebelum dibuat, dengan perbandingan Frappe UI vs ERP Design System beserta pro dan kontra (dicatat di `design.md`).

**Di luar M3a (non-goals):** sistem Desk dan DeskSwitcher (M3b); list view, FilterBuilder, search lanjutan dan search global (M3c); FormPage/FormDialog (M3d); tree/kanban (M3e); dashboard, chart, laporan (M3f); PWA/Service Worker, `SafeHtml`, ZAP, E2E Playwright (M3g); backend notifikasi dan identitas (PRD Notifications, PRD-001); logika sync (PRD sync); format, runtime, API katalog, dan alat i18n (spec `i18n-platform`, dikerjakan sebelum M3a).

## Glossary

- **Shell**: kerangka halaman yang sudah dimuat: sidebar, navbar, dan area konten.
- **Registry modul frontend**: daftar `FrontendModule` hasil `defineModule()` yang dicocokkan dengan entri bootstrap berdasarkan `key`.
- **Bootstrap**: respons `GET /api/bootstrap` (modul dan statusnya, `tenantId`, `revision`).
- **Modul aktif**: modul bootstrap berstatus `enabled`.
- **Aksi halaman**: tombol yang dideklarasikan halaman dan dirender di navbar.
- **Token**: variabel desain semantik (`background`, `primary`, `muted`, `sidebar-*`, dst.) dari ERP Design System.
- **ERP Design System (DS)**: artefak desain milik pemilik produk (https://claude.ai/artifact/J3fXHW1j7gWWXdqo3SRau5), sumber token dan acuan komponen.

## Requirements

### Requirement 1: Toolchain frontend React

**User Story:** Sebagai developer, saya ingin toolchain React + TypeScript + Tailwind yang berjalan di Bun, sehingga komponen dan halaman dapat dibangun dan dicek otomatis.

#### Acceptance Criteria

1. THE web workspace SHALL memakai React dan TypeScript `strict` dengan `noUncheckedIndexedAccess`, dan `bun run typecheck` SHALL lolos tanpa error.
2. THE build SHALL memakai Tailwind CSS v4 lewat `bun-plugin-tailwind` pada `Bun.build` dan dev server, dengan `@source` yang memindai `packages/ui` dan `modules/*/web`.
3. WHEN `bun run build` dijalankan, THE hasil build SHALL lolos pemeriksaan `csp-check` (tanpa `<script>` inline, event handler inline, atau URL `javascript:`).
4. THE build SHALL menjaga entry JS app shell ≤ 200 KB gzip (NFR-04) dan melaporkan ukurannya di output build; WHEN anggaran terlampaui, THE build SHALL gagal.
5. THE dependency baru SHALL dikunci versinya di `bun.lock` (NFR-02), dan aturan impor F1–F3 (`dependency-cruiser`) SHALL tetap lolos.
6. [TODO: konfirmasi] Versi React (usulan: 19.x terbaru yang stabil), Tailwind v4, dan shadcn CLI terbaru; semuanya dipin versi pasti.

### Requirement 2: Token dan tipografi dari ERP Design System

**User Story:** Sebagai pemilik produk, saya ingin tampilan konsisten dengan ERP Design System, sehingga ERPora terasa seperti ERPNext terbaru yang netral dan padat.

#### Acceptance Criteria

1. THE `@erpora/ui` SHALL menyediakan token semantik DS (skala zinc: `background`, `foreground`, `card`, `popover`, `primary`, `secondary`, `muted`, `accent`, `destructive`, `border`, `input`, `ring`, keluarga `sidebar-*`, token status dan umpan balik) untuk tema terang dan gelap sebagai CSS variable.
2. THE komponen SHALL memakai token semantik dan SHALL NOT memakai nilai palet langsung (mis. `zinc-100`) kecuali sebagai target alias token.
3. THE tipografi SHALL memakai Figtree (400, 500, 600) dengan fallback `ui-sans-serif, system-ui`, ukuran teks UI default 14px, dan kontrol padat (Button md 34px, Input sm 32px) sesuai DS.
4. THE fokus keyboard SHALL tampil sebagai outline 2px `ring` pada semua elemen interaktif.
5. THE pasangan teks/latar SHALL memenuhi kontras 4,5:1 di kedua tema; penyimpangan dari DS yang gagal kontras SHALL dicatat di `design.md` beserta keputusannya.
6. [TODO: konfirmasi] **Hosting font:** DS memuat Figtree dari Bunny/Google Fonts. Usulan: self-host berkas `woff2` di `packages/ui` agar CSP ketat dan PWA offline (M3g) tidak bergantung CDN.
7. [TODO: konfirmasi] Hal yang disesuaikan dari DS (mis. Figtree vs Inter milik Frappe, warna item sidebar terpilih) dirinci di `design.md`.

### Requirement 3: Paket `@erpora/ui` (komponen fondasi)

**User Story:** Sebagai developer modul, saya ingin komponen generik di satu paket milik repo, sehingga semua modul memakai komponen yang sama.

#### Acceptance Criteria

1. THE `packages/ui` SHALL menjadi paket `@erpora/ui` berisi komponen shadcn terbaru yang disalin ke repo (milik repo, bukan dependensi terpasang) dan diadaptasi ke token DS.
2. THE komponen fondasi M3a SHALL mencakup yang dibutuhkan shell dan halaman fondasi: Button, Sidebar, Sheet, Tooltip, DropdownMenu, Popover, Avatar, Badge, Breadcrumb, Separator, ScrollArea, Skeleton, Toast, Dialog, AlertDialog, Table, Switch, dan sejenisnya; daftar final ditetapkan di `design.md`.
3. WHEN sebuah komponen akan dibuat, THE pengembang SHALL meminta konfirmasi pemilik produk lebih dulu, dengan menyebut kartu DS yang dirujuk, perbandingan Frappe UI vs DS beserta pro dan kontra, dan penyesuaian yang diusulkan.
4. THE `@erpora/ui` SHALL tidak mengimpor modul bisnis maupun `@erpora/module-*`; komponen generik saja (F1–F3).
5. THE setiap komponen SHALL punya test (bun test) untuk perilaku utamanya dan aksesibilitas dasar (peran ARIA, navigasi keyboard).
6. THE lint SHALL melarang `dangerouslySetInnerHTML` di luar komponen `SafeHtml` (komponen itu sendiri dibuat di M3g; sampai saat itu aturan lint melarangnya di mana pun).
7. [TODO: konfirmasi] Stack test komponen (usulan: `bun test` + `@testing-library/react` + `happy-dom`).

### Requirement 4: Theme switcher

**User Story:** Sebagai pengguna, saya ingin memilih tema terang, gelap, atau mengikuti sistem, sehingga tampilan nyaman di lingkungan saya.

#### Acceptance Criteria

1. THE aplikasi SHALL mendukung tiga pilihan tema: terang, gelap, dan sistem.
2. WHEN tema "sistem" aktif dan preferensi OS berubah, THE aplikasi SHALL mengikuti perubahan itu tanpa reload.
3. THE pilihan tema SHALL disimpan di `localStorage` dengan penanganan gagal (akses diblokir tidak boleh membuat aplikasi rusak; fallback ke "sistem").
4. WHEN halaman dimuat, THE tema SHALL diterapkan sebelum render pertama sehingga tidak ada kilatan tema yang salah, tanpa `<script>` inline (kompatibel CSP).
5. THE theme switcher SHALL tersedia di menu avatar (Requirement 12).

### Requirement 5: Routing (TanStack Router)

**User Story:** Sebagai developer modul, saya ingin route modul didaftarkan secara terketik dan dimuat malas, sehingga bundle awal kecil.

#### Acceptance Criteria

1. THE aplikasi SHALL memakai TanStack Router dengan code-based routing; setiap modul mengekspor fungsi registrasi route yang menerima parent route bertipe spesifik (bukan `AnyRoute`).
2. THE `app/router` SHALL mengagregasi route modul secara statis saat build dan halaman modul SHALL memakai `.lazy()` untuk code splitting.
3. THE aplikasi SHALL menyediakan halaman `/` (dashboard kosong), `/example` dan `/example-addon` (route modul referensi, lazy), `/settings/modules`, dan halaman 404.
4. WHEN `/example` dibuka, THE aplikasi SHALL memuat chunk JS terpisah untuk modul itu.
5. THE URL, judul halaman, dan breadcrumb SHALL berasal dari definisi route sehingga tidak ada sumber ganda.

### Requirement 6: Registry modul frontend (`defineModule`)

**User Story:** Sebagai developer modul, saya ingin mendeklarasikan modul frontend satu kali dan didaftarkan otomatis ke shell, sehingga menu dan route muncul tanpa mengubah kode platform.

#### Acceptance Criteria

1. THE `packages/module-sdk` SHALL menyediakan `defineModule()` dan tipe `FrontendModule` dengan field `key`, `slug`, `version`, `navigation[]`, `registerRoutes`, dan `offline: OfflinePolicy` (UI-side, mengikuti `docs/architecture/offline-strategy.md` §3).
2. WHEN dua modul frontend memakai `key` sama, THE registry SHALL menolaknya dengan error jelas.
3. WHEN sebuah modul frontend tidak ada di bootstrap, THE registry SHALL mengabaikannya dan mencatat warning.
4. WHEN versi frontend berbeda dari versi backend di bootstrap, THE registry SHALL mencatat warning (indikasi build tidak sinkron).
5. THE registry SHALL hanya memunculkan navigasi untuk modul berstatus `enabled`.
6. THE backend SHALL tetap menjadi otoritas; filter di frontend hanya untuk UX.
7. THE `web/src/app/modules.ts` (daftar yang disisipi `make:module --frontend`) SHALL menjadi sumber agregasi registry.
8. THE unit test (`bun test`) SHALL mencakup filter modul aktif, penolakan `key` duplikat, dan perilaku bila modul tidak ada di bootstrap.

### Requirement 7: Bootstrap dan lapisan data

**User Story:** Sebagai pengguna, saya ingin menu dan akses modul selalu mengikuti status terbaru tanpa reload, sehingga aplikasi tidak menampilkan modul yang sudah dimatikan.

#### Acceptance Criteria

1. THE aplikasi SHALL memakai satu `QueryClient` TanStack Query dengan query key berprefiks scope `[tenantId, ...]` (tenantId dari bootstrap, FR-21).
2. THE bootstrap SHALL di-refetch saat window focus dan setelah operasi modul, memakai `ETag`/`If-None-Match` dan menangani respons `304` tanpa mengganti state.
3. WHEN bootstrap gagal dimuat, THE shell SHALL menampilkan keadaan galat yang bisa dicoba ulang, bukan layar kosong.
4. THE aplikasi SHALL menginisialisasi provider TanStack DB dengan satu collection contoh (`example`) yang bersumber dari Query sebagai bukti integrasi (FR-08); persistence IndexedDB menyusul di PRD offline.
5. THE klien API SHALL memakai tipe dari `packages/platform-contracts` (hasil generate), dan SHALL NOT mengirim `tenantId` atau `userId` dalam body request (aturan IDOR).

### Requirement 8: Route guard modul tidak aktif

**User Story:** Sebagai pengguna, saya ingin penjelasan yang jelas ketika membuka modul yang dimatikan, sehingga saya tahu itu bukan galat aplikasi.

#### Acceptance Criteria

1. WHEN route modul dibuka dan status modul di bootstrap bukan `enabled`, THE aplikasi SHALL menampilkan halaman "Modul tidak aktif" (UX saja).
2. WHEN sebuah modul di-disable lewat `/settings/modules`, THE menunya SHALL hilang tanpa reload halaman (revision berubah → bootstrap di-refetch).
3. THE backend SHALL tetap menolak akses ke endpoint modul yang tidak aktif (FR-19); frontend tidak menjadi otoritas.

### Requirement 9: Shell dan sidebar

**User Story:** Sebagai pengguna, saya ingin sidebar yang jelas dengan notifikasi di atas menu dan akun saya di bawah, sehingga navigasi cepat dan akrab seperti ERPNext.

#### Acceptance Criteria

1. THE sidebar SHALL berisi, dari atas ke bawah: logo dan nama aplikasi, item Notifikasi, daftar menu dari registry modul, dan avatar akun di paling bawah (keputusan Q33; mockup disetujui 29 Okt 2026).
2. THE daftar menu SHALL dibangun dari `navigation[]` modul aktif di registry, mendukung item langsung dan grup yang bisa dilipat dengan anak, serta menandai item aktif sesuai route.
3. THE sidebar SHALL punya tiga keadaan: diperluas, terlipat ke ikon (dengan tooltip nama item), dan panel geser (Sheet) di layar kecil.
4. WHEN tombol collapse di navbar ditekan atau Ctrl/Cmd+B ditekan, THE sidebar SHALL berganti antara diperluas dan terlipat.
5. THE keadaan sidebar SHALL disimpan (cookie atau `localStorage`) dan dipulihkan saat dimuat ulang.
6. WHEN tidak ada modul aktif dengan navigasi, THE sidebar SHALL menampilkan keadaan kosong yang jelas, bukan area kosong.
7. THE sidebar SHALL dapat dioperasikan penuh dengan keyboard dan memakai landmark `nav` bernama.
8. [TODO: konfirmasi] Sumber ikon menu: usulan `lucide-react` dengan nama ikon di `navigation[]`; ikon tak dikenal jatuh ke lingkaran berinisial (perilaku DS).

### Requirement 10: Navbar dan aksi halaman

**User Story:** Sebagai pengguna, saya ingin navbar dengan tombol collapse, breadcrumb, dan tombol aksi halaman, sehingga konteks dan aksi utama selalu terlihat.

#### Acceptance Criteria

1. THE navbar SHALL berisi tombol collapse sidebar, breadcrumb halaman, dan area aksi halaman di sisi kanan, dengan tinggi tetap dan lengket di atas area konten.
2. THE breadcrumb SHALL berasal dari definisi route; WHEN tingkatnya lebih dari 3 atau layar kecil, THE tingkat tengah SHALL dilipat ke menu elipsis dan halaman aktif SHALL selalu tampil sebagai teks (bukan tautan).
3. THE halaman SHALL mendeklarasikan aksinya sendiri (mis. "+ Pesanan Baru") lewat API shell (portal/slot, tanpa modul mengimpor struktur shell), dan aksi itu SHALL dirender di navbar serta dibersihkan saat halaman berganti.
4. THE shell SHALL memiliki kontainer gulir area konten, dengan opsi per halaman untuk mematikan gulir halaman (layout list-detail).
5. [TODO: konfirmasi] Pola portal vs context untuk aksi halaman, dan perilaku di layar kecil (aksi dilipat ke menu ⋯).

### Requirement 11: Notifikasi (sidebar item dan panel)

**User Story:** Sebagai pengguna, saya ingin melihat jumlah notifikasi belum dibaca dan daftarnya dari sidebar, sehingga tidak ketinggalan hal penting.

#### Acceptance Criteria

1. THE sidebar SHALL menampilkan item Notifikasi sebelum daftar menu, dengan lencana jumlah belum dibaca (99+ bila lebih), dan titik penanda saat sidebar terlipat.
2. WHEN item Notifikasi diklik, THE aplikasi SHALL membuka panel daftar notifikasi (judul, pesan, waktu relatif, status baca) dengan tombol "Tandai semua telah dibaca".
3. THE panel SHALL menampilkan keadaan memuat, kosong ("Belum ada notifikasi"), dan galat.
4. THE UI SHALL membaca data lewat antarmuka `NotificationSource` yang terpisah dari komponen, sehingga backend dapat dipasang tanpa mengubah UI.
5. [TODO: keputusan pemilik produk] Backend notifikasi belum ada (PRD Notifications). Usulan M3a: antarmuka `NotificationSource` + implementasi kosong (panel tampil keadaan kosong, lencana tersembunyi) + test dengan sumber tiruan; kontrak API notifikasi ditentukan di PRD Notifications. Alternatif: buat endpoint minimal di M3a.

### Requirement 12: Menu avatar akun

**User Story:** Sebagai pengguna, saya ingin menu akun di dasar sidebar, sehingga profil dan preferensi mudah dijangkau.

#### Acceptance Criteria

1. THE sidebar SHALL menampilkan avatar (gambar atau inisial) beserta nama dan email di dasar; WHEN terlipat, hanya avatar yang tampil.
2. WHEN avatar diklik, THE aplikasi SHALL membuka menu berisi: Kelola Akun, pilihan Bahasa, pilihan Tema (Requirement 4), dan Keluar.
3. THE data pengguna SHALL dibaca lewat antarmuka `CurrentUserSource` terpisah dari komponen.
4. [TODO: keputusan pemilik produk] Identitas pengguna dan sesi belum ada (PRD-001 Identity); bootstrap belum memuat data pengguna. Usulan M3a: `CurrentUserSource` dengan implementasi sementara (pengguna anonim/"Pengguna" dan aksi Kelola Akun/Keluar dinonaktifkan dengan tooltip), dipasang sungguhan di PRD-001.
5. THE pilihan Bahasa SHALL memakai runtime dari spec `i18n-platform` (ICU MessageFormat, katalog per namespace, dimuat dari backend); M3a SHALL tidak mendefinisikan format atau runtime i18n sendiri. Teks shell berada di namespace `platform` dan teks komponen di namespace `ui`, bahasa `id` (default) dan `en`; pilihan bahasa tersimpan di `localStorage` (preferensi UI) sampai PRD-001 menyediakan `user_preferences.locale`. M3a bergantung pada selesainya runtime frontend `i18n-platform`.

### Requirement 13: Halaman `/settings/modules`

**User Story:** Sebagai administrator, saya ingin mengelola modul dari UI, sehingga saya melihat status dan dampak sebelum bertindak.

#### Acceptance Criteria

1. THE halaman SHALL menampilkan tabel modul: nama, key, versi (binary vs ter-install), badge status, dependency, dan modul yang bergantung padanya.
2. THE aksi (install/enable/disable/upgrade/uninstall/retry) SHALL hanya tampil bila API mutasi aktif (`ERP_ENV=development` atau `ERP_MODULES_ADMIN_API=true`); WHEN tidak aktif, THE halaman SHALL tampil hanya-baca dengan penjelasan.
3. WHEN pengguna memilih aksi, THE aplikasi SHALL menampilkan dialog konfirmasi berisi dampaknya (mis. "akan menonaktifkan route X, Y") sebelum mengeksekusi.
4. THE halaman SHALL menampilkan riwayat operasi dan langkah per modul dari `GET /api/platform/modules/{key}/history` dan `operations/{id}`.
5. WHEN operasi mutasi sukses, THE aplikasi SHALL meng-invalidate bootstrap sehingga sidebar dan route guard terbarui tanpa reload.
6. WHILE operasi berjalan (respons 202), THE UI SHALL melakukan polling `operations/{id}` dan menampilkan kemajuan serta kegagalan (`migration_failed` dst.) dengan pesan yang dapat dipahami.
7. THE `409`/`412`/`415` dan error lifecycle lain SHALL dipetakan ke pesan yang jelas berdasarkan kode di `error.details[].code`.
8. [TODO: verifikasi] API saat ini tidak punya endpoint `plan` (dry-run ada di CLI). Dialog dampak butuh sumber data: usulan menambah penghitungan dampak dari data modul (dependency/dependents) di klien, atau menambah endpoint `plan` additive di backend; ditetapkan di `design.md`.

### Requirement 14: Aksesibilitas, keamanan, dan kualitas

**User Story:** Sebagai pemilik produk, saya ingin fondasi yang aman dan dapat diakses, sehingga semua halaman berikutnya mewarisi kualitas yang sama.

#### Acceptance Criteria

1. THE shell dan komponen fondasi SHALL dapat dioperasikan dengan keyboard, memakai peran dan label ARIA yang benar, dan menghormati `prefers-reduced-motion`.
2. THE aplikasi SHALL tidak menyimpan rahasia, token, atau data sensitif di `localStorage`/`sessionStorage`; hanya preferensi UI (tema, bahasa, keadaan sidebar).
3. THE permission di frontend SHALL hanya dipakai untuk UX; setiap keputusan akses tetap ditegakkan backend.
4. THE setiap teks UI SHALL melalui kunci terjemahan dari runtime `i18n-platform` (Requirement 12.5) dan SHALL NOT memakai emoji; satu-satunya pengecualian adalah layar "bootstrap gagal" yang memakai kamus bawaan id/en karena runtime i18n membutuhkan bootstrap (PRD-000 Q36).
5. THE aplikasi SHALL menampilkan batas galat (error boundary) di shell sehingga galat satu halaman tidak merobohkan sidebar dan navbar.

### Requirement 15: Gate pengerjaan M3a

**User Story:** Sebagai pemilik produk, saya ingin bukti bahwa M3a selesai dan tidak merusak yang sudah ada, sehingga blok berikutnya aman dibangun di atasnya.

#### Acceptance Criteria

1. THE `bun run typecheck`, `bun run lint` (Biome), `bun test`, dan `bun run build` SHALL lolos; `task web:check` SHALL mencakup semuanya.
2. THE `task lint arch check:gowork-off test` dan `task contracts:check` SHALL tetap lolos (tidak ada regresi backend).
3. THE build SHALL tetap tertanam di binary (`go:embed`) dan `erpora serve` SHALL menyajikan SPA baru; test integrasi yang ada SHALL tetap lolos.
4. THE `make:module --frontend` SHALL tetap menghasilkan modul yang lolos semua gate dan terdaftar di `web/src/app/modules.ts`; `task test:generator` SHALL dijalankan bila template frontend berubah.
5. THE blok M3a SHALL dianggap selesai bila: men-disable `example` lewat `/settings/modules` menghilangkan menunya tanpa reload, `/example` menampilkan "Modul tidak aktif", dan enable kembali memulihkannya (verifikasi manual sampai E2E Playwright di M3g).
6. THE spec M3a SHALL memenuhi aturan PRD-000, PRD-001, api-guidelines, security-guidelines, dan transaction-data-standards yang relevan untuk frontend, dan perubahan di luar spek SHALL dicatat sebagai keputusan di PRD-000 (tabel Q).

## Pertanyaan terbuka (untuk dikonfirmasi sebelum `/spec-design`)

| # | Pertanyaan | Usulan |
| - | ---------- | ------ |
| 1 | Hosting font Figtree (R2.6) | Self-host `woff2` |
| 2 | Stack test komponen (R3.7) | `bun test` + Testing Library + `happy-dom` |
| 3 | Sumber data notifikasi (R11.5) | Antarmuka + implementasi kosong; kontrak di PRD Notifications |
| 4 | Sumber data pengguna (R12.4) | `CurrentUserSource` sementara; identitas di PRD-001 |
| 5 | Kerangka i18n di M3a (R12.5) | **Diputuskan 29 Okt 2026:** memakai runtime spec `i18n-platform` (ICU, katalog per modul), id default dan en |
| 6 | Dampak aksi modul tanpa endpoint `plan` (R13.8) | Hitung dari dependency/dependents di klien; endpoint `plan` additive bila perlu |
| 7 | Sumber ikon menu (R9.8) | `lucide-react` |
| 8 | Pola aksi halaman (R10.5) | Portal ke slot navbar |
