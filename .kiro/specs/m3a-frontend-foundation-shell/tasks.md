# Implementation Plan: M3a Fondasi Frontend dan Shell

## Overview

Implementasi bertahap dari bawah ke atas: prasyarat dan spike (grup 1), toolchain React + Tailwind (2), paket `@erpora/ui` (4 sampai 7), `@erpora/module-sdk` (8), `@erpora/platform-web` (9), perubahan backend kecil di bootstrap (10), shell (12), modul referensi dan generator (13), halaman `/settings/modules` (14), dokumentasi (16), lalu gate akhir (17). Pola yang dipakai ulang: `web/build.ts` + `csp-check.ts`, klien terketik `@erpora/platform-contracts`, `contracts:generate --check`, golden test generator, dan runtime `i18n-platform`. Yang **tidak** berubah: amplop API dan kode error, mode single-tenant, lifecycle modul, aturan D1 sampai D11 dan F1 sampai F3, CSP backend. Tidak ada migrasi dan tidak ada endpoint baru. Keputusan K-1 sampai K-34 di `design.md` sudah diambil atas delegasi pemilik produk (29 Okt 2026); **PK-1 sampai PK-6 belum dikonfirmasi**: implementasi sementara mengikuti rekomendasi dan jangan mengerjakan perubahan di luar spek di luar daftar itu tanpa bertanya.

## Tasks

- [x] 1. Prasyarat dan spike (Kelas beban: Sonnet; 1.2 Haiku)
  - [x] 1.1 Verifikasi prasyarat spec `i18n-platform`
    - Pastikan grup 12 `.kiro/specs/i18n-platform/tasks.md` selesai: `packages/platform-web` ada dengan `I18nProvider`, `useTranslation`, `createTestI18n`, `keys.gen.ts` (platform, ui, modul); `GET /api/platform/i18n/{locale}/{namespace}` dan bidang `i18n` di bootstrap tersedia. Bila belum, **berhenti dan laporkan**.
    - Catat bentuk aktual yang dipakai M3a di bagian "Catatan integrasi" (akhir berkas ini): cara `I18nProvider` mendapat `tenantId` dan `bootstrap.i18n`, lokasi `keys.gen.ts` untuk `ui`, apakah `UiLabels` sudah ada, dan ada tidaknya `modules/example/web/src/i18n/keys.gen.ts`.
    - _Requirements: 12.5, 14.4_
  - [x] 1.2 Verifikasi ulang versi pustaka
    - Jalankan `npm view <paket> time dist-tags --json` untuk semua paket di tabel B `design.md`; untuk tiap paket pilih rilis stabil tertinggi yang berumur ≥ 14 hari **pada hari ini**; perbarui tabel B bila ada yang berubah. `typescript`, `@biomejs/biome`, `@types/bun` tidak diubah.
    - _Requirements: 1.5, 1.6_
  - [x] 1.3 Spike konfigurasi `shadcn` di `packages/ui` (kode spike boleh dibuang, hasil dicatat)
    - Buat `packages/ui` sementara dengan `package.json` (`exports` seperti `design.md` §2) dan `components.json` (alias `@erpora/ui/*`). Jalankan `bun run shadcn add button --dry-run` (atau `--diff`; cek `shadcn add --help` pada versi terpin) lalu `add button` sungguhan. Verifikasi: file masuk ke `packages/ui/src/components/ui`, impor alias terbaca oleh `tsc --noEmit` dari `web`, dan `Bun.build` dari `web` membundelnya; catat dependensi yang ditulis CLI dan pin ulang ke versi pasti.
    - Bila alias self-reference gagal di salah satu (CLI, `tsc`, Bun), catat penyebab dan putuskan alternatif minimal (mis. impor relatif hasil rewrite) di `design.md` K-10 sebelum lanjut.
    - _Requirements: 3.1, 1.5_

- [x] 2. Toolchain React + Tailwind di Bun (Kelas beban: Sonnet; 2.4 sampai 2.6 Haiku)
  - [x] 2.1 Tulis test fungsi build murni (merah dulu)
    - `web/tools/budget.test.ts`: `initialJsGzip(metafile)` menelusuri hanya `kind: "import-statement"` (mengabaikan `dynamic-import`), menjumlah gzip tiap berkas sekali (tanpa dobel hitung chunk bersama), gagal bila > `200 * 1024`; `web/tools/fonts.test.ts`: `extractInlinedFonts` memindahkan `url(data:font/woff2;base64,…)` ke `assets/figtree-<sha8>.woff2` dan mengganti URL ke `/assets/…`, idempoten, no-op tanpa font; `web/tools/head.test.ts`: `injectHead` menyisipkan `<script src>` sinkron (tanpa `type="module"`) tepat setelah `<head>` dan preload font, idempoten; `web/tools/csp-check.test.ts`: `cssViolations` menolak `url(data:font…)` dan `url(http…)`, meloloskan CSS bersih; `cspViolations` tetap menolak skrip inline, handler inline, `javascript:`.
    - **Validates: Requirements 1.3, 1.4**
  - [x] 2.2 Implementasi `web/tools/{budget,fonts,head}.ts` dan `cssViolations` di `csp-check.ts`
    - Fungsi murni (masukan string/objek, tanpa I/O di dalam logika; I/O dilakukan `build.ts`). Laporan anggaran: tabel JS awal, tiap chunk lazy, CSS, font, 15 kontributor `inputs` terbesar pada chunk awal.
    - _Requirements: 1.3, 1.4_
  - [x] 2.3 Konfigurasi toolchain dan `web/build.ts`
    - `web/package.json`: tambah dependensi runtime dan devDependency sesuai `design.md` §1 (versi pasti dari tabel B; `workspace:*` untuk paket repo **yang sudah ada saat ini**: `@erpora/platform-contracts`, `@erpora/module-sdk`, `@erpora/platform-web`; `@erpora/ui` ditambahkan di 4.1 dan `@erpora/module-example`/`@erpora/module-exampleaddon` di 13.2 agar `bun install --frozen-lockfile` tidak gagal); jalankan `bun install` sehingga `bun.lock` terkunci. `web/tsconfig.json`: `jsx: react-jsx`, `include` memuat `src`, `tools`, `test`. `bunfig.toml` (root: `[test] preload`), `web/bunfig.toml` (`[serve.static] plugins` dan `[test] preload`).
    - `web/src/index.html` (`<div id="root">` + `<script type="module" src="./main.tsx">`), `main.tsx` placeholder (merender "ERPora" lewat React untuk membuktikan pipeline), `styles.css` (`@import "tailwindcss" source(none)` + `@source` untuk `packages/ui`, `packages/platform-web`, `modules/*/web`; baris `@import "@erpora/ui/theme.css"` ditambahkan di 4.3), `theme-init.ts` (mandiri, ±15 baris), hapus `main.ts`.
    - Tulis ulang `web/build.ts` sesuai `design.md` §1: `plugins: [tailwind]`, `splitting: true`, `metafile: true`, `define NODE_ENV=production`, build `theme-init` terpisah, `extractInlinedFonts`, `injectHead`, `cspViolations`/`cssViolations`, `reportBudget`; pertahankan pembersihan `dist` (`.gitkeep`). `dev.ts` tetap (plugin dari bunfig).
    - **Verifikasi di repo:** `bun run build` sukses; `dist/index.html` memuat `<script src>` theme-init sebelum stylesheet; CSS tanpa `data:font`; `@source` memindai `packages/ui` dan `modules/*/web`; chunk lazy terbentuk dari `import()` percobaan. Bila `bun-plugin-tailwind` gagal, pakai cadangan `@tailwindcss/cli` (hasilkan CSS sebelum `Bun.build`) dan catat di `design.md` Risiko.
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 4.4_
  - [x] 2.4 Biome: larang `dangerouslySetInnerHTML` (test dulu)
    - `biome.json`: `linter.rules.security.noDangerouslySetInnerHtml` dan `noDangerouslySetInnerHtmlWithChildren` = `error`; pastikan `includes` memuat `**/*.tsx`. `web/tools/lint-rule.test.ts`: jalankan `biome lint --stdin-file-path=x.tsx` pada kode berisi `dangerouslySetInnerHTML` dan pastikan gagal; kode bersih lolos.
    - _Requirements: 3.6_
  - [x] 2.5 Aturan arsitektur F4 (test dulu)
    - Test `web/tools/depcruise-rules.test.ts`: bangun pohon sementara (`packages/ui/src/x.ts` mengimpor `@erpora/platform-web`, `@erpora/module-sdk`, `@erpora/platform-contracts`, `@erpora/module-example`) dan jalankan `cruise` dengan konfigurasi repo; harapkan pelanggaran `F4-ui-is-a-leaf` untuk tiap impor, dan nol pelanggaran untuk impor `react`/`radix-ui`. Tambahkan aturan F4 ke `.dependency-cruiser.cjs` (komentar atas diperbarui). Aturan F1 sampai F3 tidak berubah.
    - _Requirements: 3.4, 1.5_
  - [x] 2.6 Taskfile dan CI
    - `Taskfile.yml`: `web:check` ditambah `bun test --pass-with-no-tests` (dari root) dan `bun run --filter @erpora/web build`; task baru `web:test`. `.github/workflows/ci.yml` job `web-checks`: tambah langkah `bun test --pass-with-no-tests` dan `bun run --filter @erpora/web build`. Tanpa `rm`/`cp`/`sed`.
    - _Requirements: 15.1_
  - [x] 2.7 Setup uji komponen
    - `web/test/dom.ts` (`GlobalRegistrator.register({ url: "http://localhost/" })`, **berkas terpisah dan dimuat pertama**) dan `web/test/setup.ts` (matcher `jest-dom` + deklarasi tipe untuk `bun:test`, `cleanup` setelah tiap test, stub `ResizeObserver`, polyfill `Element.prototype.hasPointerCapture/scrollIntoView` bila perlu). Tambahkan `web/test/smoke.test.tsx` (Radix DropdownMenu terbuka lewat klik dan menutup lewat Esc, Dialog berperan `dialog`) sebagai bukti stack. Paket yang punya test (`packages/platform-web` yang sudah punya skrip `test`, lalu `packages/ui`, `packages/module-sdk`, `modules/*/web` saat dibuat) menyertakan `bunfig.toml` dengan `[test] preload` relatif ke dua berkas itu agar `bun test` di dalam paket juga bekerja.
    - _Requirements: 3.5, 3.7_

- [x] 3. Checkpoint — toolchain
  - Jalankan `bun install --frozen-lockfile`, `bun run --filter '*' typecheck`, `bun run --filter '*' lint`, `bun run --filter @erpora/web deps`, `bun test --pass-with-no-tests`, `bun run --filter @erpora/web build`; pastikan laporan build mencetak ukuran JS awal dan tidak ada pelanggaran CSP. Tanyakan ke user bila anggaran sudah mendekati batas.

- [x] 4. Fondasi `@erpora/ui`: paket, tema, font, utilitas (Kelas beban: Sonnet; 4.1 Haiku)
  - [x] 4.1 Kerangka paket `packages/ui`
    - Dari hasil spike 1.3: `package.json` (`@erpora/ui`, `exports`, `sideEffects`, `peerDependencies` react/react-dom, dependensi dipin), `tsconfig.json` (`strict`, `noUncheckedIndexedAccess`, `jsx`), `components.json`, `src/index.ts` (kosong dulu), `src/lib/utils.ts` (`cn` = `twMerge(clsx(...))`), `README.md` singkat (cara `bun run shadcn add`, aturan "komponen milik repo", larangan nilai palet langsung). Tidak ada impor paket repo lain.
    - _Requirements: 3.1, 3.4_
  - [x] 4.2 Test tema dan token (merah dulu)
    - `src/styles/theme.test.ts`: (a) paritas blok `.dark` dan `@media (prefers-color-scheme: dark)` (**P15**); (b) semua token wajib (`design.md` §2) ada di `:root` dan `.dark`; (c) kontras dihitung dari nilai `theme.css` (util konversi oklch ke sRGB di test): pasangan `foreground`/`background`, `card`, `popover`, `muted-foreground` di `background`/`muted`/`secondary`/`sidebar`, `sidebar-foreground`/`sidebar`, `sidebar-accent-foreground`/`sidebar-accent`, `primary-foreground`/`primary`, `destructive-foreground`/`destructive`, dan pasangan `status-*` ≥ 4,5:1 di kedua tema; `ring` terhadap `background` dan `muted` ≥ 3:1 (angka K-14); (d) pemindai `src/components/**/*.tsx` menolak kelas palet langsung (`zinc-`, `gray-`, `slate-`, `red-`, … dan `#hex`/`rgb(`) di luar `theme.css` (R2.2).
    - **Validates: Requirements 2.1, 2.2, 2.4, 2.5**
  - [x] 4.3 `src/styles/theme.css` dan font
    - Implementasi sesuai `design.md` §2: token DS dengan penyesuaian K-14, `@theme inline`, `@custom-variant dark`, blok `.dark` dan salinan `@media`, token ukuran kontrol (28/34/40 px) dan `--sidebar-width: 14.25rem`, `--sidebar-width-icon: 3.25rem`, `--navbar-height: 3rem`, lapisan dasar (border, body `text-sm`, fokus 2 px `ring` offset 3 px, `prefers-reduced-motion`). `@font-face` Figtree variabel dengan jalur relatif ke berkas dari `@fontsource-variable/figtree` (salin atau rujuk lewat `node_modules` sesuai hasil spike); tambahkan dependensi `@fontsource-variable/figtree` dan `tw-animate-css`.
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7_
  - [x] 4.4 Label bawaan dan ikon (test dulu)
    - Test: `useUiLabel()` tanpa provider mengembalikan kunci; dengan `UiLabelsProvider` memanggil fungsi `t`; `resolveIcon` mengenali nama terdaftar dan mengembalikan `undefined` untuk nama asing; `initialsOf("Rahmad Maulana") === "RM"`, nama satu kata, kosong → `"?"`. Implementasi `src/labels.tsx` dan `src/icons.ts` (±40 ikon lucide, impor bernama) serta ekspor `IconName`.
    - _Requirements: 9.8, 14.4_

- [x] 5. Komponen `@erpora/ui` sederhana (Kelas beban: Haiku; spesifikasi lengkap di tabel C `design.md`)
  - [x] 5.1 Tulis test komponen sederhana (merah dulu)
    - Satu `*.test.tsx` per komponen di `packages/ui/src/components/`: **Button** (varian primary/secondary/outline/ghost/destructive, ukuran sm/md/lg/icon, `loading` → `aria-busy` + `LoadingIcon`, `size="icon"` tanpa `aria-label` memicu peringatan dev), **LoadingIcon** (delapan titik, `aria-label` diteruskan, tanpa SMIL), **Separator**, **Skeleton**, **Kbd**, **Label**, **Badge**, **BadgeStatus** (`tone` → kelas token `status-*`, label dari children), **Avatar** (gambar gagal → inisial), **Checkbox** (Spasi mengubah, `aria-checked`), **Alert** (peran `alert` untuk destructive, `status` lainnya), **EmptyState**, **ErrorPage** (judul/deskripsi/aksi; `status` 404/403/500/503).
    - **Validates: Requirements 3.5, 14.1**
  - [x] 5.2 Pasang dan adaptasi komponen
    - Jalankan `bun run shadcn add button separator skeleton label badge avatar checkbox alert` (versi CLI terpin), lalu adaptasi: token DS (tanpa palet langsung), varian dan ukuran DS, `loading` pada Button, denyut Skeleton mati di reduced-motion, Alert hanya tampilan outline (K-14). Tulis komposit `LoadingIcon` (CSS keyframes), `Kbd`, `BadgeStatus`, `EmptyState`, `ErrorPage` tanpa `shadcn add`. Pin ulang versi dependensi yang ditulis CLI.
    - _Requirements: 2.2, 3.1, 3.2, 14.1_
  - [x] 5.3 Ekspor barrel dan periksa aturan
    - `src/index.ts` mengekspor semua komponen dan tipe (`StatusTone`, `IconName`); jalankan `bun run --filter '*' typecheck lint`, `deps` (F2 dan F4 hijau).
    - _Requirements: 3.2, 3.4_

- [x] 6. Komponen `@erpora/ui` interaktif dan Sidebar (Kelas beban: Sonnet)
  - [x] 6.1 Tulis test komponen interaktif (merah dulu)
    - **Tooltip** (muncul saat fokus/hover, teks saja), **DropdownMenu** (buka dengan klik dan keyboard, panah, Esc, radio group, submenu, item `destructive`), **Popover** (buka, tutup klik luar dan Esc), **Collapsible** (Enter/Spasi, `aria-expanded`), **Dialog** (fokus terperangkap, Esc menutup, tombol tutup berlabel dari `UiLabels`, `aria-labelledby`), **AlertDialog** (tidak tertutup lewat Esc/klik luar... sesuai DS hanya Batal/aksi; fokus awal pada Batal), **Sheet** (sisi `left`, tombol tutup berlabel), **Breadcrumb** (`nav` bernama dari `UiLabels`, item terakhir `aria-current="page"`, elipsis membuka menu), **Table** (peran `table`, header `th scope`), **Toast** (`toast.error` dengan aksi "Coba lagi", wilayah `aria-live`).
    - **Validates: Requirements 3.5, 14.1**
  - [x] 6.2 Pasang dan adaptasi komponen interaktif
    - `bun run shadcn add tooltip dropdown-menu popover collapsible dialog alert-dialog sheet breadcrumb table sonner`; adaptasi sesuai tabel C: teks bawaan lewat `useUiLabel`, `DropdownMenuItem variant="destructive"`, `Dialog` dengan `align`/`forceAsDialog`/lebar 512 dan 896 px, `Toaster` gaya token (tengah atas, 5 detik, tombol tutup), tautan breadcrumb bergaya teks muted, animasi menghormati reduced-motion.
    - _Requirements: 2.2, 3.1, 3.2, 14.1_
  - [x] 6.3 `collapseCrumbs` dan property test (test dulu)
    - `src/lib/breadcrumb.ts`: `collapseCrumbs(crumbs, { compact })` → `{ visible, folded }` (`n ≤ 3` dan tidak compact: semua; selain itu `[pertama, elipsis, terakhir]`, compact: `[elipsis, terakhir]`). Property test `fast-check` **P4**: terakhir selalu halaman, `visible + folded` selalu memuat semua masukan, tidak lebih dari 3 tingkat terlihat.
    - **Validates: Requirements 10.2**
  - [x] 6.4 Test Sidebar (merah dulu)
    - Tiga keadaan (diperluas, terlipat ke ikon dengan tooltip nama, `Sheet` di layar < 768 px dengan `matchMedia` palsu), Ctrl+B dan Cmd+B membalik keadaan, tombol `SidebarTrigger`, persistensi di `localStorage["erpora.sidebar"]` dan pemulihan saat dimuat ulang, nilai rusak/`localStorage` melempar → diperluas, `<nav>` bernama, navigasi keyboard penuh (Tab/panah/Enter), `SidebarMenuBadge` dan titik saat terlipat.
    - **Validates: Requirements 9.3, 9.4, 9.5, 9.7, 14.1**
  - [x] 6.5 Pasang dan adaptasi Sidebar
    - `bun run shadcn add sidebar`; adaptasi: lebar dari token (14,25/3,25 rem), penyimpanan `localStorage` dengan `try/catch` menggantikan cookie, label lewat `UiLabels` ("Toggle Sidebar", judul/deskripsi Sheet), `side="left"` `collapsible="icon"`, item terpilih `sidebar-accent` (K-16), hapus `Rail`, varian `floating`/`inset` yang tidak dipakai dibiarkan tetapi tidak diekspor. Ctrl/Cmd+B tetap.
    - _Requirements: 9.3, 9.4, 9.5, 9.7_

- [x] 7. Checkpoint — `@erpora/ui`
  - Jalankan `bun test`, `typecheck`, `lint`, `deps`, `build`; semua komponen di tabel C ada dan bertest; larangan palet langsung (4.2d) hijau; tanyakan ke user bila ada komponen/varian di luar tabel yang dibutuhkan.

- [x] 8. `@erpora/module-sdk` (Kelas beban: Sonnet; 8.4 Haiku)
  - [x] 8.1 Tulis test `defineModule` dan tipe (merah dulu)
    - Modul valid lolos dan hasilnya beku; `key`/`slug`/`version` tidak valid, `id` navigasi ganda, `to` tanpa awalan `/`, grup tanpa anak, `OfflinePolicy` tak valid → `ModuleDefinitionError` dengan pesan menyebut bidang; `defaultOfflinePolicy` sama dengan `docs/architecture/offline-strategy.md` §3; `FrontendModule<"example">` menyimpulkan `slug` literal.
    - **Validates: Requirements 6.1, 6.2**
  - [x] 8.2 Implementasi `types.ts`, `define-module.ts`, `offline.ts`
    - Sesuai `design.md` §3 (`MessageRef`, `NavigationEntry`, `FrontendModule`, `OfflinePolicy`, `defaultOfflinePolicy`, `defineModule`, `ModuleDefinitionError`); ekspor lewat `src/index.ts` (selain `ModuleManifest` yang sudah ada).
    - _Requirements: 6.1_
  - [x] 8.3 Tipe route spesifik dan fixture tipe
    - `src/routing.ts`: `AppRouterContext`, `createAppRootRoute`, `createModuleBaseRoute`, `ModuleBaseRoute`, `AnyModuleChildRoute`, augmentasi `StaticDataRouteOption` (`crumb`), re-ekspor primitif route (`createRoute`, `createLazyRoute`, `lazyRouteComponent`, `Link`, `Outlet`, `useNavigate`); tambahkan `@tanstack/react-router` dan `@tanstack/react-query` sebagai `peerDependencies` (dan devDependency terpin). Fixture tipe (`routing.check.ts`, dijalankan lewat `typecheck`): anak route dengan `getParentRoute: () => base` valid; parent lain atau `AnyRoute` tak berlabel ditolak (`@ts-expect-error`). Bila generik tak dapat diekspor stabil, pakai fungsi pembangun sebagai satu-satunya pintu (K-28) dan catat.
    - _Requirements: 5.1_
  - [x] 8.4 README dan pemeriksaan manifest
    - Perbarui `packages/module-sdk/README.md` (hapus kalimat "menyusul", jelaskan `defineModule`, `OfflinePolicy`, routing); pastikan `manifest.check.ts` tetap lolos.
    - _Requirements: 6.1_

- [x] 9. `@erpora/platform-web` lapisan layanan (Kelas beban: Sonnet; 9.10 Haiku)
  - [x] 9.1 Tulis test unit dan properti (merah dulu)
    - `prefs` (baca/tulis aman, `localStorage` melempar); `theme` (`parseThemePreference`, `resolveTheme`, `applyTheme`, `ThemeProvider` mengikuti `matchMedia` tanpa reload dan event `storage`; **P6** property test; konsistensi dengan `web/src/theme-init.ts`); `tenancy` (`scopedKey`, **P8**; pemindai: tidak ada `queryKey` selain bootstrap yang tanpa awalan tenant di `web/src` dan `packages/*/src`); `format` (`formatRelativeTime`, `formatDateTime` id/en); `sources` (`emptyNotificationSource`, `anonymousUserSource`, `safeInternalHref` **P9** property test); `api` (`describeApiError` **P10**: semua kode di `design.md` Error Handling terpetakan, kode asing → `errors.unknown`, tidak ada `details` mentah); `permissions` (`usePermission` optimistis).
    - **Validates: Requirements 4.1, 4.2, 4.3, 7.1, 11.4, 12.3, 14.2, 14.3**
  - [x] 9.2 Implementasi `prefs/`, `theme/`, `tenancy/`, `format/`, `permissions/`
    - Sesuai `design.md` §5.1, 5.3, 5.6, 5.10. `ThemeProvider` memasang kelas pada `document.documentElement` dan `color-scheme`.
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 7.1, 14.2_
  - [x] 9.3 Perluas klien `@erpora/platform-contracts` (test dulu)
    - Test: hasil sukses memuat `etag`; `304` → `{ ok: false, status: 304, error.code: "not_modified" }`; header `If-None-Match`/`If-Match` lewat `init.headers` terkirim; mutasi tanpa body tidak menyetel `Content-Type` (pemanggil wajib `body: {}`). Implementasi aditif di `client.ts`; perbarui `client.check.ts` dan `README.md`; `getLocale()` mengikuti spec i18n (12.6).
    - _Requirements: 7.2, 7.5_
  - [x] 9.4 Bootstrap dan keselarasan `I18nProvider` (test dulu)
    - Test **P5** (`createBootstrapFetcher`: 200 menyimpan `etag`; 304 mengembalikan referensi objek sama; galat melempar), `BootstrapProvider` (memuat → ready; galat → `error` dengan `retry`; refetch saat focus lewat `QueryClient` palsu; `useFeature`, `useTenantId`). Implementasi sesuai §5.2. **Selaraskan `I18nProvider`** hasil spec i18n agar membaca `tenantId` dan `bootstrap.i18n` dari konteks `BootstrapProvider` (satu query `["bootstrap"]`), bukan query sendiri; jalankan ulang test i18n.
    - **Validates: Requirements 7.1, 7.2, 7.3, 12.5**
  - [x] 9.5 Registry modul (test dulu)
    - Test **P1** dan **P2** (property test `fast-check`: daftar modul dan bootstrap acak), `resolveModules` (tidak ada di bootstrap → diabaikan + peringatan sekali; versi berbeda → peringatan; `offline` tak cocok → peringatan), `selectNavigation` (hanya `enabled`, urutan stabil, `to` absolut), `moduleStatus`, `useModuleRegistry` bereaksi terhadap perubahan bootstrap. Implementasi sesuai §5.4.
    - **Validates: Requirements 6.2, 6.3, 6.4, 6.5, 6.8**
  - [x] 9.6 Klien platform, `msg()`/`crumb()`/`Message`
    - `createPlatformClient`, `describeApiError`, `msg`, `crumb`, komponen `Message` (memakai `useTranslation(ref.ns)`); fixture tipe: kunci salah ketik pada `msg("example", "tidak.ada")` gagal `tsc`.
    - _Requirements: 14.4, 6.1_
  - [x] 9.7 Aksi halaman dan tata letak (test dulu)
    - Test **P13** dan perilaku: `usePageActions` mendaftar saat mount (sinkron), menggantikan saat berubah, membersihkan saat unmount; `onSelect` memakai closure terbaru tanpa render ulang konsumen; `usePageLayout({ scroll: false })` dan kembalinya ke `true`; beberapa pemanggil digabung berurutan. Implementasi `page/` sesuai §5.7.
    - **Validates: Requirements 10.3, 10.4**
  - [x] 9.8 Sumber notifikasi dan pengguna
    - Implementasi `sources/` sesuai §5.8 (`NotificationSourceProvider`, `useNotifications` dengan Query, `CurrentUserSourceProvider`, `useCurrentUser`); test dengan sumber tiruan: memuat/kosong/galat, `markRead` dan `markAllRead` memperbarui hitungan, kunci query berawalan tenant, tidak ada data pengguna di storage.
    - _Requirements: 11.2, 11.3, 11.4, 12.3, 14.2_
  - [x] 9.9 Pemuat TanStack DB lazy
    - `db/index.ts` (`loadDb`, `useDb` dengan Suspense, hasil ter-cache); tambahkan dependensi `@tanstack/react-db` dan `@tanstack/query-db-collection` (versi pasti). Test memastikan `loadDb()` memberi `createCollection`/`useLiveQuery`/`queryCollectionOptions` dan hanya dimuat sekali. **Impor DB hanya lewat `import()`** (diperiksa test pemindai: tidak ada impor statis `@tanstack/react-db` di `index.ts` atau berkas yang diimpor entry).
    - _Requirements: 7.4, 1.4_
  - [x] 9.10 Ekspor dan gate paket
    - `src/index.ts` mengekspor API publik; `typecheck`, `lint`, `deps` (F1 sampai F4 hijau), `bun test`.
    - _Requirements: 1.5, 7.1_

- [x] 10. Backend: bendera API mutasi di bootstrap (Kelas beban: Sonnet; Go)
  - [x] 10.1 Test dulu
    - `internal/platform/bootstrap/bootstrap_test.go`: `Manifest.Features` terserialisasi sebagai `"features":{"modulesAdminApi":…}`; test `internal/app` untuk `bootstrapFrom` memakai `cfg.ModulesAdminAPIEnabled()` (development → true; production tanpa flag → false; production dengan `ERP_MODULES_ADMIN_API=true` → true). Perbarui test yang membandingkan JSON bootstrap penuh.
    - **Validates: Requirements 13.2**
  - [x] 10.2 Implementasi
    - `bootstrap.Features{ModulesAdminAPI bool}` dan `Manifest.Features`; `bootstrapFrom` menerima bendera dan mengisinya **setelah** `bootstrap.Build(...)` (tanda tangan `Build` tidak berubah); `assemble.go` meneruskan `cfg.ModulesAdminAPIEnabled()`.
    - _Requirements: 13.2_
  - [x] 10.3 Kontrak dan tipe TS
    - `contracts/openapi/platform.yaml`: `features` (opsional) pada `BootstrapEnvelope` + contoh respons; `task erpora -- contracts:generate` (hasilnya di-commit: `platform.gen.ts`); `task contracts:check` dan test kesesuaian route ↔ kontrak lolos; catat di `api-guidelines.md` bila perlu (tidak ada endpoint baru).
    - _Requirements: 13.2, 15.2_

- [ ] 11. Checkpoint — SDK, layanan, dan backend
  - Jalankan `task lint arch check:gowork-off test contracts:check`, `task web:check`; bila menyentuh DB jalankan `task test:integration` lewat PostgreSQL di VPS (tunnel + `ERP_TEST_DATABASE_URL`). Tanyakan ke user bila bentuk runtime i18n memaksa perubahan desain.

- [x] 12. Shell aplikasi (Kelas beban: Sonnet; 12.8 Haiku)
  - [x] 12.1 Tulis test shell (merah dulu)
    - `NavMenu` (urutan Beranda → modul → Pengaturan, **P3** item aktif unik, grup terbuka bila ada anak aktif, `EmptyState` bila tidak ada modul, ikon tak dikenal → lingkaran berinisial); `AppSidebar` (logo + nama, Notifikasi sebelum menu, lencana 0/5/`99+`, titik saat terlipat, avatar di dasar); `NotificationsPanel` (memuat/kosong/galat, tandai terbaca, `safeInternalHref`); `UserMenu` (isi menu, aksi nonaktif + tooltip, pilihan bahasa dan tema); `Navbar` + `Breadcrumbs` + `PageActionsView` (inline vs ⋯ di layar kecil, crumb terakhir bukan tautan); `AppRouter` dengan `fetch` palsu: **P17** guard (nonaktif → "Modul tidak aktif" dan chunk tidak dimuat; diubah ke `enabled` → pulih; disable saat halaman terbuka → konten diganti), modul frontend tak ada di bootstrap → 404, `router.invalidate()` saat `revision` berubah, error boundary menjaga sidebar/navbar, judul dokumen dan `RouteAnnouncer`; `BootErrorScreen` (kamus bawaan id/en, tombol "Coba lagi").
    - **Validates: Requirements 5.1, 5.3, 5.5, 8.1, 8.2, 9.1, 9.2, 9.6, 10.1, 10.2, 10.4, 11.1, 11.2, 11.3, 12.1, 12.2, 14.1, 14.5**
  - [x] 12.2 Providers dan `main.tsx`
    - `providers.tsx` (urutan `design.md` §6.1), `ui-labels-bridge.tsx`, `boot-screens.tsx` (`BootErrorScreen`, kerangka memuat), `QueryClient` tunggal (tanpa persister), `main.tsx` merender `<Providers/>`; dua `QueryClient` tidak boleh ada.
    - _Requirements: 7.1, 7.3, 4.1_
  - [x] 12.3 Router
    - `router/` sesuai §6.2: `buildRouter`, root route, base route per modul yang ada di bootstrap dengan guard (`beforeLoad` + komponen reaktif + `errorComponent`), `/`, `/settings/modules` (lazy; halaman dikerjakan di grup 14, sementara placeholder), `notFound`, `defaultErrorComponent`, `document.title`, `RouteAnnouncer`, `router.invalidate()` pada perubahan `revision`. `web/src/app/modules.ts` tetap sumber agregasi (`validateModules(frontendModules)` dipanggil saat start).
    - _Requirements: 5.1, 5.2, 5.3, 5.5, 6.3, 6.7, 8.1, 8.2_
  - [x] 12.4 AppShell, AppSidebar, NavMenu
    - Sesuai §6.3 sampai 6.4: `SidebarProvider` + `SidebarInset`, tautan "Lewati ke konten", `<main id="content" tabindex="-1">` dengan Suspense dan `CatchBoundary`, header (logo "E" + `platform:app.name`), item Notifikasi, label Menu, `NavMenu` (item, grup `Collapsible`, popover grup saat terlipat, tooltip), navigasi bawaan Beranda dan Pengaturan › Modul.
    - _Requirements: 9.1, 9.2, 9.3, 9.6, 9.7, 14.1, 14.5_
  - [x] 12.5 NotificationsPanel dan UserMenu
    - Sesuai §6.4 sampai 6.5: panel (Popover `w-96`, 460 px), lencana/titik, UserMenu (Avatar, nama, email, Kelola Akun, submenu Bahasa dan Tema, Keluar), pilihan bahasa memanggil `setLocale` runtime i18n, tema memanggil `useTheme().setPreference`.
    - _Requirements: 4.5, 11.1, 11.2, 11.3, 11.4, 12.1, 12.2, 12.3, 12.4, 12.5_
  - [x] 12.6 Navbar, Breadcrumbs, PageActionsView
    - Sesuai §6.6: `SidebarTrigger` + tooltip + `Kbd`, `collapseCrumbs`, `PageActionsView` responsif, area konten mengikuti `usePageChrome().scroll`.
    - _Requirements: 10.1, 10.2, 10.3, 10.4_
  - [x] 12.7 Halaman bawaan
    - `DashboardPage`, `NotFoundPage`, `ModuleInactivePage` (teks per status), `RouteErrorPage` (menampilkan `requestId`/`traceId` bila galat API).
    - _Requirements: 5.3, 8.1, 14.5_
  - [x] 12.8 Katalog `platform` dan `ui`
    - Kunci **ditambahkan bersama komponen yang memakainya** (12.2 sampai 12.7, 14.4) karena `t()` terketik: kunci yang belum ada gagal `tsc`. Task ini adalah rekonsiliasi akhir: pastikan kunci shell/navigasi/notifikasi/menu avatar/tema/bahasa/galat/modul tidak aktif (`design.md` Data Models) ada di `internal/platform/i18n/catalog/{id,en}.json` dan `packages/ui/i18n/{id,en}.json` (Title Case, tanpa emoji; `id` default dan `en`), tidak ada kunci yatim, lalu `task erpora -- i18n:generate` (`--check` bersih) dan `i18n:check` (tanpa peringatan paritas).
    - _Requirements: 12.5, 14.4_
  - [x] 12.9 Gate frontend dan laporan bundle
    - `bun run typecheck`, `lint`, `bun test`, `bun run build`; catat ukuran JS awal (anggaran 200 KB), chunk `/example` terpisah, kontributor terbesar; bila > 190 KB terapkan mitigasi K-12 berurutan dan lapor.
    - _Requirements: 1.3, 1.4, 5.4, 15.1_

- [x] 13. Modul referensi dan generator (Kelas beban: Sonnet)
  - [x] 13.1 Tulis test modul referensi (merah dulu)
    - Untuk `example` dan `exampleaddon`: `defineModule` valid (`key`/`slug` cocok dengan `module.json`, `version` sama), `registerRoutes` mengembalikan route dengan parent yang benar, navigasi satu item, halaman dirender dengan `fetch` palsu (daftar notes; ringkasan `noteCount`), `PageActions` "Segarkan".
    - **Validates: Requirements 5.3, 6.1, 6.7**
  - [x] 13.2 Buat `modules/example/web` dan `modules/exampleaddon/web`
    - Sesuai `design.md` §8: `package.json` (`@erpora/module-<dir>`, dependensi paket repo; addon juga `@erpora/module-example`), `tsconfig.json`, `src/index.ts` (`moduleInfo`, `api`, `frontendModule`), `src/module.ts`, `src/routes.tsx`, halaman lazy, kunci i18n modul (`nav.*`, `page.*`) di `modules/<x>/i18n/{id,en}.json` dan `keys.gen.ts` (koordinasi dengan hasil spec i18n; bila `web/` sudah dibuat spec itu, lengkapi saja), `contracts:generate` untuk `api/contract.gen.ts`. Daftarkan di `web/package.json` dan `web/src/app/modules.ts`; `bun install`.
    - _Requirements: 5.3, 6.1, 6.7_
  - [x] 13.3 Koleksi TanStack DB `example` dan bukti lazy
    - `modules/example/web/src/db/notes-collection.ts` (`createCollection(queryCollectionOptions(...))` lewat `useDb()`, kunci `scopedKey(tenantId, "example", "notes")`); halaman `/example` memakainya dengan `useLiveQuery`. Verifikasi build: chunk DB terpisah dan **tidak** ada di JS awal (periksa metafile), chunk `/example` terpisah.
    - _Requirements: 5.4, 7.4_
  - [x] 13.4 Templat generator (test dulu)
    - Perbarui golden test dulu (merah): `internal/devtools/gen/testdata/golden/module_*` memuat `src/module.ts`, `src/routes.tsx`, `src/pages/home-page.tsx`, `index.ts` yang mengekspor `frontendModule`, `web/src/app/modules.ts` dengan `import { frontendModule as <x>Module }`. Lalu ubah templat di `internal/devtools/templates/module/web/**` dan `modifyRepoFiles` (`internal/devtools/gen/module.go`) agar sesuai; jaga `moduleInfo` dan `api` tetap diekspor (aditif).
    - _Requirements: 15.4_
  - [x] 13.5 `task test:generator`
    - Jalankan di VPS (Docker + Bun; ±12 menit): modul hasil `make:module inventory --frontend --depends acme.example` lolos semua gate termasuk `web:check` dan `i18n:check`; catat hasil.
    - _Requirements: 15.4_

- [x] 14. Halaman `/settings/modules` (Kelas beban: Sonnet)
  - [x] 14.1 Tulis test halaman (merah dulu)
    - `errors.test.ts` (setiap kode `design.md` §7 terpetakan, `409/412/415/422`, kegagalan operasi); `impact.test.ts` (setiap baris tabel transisi ADR-007 untuk install/enable/disable/upgrade/uninstall/retry, blocker `module_required`/`dependency_not_ready`/`dependents_active`/`module_incompatible`, efek menu/route dari `navigation`, opsi install; **P7** property test dengan daftar modul acak); `api.test.ts` (paginasi mengikuti `hasMore`, `If-Match` dari ETag daftar, body `{}` dan `Content-Type: application/json`, **P11** dengan `fetch` perekam: tidak ada `tenantId`/`userId`); `polling.test.ts` (**P12** dengan jam palsu); komponen: tabel (kolom, nada status, versi binary vs ter-install), mode hanya-baca bila `features.modulesAdminApi` false (tanpa kolom aksi, ada `Alert`), `403` → `ErrorPage`, dialog konfirmasi (efek, blocker menonaktifkan tombol, opsi install), alur `202` + polling, sukses meng-invalidate bootstrap dan sidebar menghilangkan menu, dialog riwayat (event, langkah operasi saat dibuka).
    - **Validates: Requirements 8.2, 13.1, 13.2, 13.3, 13.4, 13.5, 13.6, 13.7, 7.5**
  - [x] 14.2 `api.ts`, `errors.ts`, `polling.ts`
    - Sesuai `design.md` §7: `useModules` (`pageSize=200`, ikuti `hasMore`, kembalikan `etag`), `useModuleHistory`, `useOperation` (interval 1 s → 5 s, berhenti saat selesai/unmount/10 menit), `runModuleOperation` (`kind: "done"` atau `"accepted"`), pemetaan galat ke kunci `platform:modules.error.*`/`failure.*`. Semua key lewat `scopedKey`.
    - _Requirements: 13.5, 13.6, 13.7, 7.5_
  - [x] 14.3 `impact.ts` (`computeImpact`)
    - Fungsi murni sesuai `design.md` §7 dengan antarmuka terisolasi sehingga dapat diganti hasil `plan` server (PK-1). Bila PK-1 ditolak: ganti task ini dengan endpoint `plan` additive di backend (lihat `design.md` Perlu konfirmasi) dan sesuaikan UI.
    - _Requirements: 13.3, 13.8_
  - [x] 14.4 Komponen halaman
    - `page.tsx` (lazy; `usePageActions` "Segarkan"), `modules-table.tsx` (`Table`, `BadgeStatus`, menu aksi `DropdownMenu`), `action-dialog.tsx` (`AlertDialog` + `Checkbox` opsi install + hasil/polling), `history-dialog.tsx` (`Dialog` lebar + `Collapsible` langkah). Daftarkan route `/settings/modules` (lazy) di router (menggantikan placeholder 12.3).
    - _Requirements: 5.3, 13.1, 13.2, 13.3, 13.4, 13.5, 13.6_
  - [x] 14.5 Katalog modul
    - Rekonsiliasi kunci `platform:modules.*` (kolom, `status.<9 status>`, `op.<6 operasi>`, `impact.*`, `error.*`, `failure.*`, `history.*`, `readonly.*`) id/en (ditambahkan bersama 14.2 sampai 14.4); `i18n:generate` dan `i18n:check`.
    - _Requirements: 13.7, 14.4_

- [ ] 15. Checkpoint — shell dan halaman modul
  - Jalankan `task web:check` dan `task lint arch check:gowork-off test contracts:check`; verifikasi manual cepat dengan `erpora serve` dev: `/settings/modules` memuat daftar, disable/enable `example` mengubah sidebar tanpa reload. Tanyakan ke user bila ada temuan UX.

- [x] 16. Dokumentasi dan sinkron dokumen proyek (Kelas beban: Haiku)
  - [x] 16.1 PRD-000
    - Tambahkan **Q36** (keputusan M3a: K-1 sampai K-34 ringkas, PK-1 sampai PK-6 dengan status konfirmasi, tanggal absolut, alasan, pemilik produk menyetujui via delegasi 29 Okt 2026); perbarui FR-08, FR-09 (field `title` opsional, helper `msg/crumb`), FR-20.3 (dialog dampak, bila PK-1 disetujui), §4.2 (aturan F4), tabel M3 (gate `bun test` lebih awal), FR-22.2 (`make:page` ke M3d bila PK-5 disetujui); ubah R14.4 di `requirements.md` agar menyebut pengecualian layar boot (bila PK-4 disetujui) dan tutup butir "TODO" yang sudah diputuskan.
    - _Requirements: 15.6_
  - [x] 16.2 ADR-011
    - Isi `docs/adr/0011-kontrak-ui-extension-dan-design-system.md` (Status Accepted): keputusan design system (ERP DS + shadcn, token), batas paket (F1 sampai F4), registry/route modul, tanpa kontrak UI extension (tetap fase lanjutan).
    - _Requirements: 15.6_
  - [x] 16.3 Panduan modul frontend
    - `docs/module-sdk/frontend.md` baru (struktur `modules/<x>/web`, `defineModule`, `msg()`, route lazy, `PageActions`, i18n, pemakaian `@erpora/ui`, uji komponen, kuota bundle); tautkan dari `docs/module-sdk/README.md`; perbarui `docs/module-sdk/generators.md` (templat `--frontend` baru) dan `docs/module-sdk/creating-a-module.md` bagian frontend; `packages/ui/README.md`, `packages/platform-web/README.md`.
    - _Requirements: 6.7, 15.4_
  - [x] 16.4 `CLAUDE.md` dan `AGENTS.md` (harus identik; `TestAgentsMDMatchesClaudeMD`)
    - Tambahkan butir frontend: `@erpora/ui` milik repo (shadcn via `bun run shadcn add`, tanpa palet langsung), ui adalah daun (F4), teks UI lewat i18n, `bun test` dari root dan `task web:check`, `dangerouslySetInnerHTML` dilarang. Jalankan `go test ./tests/architecture/...`.
    - _Requirements: 15.6_

- [ ] 17. Gate akhir dan verifikasi (Kelas beban: Sonnet; review akhir oleh model tertinggi yang tersedia)
  - [ ] 17.1 Gate lengkap
    - Berurutan: `task lint arch check:gowork-off test`, `task contracts:check`, `task web:check` (termasuk `bun test` dan build + anggaran), `task test:integration` (PostgreSQL di VPS), `task test:generator`; pastikan `erpora serve` menyajikan SPA baru (`go:embed` dari `web/dist`) dan test SPA/integrasi yang ada lolos. Catat hasil dan yang tidak terverifikasi.
    - _Requirements: 15.1, 15.2, 15.3, 15.4_
  - [ ] 17.2 Verifikasi manual R15.5
    - `task dev:setup`, `erpora serve` (dev) + build web; di peramban: disable `example` lewat `/settings/modules` → menu hilang tanpa reload, `/example` menampilkan "Modul tidak aktif", enable kembali → pulih; mode hanya-baca saat API mutasi mati (`ERP_ENV=production`, tanpa flag); `localStorage` hanya berisi `erpora.theme`, `erpora.sidebar`, `erpora.locale`; tema, bahasa, collapse sidebar (klik dan Ctrl/Cmd+B), layar kecil (Sheet); simpan temuan beserta tangkapan bila ada.
    - _Requirements: 15.5, 8.2, 4.2, 9.4, 9.5_
  - [ ] 17.3 Review akhir independen
    - Satu review menyeluruh oleh agent baru (diff + spec, tanpa kesimpulan implementer) atas kesesuaian spec dan skill `security-audit` terfokus pada perubahan (XSS lewat konten dinamis, `localStorage`, hygiene request, CSP/font/skrip, rantai pasok dependensi baru). Putaran perbaikan sesuai aturan `CLAUDE.md` global: yang sesuai spek dikerjakan oleh implementer yang sama; yang di luar spek menunggu konfirmasi.
    - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5, 15.6_

- [ ] 18. Final checkpoint — Ensure all tests pass
  - Semua gate hijau dan tidak ada temuan terbuka; laporkan terpisah: yang dikerjakan, yang menunggu konfirmasi (PK-1 sampai PK-6 beserta rekomendasi), dan yang sengaja tidak dikerjakan (Desk, daftar, form, pencarian, PWA, notifikasi dan identitas backend). Blok M3b boleh dimulai setelah ini.

## Notes

- Setiap task mereferensi nomor requirement (`R.n`) di `requirements.md`; grup 2 menutup Requirement 1, grup 4 sampai 7 Requirement 2 dan 3, grup 8 sampai 9 Requirement 5 sampai 7 dan sebagian 4, 10, 11, 12, grup 10 menutup bendera API mutasi (Requirement 13.2), grup 12 Requirement 8 sampai 12 dan 14, grup 14 Requirement 13, grup 17 Requirement 15.
- **TDD:** untuk setiap implementasi, test ditulis dan dijalankan merah lebih dulu (ditandai "test dulu"/"merah dulu"), lalu implementasi sampai hijau. Satu commit per sub-task dengan pesan konvensional (`feat(web): …`, `feat(ui): …`, `test:`, `docs:`, `chore:`). Jangan push atau membuat PR tanpa diminta.
- **Konfirmasi per komponen (R3.3):** paket keputusan di `design.md` tabel C adalah konfirmasi yang sudah didelegasikan pemilik produk. Implementer **berhenti dan bertanya** hanya bila memerlukan komponen, varian, atau penyimpangan yang tidak ada di tabel itu; semua penyesuaian dari kartu DS dicatat di tabel C.
- **Kelas beban untuk delegasi** (sebut model secara eksplisit): **Haiku** untuk 1.2, 2.4, 2.5, 2.6, grup 4.1, grup 5 (komponen sederhana, spesifikasi lengkap), 8.4, 9.10, 12.8, dan grup 16 (dokumen); **Sonnet** untuk sisanya (toolchain, logika inti, integrasi lintas file, test, router, halaman modul, Go backend); review akhir 17.3 oleh model tertinggi yang tersedia. Satu agent per kelas beban dan lanjutkan lewat `SendMessage` dalam satu putaran (tanpa review per task); review independen sekali di akhir putaran; compact agent bila task berikutnya tidak berhubungan.
- **Estimasi jumlah task:** grup 1 (3), 2 (7), 3 (checkpoint), 4 (4), 5 (3), 6 (5), 7 (checkpoint), 8 (4), 9 (10), 10 (3), 11 (checkpoint), 12 (9), 13 (5), 14 (5), 15 (checkpoint), 16 (4), 17 (3), 18 (final): 65 sub-task kerja. Pembagian kasar: Haiku ±20% (±13 sub-task), Sonnet ±80%.
- **shadcn:** selalu `bun run shadcn add <komponen>` dari `packages/ui` (CLI terpin, tanpa `bunx`/`npx`); setelah setiap `add`, tinjau diff, adaptasi ke token DS, pin ulang versi dependensi yang ditulis CLI, dan jangan menyisakan nilai palet langsung (test 4.2d akan gagal).
- **Bun dan Windows:** gate menjalankan `bun test` dari **root** (preload di `bunfig.toml` root; tiap paket bertest punya `bunfig.toml` dengan preload yang sama); `web/test/dom.ts` harus lebih dulu dari `setup.ts` (impor statis di-hoist). `-race` dilewati lokal untuk Go; CI Linux tetap menjalankannya. Jangan menambah perintah Taskfile yang memakai utilitas Unix.
- **Test yang butuh PostgreSQL** (`task test:integration`) dan `task test:generator` dijalankan di VPS lewat tunnel SSH (`ERP_TEST_DATABASE_URL`); jangan memulai container di laptop.
- **Anggaran bundle:** build gagal bila JS awal gzip > 200 KB. Angka terukur 29 Okt 2026: react-dom 65,7 KB, + Router 28,3, + Query 9,7, + 9 primitif Radix 23,7, + 17 ikon 3,2, + sonner 9,9, + intl-messageformat 10 = 159 KB tanpa kode aplikasi; TanStack DB ±100 KB harus lazy (task 9.9 dan 13.3).
- **Koordinasi dengan `i18n-platform`:** M3a menambah kunci ke katalog `platform` dan `ui` serta ke katalog `example`/`exampleaddon`; bidang bootstrap `features` (M3a) dan `i18n` (spec i18n) sama-sama aditif dan ditambahkan setelah `bootstrap.Build(...)`, jadi konflik penggabungan kecil.
- **Di luar cakupan:** Desk (M3b), list/filter/pencarian (M3c), form (M3d), tree/kanban (M3e), dashboard/chart/laporan (M3f), PWA/Service Worker, `SafeHtml`, ZAP, Playwright (M3g), backend notifikasi dan identitas, `make:page` (M3d bila PK-5 disetujui), perubahan di luar spek tanpa konfirmasi.

## Catatan integrasi

_(Diisi saat task 1.1: bentuk aktual runtime `i18n-platform` yang dipakai M3a.)_

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2", "1.3"] },
    { "id": 1, "tasks": ["2.1", "2.4", "2.5", "10.1"] },
    { "id": 2, "tasks": ["2.2", "2.3", "2.6", "10.2"] },
    { "id": 3, "tasks": ["2.7", "10.3"] },
    { "id": 4, "tasks": ["3"] },
    { "id": 5, "tasks": ["4.1", "8.1"] },
    { "id": 6, "tasks": ["4.2", "4.4", "8.2"] },
    { "id": 7, "tasks": ["4.3", "8.3", "9.1", "9.3"] },
    { "id": 8, "tasks": ["5.1", "6.1", "6.3", "6.4", "8.4", "9.2"] },
    { "id": 9, "tasks": ["5.2", "6.2", "9.4"] },
    { "id": 10, "tasks": ["5.3", "6.5", "9.5", "9.6", "9.7", "9.8", "9.9"] },
    { "id": 11, "tasks": ["7", "9.10"] },
    { "id": 12, "tasks": ["11"] },
    { "id": 13, "tasks": ["12.1", "13.1", "14.1"] },
    { "id": 14, "tasks": ["12.2", "14.2", "14.3"] },
    { "id": 15, "tasks": ["12.3", "12.4", "13.2"] },
    { "id": 16, "tasks": ["12.5", "12.6", "12.7", "13.3", "13.4"] },
    { "id": 17, "tasks": ["12.8", "14.4", "14.5", "13.5"] },
    { "id": 18, "tasks": ["12.9"] },
    { "id": 19, "tasks": ["15"] },
    { "id": 20, "tasks": ["16.1", "16.2", "16.3", "16.4"] },
    { "id": 21, "tasks": ["17.1"] },
    { "id": 22, "tasks": ["17.2"] },
    { "id": 23, "tasks": ["17.3"] },
    { "id": 24, "tasks": ["18"] }
  ]
}
```
