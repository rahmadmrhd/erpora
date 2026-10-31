# Implementation Plan: Revisi M3a (shadcn persis, shell Desk v16, Desk per modul)

## Overview

Implementasi berurutan dari fondasi ke permukaan: alat pemeriksaan shadcn dan audit komponen (grup 1), resync komponen `@erpora/ui` dan migrasi pemakaiannya (2), manifest `desk` dan validasi registry (4), backend Desk beserta timpaan baca dan endpoint (5), logika Desk di klien (7), shell baru (8), modul referensi dan templat generator (9), dokumentasi (11), lalu gate dan review akhir (12). Pola yang dipakai ulang: `i18n/admin` untuk endpoint platform, `CheckETag`, pola properti manifest opsional `i18n`, `checkMatchesRaw`, allowlist D3b, golden generator, dan `contracts:generate`. Yang **tidak** berubah: amplop API, kode error, mode single-tenant, lifecycle modul, aturan D1 sampai D11 dan F1 sampai F4, CSP backend. **Ditambahkan:** satu migrasi control (`0004_desk_overrides`), satu endpoint baca (`GET /api/platform/desks`), dua kapabilitas platform (`platform.desks.read`, `platform.desks.update`), slug tercadang `desks`.

Keputusan desain D-1 sampai D-10 dan enam pertanyaan terbuka `design.md` **disetujui pemilik produk pada 31 Okt 2026** (D-1 mengubah mekanisme R1.3 dari "bandingkan langsung ke registry" menjadi "bandingkan ke kunci snapshot"; catat di `requirements.md` R1.3 dan PRD-000 Q40 saat grup 11). Penyimpangan lain dari spek **menunggu konfirmasi**; jangan memutuskan sendiri.

## Tasks

- [x] 1. Prasyarat dan alat shadcn (Kelas beban: Sonnet; 1.1 Haiku)
  - [x] 1.1 Verifikasi baseline
    - Pastikan branch bersih dan M3a ter-merge (PR #17); jalankan `bun install --frozen-lockfile`, `task web:check`, `task lint arch check:gowork-off test` dan catat di "Catatan hasil": jumlah tes frontend, jumlah tes Go, ukuran JS awal gzip (acuan 189,6 KB), versi CLI shadcn terpin (`packages/ui/package.json`). Bila ada yang merah sebelum perubahan, **berhenti dan laporkan**.
    - _Requirements: 21.1_
  - [x] 1.2 Tulis test alat `shadcn-check` (merah dulu)
    - `web/tools/shadcn-check.test.ts` (fungsi murni dengan pohon sementara): (a) berkas sama dengan kunci lolos; (b) berkas diubah satu karakter gagal dan pesan menyebut nama komponen serta `task shadcn:sync`; (c) berkas ada tanpa entri kunci gagal; (d) entri kunci tanpa berkas gagal; (e) normalisasi: `from "cn"` dan `from "@erpora/ui/lib/utils"` menghasilkan hash yang sama; (f) berkas di luar `src/components/ui` (mis. `extended/`) diabaikan.
    - **Validates: Requirements 1.1, 1.3**
  - [x] 1.3 Implementasi `shadcn-sync`, `shadcn-check`, `shadcn-verify`
    - `packages/ui/scripts/shadcn-sync.ts` (`bun run shadcn add <k> -o -y` memakai CLI terpin, ganti `from "cn"` menjadi `from "@erpora/ui/lib/utils"`, hapus dependensi `cn`, pin ulang versi dependensi baru, tulis `shadcn.lock.json`), `shadcn-check.ts` (fungsi murni untuk hash dan perbandingan; I/O di skrip; tanpa jaringan), `shadcn-verify.ts` (daring, `--dry-run --diff` per komponen; keluaran laporan drift, kode keluar 0). `Taskfile.yml`: `shadcn:sync`, `shadcn:verify`, dan `shadcn-check` masuk `web:check`; tanpa `rm`/`cp`/`sed`. Bila alias `cn` di CLI berubah perilakunya, catat di `design.md` D-1.
    - _Requirements: 1.1, 1.3, 1.9, 21.1_
  - [x] 1.4 Audit padanan komponen dan paket konfirmasi (**berhenti untuk konfirmasi pemilik produk**)
    - Jalankan `shadcn add <k> --dry-run` untuk `empty`, `spinner`, `kbd`, `command`, `drawer`, `scroll-area`, `toggle-group`, `radio-group`, `input`, `tabs`, `switch`, `card`, `select`, dan periksa apakah `badge` terbaru punya varian bernada status. Tulis hasil di "Catatan hasil": ada/tidak, ukuran, dependensi yang ditarik, dan padanan Frappe UI serta DS untuk **tiap** komponen (perbandingan dan pro/kontra, aturan konfirmasi komponen R1.6).
    - Sajikan ke pemilik produk satu per satu; hanya komponen yang disetujui yang boleh dipasang di 2.4. Komponen yang tidak disetujui diganti rancangan alternatif di `design.md` sebelum lanjut.
    - _Requirements: 1.5, 1.6_

- [x] 2. Komponen `@erpora/ui` persis shadcn (Kelas beban: Sonnet; 2.2 sampai 2.4 Haiku bila spesifikasi tabel komponen cukup)
  - [x] 2.1 Perbarui tes komponen ke perilaku shadcn (merah dulu)
    - Ubah `*.test.tsx` di `packages/ui/src/components/ui/` ke API shadcn terbaru: peran ARIA dan keyboard Radix; **jangan** menguji kelas Tailwind internal. Hapus tes varian/ukuran lama (`primary`, `md`, `loading` pada Button) dan laporkan jumlah tes yang dihapus beserta alasan. Tambah tes untuk `extended/loading-button` (`aria-busy`, `Spinner`, tombol nonaktif saat loading) dan `extended/status-badge` (`tone` memetakan ke kelas token `status-*`, label dari children).
    - **Validates: Requirements 3.2, 3.3**
  - [x] 2.2 Resync kelompok besar
    - `task shadcn:sync -- sidebar avatar alert-dialog dropdown-menu popover`; tulis ulang pemakaian `sidebar` di shell sementara cukup agar `tsc` paket `ui` lolos (penulisan ulang shell di grup 8). Perbarui kunci snapshot.
    - _Requirements: 1.1, 1.9_
  - [x] 2.3 Resync kelompok sedang dan migrasi varian
    - `task shadcn:sync -- button dialog table breadcrumb collapsible alert sheet`; varian `primary` menjadi `default`, ukuran `md` menjadi `default`, `icon` menjadi `icon`, tanpa prop `loading`.
    - _Requirements: 1.1, 1.2_
  - [x] 2.4 Resync kelompok kecil dan pasang komponen baru yang **sudah disetujui** di 1.4
    - `task shadcn:sync -- badge checkbox label tooltip separator skeleton sonner` ditambah komponen baru dari 1.4 (kemungkinan `command`, `drawer`, `spinner`, `empty`, `kbd`, `scroll-area`, `toggle-group`/`radio-group`, `input`, `tabs`, `switch`, `card`, `select`). Pin versi dependensi yang ditulis CLI.
    - _Requirements: 1.1, 1.5, 1.6, 1.9_
  - [x] 2.5 Token `theme.css` dan lapisan pembungkus
    - `src/styles/theme.css`: variabel semantik shadcn bernilai DS (terang/gelap), tanpa menyentuh tinggi kontrol (selisih 36 vs 34 px dicatat di `design.md`); `theme.test.ts` yang ada tetap lolos (paritas gelap, kontras ≥ 4,5:1, tanpa palet langsung). `src/components/extended/{loading-button,status-badge}.tsx` membungkus `Button`+`Spinner` dan `Badge` tanpa mengubah berkas shadcn (bila 1.4 menemukan `Badge` punya varian setara, `status-badge` cukup memetakan ke varian itu). Perbarui `src/index.ts` (ekspor lapisan pembungkus, hapus `BadgeStatus`/`EmptyState`/`ErrorPage`/`Kbd`/`LoadingIcon`).
    - _Requirements: 1.4, 1.5, 2.1, 2.2, 2.3, 2.4, 2.5_
  - [x] 2.6 Migrasi seluruh pemakaian
    - Ganti `variant="primary"`, `size="md"`, `BadgeStatus`, `EmptyState`, `ErrorPage`, `Kbd`, `LoadingIcon` di `packages/*`, `modules/*/web`, dan `web/` ke API baru. `bun run --filter '*' typecheck`, `lint`, `deps` (F4: `ui` tetap daun), dan `bun test` lolos; laporkan tes yang dihapus. Entry JS awal ≤ 200 KB gzip; bagian berat dimuat malas.
    - _Requirements: 1.7, 3.1, 3.2, 3.3, 3.4_

- [x] 3. Checkpoint — komponen
  - Jalankan `task web:check` (termasuk `shadcn-check`); pastikan tak ada pemakaian varian lama tersisa (`grep` `variant="primary"`, `size="md"`), laporan build mencetak ukuran JS awal, dan jumlah tes dibandingkan baseline 1.1. Tanyakan ke user bila anggaran mendekati batas.

- [x] 4. Manifest `desk` dan validasi registry (Kelas beban: Sonnet; 4.5 Haiku)
  - [x] 4.1 Tulis test skema, manifest, dan registry (merah dulu)
    - `contracts/validate_test.go` (pola `TestValidate_I18nSection`): `desk` valid lolos; ditolak: tanpa `label`/`icon`/`menus`, `menus` > 100, `to` bermuatan skema/host atau `//`, `permission` kosong, grup bersarang dua tingkat, properti tak dikenal. `sdk/module` test `checkMatchesRaw` untuk `Desk`. `registry` test kode baru: `desk_duplicate_id`, `desk_unknown_permission`, `desk_invalid_to`, `desk_label_unknown`, `desk_dashboard_locked`. `reserved_test`: 26 slug, subtest `desks`. `modcheck` test aturan `DESK`: modul dengan `web/` tanpa `desk` ditolak (menyebut berkas dan kunci); modul tanpa `web/` lolos.
    - **Validates: Requirements 10.2, 11.2, 11.3, 11.4**
  - [x] 4.2 Skema, struct Go, dan pembangkit TS
    - `contracts/module.schema.json` (`desk`, `$defs/deskMenu`, `additionalProperties:false`), `sdk/module/manifest.go` (`Desk`, `DeskDashboard`, `DeskMenu`; tag `json`), `task erpora -- contracts:generate` membangkitkan `manifest.gen.ts` yang memuat `desk`; `task contracts:check` lolos. Tidak ada perubahan breaking pada manifest yang ada.
    - _Requirements: 11.1, 11.2_
  - [x] 4.3 Validasi registry dan slug tercadang
    - `internal/platform/modules/registry/desk.go` (lima kode galat baru di `errors.go`; `desk_label_unknown` memakai `CatalogSource`), `reserved.go`: tambah `desks` (25 menjadi 26) dan perbarui `rules_test.go`; **hanya** `internal/app` merakit modul (D7) dan `internal/platform/**` tidak mengimpor `modules/**` (D2).
    - _Requirements: 10.2, 11.2, 11.3_
  - [x] 4.4 Aturan `DESK` di `module:check`
    - `internal/devtools/modcheck/check.go`: modul berdirektori `web/` wajib `desk`; semua temuan registry di 4.3 dilaporkan dengan berkas dan kunci; Dashboard dibangkitkan, tidak boleh dideklarasikan.
    - _Requirements: 11.3, 11.4_
  - [x] 4.5 Manifest modul referensi
    - `modules/example/module.json`: `desk` (label `desk.title`, ikon `file-text`, menu `notes` → `/notes`, permission `example.notes.read`); `modules/exampleaddon/module.json`: menu `summary` → `/summary`, permission `example-addon.summary.read`. Tambah kunci i18n (`desk.title`, `nav.notes`, `nav.summary`) pada katalog `id`/`en` tiap modul; `task erpora -- module:check` lolos untuk keduanya.
    - _Requirements: 11.1, 11.6, 19.1_

- [x] 5. Backend Desk (Kelas beban: Sonnet)
  - [x] 5.1 Migrasi, store, dan allowlist (test dulu)
    - Test integrasi (`-tags integration`, PostgreSQL via tunnel, `-p 2`): `migrations/control/0004_desk_overrides.sql` (skema `design.md` §6) naik dan turun; `store.ListSite(ctx, tenant)` mengisolasi tenant (tenant A tidak melihat baris tenant B); CHECK menolak `desk_id` tidak valid dan `scope` selain `site|user`. Implementasi `internal/platform/desks/store/store.go` (`Store{control database.ControlDB}`, metode menerima `tenancy.TenantID` eksplisit, semua kueri menyaring `tenant_id`; **hanya baca**); tambahkan ke allowlist D3b (`internal/archcheck/rules.go`) beserta tesnya. Tanpa `AutoMigrate`.
    - _Requirements: 13.4, 13.5, 20.1_
  - [x] 5.2 Tulis test `desks.Service` (merah dulu)
    - Tabel tes: modul nonaktif tidak membentuk Desk; menu tanpa izin tidak dikirim; grup tanpa anak dibuang; Desk dibuang bila tak ada menu non-Dashboard yang diizinkan (Dashboard tidak dihitung, D-5); Dashboard selalu item pertama dan `locked`; merge timpaan (sembunyi/urut) tidak pernah menyembunyikan Dashboard; urutan: Platform pertama lalu urutan registry; `ownerDesk` terisi; ETag stabil untuk isi sama dan **berubah bila izin berubah**; `List` tidak menerima `tenantId`/`userId` (tenant dari `tenancy.FromContext`, tanpa tenant → galat). Tes IDOR kontrak: dua principal dengan izin berbeda mendapat respons berbeda tanpa kebocoran silang.
    - **Validates: Requirements 11.7, 12.2, 12.3, 12.4, 12.5, 12.6, 20.1**
  - [x] 5.3 Implementasi `internal/platform/desks`
    - `desk.go` (tipe), `merge.go` (terapkan `config{hidden,order}`, validasi skema `config`, abaikan `hidden` yang memuat Dashboard), `platform.go` (Desk Platform sebagai `module.Desk` Go: menu `Modul` → `/modules`, permission `platform.modules.read`; label di katalog `platform`), `service.go` (`List(ctx) (View, error)`; ETag SHA-256 atas JSON akhir). Penyaringan hanya lewat `runtime.Authorizer.Allow`.
    - _Requirements: 11.4, 11.5, 12.1, 12.2, 12.3, 12.4, 12.5, 13.5_
  - [x] 5.4 Endpoint, kontrak, dan perakitan
    - `internal/platform/desks/api.go` (pola `i18n/admin`: `atomic.Pointer[Backend]`, `Bind`, 503 `not_ready`; `GET /api/platform/desks`, `httpserver.CheckETag`, `Cache-Control: private, no-cache`, `Vary: Cookie, Authorization`; amplop `{"data":{"desks":[...],"revision":"<etag>"}}`). Daftarkan di `buildRegistry` (`platformAPIs.Desks`) dengan `PlatformCapabilities` + `platform.desks.read` dan `platform.desks.update`; `Bind` di `assemble.go`. `contracts/openapi/platform.yaml`: path `/desks` (`operationId: listDesks`, `x-erp-permission: platform.desks.read`, respons 200 dengan `example`, 304, 403, 503), skema `Desk`/`DeskMenuItem`/`DeskMenuGroup`/`DesksEnvelope`, `x-erp-classification: internal`; `task erpora -- contracts:generate`; perbarui `inspect_routes.golden` (`-update`). Tes handler: 200, 304 (`If-None-Match`), 403, 503, dan route **tidak** ada di `public_routes.txt`. Endpoint tidak menerima id; bila ditambah kelak wajib tes IDOR A vs B.
    - _Requirements: 12.1, 12.5, 12.6, 12.7, 12.8, 13.1, 13.3, 20.1, 21.1_

- [ ] 6. Checkpoint — backend
  - `task lint arch check:gowork-off test`, `task contracts:check`, `task test:integration` (VPS via tunnel; jangan memulai container di laptop); `module:check` untuk `example` dan `exampleaddon`; tanyakan ke user bila ada kegagalan di luar perubahan ini.

- [x] 7. Logika Desk di klien dan SDK modul (Kelas beban: Sonnet; 7.4 Haiku untuk dokumentasi kode saja)
  - [x] 7.1 Tulis test `resolveDesk`, `matchMenu`, dan penyimpanan (merah dulu)
    - `packages/platform-web/src/desk/*.test.ts` dengan `fast-check`: kandidat yang tidak ada di daftar server tidak pernah dipilih; urutan tahap 1–5 dan hasil `step`; tahap 4–5 hanya bila `page` ada; kandidat yang tidak memuat `page` dilewati; nilai storage berbahaya (tidak cocok `^[a-z0-9:-]{1,64}$`, terlalu panjang, bukan string) diabaikan tanpa galat; `matchMenu`: Dashboard (`/<slug>`) cocok hanya persis, item lain cocok pada `path === to` atau `to + "/"`, terpanjang menang, `/example2` tidak cocok dengan `/example`; storage melempar (mode privat) → perilaku bawaan tanpa galat.
    - **Validates: Requirements 9.1, 9.2, 9.3, 9.4, 9.6, 9.7, 20.2, 20.3**
  - [x] 7.2 Implementasi fungsi murni dan klien API
    - `src/desk/{types,resolver,match-menu,storage,api}.ts`: tipe `Desk`, `DeskMenuItem`, `DeskMenuGroup`, `DeskList`; `resolveDesk`, `matchMenu`; storage `sessionStorage["erpora.desk.tab"]` dan `localStorage["erpora.desk"]` (try/catch, hanya id); `useDesks` memakai TanStack Query dengan `If-None-Match` lewat klien terketik `@erpora/platform-contracts`.
    - _Requirements: 9.1, 9.2, 9.4, 12.8_
  - [x] 7.3 Provider, validasi navigasi, preferensi, sumber pengguna
    - `src/desk/{provider,validation}.tsx` (`DeskProvider`: `currentDeskId`, `setCurrentDesk`, `showDesks` dari state riwayat; `useDeskValidation` validasi Desk pada tiap perubahan lokasi tanpa muat ulang), `src/prefs/index.ts` (kunci `erpora.dock`, bawaan `floating`), `src/sources/current-user.tsx` (`defaultDesk: null` pada sumber sementara).
    - _Requirements: 4.6, 9.3, 9.5, 9.7, 10.5_
  - [x] 7.4 SDK modul: hapus `navigation`, tolak rute indeks
    - `packages/module-sdk/src/{types,define-module,routing}.ts`: `FrontendModule.navigation` dihapus; `attachModuleRoutes` menolak rute indeks (`path: "/"`) dengan `ModuleDefinitionError`; `@erpora/module-sdk/testing` mendapat `assertDeskRoutes(module, manifestDesk)` (setiap `to` ada di rute terdaftar). `platform-web/src/registry`: hapus `selectNavigation`. Tes tiap perubahan (test dulu).
    - _Requirements: 10.8, 11.3_

- [x] 8. Shell baru (Kelas beban: Sonnet; 8.12 Haiku)
  - [x] 8.1 `fitBreadcrumb` dan property test (test dulu)
    - `packages/ui/src/lib/breadcrumb-fit.ts` fungsi murni `fitBreadcrumb({segments, widths, available, mobile}): Plan`. Properti: Home dan halaman saat ini selalu tersedia (Home bisa di dalam "…"); lebar total tidak melampaui `available` kecuali tahap akhir terpotong elipsis; tahap monoton menyempit bersama `available`; tahap 4 (`mobile`) hanya `… › langkah terakhir`; tidak ada segmen terduplikasi. Ganti `collapseCrumbs` lama dan tesnya.
    - **Validates: Requirements 7.1, 7.2, 7.3, 7.4**
  - [x] 8.2 Tulis test shell (merah dulu)
    - Testing Library, peran ARIA, keyboard: Dock Pinned dan Floating (pil muncul lewat `Ctrl+Shift+/` dan hot-edge, tutup `Escape`/fokus keluar, `+N` saat tinggi kurang, `aria-current` pada Desk aktif); panel tanpa header Desk, urutan grup tetap lalu divider lalu menu Desk, Dashboard item pertama terkunci; pembatas `role="separator"` dapat diklik/Enter/Spasi dan status tersimpan di `erpora.sidebar`; popover logo (lima item, "Edit sidebar" nonaktif); panel Notifikasi (tab Semua/Peristiwa, `Empty`, fokus kembali ke pemicu); Settings (kartu tema dan Dock tersimpan seketika); palet `Ctrl+K`; breadcrumb (Home membuka Desks tanpa resolve, klik Desk menuju Dashboard, drawer di < 480 px); lebar < 1024 px panel menjadi `Sheet`.
    - **Validates: Requirements 4, 5, 6, 7, 8, 15, 16, 17, 18, 19**
  - [x] 8.3 Dock dan Dock Floating
    - `web/src/app/shell/{dock,dock-floating}.tsx`: daftar Desk dari `useDesks()`; `+N` lewat `DropdownMenu` dengan `ResizeObserver`; pil Floating (transisi 280 ms `cubic-bezier(.4,0,.2,1)`, dimatikan `prefers-reduced-motion`), zona hot-edge 12 px, pintasan `Ctrl+Shift+/`; `role="navigation"` berlabel; Dock Floating dimuat malas.
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.7, 4.8, 19.2_
  - [x] 8.4 Panel menu, grup tetap, pembatas
    - `desk-panel.tsx`, `fixed-group.tsx` (registri `registerFixedItem` di `platform-web`: Cari → palet, Notifikasi → panel + lencana, ToDo → `/platform/todo` + lencana sumber sementara), `collapse-edge.tsx` (`role="separator"`, area klik 16 px, `aria-controls`; Pinned: panel saja, Floating: strip ikon ≈48 px via `collapsible="icon"`), `Sheet` di bawah 1024 px tanpa pembatas; tulis ulang `AppShell`/`SidebarProvider` memakai `sidebar` shadcn hasil resync.
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8, 5.9, 4.6_
  - [x] 8.5 Popover logo dan menu pengguna
    - `logo-popover.tsx` (`DropdownMenu`: Pindah Desk submenu, Semua Desk, Edit sidebar `disabled` + tooltip "M3b", Bantuan dan pintasan, Tentang dengan `AppVersion`), `user-menu.tsx` (Settings, Reload, Logout nonaktif sementara; tanpa "My Space").
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 16.1, 16.2, 16.3_
  - [x] 8.6 Navbar dan breadcrumb bertahap
    - `navbar.tsx`, `breadcrumb-nav.tsx` (pengukuran `ResizeObserver` atas navbar dan blok aksi, `fitBreadcrumb`, dropdown ≥ 480 px dan `Drawer` < 480 px dimuat malas, Home → `goToDesks()`, Desk → `setCurrentDesk` + navigasi, label lewat `crumb()`); aksi sekunder masuk "⋯" sebelum breadcrumb diringkas lebih jauh, aksi utama selalu tampil (ikon di < 480 px).
    - _Requirements: 7.5, 7.6, 7.7, 7.8, 8.1, 8.2, 8.3, 9.6_
  - [x] 8.7 Panel Notifikasi
    - `notification-panel.tsx`: `Sheet side="left"` dimulai di tepi kanan panel menu tanpa overlay penuh, `Tabs` Semua/Peristiwa, tandai semua dibaca, `Empty`, `Escape` menutup dan fokus kembali; sumber `NotificationSource` M3a tak berubah; dimuat malas.
    - _Requirements: 15.1, 15.2, 15.3, 15.4_
  - [x] 8.8 Dialog Settings
    - `shell/settings/*` (dimuat malas): `Dialog` dua kolom; Appearance (kartu pratinjau Tema dan Dock lewat `RadioGroup`/`ToggleGroup`), Preferences (Bahasa memakai `useLocale`, Zona waktu `Select`); tersimpan seketika lewat `writePref`; bagian Pintasan dibuka dari popover logo; tanpa data sensitif.
    - _Requirements: 17.1, 17.2, 17.3, 17.4, 17.5, 18.3_
  - [x] 8.9 Palet perintah
    - `command-palette.tsx` (malas): `Command` dalam `Dialog`, sumber lokal menu Desk saat ini dan semua Desk diizinkan, teks cocok disorot, footer petunjuk; pemilihan memanggil `setCurrentDesk` bila lintas Desk lalu navigasi; tanpa kueri backend; `Ctrl+K`.
    - _Requirements: 18.1, 18.2, 18.4_
  - [x] 8.10 Halaman Dashboard, Desks, ToDo, dan pintu masuk
    - `pages/{desk-entry,desks-page,dashboard-page,todo-placeholder}.tsx`: `DeskEntry` menjalankan `resolveDesk` tahap 1–3 lalu `navigate('/<slug>', {replace:true})`, atau `DesksPage` bila `showDesks`/tidak ada hasil; `DesksPage` daftar kartu Desk diizinkan + `Empty`; `DashboardPage` kerangka slot widget + `Empty` + `Skeleton` selama `useDesks()` belum siap; placeholder ToDo.
    - _Requirements: 10.3, 10.4, 10.5, 10.6, 14.1, 14.2, 14.3, 14.4, 5.3_
  - [x] 8.11 Router dan pemindahan halaman platform
    - `web/src/app/router/*` sesuai tabel rute `design.md`: `/`, `/platform`, `/platform/modules`, `/platform/todo`, `/<slug>` (Dashboard, guard modul aktif tetap), `/<slug>/<halaman>`; hapus `/settings/modules` **tanpa pengalihan** dan pindahkan `settings/modules` ke `platform/modules/page.tsx`; `/desks` tidak terdaftar; perbarui tautan, tes, dan dokumen yang merujuk rute lama.
    - _Requirements: 10.1, 10.2, 10.4, 10.7, 10.8_
  - [x] 8.12 Katalog i18n shell
    - Kunci baru di namespace `ui` dan `platform` (`id` dan `en`) untuk semua teks shell baru; `task i18n:generate`, `task i18n:check --strict`, dan `i18n:generate --check` lolos.
    - _Requirements: 19.1_

- [x] 9. Modul referensi dan templat generator (Kelas beban: Haiku untuk 9.1; Sonnet untuk 9.2)
  - [x] 9.1 Frontend `example` dan `exampleaddon`
    - Hapus `navigation` dari `defineModule`; rute indeks lama bergeser ke `/example/notes` dan `/example-addon/summary` (indeks menjadi Dashboard bawaan shell); perbarui tes modul dengan `assertDeskRoutes`; `bun test` dan `typecheck` modul lolos.
    - _Requirements: 10.8, 11.1, 11.6, 14.1_
  - [x] 9.2 Templat `make:module --frontend` dan golden
    - `internal/devtools/templates/module/*`: tambahkan `desk` ke `module.json` templat, hapus `navigation` dan rute indeks dari templat web, perbarui golden generator dan tesnya; hasil generate lolos `module:check`. `task test:generator` dijalankan di 12.2 (±12 menit; Docker + Bun).
    - _Requirements: 11.6, 21.1_

- [ ] 10. Checkpoint — fitur lengkap
  - `task web:check`, `task lint arch check:gowork-off test`, `task contracts:check`; jalankan `task dev:setup` dan `web/dev-api.ts`, buka aplikasi dan pastikan Dock memuat Desk `Platform`, `example`, `exampleaddon`; tanyakan ke user bila ada kejanggalan tampilan.

- [x] 11. Dokumentasi (Kelas beban: Haiku)
  - [x] 11.1 PRD, ADR, dan catatan keputusan
    - ADR-0006 dan PRD-000 (daftar slug 25 menjadi 26, tambah `desks`); ADR-0011 (komponen persis shadcn, mekanisme kunci snapshot D-1, supersede sebagian); PRD-000 Q40: catatan hasil serta pencatatan D-1 sampai D-10 yang menyimpang dari tulisan sebelumnya (nama `platform.desks.update` bukan `desk.update`; R1.3 menjadi kunci snapshot; selisih ukuran 36 vs 34 px). `requirements.md` R1.3 dan R13.1 sudah diubah pada 31 Okt 2026 (bersamaan penulisan tasks); di sini cukup pastikan PRD-000 Q40 memuat keputusan yang sama.
    - _Requirements: 21.4, 21.5_
  - [x] 11.2 Panduan developer
    - `docs/module-sdk/{frontend,generators,creating-a-module}.md` (properti `desk`, rute `/<slug>`, tanpa `navigation`, `assertDeskRoutes`), `packages/ui/README.md` (alur `shadcn:sync`/`check`/`verify`, aturan "persis shadcn", lapisan `extended`).
    - _Requirements: 21.4_
  - [x] 11.3 `CLAUDE.md` dan `AGENTS.md`
    - Ubah **keduanya identik**: aturan komponen persis shadcn (satu-satunya selisih impor `cn`, DS lewat token, kebutuhan tambahan di `components/extended`), `task shadcn:sync|verify`, URL berawalan slug dan `/` sebagai pintu masuk; `TestAgentsMDMatchesClaudeMD` lolos.
    - _Requirements: 21.4_
  - [x] 11.4 Tandai spec M3a sebagian digantikan
    - `docs/specs/m3a-frontend-foundation-shell/{requirements,design,tasks}.md`: catatan di bagian atas bahwa shell, komponen, dan routing digantikan spec ini, tanpa menghapus isi.
    - _Requirements: 21.4_

- [ ] 12. Gate dan review akhir (Kelas beban: Sonnet; 12.4 model tertinggi yang tersedia)
  - [ ] 12.1 Gate lokal penuh
    - `task lint arch check:gowork-off test`, `task contracts:check`, `task web:check` (dengan `ERP_REQUIRE_BUN=1`), `task i18n:check`; formatter/linter dijalankan sekarang (bukan per task); anggaran bundle ≤ 200 KB.
    - _Requirements: 21.1_
  - [ ] 12.2 Integrasi dan generator di VPS
    - `task test:integration` (`go test -tags integration -p 2 ./...`) dan `task test:generator` lewat tunnel SSH ke VPS (`ERP_TEST_DATABASE_URL`); jangan memulai container di laptop; bila tunnel gagal ikuti aturan ulang dan alih ke alamat IP versi lain sebelum melapor.
    - _Requirements: 21.1_
  - [ ] 12.3 Verifikasi manual di browser
    - Dengan `web/dev-api.ts` (backend nyata): enable/disable `example` menghilangkan Desk di Dock tanpa muat ulang; tautan `/example/notes` di tab dengan Desk berbeda tetap membuka halaman dan mengoreksi Desk; `/` mengikuti resolver; Home membuka Desks tanpa resolve; breadcrumb pada beberapa lebar; Dock Pinned dan Floating; klik pembatas; `localStorage`/`sessionStorage` hanya berisi id Desk dan preferensi UI; `erpora serve` menyajikan SPA baru. Simpan temuan.
    - _Requirements: 21.2, 21.3, 20.2_
  - [ ] 12.4 Review akhir independen
    - Satu review menyeluruh oleh agent baru (diff + spec, tanpa kesimpulan implementer): (a) kesesuaian spec/PRD dan regresi; (b) skill `security-audit` terfokus pada perubahan dan permukaan yang disentuhnya (otorisasi dan kebocoran respons Desk, ETag/cache silang, validasi `to`, penyimpanan klien tidak tepercaya, IDOR kontrak, rantai pasok dependensi shadcn baru, SSRF skrip `shadcn:sync`). **Bukan** audit seluruh codebase.
    - _Requirements: 20.5, 21.5_
  - [ ] 12.5 Putaran perbaikan
    - Temuan yang sesuai spek (termasuk Minor) dikerjakan oleh implementer yang sama lewat `SendMessage`; temuan di luar spek ditanyakan dulu dan keputusan dicatat di spec/PRD; satu review atas diff perbaikan saja, diulang hanya bila ada temuan Critical/Important baru.
    - _Requirements: 21.5_

- [ ] 13. Final checkpoint — Ensure all tests pass
  - Semua gate hijau dan tak ada temuan terbuka; laporkan terpisah: yang dikerjakan, yang menunggu konfirmasi (beserta alasan dan rekomendasi), dan yang sengaja tidak dikerjakan (API tulis timpaan, halaman Desks lengkap, Start Guide, Manage Dock/Edit Sidebar, Layout Compact/Full Width, Tiptap, Comments, sidebar kustom diperluas). Blok M3b boleh dimulai setelah ini.

- [ ] 14. Putaran perbaikan 2 (31 Okt 2026): keputusan K1–K14 dan temuan review
  - Sumber: `requirements.md` bagian "Keputusan putaran perbaikan 31 Okt 2026 (K1–K14)", PRD-000 Q40 butir 24, `design.md` D-1a dan D-11 sampai D-14. Dikerjakan sebelum checkpoint akhir 13 (13 diperiksa ulang setelah grup ini). Pertanyaan P-1 dan P-3 diputuskan pemilik produk 1 Nov 2026 (lihat 14.1 dan 14.9). Penyimpangan lain dari spek ditanyakan dulu.
  - Model dan kelas beban: pekerjaan shell (14.1 sampai 14.11) dan backend (14.14, 14.20, 14.21) membutuhkan penilaian; perubahan dokumen dan mekanis (14.13, 14.15 templat, 14.25, 14.26) dapat dikerjakan lebih ringan. Gunakan satu agent per kelas beban dalam satu putaran.

  - [x] 14.1 K1: halaman Desks tanpa sidebar, Dock, atau panel (chrome khusus, D-13)
    - Tes dulu: `/` menampilkan navbar khusus (logo, Search, lonceng, avatar) tanpa Dock, panel, atau breadcrumb Home; kisi kartu gaya `DeskList`; teks lewat i18n. Implementasi: `DesksPage` menyatakan varian chrome `desks` lewat konteks shell.
    - Judul `<h1>` `sr-only` "Desks" sudah ada di `desks-page.tsx` (P-1, diputuskan 1 Nov 2026).
    - _Requirements: 10.3, 10.6, 10.9, 22.4_
  - [x] 14.2 K2: sorotan aktif tanpa outline atau cincin; latar `sidebar-accent`/`accent` pada ikon Desk aktif dan item menu aktif
    - _Requirements: 4.1, 5.4_
  - [x] 14.3 K3: animasi sidebar Pinned (ciut/buka panel) dan panel Notifikasi mulus; tanpa lompatan layout atau meluncur melintasi Dock; hormati `prefers-reduced-motion`
    - _Requirements: 5.6, 15.1, 19.2_
  - [x] 14.4 K4: verifikasi tidak ada scrollbar horizontal di sidebar pada Pinned, Floating, dan sheet di bawah 1024 px (tes dan pemeriksaan browser dengan `web/dev-api.ts`)
    - _Requirements: 5.10_
  - [x] 14.5 K5: divider vertikal antara toggle sidebar dan breadcrumb di navbar
    - _Requirements: 8.1_
  - [x] 14.6 K6: mode Pinned memindahkan Search, Notifikasi (dengan lencana), dan ToDo ke Dock tepat di bawah logo, lalu divider, ikon Desk, dan avatar di dasar; panel Pinned hanya label Desk dan menu Desk; Floating tetap. Tes urutan dan peran ARIA.
    - _Requirements: 5.2, 4.1_
  - [x] 14.7 K7: garis pembatas vertikal Settings setinggi penuh dialog (tidak terpotong)
    - _Requirements: 17.6_
  - [x] 14.8 K8: mode Pinned, klik ikon Desk (termasuk Desk aktif) membuka popover menu Desk bergaya item sidebar; memilih item menavigasi dan menetapkan Desk. Tes keyboard.
    - _Requirements: 4.2, 4.9_
  - [x] 14.9 K9: langkah terakhir breadcrumb tebal dan lebih besar, dibungkus `<h1>` di luar `BreadcrumbPage` (shadcn tidak diubah); judul visual di konten dihapus; tes satu h1 per halaman. Judul galat `ErrorNotice` menjadi `<h2>` (P-3, diputuskan 1 Nov 2026).
    - _Requirements: 7.9, 22.1, 22.2, 22.3, 22.5_
  - [x] 14.10 K10: pindah Desk memakai `resolveDeskSwitchTarget` (`packages/platform-web/src/desk/switch-target.ts` dan `switch-target.test.ts`, di-commit pada `cc36661`; dipakai pada `fdc3c98` dan `51b4280`) pada ikon Dock Floating, item Pindah Desk di popover logo, dan entri lintas Desk di palet; klik nama Desk di breadcrumb dan kartu Desks tetap ke Dashboard.    - _Requirements: 4.2, 6.5, 18.4_
  - [x] 14.11 K11: panel Notifikasi memakai jebakan fokus; klik di luar dan Escape tetap menutup; fokus kembali ke pemicu. Tes keyboard.
    - _Requirements: 15.4, 19.3_
  - [x] 14.12 K12: patch perilaku menghapus penulisan cookie `sidebar_state` lewat `packages/ui/shadcn.patches.json` (shell menulis `localStorage["erpora.sidebar"]`; resync `sidebar` dengan `task shadcn:sync -- sidebar`, perbarui kunci); tes alat (patch terpasang, teks `find` yang hilang gagal keras) dan tes shell (tidak ada penulisan cookie); perbarui `packages/ui/README.md` bagian patch.
    - _Requirements: 1.1, 9.1, 20.2_
  - [x] 14.13 K13: CLI shadcn tetap lewat `bunx shadcn@4.21.5` (keputusan pemilik produk 31 Okt 2026, D-11). Tidak ada pekerjaan kode; MINOR-1 sudah ditutup sebagai risiko diterima. Pemantauan `bun audit` ikut gate 14.27.
    - _Requirements: 1.9_
  - [ ] 14.14 K14: Desk tanpa menu berizin disembunyikan (bukan dinonaktifkan). Pastikan tes `desks.Service` dan frontend mencakup kasus ini. Catatan M3b (filter izin pemilihan menu Desk kustom) tidak dikerjakan di revisi ini.
    - _Requirements: 12.2_
  - [x] 14.15 I-1: templat `make:module` tanpa `--frontend` tidak menghasilkan `desk` di `module.json` (kondisi Frontend di `module.json.tmpl`); tes generator; koreksi `docs/module-sdk/generators.md`. `task test:generator` dijalankan di 14.27.
    - _Requirements: 11.6, 21.1_
  - [x] 14.16 I-2: preferensi zona waktu dipakai formatter tanggal dan waktu (`packages/platform-web/src/format/time.ts`, `resolveTimezone()`/`useTimezone()`), berlaku seketika; tes.
    - _Requirements: 17.3, 17.4_
  - [x] 14.17 I-3: `biome.json` tidak boleh mematikan `noDangerouslySetInnerHtml*` untuk `packages/ui/src/components/ui/*.tsx` dan `src/hooks/*.ts`; selesaikan perubahan setengah jadi di working tree (`biome.json`, `web/tools/lint-rule.test.ts`); tes regresi.
    - _Requirements: 20.4_
  - [x] 14.18 I-4: Dock Floating; menu "+N" di portal memicu `onBlur`/`onMouseLeave` yang menutup pil; abaikan blur dan leave ke dalam konten menu; tes.
    - _Requirements: 4.5_
  - [x] 14.19 Minor 1: panel Notifikasi tidak menutupi Dock saat panel menu diciutkan (offset = max(tepi kanan Dock, tepi kanan kontainer terlihat)); tes.
    - _Requirements: 15.1_
  - [x] 14.20 Minor 2: klien Desk memakai tipe lokal dan cast; hapus `toDesksClient` dan ambil tipe dari `PlatformComponents` (`platform.gen.ts`).
    - _Requirements: 12.1, 12.7_
  - [x] 14.21 Minor 3: kontrak OpenAPI `/desks` mendokumentasikan `400 tenant_unresolved`; `task contracts:check`; bangkitkan ulang TS.
    - _Requirements: 12.7_
  - [x] 14.22 Minor 4: `DashboardPage` menangani status `error` (galat muat dan tombol coba lagi).
    - _Requirements: 14.4_
  - [x] 14.23 Minor 5: hapus duplikasi `deskDashboardPath` dan `findActiveItem` di shell; pakai `deskDashboardPath` dan `matchMenu` dari `@erpora/platform-web`.
    - _Requirements: 9.3_
  - [x] 14.24 Minor 6: barrel `packages/platform-web/src/desk/index.ts` tidak mengekspor helper uji (`makeDesk`, `makeDeskItem`, `makeDashboardItem`, `makeDeskGroup`); pindahkan ke entry pengujian.
    - _Requirements: 21.1_
  - [x] 14.25 Minor 7: dokumen konsisten. Daftar D3b di `CLAUDE.md`/`AGENTS.md` sudah sama dengan `internal/archcheck/rules.go`; perbarui komentar `BadgeStatus` di `web/src/app/platform/modules/modules-table.tsx:24`; periksa ulang sisa rujukan lama.
    - _Requirements: 21.4_
  - [x] 14.26 Minor 8: `shadcn-verify` tanpa argumen ikut memeriksa kunci `hooks/use-mobile`; petakan ke komponen asal (`sidebar`) atau lewati.
    - _Requirements: 1.3_
  - [ ] 14.27 Gate putaran: `task web:check` (`ERP_REQUIRE_BUN=1`), `task i18n:check -- --strict`, `task lint arch check:gowork-off test`, `task contracts:check`, `task erpora -- module:check` untuk `example` dan `exampleaddon`, `task test:generator` (templat berubah), `task test:integration` bila menyentuh DB; `bun audit` dipantau; JS awal tetap ≤ 200 KB gzip (saat ini 197,7 KB).
    - _Requirements: 21.1, 3.4_
  - [ ] 14.28 Review akhir putaran: satu agent baru (diff putaran ini dan spec, tanpa kesimpulan implementer) untuk (a) kesesuaian spec/PRD dan regresi; (b) skill `security-audit` terfokus pada perubahan: penyimpanan klien (`erpora.sidebar`, `erpora.desk`), `resolveDeskSwitchTarget`, popover dan panel, patch perilaku, `shadcn:sync`/`bunx`. Bukan audit seluruh codebase.
    - _Requirements: 20.5, 21.5_
  - [ ] 14.29 Putaran perbaikan dari 14.28: temuan sesuai spek dikerjakan implementer yang sama; di luar spek ditanyakan dulu; satu review atas diff perbaikan saja, diulang hanya bila ada temuan Critical/Important baru.
    - _Requirements: 21.5_
  - [ ] 14.30 K15: ukuran tombol, ikon, dan jarak ikon Dock Pinned disamakan dengan strip ikon Floating (lebih rapat), dengan satu sumber ukuran bersama (tes dulu: ukuran dan jarak dari sumber yang sama)
    - _Requirements: 4.10_
  - [ ] 14.31 K16: avatar di Dock Pinned diperbesar, kontras jelas, dan terpusat horizontal (tes kelas/peran, pemeriksaan browser)
    - _Requirements: 16.1_
  - [ ] 14.32 K17: ikon di Dock Pinned memiliki sorotan latar saat hover seperti item sidebar; hormati fokus keyboard (tes)
    - _Requirements: 4.11_
  - [ ] 14.33 K18: dropdown logo di halaman Desks tidak memuat "Pindah Desk" dan "Semua Desk"; di halaman lain tetap lengkap (tes rute)
    - _Requirements: 6.6_
  - [ ] 14.34 K19: dialog Settings diperbesar secara nyata di desktop; di mode mobile navigasi kiri menjadi tab horizontal di atas konten (tes dan pemeriksaan browser; JS awal tetap ≤ 200 KB gzip)
    - _Requirements: 17.1, 17.7_

## Notes

- **Traceability:** setiap sub-task mereferensi nomor requirement (`R.n`) di `requirements.md`; grup 1 dan 2 menutup Requirement 1 sampai 3, grup 4 Requirement 11, grup 5 Requirement 12 dan 13, grup 7 Requirement 9, grup 8 Requirement 4 sampai 8 dan 14 sampai 19, grup 9 Requirement 10 dan 11, grup 11 dan 12 Requirement 20 dan 21.
- **TDD:** setiap implementasi didahului test yang dijalankan merah ("test dulu"/"merah dulu"), lalu implementasi sampai hijau. Satu commit per sub-task dengan pesan konvensional (`feat(ui): …`, `feat(web): …`, `feat(desks): …`, `test:`, `docs:`, `chore:`) dan trailer `Co-Authored-By` sesuai sistem. **Jangan push atau membuat PR tanpa diminta.**
- **Konfirmasi komponen baru (R1.6):** 1.4 menyajikan perbandingan dan pro/kontra tiap komponen baru; **jangan memasang komponen yang belum disetujui pemilik produk**, dan komponen di luar daftar 1.4 juga berhenti dan ditanyakan.
- **Kelas beban untuk delegasi** (sebut model secara eksplisit): **Haiku** untuk 1.1, 2.2 sampai 2.4 (resync mekanis), 4.5, 8.12, 9.1, dan grup 11 (dokumen); **Sonnet** untuk sisanya (logika inti, integrasi lintas file, test, Go backend, shell); review akhir 12.4 oleh model tertinggi yang tersedia. Satu agent per kelas beban dan dilanjutkan lewat `SendMessage` dalam satu putaran (tanpa review per task); review independen sekali di akhir putaran; compact agent bila task berikutnya tidak berhubungan.
- **Urutan yang menjaga build tetap utuh:** `typecheck` seluruh workspace baru diwajibkan lulus di 2.6 (resync di 2.2 sampai 2.4 sengaja mematahkan pemakai sementara); 4.x tidak bergantung pada 2.x sehingga dapat dikerjakan paralel oleh agent berbeda **bila** berkas tak bersinggungan (`sdk/`, `contracts/`, `internal/**` vs `packages/ui`, `web/`).
- **Anggaran bundle:** JS awal baseline 189,6 KB gzip (batas 200 KB). Dock Floating, Settings, palet, Drawer, dan panel Notifikasi **harus** dimuat malas; `Command` dan `Drawer` hanya di chunk malas; periksa `module-lazy.test.ts` dan laporan anggaran di setiap checkpoint.
- **Windows dan lingkungan:** gate menjalankan `bun test` dari root; `-race` dilewati lokal (CI Linux tetap); path repo tanpa spasi; jangan menambah perintah Taskfile yang memakai utilitas Unix; Docker hanya di VPS.
- **Di luar cakupan (M3b dan seterusnya):** API tulis timpaan dan antarmuka edit Desk, halaman Desks lengkap (kustomisasi, folder/grup, Desk kustom, `/desks/<slug>`), Start Guide, Manage Dock dan Edit Sidebar, Layout Compact/Full Width (M3d), Tiptap dan Comments (M3d), daftar/tree/kanban/dashboard builder/PWA (M3c, M3e, M3f, M3g), identitas dan `authz` nyata (PRD-001), sidebar kustom diperluas (tidak dijadwalkan).
- **Putaran perbaikan 2 (31 Okt 2026):** keputusan K1–K14 pemilik produk dicatat di `requirements.md` (tabel keputusan), PRD-000 Q40 butir 24, `design.md` D-11 sampai D-14, dan ADR-0015. K13 diputuskan tetap `bunx`, tanpa perubahan kode. Pertanyaan P-1 dan P-3 diputuskan pemilik produk 1 Nov 2026 (h1 `sr-only` "Desks"; judul galat `<h2>`). K15–K19 (1 Nov 2026) ditambahkan sebagai 14.30 sampai 14.34. Grup 14 sebagian sudah dikerjakan (centang sesuai commit di `git log`; 14.14 dan 14.27 sampai 14.34 belum); grup 6, 10, 12, dan 13 belum dicentang karena belum ada bukti gate backend lengkap, checkpoint fitur, verifikasi browser, dan review akhir.

## Catatan hasil

### 1.1 Baseline frontend (31 Okt 2026, sebelum perubahan; branch `feat/m3a-revisi-shell-shadcn`)

Baseline Go dicatat oleh jalur Go (bagian ini hanya frontend).

| Ukuran | Nilai |
| ------ | ----- |
| `bun install --frozen-lockfile` | hijau (397 instalasi, tanpa perubahan) |
| `task web:check` (`ERP_REQUIRE_BUN=1`) | hijau, 2 mnt 20 dtk |
| Tes frontend (`bun test` dari root) | 1064 tes di 87 berkas: **1050 lulus, 14 dilewati, 0 gagal** |
| JS awal (gzip level 9) | **189,6 KB** dari 200,0 KB (94,8%), 1 berkas; 8 chunk lazy |
| CLI shadcn terpin (`packages/ui/package.json`, skrip `shadcn`) | `shadcn@4.21.5` via `bunx` (bukan devDependency) |

Setelah 1.2 dan 1.3 (alat shadcn; jalur lain ikut menambah tes): 1142 lulus, 14 dilewati, 0 gagal (1156 tes di 94 berkas); JS awal 189,8 KB. Alat shadcn menambah 32 tes (`web/tools/shadcn-check.test.ts` 18, `shadcn-sync.test.ts` 14) dan tidak menambah bundle (skrip di luar entry). `task web:check` saat ini gagal di langkah `i18n:generate --check` (`modules/example/web/src/i18n/keys.gen.ts` dan `modules/exampleaddon/...` basi) **bukan** akibat 1.2/1.3: berkas itu di luar kepemilikan jalur ini, perlu `task i18n:generate` oleh pemilik jalur modul/i18n. Langkah lain (`shadcn:check`, typecheck, lint, `deps`, test, build) hijau bila dijalankan terpisah.

### 1.3 Keputusan alat shadcn

- **`shadcn.lock.json` awal** = `{ "cli": "4.21.5", "synced": false, "components": {} }`. Komponen sekarang menyimpang dari registry (D-1: 24 komponen berselisih), sehingga **tidak dikunci**. `shadcn-check` melewati pemeriksaan dengan peringatan selama `synced: false`; `shadcn:sync` menghitung `synced` otomatis dan menjadi `true` begitu **semua** berkas di `components/ui` punya entri, sehingga `shadcn-check` diwajibkan sendiri tanpa perubahan `Taskfile` pada tahap resync (2.2 sampai 2.4). `shadcn-check` terpasang di `web:check` (task `shadcn:check`) sejak sekarang.
- **Alias `cn` CLI 4.21.5 tidak berubah perilaku**: registry tetap menulis `import { cn } from "cn"` (bukan jalur `components.json`) dan tetap menambah dependensi `cn` ke `package.json`; D-1 tidak perlu diubah. `shadcn-sync` mengganti `from "cn"` (kutip ganda atau tunggal) hanya pada berkas yang disentuh CLI, menghapus `cn` dari `dependencies`/`devDependencies`, mem-pin dependensi baru atau yang spesifikasinya berubah (versi terpasang, atau awalan `^`/`~` dibuang; rentang lain dilaporkan, tidak ditebak), menjalankan `bun install`, dan menulis kunci. Argumen hanya nama komponen polos (`^[a-z][a-z0-9-]*$`): URL, namespace registry (`@x/y`), jalur, dan flag ditolak (cegah registry tak tepercaya/SSRF). `--prune` membuang entri kunci yang berkasnya dihapus sengaja.
- Hash dihitung atas isi **setelah** normalisasi `cn` dan akhir baris CRLF menjadi LF (checkout Windows). Berkas `*.test.ts(x)` dan subfolder di `components/ui` diabaikan; `extended/` di luar `components/ui` tidak tersentuh.
- `shadcn:verify` (daring, di luar `web:check`, kode keluar 0) menjalankan `add <k> --dry-run --diff`, membuang pertukaran impor `cn`, dan melaporkan komponen yang berbeda dari registry hidup; dicoba pada `badge` dan `button`: keduanya terdeteksi menyimpang (15 dan 86 baris).
- **Belum terverifikasi ujung-ke-ujung:** `shadcn:sync` hanya diuji pada fungsi murninya (nama, `cn`, pin) dan pemanggilan CLI dry-run; eksekusi penuh (menulis berkas + `bun install`) baru terjadi di 2.2 karena tahap ini dilarang mengubah komponen. Temuan lingkungan: di Windows, `bunx shadcn@4.21.5` sempat gagal `MODULE_NOT_FOUND` karena folder sementara `%TEMP%\bunx-*-shadcn@4.21.5` rusak; menghapus folder itu memulihkan. Registry `ui.shadcn.com` juga kadang `ECONNRESET`; ulangi perintah.

### 1.4 Audit padanan komponen (31 Okt 2026, CLI `shadcn@4.21.5`, registry `new-york-v4`, `add <k> --dry-run [--view]`; **belum ada komponen baru yang dipasang**)

Frappe UI hanya rujukan UX (ADR-0011, pustaka Vue); padanan dibaca dari catatan K-24 (`m3a-frontend-foundation-shell/design.md` tabel C), bagian yang tidak dikaji rinci ditandai. Ukuran = berkas hasil registry (byte mentah / gzip). Semua komponen memakai `radix-ui` 1.8.1, `class-variance-authority` 0.7.1, dan `lucide-react` yang sudah ada; "dependensi `cn`" yang disebut CLI adalah alias registry (dihapus oleh `shadcn-sync`).

| Komponen | Ada di registry | Ukuran | Dependensi baru | Frappe UI | DS | Pro / kontra | Rekomendasi |
| -------- | --------------- | ------ | --------------- | --------- | -- | ------------ | ----------- |
| `empty` | ya (1 berkas, 104 baris) | 2,4 KB / 0,7 KB | tidak ada | tidak ada primitif (hanya `ErrorMessage` inline) | tanpa kartu (celah DS; pola teks Notifikasi) | Pro: satu primitif untuk keadaan kosong dan halaman galat (ganti `EmptyState` dan `ErrorPage`); komposisi header/media/title/description/content. Kontra: ikon dan ilustrasi tidak bawaan; teks dari pemanggil | **Pasang** |
| `spinner` | ya (16 baris) | 0,3 KB / 0,2 KB | tidak ada (`Loader2Icon` lucide) | `LoadingIndicator`/spinner | `LoadingIcon` 8 titik (SMIL, tak patuh reduced-motion) | Pro: ringan, `animate-spin` CSS; dasar `loading-button`. Kontra: tampilan beda dari DS (disetujui "persis shadcn"); `aria-label="Loading"` Inggris tertanam (lihat temuan i18n) | **Pasang**; label lewat pembungkus |
| `kbd` | ya (29 baris, ada `KbdGroup`) | 0,9 KB / 0,4 KB | tidak ada | `KeyboardShortcut` setara | `Kbd` 20 px | Pro: ganti `Kbd` buatan sendiri; gaya tooltip bawaan. Kontra: nihil | **Pasang** |
| `command` | ya (`command.tsx` 185 baris; **menimpa `dialog`** yang sudah dalam daftar resync) | 4,8 KB / 1,3 KB | **`cmdk` 1.1.1** (MIT, peer React 19 sah, menarik `@radix-ui/react-{dialog,id,primitive,compose-refs}`) | `CommandPalette`/Awesomebar | `Command` (dikeluarkan dari M3a, kartu ada) | Pro: palet `Ctrl+K` (Awesomebar ERPNext) siap, pencarian fuzzy dan keyboard; dimuat malas. Kontra: dependensi baru (paket ±81 KB tak terkemas); ukuran bundle belum diukur (harus di chunk lazy); pencocokan bawaan tak paham sinonim bahasa | **Pasang** (hanya di chunk lazy) |
| `drawer` | ya (133 baris) | 4,2 KB / 0,9 KB | **`vaul` 1.1.2** (MIT, rilis terakhir 14 Des 2024 (±22 bulan tanpa rilis), paket ±184 KB tak terkemas) | `BottomSheet` | tidak diverifikasi (kartu tidak ditemukan di catatan) | Pro: lembar-bawah ponsel dengan geser-tutup. Kontra: dependensi baru yang tampak tak dirawat; `Sheet side="bottom"` sudah ada dan cukup untuk daftar jalur breadcrumb < 480 px (tanpa gestur geser) | **Jangan pasang**; pakai `Sheet side="bottom"`. Butuh keputusan karena `design.md` menyebut Drawer |
| `scroll-area` | ya (56 baris) | 1,6 KB / 0,6 KB | tidak ada | tidak ada (gulir bawaan) | tanpa kartu (DS menandai ScrollArea tanpa kartu) | Pro: bilah gulir seragam lintas OS. Kontra: tidak dipakai rancangan M3a (panel notifikasi dan menu cukup `overflow-y-auto`); menambah lapisan DOM | **Tunda** (pasang di M3c bila tabel/daftar membutuhkan) |
| `toggle-group` | ya (**2 berkas**: `toggle` + `toggle-group`, 129 baris) | 3,8 KB / 1,5 KB | tidak ada | `TabButtons` (segmented) | tidak diverifikasi | Pro: segmented control. Kontra: tidak dipakai rancangan (Settings memakai `RadioGroup` kartu); menarik `toggle` tambahan | **Tunda** |
| `radio-group` | ya (43 baris) | 1,4 KB / 0,6 KB | tidak ada | tidak ada primitif terpisah | tidak diverifikasi | Pro: dasar kartu pratinjau Tema dan Dock di Settings, akses keyboard Radix. Kontra: nihil | **Pasang** |
| `input` | ya (21 baris; tinggi `h-9` 36 px) | 1,0 KB / 0,5 KB | tidak ada | `TextInput`/`FormControl` | `Input` (32 px sm) | Pro: dasar semua field Settings dan pencarian. Kontra: 36 px vs 32 px DS (diterima R1.8). **Catatan: resync `sidebar` (2.2) akan membuatnya otomatis**, jadi praktis tak dapat ditolak | **Pasang** |
| `tabs` | ya (89 baris; varian `default`/`line`) | 3,8 KB / 1,0 KB | tidak ada | `Tabs` setara | `Tabs` (dikeluarkan dari M3a, kartu ada) | Pro: tab Semua/Peristiwa di panel notifikasi, ARIA Radix. Kontra: nihil | **Pasang** |
| `switch` | ya (33 baris) | 1,4 KB / 0,6 KB | tidak ada | `Switch` setara | `Switch` (aturan DS: hanya pengaturan yang langsung berlaku) | Pro: kecil. Kontra: tak ada permukaan M3a yang memakainya (Settings = radio + select; opsi install = Checkbox) | **Tunda** |
| `card` | ya (92 baris) | 2,0 KB / 0,5 KB | tidak ada | `Card` setara (tidak dikaji rinci) | `Card` (dikeluarkan dari M3a, kartu ada) | Pro: kartu Desk di halaman Desks. Kontra: nihil | **Pasang** |
| `select` | ya (188 baris) | 6,3 KB / 1,6 KB | tidak ada | `Select`/`Autocomplete` | `Select` (dikeluarkan dari M3a, kartu ada) | Pro: zona waktu Settings, kontrol form M3c. Kontra: daftar panjang (zona waktu ±400) tanpa pencarian; berat relatif, dimuat malas di Settings | **Pasang**, daftar zona waktu dipersempit atau memakai `Command` kelak |
| `badge` (varian status) | sudah ada; versi registry hanya `default`, `secondary`, `destructive`, `outline`, `ghost`, `link` | 1,8 KB / 0,7 KB | tidak ada | `Badge` tema × gaya (kaya warna) | `BadgeStatus` 5 nada | **Tidak ada varian bernada status.** `extended/status-badge.tsx` tetap diperlukan (membungkus `Badge`, memetakan `tone` ke kelas token `status-*`) | Sesuai rancangan 2.5 |

**Temuan audit yang butuh keputusan pemilik produk (di luar tabel):**

1. **Teks bawaan Inggris vs i18n (R1.1 vs R1.7 dan aturan i18n proyek).** Berkas registry menulis teks tertanam: `dialog` dan `sheet` dan `sidebar` (`sr-only` "Close"), `dialog` (`Button` "Close" di footer), `breadcrumb` (`aria-label="breadcrumb"`, `sr-only` "More"), `spinner` (`aria-label="Loading"`). Komponen sekarang memakai `useUiLabel` di 5 berkas (`breadcrumb`, `dialog`, `sheet`, `sidebar` tiga tempat, `sonner`); resync persis akan menghapusnya dan menanam teks Inggris di UI berbahasa Indonesia. Opsi: (a) **patch label deklaratif** pada `shadcn-sync` (daftar pengganti string yang kecil dan eksplisit, hash dihitung setelah patch; R1.1 diubah menjadi "persis kecuali jalur `cn` dan patch label terdaftar"); (b) tanpa patch: tutup via prop bawaan (mis. `showCloseButton={false}` lalu tombol tutup berlabel di lapisan `extended`) dan terima teks Inggris pada sisanya; (c) terima teks Inggris untuk teks khusus pembaca layar. **Rekomendasi: (a)**, dicatat di `design.md` D-1 dan R1.1/R1.7 bersama persetujuan pemilik produk.
2. **`sonner` registry menarik dependensi baru `next-themes`** (`useTheme`; kita tidak memakai Next/ThemeProvider sehingga selalu "system", tidak mengikuti tombol tema manual). `Toaster` meneruskan `...props` setelah `theme`, jadi lapisan pembungkus dapat memaksa `theme`. Rekomendasi: pasang apa adanya (dependensi dipin, dimuat di chunk `@erpora/ui/toaster` yang sudah lazy) dan override `theme` di pembungkus; alternatif: tidak me-resync `sonner` (melanggar R1.1). Butuh keputusan.
3. **Efek samping resync `sidebar`** (`add sidebar --dry-run`): menimpa `button`, `separator`, `sheet`, `tooltip`, `skeleton`, membuat `input`, menimpa `src/hooks/use-mobile.ts` (di luar `components/ui`, tidak tercakup kunci snapshot), dan **menambah 16 variabel CSS ke `src/styles/theme.css`** (token DS harus dijaga). `shadcn-sync` melaporkan berkas di luar `components/ui` dan perubahan `theme.css`; tinjau diff sebelum commit. Saran: cakup juga `src/hooks/*.ts` hasil CLI dalam kunci pada 2.2 bila pemilik produk setuju.
4. `packages/ui/README.md` bagian "Memasang komponen baru" (adaptasi di tempat, 3 langkah manual) kini kedaluwarsa dan akan diganti panduan `shadcn:sync` pada 11.2.
5. Registry ini memakai `radix-ui` tunggal (sudah terpasang 1.8.1): tidak ada paket Radix terpisah baru; dependensi baru bersih hanya `cmdk` (bila `command` disetujui) dan `next-themes` (bila `sonner` di-resync).

**Keputusan pemilik produk (31 Okt 2026, lewat pertanyaan interaktif):** dipasang `empty`, `spinner`, `kbd`, `radio-group`, `input`, `tabs`, `card`, `select`; **tidak dipasang** `command` (cmdk), `drawer` (vaul), `scroll-area`, `toggle-group`, `switch` (breadcrumb ponsel memakai `Sheet side="bottom"`); teks Inggris tertanam ditangani patch label deklaratif; `sonner` dipasang apa adanya dengan `next-themes` terpin dan `theme` ditimpa di pembungkus; ukuran kontrol 36 px diterima.

**Koreksi (31 Okt 2026, putaran perbaikan 2; D-1b dan PRD-000 Q40 butir 19):** keputusan di atas bahwa `command` (cmdk) tidak dipasang **dibatalkan** oleh pemilik produk. `command` (cmdk 1.1.1) disetujui dan dipasang (commit `c146d37`) hanya di chunk malas palet. `drawer` tetap tidak dipasang.

### 2.1 sampai 2.6 Hasil (31 Okt 2026)

**Alat (tambahan atas 1.3).** `packages/ui/shadcn.patches.json` memuat patch deklaratif `{component, find, replace, count?, reason}`; `shadcn-sync` menerapkannya setelah normalisasi `cn` (gagal keras bila string asal tidak ditemukan sebanyak `count`, mendeteksi perubahan registry); hash di `shadcn.lock.json` dihitung SETELAH patch; `shadcn-check` memverifikasi hash dan bahwa setiap patch terpasang. Kunci kini mencakup `src/hooks/*.ts` (kunci `hooks/use-mobile`). Kunci `synced: true` sejak 2.4 (28 berkas), jadi `shadcn-check` wajib di `web:check`. Perbaikan galat 1.3: `shadcn-sync` kini benar-benar menghapus dependensi `cn` yang ditulis CLI (sebelumnya dibandingkan dengan keadaan sebelum CLI sehingga tidak tertulis).

**Patch terdaftar (21).** (a) Label: `dialog`/`sheet` (sr-only "Close" → `actions.close`; footer `Button` "Close"), `breadcrumb` (`aria-label="breadcrumb"` → `breadcrumb.label`, sr-only "More" → `breadcrumb.more`), `sidebar` (judul/deskripsi Sheet mobile → `sidebar.title`/`sidebar.description`; tiga "Toggle Sidebar" → `sidebar.toggle`), `spinner` (`aria-label="Loading"` → `status.loading`). Memakai `UiLabel` (komponen baru di `labels.tsx`) atau `useUiLabel()(...)` inline pada atribut; tanpa kunci baru. (b) Palet langsung (R2.2, `theme.test.ts` melarang `text-white`/`bg-black/50` di komponen): `bg-black/50` → `bg-overlay` pada `alert-dialog`, `dialog`, `sheet`; `bg-destructive text-white` → `bg-destructive text-destructive-foreground` pada `button` dan `badge`. Nilai overlay mengikuti token DS (0,8), bukan 0,5 bawaan shadcn (keputusan 4). Patch kelas palet memperluas mekanisme patch label ke token DS; dicatat sebagai perluasan, dalam semangat keputusan pemilik produk (token DS mengalahkan nilai bawaan shadcn).

**`theme.css`.** CLI tidak mengubah `theme.css` (16 variabel sidebar sudah ada); hanya menghapus CSS `LoadingIcon` (`erpora-loading-dot`, tak terpakai). Aturan reduced-motion global yang ada menutup `animate-pulse` (Skeleton) dan `animate-spin` (Spinner). `theme.test.ts` lolos tanpa perubahan (kontras, paritas gelap, tanpa palet langsung).

**Konfigurasi.** `biome.json` (akar repo) mendapat override: berkas registry `packages/ui/src/components/ui/*.tsx` (bukan `*.test.tsx`) dan `src/hooks/*.ts` tidak diformat/di-lint (gaya registry tanpa titik koma; kebenaran dijaga `shadcn-check`).

**Perubahan perilaku akibat shadcn persis** (diterima, perlu diketahui agent shell): `Sidebar` tidak lagi merender `<nav>` dan tidak menyimpan ke `localStorage` (registry memakai cookie `sidebar_state`; `SIDEBAR_STORAGE_KEY` dihapus; shell lama kini mengendalikan `open`/`onOpenChange` dan membungkus isi dengan `<nav aria-label>`); lebar bawaan 16/3 rem ditimpa lewat prop `style` Provider (K-15 14,25/3,25 rem); pintasan Ctrl/Cmd+B tetap aktif saat fokus di kolom isian (bentrok dengan tebal di editor Tiptap M3d, tak bisa dimatikan tanpa mengubah berkas); `SidebarMenuButton` tidak menulis `aria-current` (Link TanStack menulisnya); `BreadcrumbPage` berperan `link` ber-`aria-disabled` dan `BreadcrumbEllipsis` `aria-hidden` (pemicu menu wajib punya `aria-label`); `Alert` hanya `default`/`destructive` dan selalu `role="alert"` (beri `role="status"` bila tak mendesak); `AlertDialog` menutup dengan Esc (Radix) dan tak ada lagi `hideX`/`size="wide"` pada `Dialog` (pakai `showCloseButton`, `className`); `Skeleton` tanpa `aria-hidden`; `TableHead` tanpa `scope`; `useIsMobile` membaca `window.innerWidth` setelah efek (render pertama non-mobile); `Tooltip` `delayDuration` bawaan 0.

**Tes yang dihapus atau diubah (alasan).** Dihapus bersama komposit buatan sendiri: 17 tes `BadgeStatus` (3), `EmptyState` (4), `ErrorPage` (4), `Kbd` lama (2), `LoadingIcon` (4); digantikan `StatusBadge` (3 tes baru), `Empty`/`Kbd`/`Spinner` (tes baru). `ui/sonner.test.tsx` (4) dipindah ke `extended/toaster.test.tsx` (3). Button: tes `loading` (1), peringatan dev `size=icon` tanpa `aria-label` (2), kelas token `h-control-*` (varian/ukuran `primary`/`md`) diganti tes API shadcn dan `LoadingButton` (4 tes). Tes yang menguji kelas Tailwind internal atau adaptasi DS yang hilang dihapus: Alert varian `warning/info/success` dan outline (5), Avatar ukuran/kelas (2→1), Breadcrumb gaya tautan (1), Dialog `hideX`, lebar, overlay (3→1), Sheet `data-side` (1), Skeleton `aria-hidden`/reduced-motion (2→1), Table gaya/`scope` (2→1), Tooltip jeda 300 ms dan permukaan (1), Sidebar `<nav>` bernama, peringatan dev, Ctrl+B di input, persistensi localStorage (3), dan assertion kelas lencana/item aktif (K-16) (diganti tes keadaan terkendali 1); AlertDialog Esc dibalik. Jumlah blok `test()` di `packages/ui`: 159 sebelum, 161 sesudah (di luar tes alat shadcn).

**Pemakaian dimigrasi.** `packages/*` tidak memakai komposit lama; `modules/example` dan `modules/exampleaddon` (`Button loading` → `LoadingButton`, `EmptyState` → `Empty`); `web/src/app/**`: `EmptyState`/`ErrorPage` → `EmptyNotice`/`ErrorNotice` (`web/src/app/components/notices.tsx`, komposisi `Empty`; judul galat tetap `<h1>`), `BadgeStatus` → `StatusBadge`, `LoadingIcon` → `Spinner`, `DialogContent size="wide"` → `className="sm:max-w-4xl"`, `variant="primary"` → `default`. Kontrak `PageAction.variant` (`packages/platform-web`) tetap `"primary"` dan dipetakan ke `default` di `page-actions-view` agar API modul tidak berubah.

**Gate (31 Okt 2026).** `shadcn-check` OK (28 berkas); typecheck, lint, `deps` hijau; `bun test` 1162 lulus, 14 dilewati, 0 gagal (1176 tes, 101 berkas; baseline 1050/14/0); JS awal 190,3 KB gzip (batas 200 KB; baseline 189,6 KB). `task web:check` berhenti di `i18n:generate --check` (berkas `keys.gen.ts` example, exampleaddon, dan `platform-web/src/i18n/keys.platform.gen.ts` basi akibat katalog jalur backend), bukan akibat jalur ini. `bun audit` melaporkan 1 kerentanan moderat `minimatch` melalui `@stoplight/spectral-core`/`openapi-typescript` (`@erpora/web`), bukan dari dependensi baru (`next-themes` bersih); tidak diubah.

### 11.1 Hasil (31 Okt 2026)

- **ADR baru `docs/adr/0015-komponen-persis-shadcn-dan-kunci-snapshot.md`** (Accepted) memuat aturan komponen persis shadcn, kunci snapshot, dan dua penyimpangan terdaftar (patch label dan token). Spec menyebut "ADR-0011"; ADR-0011 sudah berstatus Accepted untuk keputusan M3a, sehingga keputusan baru diberi nomor 0015 dan ADR-0011 ditandai **sebagian Superseded** (keputusan 1 oleh ADR-0015; keputusan 3 `navigation[]` dicabut oleh PRD-000 Q40 butir 21). Penyimpangan penamaan ini perlu dikonfirmasi pemilik produk.
- **PRD-000 Q40** ditambah butir 17 sampai 23: mekanisme kunci snapshot (R1.3), patch label dan token (R1.1), pemasangan komponen (`drawer` tidak dipasang, `command` disetujui, `sonner` dengan `next-themes`), permission `platform.desks.update` (bukan `desk.update`), model Desk di manifest tanpa `navigation` (D-2, D-3, D-5, D-9), penyimpanan `sessionStorage` dan `localStorage` tanpa cookie (D-10), dan keputusan desain yang tidak menyimpang (D-6, D-7, D-8, ambang dan Dock bawaan). Selisih 36 px dan 34 px sudah ada di butir (16).
- **Verifikasi slug 26 (tanpa perubahan):** ADR-0006 butir 7 dan PRD-000 Q37 menyebut 26 slug dengan `desks`; `internal/platform/modules/registry/reserved.go` memuat `ui`, `sdk`, dan `desks`.
- Berkas diubah: `docs/adr/0015-*.md` (baru), `docs/adr/0011-*.md` (status), `docs/prd/PRD-000-inisialisasi-project.md` (Q40).

### 11.2 Hasil (31 Okt 2026)

- `docs/module-sdk/frontend.md`: `defineModule` tanpa `navigation` (field dan error yang berlaku), properti `desk` di manifest dengan aturan `to`/`permission`, rute `/<slug>/<halaman>` dengan `attachModuleRoutes` (indeks milik shell, ditolak bila dari modul), `listModuleRoutePaths`, `assertDeskRoutes`, `/platform/modules`, barrel dan entry sekunder (`@erpora/ui/toaster`, `@erpora/ui/command`), padanan komponen lama, dan batas "belum tersedia" M3b.
- `docs/module-sdk/generators.md` bagian 9 dan `creating-a-module.md`: isi `make:module --frontend` yang sekarang (`desk` di templat `module.json`, `items-page`, rute `/items` tanpa indeks, kunci i18n contoh), aturan `desk` untuk modul ber-`web/`, dan contoh manifest sesuai `modules/example/module.json`.
- `packages/ui/README.md`: bagian "Memasang komponen baru" diganti alur `shadcn:sync`/`shadcn:check`/`shadcn:verify`, patch deklaratif, kunci snapshot, dan lapisan `extended`.

### 11.3 dan 11.4 Hasil (31 Okt 2026)

- `CLAUDE.md` dan `AGENTS.md` identik (salinan byte per byte); `go test ./tests/architecture -run TestAgentsMDMatchesClaudeMD` lolos.
- Spec M3a lama (`requirements.md`, `design.md`, `tasks.md`) mendapat catatan "sebagian digantikan" di bagian atas, tanpa menghapus isi dan tanpa mengubah centang.

### 8.3, 8.4, 8.5 dan perakitan AppShell (31 Okt 2026, jalur S2)

**Berkas** (`web/src/app/shell/`): `dock.tsx` (Dock Pinned, `DockDeskList`, `fitDockCount`), `dock-floating.tsx` (lazy), `desk-panel.tsx` (`DeskPanel`, `PanelSheet`, `findActiveItem`), `fixed-group.tsx`, `collapse-edge.tsx`, `icon-strip-group.tsx` (lazy), `logo-popover.tsx`, `user-menu.tsx`, `shell-context.tsx` (`ShellActions`: palet, Notifikasi, Settings, reload), `desk-navigation.ts`, `global-shortcuts.ts`, `use-compact.ts`, `sidebar-state.ts`, `app-shell.tsx`; registri item tetap di `packages/platform-web/src/fixed-items/` (`registerFixedItem`). `app-sidebar.tsx` dan `nav-menu.tsx` dihapus.

**Keputusan teknis.** (a) `useIsMobile` shadcn (768 px) tidak boleh diubah, jadi shell memakai `useIsCompact` (< 1024 px) dan menggambar `Sheet` sendiri (`PanelSheet`); `SidebarProvider` dikendalikan: >= 1024 px memakai `open` yang disimpan di `erpora.sidebar`, < 1024 px memakai state sheet yang tidak disimpan. (b) Pinned: `Sidebar collapsible="offcanvas"` dengan `left-(--dock-width)`; Floating: `collapsible="icon"`. (c) `CollapseEdge` (`role="separator"`, pola window splitter, area klik 16 px) berposisi `fixed` mengikuti variabel lebar dan transisi 200 ms. (d) `aria-current="page"` hanya untuk kecocokan persis; kecocokan awalan hanya menyorot (`data-active`); Dock memakai `aria-current="true"`. (e) Pintasan global `Ctrl+Shift+L` (ganti tema) dan `Shift+/` (Settings, bagian Pintasan; diabaikan di kolom isian) di `useGlobalShortcuts`; `Ctrl+K` lewat `useCommandPaletteHotkey`; `Ctrl+Shift+/` di `DockFloating`. (f) Palet, panel Notifikasi, Settings, Dock Floating, dan grup strip ikon dimuat malas (dipasang saat pertama dibuka).

**Perbaikan akar masalah.** Pengumuman rute (`RouteAnnouncer`) membacakan judul halaman sebelumnya karena lokasi router berubah lebih dulu daripada route yang cocok; kini menunggu status router `idle`.

**Perubahan perilaku akibat shadcn persis** (diterima): lencana Notifikasi tidak tampil pada strip ikon (shadcn menyembunyikan `SidebarMenuBadge` di mode ikon; titik lewat CSS M3a hilang).

**Gate (31 Okt 2026).** `task web:check` (`ERP_REQUIRE_BUN=1`) hijau: `bun test` 1359 lulus, 0 gagal; JS awal 197,7 KB gzip dari 200 KB (grup strip ikon dimuat malas menurunkan 1,0 KB); `task i18n:check -- --strict` dan `i18n:generate --check` hijau (8.12). **Belum terverifikasi:** tampilan browser (transisi pil, posisi pembatas, hot-edge, strip ikon); offset panel Notifikasi pada Pinned yang diciutkan (container sidebar berada di luar layar sehingga offset 0 dan panel dapat menutupi Dock).

### 3 Checkpoint komponen (bukti, 31 Okt 2026)

- `shadcn-check` OK (28 berkas, catatan 2.6). Pemakaian varian lama sudah dimigrasi (catatan 2.6).
- `task web:check` (`ERP_REQUIRE_BUN=1`) hijau tercatat pada catatan 8.3 sampai 8.5: `bun test` 1359 lulus, 0 gagal; JS awal 197,7 KB gzip dari 200 KB. Dicentang berdasarkan catatan tersebut.

### 4 Hasil (commit grup 4)

- 4.1 `934a276`: tes merah skema, manifest, registry, slug, dan aturan DESK `module:check`.
- 4.2 `366dfda`: properti `desk` pada `contracts/module.schema.json`, struct Go di `sdk/module`, dan tipe TS hasil `contracts:generate`.
- 4.3 `64e974c`: `internal/platform/modules/registry/desk.go`, kode galat `desk_*`, dan slug tercadang `desks` (26 slug).
- 4.4 `b595f7a`: aturan `DESK` di `internal/devtools/modcheck/check.go`.
- 4.5 `675ab4e`: `desk` dan kunci `desk.title` pada `modules/example` dan `modules/exampleaddon`.
- Jumlah tes dan hasil `module:check` tidak tercatat di commit; gate lengkap dicatat di checkpoint 6 (belum dicentang).

### 5 Hasil (commit grup 5)

- 5.1 `5ebb6df`: migrasi `migrations/control/0004_desk_overrides.sql`, `internal/platform/desks/store/store.go` (hanya baca `ListSite`), dan allowlist D3b di `internal/archcheck/rules.go`. Hasil tes integrasi PostgreSQL tidak tercatat di commit.
- 5.2 `49d8025`: tes merah `desks.Service` (izin, modul nonaktif, Dashboard, timpaan, ETag, IDOR kontrak).
- 5.3 `8e5982b`: `internal/platform/desks/{desk,merge,platform,service}.go` (penyaringan `Authorizer.Allow`, merge timpaan, Dashboard terkunci, ETag isi).
- 5.4 `c9bfafa`: `internal/platform/desks/api.go`, `GET /api/platform/desks`, kapabilitas `platform.desks.read` dan `platform.desks.update`, kontrak `contracts/openapi/platform.yaml`, dan perakitan.
- Verifikasi integrasi dan `inspect_routes.golden` belum tercatat terpisah; checkpoint 6 belum dicentang.

### 7 Hasil (commit grup 7)

- 7.1 `fe3d629`: tes merah `resolveDesk`, `matchMenu`, dan penyimpanan Desk.
- 7.2 `ee60226`: `packages/platform-web/src/desk/{types,resolver,match-menu,storage,api}.ts`; kunci prefs `erpora.dock` dan `erpora.desk`.
- 7.3 `44a3dd5`: `provider.tsx`, `validation.ts`, dan `default_desk` pada sumber pengguna (`defaultDesk`, bernilai `null` pada sumber sementara).
- 7.4 `ef39661`: `navigation` dihapus dari `FrontendModule`, rute indeks ditolak, `assertDeskRoutes` di `packages/module-sdk/src/testing.ts`, dan `selectNavigation` dihapus.
- `resolveDeskSwitchTarget` (`switch-target.ts`, K10) berada di luar grup ini; kemudian di-commit pada `cc36661` (lihat 14.10).

### 9 Hasil (commit grup 9)

- 9.1 `9385e26`: `modules/example` dan `modules/exampleaddon` tanpa `navigation`; rute bergeser ke `/example/notes` dan `/example-addon/summary`.
- 9.2 `bfab76f`: templat `make:module --frontend` memakai `desk` dari manifest, tanpa `navigation` dan rute indeks. Templat `module.json` juga menghasilkan `desk` tanpa `--frontend` (temuan I-1, 14.15). `task test:generator` belum tercatat (dijalankan di 12.2 dan 14.27).

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "4.1", "7.1"] },
    { "id": 2, "tasks": ["1.3", "4.2", "7.2"] },
    { "id": 3, "tasks": ["1.4", "4.3", "5.1", "7.3"] },
    { "id": 4, "tasks": ["2.1", "4.4", "5.2"] },
    { "id": 5, "tasks": ["2.2", "4.5", "5.3", "7.4"] },
    { "id": 6, "tasks": ["2.3", "5.4", "8.1"] },
    { "id": 7, "tasks": ["2.4", "8.2"] },
    { "id": 8, "tasks": ["2.5"] },
    { "id": 9, "tasks": ["2.6"] },
    { "id": 10, "tasks": ["3", "6"] },
    { "id": 11, "tasks": ["8.3", "8.4", "8.5", "8.6", "8.7", "8.8", "8.9", "8.10"] },
    { "id": 12, "tasks": ["8.11", "8.12"] },
    { "id": 13, "tasks": ["9.1", "9.2"] },
    { "id": 14, "tasks": ["10"] },
    { "id": 15, "tasks": ["11.1", "11.2", "11.3", "11.4"] },
    { "id": 16, "tasks": ["12.1"] },
    { "id": 17, "tasks": ["12.2", "12.3"] },
    { "id": 18, "tasks": ["12.4"] },
    { "id": 19, "tasks": ["12.5"] },
    { "id": 20, "tasks": ["13"] },
    { "id": 21, "tasks": ["14.12", "14.13", "14.14", "14.15", "14.16", "14.17", "14.20", "14.21", "14.22", "14.23", "14.24", "14.26"] },
    { "id": 22, "tasks": ["14.10", "14.18", "14.19", "14.25"] },
    { "id": 23, "tasks": ["14.1", "14.2", "14.5", "14.7", "14.8", "14.9"] },
    { "id": 24, "tasks": ["14.3", "14.4", "14.6", "14.11"] },
    { "id": 25, "tasks": ["14.30", "14.31", "14.32", "14.33", "14.34"] },
    { "id": 26, "tasks": ["14.27"] },
    { "id": 27, "tasks": ["14.28"] },
    { "id": 28, "tasks": ["14.29"] }
  ]
}
```
