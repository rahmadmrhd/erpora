# Panduan modul frontend

Panduan ini menjelaskan cara menulis frontend untuk modul ERPora: struktur folder, deklarasi modul dengan `defineModule`, route lazy, aksi halaman, teks lewat i18n, pemakaian `@erpora/ui`, uji komponen, dan anggaran bundle. Contoh acuannya adalah `modules/example/web` dan `modules/exampleaddon/web`.

Keputusan yang melandasinya: [ADR-011](../adr/0011-kontrak-ui-extension-dan-design-system.md) (design system, batas paket; keputusan komponen dan `navigation` sudah digantikan), [ADR-0015](../adr/0015-komponen-persis-shadcn-dan-kunci-snapshot.md) (komponen persis shadcn), PRD-000 Q40 (shell Desk, Desk per modul, URL bersih), serta spec revisi `docs/specs/m3a-revisi-shell-shadcn/` dan dasarnya `docs/specs/m3a-frontend-foundation-shell/`. Panduan teks dan kunci terjemahan ada di [`i18n.md`](i18n.md).

## Ringkasan

- Frontend modul ada di `modules/<dir>/web`, package `@erpora/module-<dir>`, dan dirakit oleh app shell `web/` (aturan F3).
- Menu dan Dashboard Desk dideklarasikan di **manifest** (`module.json`, properti `desk`), bukan di frontend. Frontend hanya mendaftarkan rute. Modul berfrontend wajib punya `desk` (aturan `DESK` di `module:check`). Lihat [`creating-a-module.md`](creating-a-module.md).
- Semua rute modul diawali slugnya: `/<slug>` adalah Dashboard Desk (milik shell, bukan modul), `/<slug>/<halaman>` adalah halaman modul. Modul tidak boleh mendaftarkan rute indeks `/`.
- Aturan F2 hanya membatasi impor paket `@erpora/*`: frontend modul boleh mengimpor `@erpora/ui`, `@erpora/platform-web`, `@erpora/module-sdk`, `@erpora/platform-contracts`, dan `@erpora/module-<x>` yang ada di `manifest.dependencies`. Pustaka pihak ketiga (`react`, `@tanstack/react-query`, dan lainnya) diizinkan; versinya dipin sebagai `peerDependencies` modul (Q39 butir 1). Impor lewat `index.ts`; deep import dilarang.
- Backend tetap otoritas. Permission dan status modul di frontend hanya untuk UX (route yang nonaktif menampilkan "Modul tidak aktif").
- Dilarang: `dangerouslySetInnerHTML` (Biome, `noDangerouslySetInnerHtml` = error), palet warna langsung di komponen, dan string UI yang ditanam di kode.

## Struktur folder

Contoh dari `modules/example/web` (templat `make:module --frontend` menghasilkan bagian inti: `bunfig.toml`, `package.json`, `tsconfig.json`, `src/index.ts`, `info.ts`, `module.ts`, `routes.tsx`, `pages/home-page.tsx`, `i18n/keys.gen.ts`, dan `module.test.ts`; lihat [`generators.md`](generators.md)):

```
modules/example/web/
├── package.json        # @erpora/module-example; dependensi paket repo + @erpora/module-<x> sesuai manifest
├── tsconfig.json
├── bunfig.toml         # [test] preload ../../../web/test/dom.ts lalu setup.ts (urutan penting)
└── src/
    ├── index.ts        # satu-satunya entry publik: api, moduleInfo, frontendModule
    ├── info.ts         # key, slug, versi; harus sama dengan module.json (diuji module.test.ts)
    ├── module.ts       # defineModule(...): key, slug, version, title?, registerRoutes, offline
    ├── routes.tsx      # route anak base route; halaman dimuat lazy
    ├── notes-api.ts    # pemanggil API modul (lewat klien platform-contracts)
    ├── db/             # koleksi TanStack DB (dimuat malas lewat useDb)
    ├── pages/          # halaman (komponen), dimuat lazy dari routes.tsx
    ├── i18n/keys.gen.ts  # GENERATED (i18n:generate); jangan diedit
    ├── testing.tsx     # dukungan uji: render route dengan router memori, bootstrap statis, katalog JSON nyata
    └── *.test.ts(x)
```

`index.ts` mengekspor `api` (dari `contract.gen.ts` hasil `contracts:generate`), `moduleInfo`, dan `frontendModule`. Modul lain dan shell hanya boleh memakai ketiganya.

Katalog terjemahan ada di `modules/<dir>/i18n/{id,en}.json` (lihat [`i18n.md`](i18n.md)).

## Deklarasi modul: `defineModule`

```ts
import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("example", "nav.notes"), // opsional: crumb dasar modul
  registerRoutes,
  offline: defaultOfflinePolicy,
});
```

Bentuk `FrontendModule` (`packages/module-sdk/src/types.ts`):

| Field | Keterangan |
| --- | --- |
| `key` | Sama dengan `manifest.key`, pola `<publisher>.<nama>` (huruf kecil, angka, `-`) |
| `slug` | Sama dengan `manifest.slug` (maks 50 karakter); basePath `/<slug>` dan namespace i18n |
| `version` | Semver; registry memberi peringatan bila berbeda dari versi backend di bootstrap |
| `title?` | `MessageRef` opsional, crumb dasar modul (Q36, PK-6) |
| `registerRoutes` | `(base: ModuleBaseRoute) => AnyModuleChildRoute[]`; hanya mendaftarkan halaman anak (lihat "Route modul") |
| `offline` | `OfflinePolicy`, mengikuti `docs/architecture/offline-strategy.md` §3; untuk modul online saja pakai `defaultOfflinePolicy` |

Field `navigation` (beserta tipe `NavigationItem` dan `NavigationGroup`) **sudah dihapus** (PRD-000 Q40 butir 21). Menu Desk berasal dari properti `desk` di `module.json`, yang disaring server per pengguna lewat `GET /api/platform/desks`. Jangan menambahkan `navigation` lagi: `defineModule` tidak lagi menerimanya.

`defineModule` melempar `ModuleDefinitionError` (dengan `key` modul dan `field` bidang yang salah) bila: `key`, `slug`, atau `version` tidak valid; `title` bukan `MessageRef`; `registerRoutes` bukan fungsi; atau `offline` tidak valid. Objek yang dikembalikan dibekukan (`Object.freeze`).

Contoh properti `desk` di `modules/example/module.json` (sumber menu, dibaca server; bukan bagian `defineModule`):

```json
"desk": {
  "label": "desk.title",
  "icon": "file-text",
  "menus": [
    { "id": "notes", "label": "nav.notes", "to": "/notes", "permission": "example.notes.read" }
  ]
}
```

- `label` dan `menus[].label` adalah kunci i18n di namespace modul; `icon` memakai nama dari `iconRegistry`.
- `menus` berisi item atau grup satu tingkat (grup tidak boleh bersarang, anak grup minimal satu).
- `to` relatif terhadap base route modul (`/notes` menjadi `/example/notes`). Tanpa skema, host, `//`, query, atau fragmen. **Tidak boleh `/`**: indeks adalah Dashboard shell.
- Setiap item memuat tepat satu dari `permission` atau `access` (PRD-000 Q43). `permission` harus salah satu `capabilities` modul; item disembunyikan bila pengguna tidak memilikinya. `"access": "authenticated"` menggantikan `permission` untuk item yang terlihat bagi semua pengguna yang login (server menyaringnya dari Principal, bukan dari permission); keduanya sekaligus atau tak satu pun ditolak.
- `dashboard` (opsional) hanya mengisi `label` dan `icon` item Dashboard bawaan. Dashboard sendiri dibangkitkan platform di `/<slug>`, tidak dideklarasikan sebagai menu.

Registry di shell (`@erpora/platform-web`, `registry/`) mencocokkan modul dengan bootstrap berdasarkan `key`:

- Modul tidak ada di bootstrap: diabaikan dan dicatat sebagai peringatan `not_in_bootstrap`.
- Versi berbeda dari backend: peringatan `version_mismatch`.
- `slug` berbeda dari bootstrap: peringatan `slug_mismatch`; `offline` tidak cocok: `offline_mismatch`.
- Key atau slug ganda: `DuplicateModuleKeyError` / `DuplicateModuleSlugError`.
- Menu Desk tidak dibaca dari modul frontend. Server membangun daftar Desk per pengguna dari manifest `desk` (`GET /api/platform/desks`, permission `platform.desks.read`; hanya modul `enabled` dan menu yang diizinkan). Klien menentukan Desk saat ini dan mencocokkan halaman dengan menu lewat fungsi murni `resolveDesk` dan `matchMenu` (`@erpora/platform-web`, folder `desk/`). `to` di manifest menjadi absolut lewat base route modul.

## Route modul (lazy)

Modul tidak pernah menerima `AnyRoute` dan tidak membuat base route. Shell membuat base route `/<slug>` yang sudah berguard, lalu memanggil `registerRoutes(base)`:

```tsx
import { type AnyModuleChildRoute, createRoute, lazyRouteComponent, type ModuleBaseRoute } from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";
import "./i18n/keys.gen";

export function registerRoutes(base: ModuleBaseRoute): readonly AnyModuleChildRoute[] {
  const notes = createRoute({
    getParentRoute: () => base,
    path: "/notes",
    staticData: { crumb: crumb("example", "page.notes.title") },
    component: lazyRouteComponent(() => import("./pages/notes-page"), "NotesPage"),
  });
  return [notes];
}
```

- Path halaman (`/notes`) harus sama dengan `to` menu di `desk.menus` pada `module.json`. Halaman dimuat dengan `import()` agar menjadi chunk terpisah (R5.4). Modul yang tidak dibuka tidak ikut dimuat.
- **Rute indeks dilarang.** `/<slug>` adalah Dashboard bawaan shell (slot widget). Shell memasangnya lewat `attachModuleRoutes(base, children, { index })`, dan `attachModuleRoutes` menolak path `/` atau kosong dari modul dengan `ModuleDefinitionError`. Halaman yang dulu berada di indeks pindah menjadi anak (`/example/notes`).
- `listModuleRoutePaths(module)` (`@erpora/module-sdk`) mengembalikan path halaman relatif base, termasuk rute bersarang dan tanpa indeks. Dipakai pengujian dan analisis dampak.
- `staticData.crumb` adalah sumber tunggal breadcrumb dan judul dokumen. Untuk teks data (mis. nama record) pakai `{ text: "..." }`, bukan `crumb(...)`.
- Gunakan `createRoute` dan `lazyRouteComponent` dari `@erpora/module-sdk`, bukan langsung dari `@tanstack/react-router`.

**Guard.** Shell memasang `beforeLoad` pada base route: bila status modul di bootstrap bukan `enabled`, route melempar `ModuleInactiveError` dan halaman "Modul tidak aktif" tampil **sebelum** chunk halaman dimuat. Saat modul di-disable lewat `/platform/modules`, bootstrap di-refetch (revision berubah), router di-`invalidate`, dan menu hilang tanpa reload.

## Aksi halaman

Halaman mendeklarasikan tombol aksinya sendiri; shell merendernya di navbar dan membersihkannya saat halaman berganti:

```tsx
usePageActions([
  { id: "refresh", label: refresh, icon: "refresh-cw", onSelect: () => void reload(), pinned: true },
]);
```

- `PageAction`: `id`, `label` (sudah diterjemahkan oleh pemanggil), `icon?` (nama di registry `@erpora/ui`), `variant?` (`primary`, `outline`, `ghost`, `destructive`; `primary` ditampilkan sebagai `default` shadcn di shell), `onSelect`, `disabled?`, `loading?` (ditampilkan lewat `LoadingButton`), `pinned?`.
- `pinned: true` tetap tampil sebagai tombol ikon di layar kecil; aksi lain dilipat ke menu ⋯.
- `PageActionNode` (`{ id, node }`) untuk simpul kustom hanya tampil di layar `md` ke atas.
- `onSelect` selalu memakai closure terbaru tanpa render ulang konsumen; pendaftaran dibersihkan saat unmount (diuji P13).
- Layout halaman: `usePageLayout({ scroll: false })` mematikan gulir kontainer konten (mis. layout list-detail). Nilai bawaan `scroll: true`.

## Teks dan i18n

- Setiap teks UI memakai kunci terjemahan. Dilarang menanam string UI di JSX.
- `msg(ns, key)` dan `crumb(ns, key)` membuat rujukan **terketik** dari `I18nCatalogs` hasil `keys.gen.ts`: kunci salah ketik atau namespace yang tidak ada gagal `tsc`.
- Di komponen, `useMessage(ref, args?)` menghasilkan string, dan `<Message message={ref} args={...} />` merendernya. Argumen plural dan select mengikuti sintaks ICU.
- `useTranslation(ns)` (dari `@erpora/platform-web`) memberi `t` (teks polos), `rich` (tag rich sebagai elemen React), `locale`, dan `setLocale`. Gunakan bila perlu mengakses kunci langsung.
- Katalog dimuat malas per namespace dengan `useSuspenseQuery`. Komponen yang memakai terjemahan harus berada di bawah batas `Suspense` (shell sudah menyediakannya). Selama memuat, kunci mentah tidak pernah tampil.
- Kunci dibangkitkan ke `src/i18n/keys.gen.ts` oleh `i18n:generate`. Impor `"./i18n/keys.gen"` di entry modul agar augmentasi tipe berlaku.
- Kunci yang tidak ada di katalog: `t` mengembalikan `ns::kunci` (paritas dengan Go) dan mencatat peringatan dev sekali per (bahasa, namespace, kunci); di produksi tidak ada peringatan. `t` tidak pernah melempar.
- Katalog namespace modul yang nonaktif (`404 not_found`) tidak fatal: teksnya menjadi `ns::kunci`. Katalog `platform` dan `ui` yang 404 tetap fatal, karena berarti backend rusak.

## Halaman modul dan mode hanya-baca

- Route modul yang statusnya bukan `enabled` menampilkan "Modul tidak aktif" tanpa memuat chunk halaman (guard di `beforeLoad`).
- `/platform/modules` (sebelumnya `/settings/modules`, dipindah tanpa pengalihan, PRD-000 Q40 butir 13) berada dalam mode hanya-baca bila `features.modulesAdminApi` di bootstrap tidak aktif, atau bila mutasi membalas `404`. Dalam mode ini kolom Aksi disembunyikan, sedangkan Riwayat operasi tetap tersedia lewat tombol di sel nama (Q39 butir 2). Pemakaian `features` lewat `useFeature("modulesAdminApi")`.

Aturan lengkap kunci, katalog, dan alur penerjemah ada di [`i18n.md`](i18n.md).

## Pemakaian `@erpora/ui`

- Impor hanya dari barrel `@erpora/ui` (`src/index.ts`). Entry sekunder yang dikecualikan (mis. `@erpora/ui/toaster`) dimuat lazy dan hanya bila perlu.
- Komponen `components/ui` adalah salinan **persis** registry shadcn (ADR-0015): nama varian dan ukuran bawaan shadcn (`default` bukan `primary`; `default`, `xs`, `sm`, `lg`, `icon*` bukan `md`). Jangan mengubah berkasnya; kebutuhan DS dipenuhi lewat pembungkus atau token.
- Barrel `@erpora/ui` mengekspor komponen shadcn (Alert, Avatar, Badge, Breadcrumb, Button, Card, Checkbox, Collapsible, Dialog, AlertDialog, DropdownMenu, Empty, Input, Kbd, Label, Popover, RadioGroup, Select, Separator, Sheet, Sidebar, Skeleton, Spinner, Table, Tabs, Tooltip), komponen pembungkus `LoadingButton` dan `StatusBadge` (`components/extended`), serta `cn`, `useIsMobile`, `iconRegistry`, dan `UiLabelsProvider`.
- Entry sekunder yang dimuat malas, tidak diekspor dari barrel: `@erpora/ui/toaster` (`Toaster`, `toast`, bergaya token) dan `@erpora/ui/command` (`Command`, pustaka cmdk; palet perintah). Impor keduanya langsung dari `exports` paket hanya bila perlu.
- Padanan komponen lama: `BadgeStatus` menjadi `StatusBadge`; `EmptyState` dan `ErrorPage` menjadi `Empty` (komposisi `EmptyHeader`, `EmptyMedia`, `EmptyTitle`, `EmptyDescription`, `EmptyContent`); `LoadingIcon` menjadi `Spinner`; `Button` dengan `loading` menjadi `LoadingButton`; `Button variant="primary"` dan `size="md"` menjadi `default`. Lembar bawah (mis. breadcrumb di ponsel) memakai `Sheet side="bottom"`; `Drawer` tidak dipasang.
- Komponen tidak menanam teks. Teks bawaan (mis. tombol tutup dialog) diisi shell lewat `UiLabelsProvider`. Modul tidak perlu mengatur ini.
- Warna dan radius lewat token semantik (`bg-background`, `text-muted-foreground`, `bg-status-success-bg`, `border-border`, dan seterusnya). Nilai palet langsung (`zinc-100`, `red-500`, `#fff`, `rgb(...)`) dilarang di luar `theme.css` dan diuji.
- Ikon: `icon` pada menu Desk (manifest) dan aksi halaman memakai nama kebab dari `iconRegistry` (mis. `file-text`, `bell-ring`). Nama tak dikenal tampil sebagai lingkaran berinisial dan dicatat sekali di development.
- `cn` dari `@erpora/ui` untuk menggabungkan kelas.

## Uji

- Jalankan dari root: `bun test --pass-with-no-tests` (atau `task web:test`). Setiap paket yang punya skrip `test` (`web`, `packages/ui`, `packages/platform-web`, `packages/platform-contracts`, `packages/module-sdk`, `modules/example/web`, `modules/exampleaddon/web`, dan modul baru hasil `make:module --frontend`) punya `bunfig.toml` dengan `[test] preload` ke `web/test/dom.ts` lalu `web/test/setup.ts` (path relatif terhadap paket). `dom.ts` harus dimuat lebih dulu.
- Stack: `bun test`, `@testing-library/react`, `user-event`, `jest-dom`, dan happy-dom. Radix (DropdownMenu, Dialog) dan perilaku ARIA diuji dengan peran dan keyboard.
- Untuk halaman yang membutuhkan bootstrap, pakai `StaticBootstrapProvider` dan `makeBootstrap` (`@erpora/platform-web`). Untuk teks, pakai `createTestI18n({ ns: { "kunci.lengkap": "pesan" } })`, atau `testing.tsx` modul yang memuat katalog JSON nyata.
- Panggilan API di test memakai `fetch` palsu yang merekam permintaan. Test harus membuktikan bahwa body tidak memuat `tenantId` atau `userId`.
- Uji Desk: `assertDeskRoutes(frontendModule, manifest.desk)` dari `@erpora/module-sdk/testing` memastikan setiap `to` menu di `desk` punya rute terdaftar di `registerRoutes` (termasuk rute bersarang), dan tidak ada `to` indeks. Melempar `Error` yang menyebut `to` bermasalah. `module.test.ts` hasil `make:module --frontend` memanggilnya dan juga memastikan tidak ada properti `navigation`.
- Uji tipe: kunci atau argumen yang salah harus gagal `tsc`. Gunakan berkas `*.check.ts` (dijalankan lewat `typecheck`) dengan `@ts-expect-error`, seperti `packages/platform-web/src/msg/msg.check.ts`.
- End-to-end browser (Playwright) belum tersedia; direncanakan di M3g.

## Anggaran bundle dan performa

- Anggaran entry shell: **210 KB gzip** (NFR-04). Build (`web/build.ts`) gagal bila JS awal melampauinya (`BUDGET` di `web/tools/budget.ts`). Build juga melaporkan JS awal, chunk lazy, CSS, font, dan 15 kontributor terbesar.
- Entry terukur pada 29 Okt 2026: 187,9 KB gzip (Q38). Ruang tersisa sempit.
- Aturan untuk modul:
  - Halaman dan komponen berat dimuat lewat `lazyRouteComponent(() => import(...))`.
  - TanStack DB dimuat malas lewat `useDb()` (K-21); modul yang memakai koleksi tidak boleh mengimpornya statis.
  - Hindari dependensi baru di entry; bila perlu, taruh di chunk halaman.
- Periksa dengan `bun run --filter @erpora/web build`.

## Metadata resource (form, daftar, dan filter generik)

Bentuk data satu resource (field, tipe, batasan, grup, kondisi tampil/wajib/hanya-baca, relasi, penguncian) dinyatakan sekali di `modules/<dir>/metadata/*.resource.yaml` dan dibangkitkan ke frontend sebagai berkas statis `modules/<dir>/web/src/metadata/<nama>.meta.gen.ts` (hanya untuk modul ber-`web/`). Berkas itu **tidak diedit tangan** (berkepala `Code generated ...`; lihat [`generators.md`](generators.md) §10) dan diperiksa drift-nya oleh `task metadata:generate:check`, yang ikut `task web:check`.

- Setiap berkas mengekspor satu konstanta `resourceMeta` bertipe `ResourceMeta`, diimpor dari `@erpora/platform-web/metadata`. Paket itu **bukan** bagian barrel `@erpora/platform-web` agar tidak masuk bundel awal; muat malas, satu chunk per resource, lewat `resourceMetaLoader.register("<slug>.<nama>", () => import("./metadata/<nama>.meta.gen"))` saat registrasi modul.
- Isi paket (tipe, evaluator kondisi dan ekspresi, templat label, desimal berbasis `BigInt`, validator record, registri tipe field dengan editor form dan filter, pemuat malas) dan seluruh aturan evaluasinya dijelaskan di [`metadata.md`](metadata.md). Komponen perender generik (form, tabel data, FilterBuilder) dibuat di M3c dan M3d di atas paket ini.
- Chunk metadata bersifat publik (SPA statis tanpa autentikasi), sehingga pembangkit tidak memancarkan aturan `scope: server`, kondisi yang merujuk field `confidential` ke atas atau di luar grup default, field `clientVisible: false`, dan ekspresi accessor `stored`; pemuat menolak chunk yang tetap memuatnya (`checkClientMeta`). Server tetap otoritas atas semua validasi, otorisasi, dan penguncian; validasi klien hanya kenyamanan.
- Label dan pesan aturan berupa kunci i18n pada katalog namespace modul pemilik resource (`labelRef(meta, key)`); tidak ada teks UI bebas di metadata.

## Perintah

| Perintah | Keterangan |
| --- | --- |
| `bun run --filter '*' typecheck` | Typecheck semua paket frontend |
| `bun run --filter '*' lint` | Biome (termasuk larangan `dangerouslySetInnerHTML`) |
| `bun run --filter @erpora/web deps` | dependency-cruiser: aturan F1 sampai F4 |
| `bun test --pass-with-no-tests` | Uji dari root |
| `bun run --filter @erpora/web build` | Build, anggaran bundle, dan csp-check |
| `task web:check` | Gabungan: install beku, `metadata:generate --check` (drift `*.meta.gen.ts`), typecheck, lint, `i18n:generate --check`, deps, test, build |
| `task web:test` | Hanya uji frontend |
| `task i18n:check` / `task i18n:generate` | Lihat [`i18n.md`](i18n.md) |

## Aturan batas (ringkas)

- F1: `@erpora/ui`, `@erpora/platform-web`, dan `@erpora/module-sdk` tidak bergantung pada `@erpora/module-*`.
- F2: impor `@erpora/*` oleh frontend modul hanya ke paket yang diizinkan dan `@erpora/module-<x>` dari `manifest.dependencies` (Q39 butir 1 membatasi F2 pada paket `@erpora/*`). Dependensi `@erpora/module-*` di `package.json` modul harus cocok dengan manifest (Q27). Antarmodul hanya lewat `index.ts`.
- F3: hanya app shell `web/` yang bergantung pada semua modul.
- F4: `@erpora/ui` adalah daun dan tidak mengimpor `@erpora/*` mana pun.

## Yang belum tersedia

- **`make:page`**: ditunda ke M3d (Q36 PK-5). Halaman baru ditulis manual mengikuti `modules/example/web`.
- **Kontrak UI extension**: belum diputuskan (ADR-011, fase lanjutan).
- **Desk kustom, halaman Desks lengkap, Start Guide, dan penulisan timpaan Desk** (permission `platform.desks.update`): M3b. Revisi M3a hanya membaca `GET /api/platform/desks`; `desk_overrides` belum diisi lewat API.
- **Layar notifikasi dan pengguna nyata**: masih sumber sementara (`emptyNotificationSource`, `anonymousUserSource`) sampai PRD Notifications dan PRD-001.
- **Layar bootstrap gagal** memakai kamus bawaan id/en (PK-4), bukan kunci i18n. Ini satu-satunya pengecualian aturan teks.
