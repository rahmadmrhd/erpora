# Requirements Document

## Introduction

Revisi M3a (PRD-000 Q40, 31 Okt 2026) memperbaiki dua hal pada hasil M3a yang sudah ter-merge: (1) komponen `@erpora/ui` kini menyimpang dari registry shadcn terbaru karena disalin lalu diadaptasi di tempat; (2) shell sidebar tunggal belum meniru UI/UX ERPNext Desk v16 yang menjadi acuan pemilik produk. Revisi ini mengganti pendekatan komponen menjadi **persis shadcn** (ERP Design System hanya lewat token CSS), membangun ulang shell menjadi **Dock + panel menu** dengan konsep **Desk per modul**, menetapkan **URL bersih**, dan menambah kerangka backend Desk yang dibutuhkan agar Dock, penentuan Desk saat ini, dan aturan tampil dapat bekerja dengan otoritas server.

**Sumber keputusan:** PRD-000 Q40 butir 1–15 (pembagian pekerjaan, komponen persis shadcn, struktur sidebar, Dashboard terkunci, penentuan Desk saat ini, aturan tampil dan edit Desk, URL bersih, Desk bawaan wajib per modul). Pengamatan UI: `docs/superpowers/research/2026-10-31-erpnext-desk-v16-ux-study.md`. Mockup yang disetujui: Dock Pinned/Floating, struktur sidebar, dan tahap breadcrumb (visual companion, 31 Okt 2026).

**Kondisi awal (diverifikasi 31 Okt 2026):** M3a ter-merge (PR #17) dengan 24 komponen di `packages/ui`. Selisih terhadap registry shadcn terbaru (CLI 4.21.5, `shadcn add --dry-run --diff`): `sidebar` +246/−90 baris, `avatar` +62, `alert-dialog` +61, `dropdown-menu` +49, `popover` +45, `button` varian `primary` dan ukuran `sm|md|lg|icon` (shadcn: `default` dan `xs|sm|default|lg|icon*`), `loading` dan `BadgeStatus` buatan sendiri. Rute modul berawalan `/${slug}` (`packages/module-sdk/src/routing.ts`), shell berupa sidebar tunggal, `/settings/modules` milik pseudo-modul `platform`. Backend belum punya konsep Desk; `authz` masih stub (`ERP_AUTHZ_STUB`) sampai PRD-001.

**Di luar revisi ini (non-goals):**
- **M3b:** halaman Desks lengkap (kustomisasi, folder/grup), Desk kustom beserta pemilik dan jangkauan pengguna/peran, penonaktifan manual, **Start Guide**, dialog **Manage Dock** dan **Edit Sidebar** (pengatur susunan), dan API tulis timpaan Desk.
- **M3d:** editor Tiptap, Comments dengan mention. **M3c, M3e, M3f, M3g:** daftar, tree/kanban, dashboard builder dan laporan, PWA dan E2E.
- **Tidak dijadwalkan:** manajemen sidebar kustom yang diperluas (tautan spesifik, laporan).
- Identitas pengguna dan `authz` nyata (PRD-001); revisi ini memakai sumber sementara dan mengikuti kontrak `authz` yang ada.

## Glossary

- **Desk**: kumpulan menu dan satu Dashboard yang dikelompokkan untuk satu tujuan kerja. Desk bawaan **berasal dari satu modul** (satu Desk per modul berfrontend); Desk kustom (M3b) tidak terikat modul.
- **Dock**: rel ikon berisi daftar Desk yang diizinkan, berdiri dalam mode **Pinned** (menempel di tepi kiri) atau **Floating** (pil melayang yang meluncur dari tepi kiri).
- **Panel menu**: panel di sebelah Dock (Pinned) atau satu-satunya sidebar (Floating), berisi grup tetap dan menu Desk saat ini.
- **Grup tetap**: kelompok item yang selalu ada di panel (Cari, Notifikasi, ToDo), dipisah divider dari menu Desk.
- **Desk saat ini**: Desk yang sedang aktif bagi pengguna pada sebuah tab, hasil penentuan berurutan (Requirement 9).
- **Dashboard**: halaman wajib setiap Desk di `/<modul>`; tujuan klik nama Desk di breadcrumb dan item terkunci di grup menu.
- **Penanda state navigasi**: penanda di state riwayat browser (bukan URL) yang membuat `/` menampilkan halaman Desks tanpa menjalankan penentuan Desk.
- **Timpaan Desk**: perubahan pengguna atas definisi Desk bawaan, disimpan terpisah sehingga definisi modul tidak berubah ("Reset to Standard" menghapus timpaan).
- **Shell**, **Registry modul frontend**, **Bootstrap**, **Modul aktif**, **Aksi halaman**, **Token**, **ERP Design System (DS)**: sama dengan glossary spec `m3a-frontend-foundation-shell`.

## Requirements

### Requirement 1: Komponen `@erpora/ui` persis dari registry shadcn

**User Story:** Sebagai pemilik produk, saya ingin komponen yang sama persis dengan registry shadcn terbaru, sehingga pembaruan shadcn cukup di-`add` ulang tanpa merge manual.

#### Acceptance Criteria

1. THE setiap komponen shadcn di `packages/ui/src/components/ui` SHALL identik dengan hasil `shadcn add <komponen>` versi terpin, dengan satu-satunya penyimpangan jalur impor `cn` (`from "cn"` menjadi `from "@erpora/ui/lib/utils"`).
2. THE nama varian dan ukuran SHALL mengikuti bawaan shadcn (mis. `Button`: `default|destructive|outline|secondary|ghost|link`, `default|xs|sm|lg|icon|icon-xs|icon-sm|icon-lg`); varian dan ukuran buatan sendiri (`primary`, `md`) SHALL dihapus.
3. THE pemeriksaan otomatis (skrip `bun`, dijalankan dalam `task web:check`) SHALL menjalankan `shadcn add --dry-run --diff` per komponen dan gagal bila ada selisih selain jalur impor `cn`.
4. THE kebutuhan DS yang tidak punya padanan shadcn (indikator `loading` pada tombol, badge bernada status) SHALL berada di lapisan pembungkus di luar `components/ui` (mis. `components/extended`), dan SHALL membungkus komponen shadcn tanpa mengubah berkasnya.
5. THE komposit buatan sendiri `BadgeStatus`, `EmptyState`, `ErrorPage`, `Kbd`, `LoadingIcon` SHALL diganti padanan resmi shadcn (`Badge`, `Empty`, `Kbd`, `Spinner`) bila padanan itu ada di registry; WHEN tidak ada, THE komposit SHALL pindah ke lapisan pembungkus dan alasannya dicatat di `design.md`.
6. THE komponen baru dari registry SHALL tetap diminta konfirmasi pemilik produk sebelum dipasang (aturan konfirmasi komponen), memuat perbandingan dengan padanan Frappe UI dan DS.
7. THE `@erpora/ui` SHALL tetap daun (F4): tidak mengimpor paket `@erpora/*`; teks bawaan komponen tetap lewat konteks label (`useUiLabel`).
8. [TODO: konfirmasi] **Ukuran kontrol:** shadcn memakai tinggi `h-9` (36 px), sedangkan DS menetapkan 34 px (Button md) dan 32 px (Input sm). Karena komponen tidak boleh diubah, usulan: ukuran shadcn bawaan diterima dan selisih dari DS dicatat; kepadatan hanya dapat dipengaruhi lewat token (mis. `--spacing`) bila diputuskan lain.
9. THE paket SHALL tidak menambah dependensi `cn` dan SHALL menyematkan versi pasti semua dependensi hasil `shadcn add` (aturan pin `package.json` yang ada).

### Requirement 2: Token ERP Design System lewat CSS

**User Story:** Sebagai pemilik produk, saya ingin tampilan tetap mengikuti ERP Design System walau komponen persis shadcn, sehingga warna, radius, dan font tetap konsisten.

#### Acceptance Criteria

1. THE `theme.css` SHALL menjadi satu-satunya tempat nilai warna, radius, dan font DS (variabel CSS semantik shadcn: `--background`, `--primary`, `--sidebar-*`, `--radius`, dst.), dengan tema terang dan gelap.
2. THE komponen dan halaman SHALL memakai token semantik dan SHALL NOT memakai nilai palet langsung (`zinc-100`, `#hex`, `rgb(...)`) di luar `theme.css` (aturan yang ada tetap diuji).
3. THE tipografi SHALL tetap Figtree (self-host `woff2`, keputusan M3a) dan teks UI 14 px.
4. THE fokus keyboard SHALL mengikuti bawaan shadcn (`focus-visible:ring-*`); WHEN kontras gagal 4,5:1 pada token DS, THE token SHALL disesuaikan di `theme.css`, bukan di komponen.
5. THE pasangan teks/latar SHALL memenuhi kontras 4,5:1 di kedua tema.

### Requirement 3: Migrasi pemakaian komponen M3a

**User Story:** Sebagai developer, saya ingin seluruh kode M3a memakai API komponen shadcn baru, sehingga tidak ada pemakaian varian atau ukuran lama yang tersisa.

#### Acceptance Criteria

1. THE seluruh pemakaian `variant="primary"`, `size="md"`, `BadgeStatus`, `EmptyState`, `ErrorPage`, `Kbd`, dan `LoadingIcon` di `packages/*`, `modules/*/web`, dan `web/` SHALL dimigrasi ke API baru; `bun run typecheck` SHALL lolos tanpa celah tipe.
2. THE tes komponen SHALL diperbarui ke perilaku shadcn (mis. peran ARIA dan navigasi keyboard Radix) dan SHALL tidak menguji kelas Tailwind internal komponen shadcn.
3. THE `bun test` SHALL lolos dan jumlah tes yang dihapus SHALL dilaporkan beserta alasannya.
4. THE entry JS awal SHALL tetap ≤ 200 KB gzip (NFR-04); WHEN komponen baru menambah ukuran, THE bagian berat (Dock Floating, Settings, Awesomebar) SHALL dimuat malas.

### Requirement 4: Dock

**User Story:** Sebagai pengguna, saya ingin berpindah Desk dari mana saja lewat Dock tanpa kembali ke halaman Desks, sehingga navigasi cepat.

#### Acceptance Criteria

1. THE Dock SHALL menampilkan satu ikon per **Desk yang diizinkan** (Requirement 12), dengan Desk saat ini disorot dan tooltip berisi nama Desk.
2. WHEN pengguna mengklik ikon Desk, THE aplikasi SHALL menetapkan Desk itu sebagai Desk saat ini dan menuju Dashboard-nya (`/<modul>`).
3. WHERE jumlah Desk melebihi tinggi yang tersedia, THE Dock SHALL menampilkan sebagian dan sisanya lewat ikon "+N" yang membuka daftar lengkap.
4. THE Dock SHALL punya dua mode: **Pinned** (rel menempel di kiri bersama panel menu) dan **Floating** (pil vertikal melayang yang meluncur dari tepi kiri dengan transisi ≈0,28 detik dan menghormati `prefers-reduced-motion`).
5. WHILE mode Floating aktif, THE pil Dock SHALL muncul saat pengguna mendekati tepi kiri atau memakai pintasan keyboard, dan SHALL menutup setelah memilih Desk atau saat fokus keluar.
6. THE mode Dock SHALL dapat dipilih di Settings (Requirement 17) dan disimpan per pengguna; THE bawaan SHALL **Floating**, dan WHEN lebar tersedia < 1024 px, THE shell SHALL memakai sidebar sebagai sheet yang dibuka lewat tombol `☰` tanpa memandang pilihan mode.
7. THE Dock SHALL dapat dioperasikan dengan keyboard (peran `navigation`/`toolbar` yang benar, panah untuk berpindah, Enter untuk memilih) dan setiap ikon SHALL punya nama aksesibel.
8. THE urutan ikon SHALL mengikuti urutan Desk dari backend (Requirement 12); pengaturan urutan oleh pengguna adalah M3b.

### Requirement 5: Panel menu

**User Story:** Sebagai pengguna, saya ingin panel menu yang rapi dengan item global yang selalu ada, sehingga saya tidak mencari fitur umum di setiap Desk.

#### Acceptance Criteria

1. THE panel menu SHALL **tidak** memiliki header Desk (judul dan dropdown Desk dihapus).
2. THE panel SHALL berisi grup berurutan yang dipisah **divider**: (i) **grup tetap**: Cari, Notifikasi (dengan lencana jumlah), ToDo; (ii) **menu Desk saat ini** dengan label kecil nama Desk; (iii) pada mode Floating, pengguna (avatar, nama, email) di dasar.
3. THE grup tetap SHALL dapat bertambah kelak lewat registri item tetap di `@erpora/platform-web` tanpa mengubah komponen panel; THE ToDo SHALL tampil sebagai item dengan sumber data sementara sampai modul tugas ada [TODO: konfirmasi: ToDo hanya penampung rute/lencana tanpa halaman, atau halaman daftar kosong].
4. THE menu Desk SHALL mendukung grup bersarang yang membuka inline (chevron) dan item aktif disorot; Dashboard SHALL selalu menjadi item pertama dan SHALL tidak dapat dihapus atau disembunyikan (terkunci).
5. THE **pembatas** panel SHALL dapat diklik untuk menciutkan atau membuka: pada **Pinned** hanya panel menu yang hilang dan Dock tetap; pada **Floating** sidebar menjadi **strip ikon** (≈48 px) berisi semua grup.
6. THE area klik pembatas SHALL lebih lebar dari 8 px dan kursor SHALL berubah menjadi penanda resize; keadaan menciut SHALL disimpan per pengguna dan bertahan saat muat ulang.
7. THE pintasan keyboard SHALL tersedia untuk menciutkan atau membuka panel dan terdaftar di daftar pintasan.
8. WHEN lebar tersedia < 1024 px, THE panel SHALL tampil sebagai sheet (Requirement 4.6) dan pembatas klik SHALL tidak ditampilkan.
9. THE panel SHALL tidak menampilkan menu yang tidak diizinkan bagi pengguna (daftar datang dari backend yang sudah menyaring).

### Requirement 6: Logo dan popover

**User Story:** Sebagai pengguna, saya ingin satu titik untuk tindakan tingkat aplikasi, sehingga pindah Desk dan bantuan mudah ditemukan.

#### Acceptance Criteria

1. THE logo dan nama proyek SHALL tampil sebagai kepala panel pada mode Floating dan sebagai logo di atas Dock pada mode Pinned.
2. WHEN logo diklik, THE aplikasi SHALL membuka popover yang sama pada kedua mode.
3. THE popover SHALL memuat: **Pindah Desk** (daftar Desk yang diizinkan), **Semua Desk** (membuka halaman Desks), **Edit sidebar** (dinonaktifkan dengan keterangan sampai M3b), **Bantuan dan pintasan**, **Tentang** (versi aplikasi dari bootstrap). [TODO: konfirmasi isi popover; ini usulan perancang]
4. THE popover SHALL dibangun dari `Popover` atau `DropdownMenu` shadcn tanpa mengubah berkasnya dan dapat dioperasikan dengan keyboard.

### Requirement 7: Breadcrumb bertahap

**User Story:** Sebagai pengguna, saya ingin breadcrumb yang tetap berguna di layar sempit, sehingga saya tahu posisi dan dapat naik tingkat.

#### Acceptance Criteria

1. THE breadcrumb SHALL berurutan: ikon **Home** (menuju halaman Desks), **Desk saat ini** (menuju Dashboard Desk), lalu langkah halaman; langkah terakhir (halaman saat ini) SHALL bukan tautan.
2. THE peringkasan SHALL ditentukan oleh **lebar kontainer navbar** (bukan jenis perangkat) setelah ruang tombol aksi dicadangkan, memakai pengukuran kontainer (`ResizeObserver`), bukan media query.
3. WHEN semua langkah muat, THE breadcrumb SHALL **tidak diringkas**.
4. WHEN tidak muat, THE breadcrumb SHALL beralih berurutan: (a) **Opsi A**: Home dan Desk tetap, bagian tengah masuk menu "…", dua langkah terakhir terlihat; (b) **Opsi B**: Desk ikut masuk "…", Home dan dua langkah terakhir terlihat bila muat, bila tidak hanya langkah terakhir; (c) **ponsel/sangat sempit**: hanya langkah terakhir (`… › Halaman`), dan Home ikut masuk "…"; (d) nama halaman dipotong dengan elipsis dan judul penuh tersedia lewat tooltip dan lembar bawah.
5. THE menu "…" SHALL berupa dropdown pada layar lebar dan **lembar bawah** (drawer) pada layar sempit, berisi jalur lengkap dengan tautan setiap langkah.
6. THE aksi sekunder navbar SHALL masuk menu "⋯" lebih dulu sebelum breadcrumb diringkas lebih jauh, dan tombol utama SHALL selalu tampil (ponsel: ikon).
7. THE komponen SHALL dibangun dari `Breadcrumb` (dengan `BreadcrumbEllipsis`), `DropdownMenu`, dan `Drawer` shadcn tanpa mengubah berkasnya.
8. THE label langkah SHALL berasal dari `crumb()` dan katalog i18n; Desk saat ini SHALL berasal dari penentuan Desk, bukan dari rute.

### Requirement 8: Navbar dan aksi halaman

**User Story:** Sebagai pengguna, saya ingin satu tombol utama yang konsisten per halaman, sehingga saya tahu aksi utamanya.

#### Acceptance Criteria

1. THE navbar SHALL berisi tombol collapse/`☰` (sesuai lebar), breadcrumb, dan aksi halaman di kanan.
2. THE halaman SHALL mendeklarasikan satu **aksi utama** dan nol atau lebih **aksi sekunder** lewat mekanisme aksi halaman M3a; aksi sekunder SHALL masuk menu "⋯" saat ruang menyempit.
3. THE aksi utama SHALL tidak pernah hilang; pada lebar sempit SHALL menjadi tombol ikon dengan nama aksesibel.

### Requirement 9: Penentuan Desk saat ini

**User Story:** Sebagai pengguna, saya ingin aplikasi selalu berada di Desk yang masuk akal bagi saya, sehingga tautan yang dibagikan terbuka di konteks kerja saya sendiri.

#### Acceptance Criteria

1. THE aplikasi SHALL menentukan Desk saat ini berurutan: (1) **memori klien per tab** (bertahan saat muat ulang di tab yang sama); (2) **cookie/`localStorage`**; (3) **`default_desk` pada pengguna**; (4) **Desk pemilik utama** menu/halaman yang dibuka; (5) **Desk pertama** dari daftar Desk menu itu yang diizinkan.
2. THE setiap tahap SHALL memeriksa bahwa kandidat **diizinkan** bagi pengguna (ada di daftar Desk server, Requirement 12) **dan memuat halaman yang dibuka**; WHEN salah satu gagal, THE penentuan SHALL lanjut ke tahap berikutnya.
3. THE **setiap perpindahan menu** SHALL memvalidasi Desk saat ini terhadap halaman yang dibuka; WHEN tidak valid, THE aplikasi SHALL beralih ke Desk valid mengikuti urutan di atas tanpa memuat ulang halaman.
4. THE nilai dari `localStorage`/cookie SHALL diperlakukan sebagai masukan tidak tepercaya: dipakai hanya bila id-nya ada di daftar Desk yang diterima dari server, dan hanya berisi id Desk (tanpa data sensitif).
5. THE Desk saat ini SHALL disimpan di state klien (bukan di URL); THE tiap tab SHALL boleh berada di Desk berbeda.
6. WHEN tidak ada Desk yang valid untuk sebuah halaman (halaman diizinkan tetapi tidak masuk Desk mana pun), THE halaman SHALL tetap terbuka dengan breadcrumb `Home › halaman` tanpa segmen Desk.
7. THE `default_desk` pengguna SHALL dibaca dari sumber pengguna sementara dan SHALL tidak dianggap tersedia sampai PRD-001 menyediakannya (nilai kosong lolos ke tahap berikutnya).

### Requirement 10: Routing dan URL bersih

**User Story:** Sebagai pengguna, saya ingin URL yang pendek dan dapat dibagikan, sehingga tautan terbuka di Desk saya sendiri.

#### Acceptance Criteria

1. THE URL SHALL berbentuk: `/<modul>` (Dashboard Desk modul, termasuk `/core`), `/<modul>/<halaman>` (halaman modul, termasuk `/core/users`), dan `/desks/<slug-desk>` (Dashboard Desk kustom, direalisasikan di M3b).
2. THE **semua modul, termasuk core**, SHALL diawali slug modulnya; THE registry SHALL tetap menolak slug ganda dan tercadang, dan `desks` SHALL ditambahkan ke slug tercadang (ADR-0006, `internal/platform/modules/registry/reserved.go` dan tesnya).
3. THE `/` SHALL berfungsi sebagai pintu masuk tanpa konten sendiri: menjalankan penentuan Desk tahap 1–3 (Requirement 9) dan mengarahkan ke Dashboard-nya; WHEN tidak ada yang berlaku, THE `/` SHALL menampilkan halaman Desks.
4. THE halaman Desks SHALL tidak punya URL sendiri; THE `/desks` SHALL tidak terdaftar sebagai rute halaman.
5. WHEN pengguna menekan ikon Home di breadcrumb, THE aplikasi SHALL menampilkan halaman Desks di `/` dengan **melewati** penentuan Desk (penanda state navigasi, bukan URL); WHEN halaman itu dimuat ulang, THE `/` SHALL mengikuti penentuan biasa.
6. THE halaman Desks pada revisi ini SHALL berupa kerangka: daftar Desk yang diizinkan dan keadaan kosong bila tidak ada; kustomisasi, folder/grup, dan pengelolaan adalah M3b.
7. [TODO: konfirmasi] **Halaman platform `/settings/modules`** (pseudo-modul `platform`, spec M3a) menyalahi aturan "semua modul berawalan"; usulan: dipindah ke `/platform/modules` dengan `/settings/modules` dialihkan sementara satu rilis, atau dipertahankan sebagai pengecualian tercatat.
8. THE kontrak `defineModule` dan rute modul SHALL diperbarui sehingga rute anak tetap berada di bawah base route `/<slug>` (tidak berubah) dan tidak ada modul yang dapat mendaftarkan rute di luar base route-nya.

### Requirement 11: Definisi Desk per modul

**User Story:** Sebagai pengembang modul, saya ingin mendeklarasikan Desk bawaan modul saya, sehingga menu dan Dashboard modul tampil di Dock tanpa konfigurasi tambahan.

#### Acceptance Criteria

1. THE setiap modul **berfrontend** (`modules/<dir>/web`) SHALL mendefinisikan **Desk bawaannya**: label (kunci i18n), ikon, susunan menu (grup, item, rute, permission per item, ikon), item Dashboard, dan pengaturan relevan lain; modul tanpa frontend tidak punya Desk. [TODO: konfirmasi penafsiran: modul tanpa `web/` tidak diwajibkan]
2. THE definisi SHALL berada di **manifest** modul sebagai properti `desk` (opsional secara skema, wajib bagi modul berfrontend; tambahan aditif seperti properti `i18n`), karena server memerlukan pemetaan menu ke permission untuk menyaring.
3. THE `module:check` SHALL menolak modul berfrontend tanpa definisi Desk, tanpa item Dashboard, atau dengan item yang merujuk rute/permission yang tidak terdaftar di modul itu, dengan galat yang menyebut berkas dan kunci.
4. THE item Dashboard SHALL wajib dan terkunci: tidak dapat dihapus atau disembunyikan oleh timpaan.
5. THE definisi bawaan SHALL dapat diedit melalui **timpaan** yang disimpan terpisah (Requirement 13); definisi modul SHALL tidak berubah oleh edit pengguna, dan menghapus timpaan memulihkan bawaan ("Reset to Standard").
6. THE modul referensi `example` dan `exampleaddon`, dan templat `make:module --frontend`, SHALL diperbarui agar memuat definisi Desk yang lolos `module:check`; `task test:generator` SHALL dijalankan karena templat berubah.
7. THE satu menu SHALL dapat muncul di beberapa Desk; setiap menu SHALL punya **Desk pemilik utama** (Desk modul yang mendefinisikannya) untuk tahap 4 penentuan Desk.

### Requirement 12: Backend Desk (daftar dan aturan tampil)

**User Story:** Sebagai pemilik produk, saya ingin server yang menentukan Desk dan menu mana yang boleh dilihat pengguna, sehingga klien tidak bisa melihat lebih dari haknya.

#### Acceptance Criteria

1. THE platform SHALL menyediakan endpoint baca `GET /api/platform/desks` (namespace `platform.core`, mengikuti api-guidelines: amplop `data`/`error`, camelCase, additive tanpa versi di URL) yang mengembalikan Desk yang diizinkan bagi principal di context, beserta menu yang diizinkan, urutan, dan Desk pemilik utama tiap menu.
2. THE sebuah Desk SHALL ikut dalam respons hanya bila: (i) modul asalnya **enabled**; (ii) Desk tidak dinonaktifkan; (iii) **minimal satu menu** di dalamnya diizinkan bagi principal lewat `authz.Check`; (iv) lolos aturan kepemilikan dan visibilitas (Desk bawaan terlihat bagi yang berhak; Desk kustom pribadi hanya bagi pemilik, atau bagi pengguna/peran yang ditetapkan, M3b).
3. THE menu yang tidak diizinkan SHALL tidak muncul di respons (tidak dikirim lalu disembunyikan klien).
4. THE penyaringan SHALL memakai kontrak `authz` yang ada (stub saat ini) tanpa membuka jalur baru; parameter tidak boleh membawa `tenantId` atau `userId`, dan tenant diambil dari context.
5. THE endpoint SHALL mendukung `ETag`/`304` dan SHALL berubah revisinya saat status modul, definisi, atau timpaan berubah.
6. THE record Desk di luar scope SHALL diperlakukan seperti tidak ada (404 seragam pada endpoint per-Desk yang ditambahkan kelak); setiap endpoint atau kontrak yang menerima id Desk SHALL punya tes IDOR (pengguna A vs B).
7. THE kontrak OpenAPI platform SHALL diperbarui dan tipe TS dibangkitkan; `task contracts:check` SHALL lolos dan tes kesesuaian route, permission, dan klasifikasi SHALL tetap lolos.
8. THE bootstrap SHALL tidak berisi daftar Desk (agar bootstrap tetap tanpa data per-pengguna); klien SHALL mengambilnya dari endpoint di atas setelah bootstrap.

### Requirement 13: Timpaan dan aturan edit Desk

**User Story:** Sebagai administrator, saya ingin hak mengedit Desk terpisah dari hak melihatnya, sehingga tidak semua orang yang melihat Desk bisa mengubahnya untuk semua pengguna.

#### Acceptance Criteria

1. THE permission **`desk.update`** (terpisah dari hak lihat, dengan scope per Desk) SHALL didaftarkan di platform sebagai resource baru; nama resource, aksi, dan scope final mengikuti PRD-001 dan ditetapkan di `design.md`.
2. THE Desk **bawaan modul**, dan Desk yang diatur untuk beberapa pengguna/peran, SHALL hanya dapat diedit pengguna yang memiliki `desk.update` atas Desk itu; THE Desk **kustom pribadi** SHALL dapat diedit pemiliknya dan pengguna yang memiliki `desk.update` atas Desk itu.
3. THE penegakan SHALL di backend lewat `authz` (klien hanya menyembunyikan kontrol); hak sistem SHALL tidak diberikan otomatis dan setiap penggunaan `authz.AsSystem` SHALL teraudit.
4. THE migrasi SHALL membuat tabel timpaan Desk (kontrol atau tenant sesuai `design.md`) dengan skema goose, tanpa `AutoMigrate`, dan akses GORM hanya di paket infrastruktur yang masuk allowlist D3b.
5. THE jalur **baca** SHALL menerapkan timpaan di atas definisi bawaan pada revisi ini; THE **API tulis** timpaan dan antarmuka pengeditan SHALL dikerjakan di M3b. [TODO: konfirmasi pembagian; alternatif: API tulis ikut di revisi ini]
6. THE setiap perubahan timpaan (kelak, M3b) SHALL tercatat di audit.

### Requirement 14: Dashboard per Desk (kerangka)

**User Story:** Sebagai pengguna, saya ingin setiap Desk memiliki satu Dashboard yang menjadi pintu masuknya, sehingga saya tahu mulai dari mana.

#### Acceptance Criteria

1. THE setiap Desk SHALL punya halaman Dashboard di `/<modul>` yang menjadi tujuan klik nama Desk di breadcrumb dan klik ikon Dock.
2. THE halaman SHALL menyatukan "Home" dan "Dashboard" ERPNext menjadi satu Dashboard; THE sistem SHALL tidak punya halaman Home terpisah.
3. THE revisi ini SHALL menyediakan **kerangka** Dashboard: tata letak dengan slot widget dan keadaan kosong; modul dapat mendaftarkan widget bawaan lewat definisi Desk (isi dan pembangun adalah M3f; kartu "Panduan awal" adalah M3b). [TODO: konfirmasi isi kerangka: hanya keadaan kosong, atau menyertakan kartu contoh]
4. THE Dashboard SHALL memuat bagian beratnya secara malas dan SHALL menampilkan kerangka pemuatan (`Skeleton`) selama data diambil.

### Requirement 15: Panel Notifikasi

**User Story:** Sebagai pengguna, saya ingin melihat notifikasi tanpa meninggalkan halaman, sehingga konteks kerja tidak hilang.

#### Acceptance Criteria

1. WHEN item Notifikasi diklik, THE aplikasi SHALL membuka panel yang meluncur di sebelah panel menu (kolom ketiga di atas konten) dengan tab **Semua** dan **Peristiwa**, aksi tandai semua dibaca, dan tutup.
2. THE keadaan kosong SHALL memakai komponen `Empty` shadcn dengan pesan yang dapat dipahami.
3. THE sumber data SHALL tetap antarmuka sementara dari M3a (kontrak di PRD Notifications); lencana jumlah di panel menu SHALL ikut dari sumber yang sama.
4. THE panel SHALL dapat ditutup dengan `Escape`, fokus SHALL dikembalikan ke item pemicu, dan panel SHALL dapat dioperasikan dengan keyboard.

### Requirement 16: Menu pengguna

**User Story:** Sebagai pengguna, saya ingin satu menu akun yang konsisten, sehingga pengaturan dan keluar mudah dijangkau.

#### Acceptance Criteria

1. THE avatar SHALL tampil di dasar Dock (Pinned) atau di dasar panel bersama nama dan email (Floating) dan membuka menu berisi **Settings**, **Reload**, dan **Logout**.
2. THE Logout dan aksi akun yang bergantung identitas SHALL dinonaktifkan dengan keterangan sampai PRD-001 menyediakannya (sama seperti M3a).
3. THE menu SHALL tidak berisi "My Space" (tidak ada konsepnya di ERPora).

### Requirement 17: Dialog Settings

**User Story:** Sebagai pengguna, saya ingin mengatur tampilan saya sendiri di satu tempat, sehingga aplikasi nyaman dipakai.

#### Acceptance Criteria

1. THE Settings SHALL berupa dialog dua kolom (navigasi kiri, konten kanan) dengan bagian **Appearance** dan **Preferences**.
2. THE **Appearance** SHALL memuat **Tema** (Otomatis/Terang/Gelap, berupa kartu pratinjau) dan **Dock** (Pinned/Floating, berupa kartu pratinjau); [TODO: konfirmasi apakah **Layout** (Compact/Full Width) ikut sekarang atau M3d bersama form].
3. THE **Preferences** SHALL memuat **Bahasa** memakai runtime `i18n-platform` dan **Zona waktu** [TODO: konfirmasi isi minimal Preferences].
4. THE perubahan SHALL tersimpan otomatis per pengguna dan berlaku seketika; penyimpanan SHALL memakai `localStorage` untuk preferensi UI (tema, mode Dock, keadaan panel, bahasa) dan SHALL tidak menyimpan data sensitif.
5. THE dialog SHALL dimuat malas dan dibangun dari `Dialog` shadcn; Settings SHALL dapat dibuka dari menu pengguna dan dari pintasan keyboard.

### Requirement 18: Awesomebar dan pintasan

**User Story:** Sebagai pengguna, saya ingin satu palet perintah untuk berpindah cepat, sehingga saya tidak menelusuri menu.

#### Acceptance Criteria

1. WHEN `Ctrl+K` ditekan atau item Cari diklik, THE aplikasi SHALL membuka palet perintah ("Search or type a command") dengan hasil dari menu Desk yang diizinkan dan daftar Desk, teks yang cocok disorot, dan petunjuk pintasan di footer.
2. THE palet SHALL dibangun dari `Command` dan `Dialog` shadcn, dimuat malas, dan SHALL tidak mengirim kueri ke backend pada revisi ini (pencarian dokumen adalah M3c).
3. THE daftar pintasan global SHALL tersedia (buka palet, tampilkan pintasan, menciutkan panel, buka Dock, ganti tema) dan dapat dilihat dari popover logo dan Settings.
4. THE pemilihan hasil SHALL menetapkan Desk lewat penentuan Desk (Requirement 9) sebelum menavigasi.

### Requirement 19: Internasionalisasi dan aksesibilitas

**User Story:** Sebagai pengguna dan pemilik produk, saya ingin shell yang dapat diterjemahkan dan dapat diakses, sehingga semua orang dapat memakainya.

#### Acceptance Criteria

1. THE setiap teks baru SHALL melalui kunci terjemahan (`useTranslation`, `msg()`, `crumb()`) di namespace `ui` atau `platform` dengan `id` dan `en`; `task i18n:check --strict` dan `i18n:generate --check` SHALL lolos.
2. THE Dock, panel, breadcrumb, panel Notifikasi, popover, Settings, dan palet SHALL dapat dioperasikan dengan keyboard, memakai peran dan label ARIA yang benar, dan menghormati `prefers-reduced-motion`.
3. THE fokus SHALL dikelola pada setiap overlay (jebakan fokus dan pengembalian fokus ke pemicu) memakai perilaku bawaan Radix/shadcn.

### Requirement 20: Keamanan

**User Story:** Sebagai pemilik produk, saya ingin otorisasi tetap di server dan klien tidak membocorkan data, sehingga Desk dan menu aman.

#### Acceptance Criteria

1. THE otorisasi menu dan Desk SHALL ditegakkan di backend; izin di klien hanya untuk UX (PRD-000 Q33) dan setiap endpoint yang melayani data menu tetap memeriksa `authz`.
2. THE tidak ada rahasia atau data sensitif di `localStorage`, `sessionStorage`, atau cookie; hanya id Desk dan preferensi UI.
3. THE nilai tersimpan yang tidak ada di daftar Desk server SHALL diabaikan tanpa galat yang membocorkan informasi.
4. THE `dangerouslySetInnerHTML` SHALL tetap dilarang di seluruh kode frontend.
5. THE bagian **Keamanan** di `design.md` SHALL memuat STRIDE untuk endpoint Desk dan penyimpanan klien, titik masuk ID/IDOR, dan konfirmasi tidak ada koneksi keluar baru.

### Requirement 21: Gate pengerjaan revisi

**User Story:** Sebagai pemilik produk, saya ingin bukti bahwa revisi selesai dan tidak merusak yang sudah ada, sehingga blok M3b aman dibangun di atasnya.

#### Acceptance Criteria

1. THE `task web:check` (typecheck, lint, dependency rules, `i18n:generate --check`, test, build dengan anggaran bundle), `task lint arch check:gowork-off test`, dan `task contracts:check` SHALL lolos; `task test:integration` (PostgreSQL via tunnel) SHALL lolos bila menyentuh DB, dan `task test:generator` SHALL lolos karena templat berubah.
2. THE build SHALL tetap tertanam di binary dan `erpora serve` SHALL menyajikan SPA baru.
3. THE revisi SHALL dianggap selesai bila: (a) semua komponen shadcn lolos pemeriksaan diff (Requirement 1.3); (b) Dock Pinned dan Floating, panel menu, breadcrumb bertahap, dan penentuan Desk bekerja pada modul `example` dan `exampleaddon`; (c) men-disable modul menghilangkan Desk-nya dari Dock tanpa muat ulang; (d) `/` mengikuti penentuan Desk dan Home membuka halaman Desks; (e) verifikasi manual di browser terhadap backend nyata lulus untuk skenario inti.
4. THE spec M3a (`m3a-frontend-foundation-shell`) SHALL dicatat sebagian digantikan (bagian shell, komponen, dan routing) tanpa menghapus isinya, dan PRD-000 Q40, ADR-0011, `docs/module-sdk/frontend.md`, `docs/module-sdk/generators.md`, README `packages/ui`, dan `CLAUDE.md`/`AGENTS.md` (identik) SHALL diperbarui.
5. THE perubahan di luar spek SHALL dicatat sebagai keputusan di PRD-000 (Q-number) atau ADR, dan review akhir SHALL mencakup review keamanan memakai skill `security-audit` untuk permukaan yang disentuh.

## Pertanyaan terbuka (untuk dikonfirmasi sebelum `/spec-design`)

| # | Pertanyaan | Usulan |
| - | ---------- | ------ |
| 1 | Ukuran kontrol shadcn 36 px vs DS 34 px (R1.8) | Terima ukuran shadcn bawaan; selisih dicatat |
| 2 | Isi ToDo di grup tetap (R5.3) | Item dengan lencana dan rute placeholder sampai modul tugas ada |
| 3 | Isi popover logo (R6.3) | Pindah Desk, Semua Desk, Edit sidebar (nonaktif), Bantuan dan pintasan, Tentang |
| 4 | Nasib `/settings/modules` (R10.7) | Pindah ke `/platform/modules`, alihkan lama satu rilis |
| 5 | Modul tanpa frontend dan Desk (R11.1) | Tidak diwajibkan |
| 6 | API tulis timpaan Desk (R13.5) | M3b; revisi ini hanya jalur baca |
| 7 | Isi kerangka Dashboard (R14.3) | Hanya slot widget dan keadaan kosong |
| 8 | Layout Compact/Full Width di Settings (R17.2) | Ditunda ke M3d |
| 9 | Isi minimal Preferences (R17.3) | Bahasa dan Zona waktu |
