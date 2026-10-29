# Panduan modul frontend

Panduan ini menjelaskan cara menulis frontend untuk modul ERPora: struktur folder, deklarasi modul dengan `defineModule`, route lazy, aksi halaman, teks lewat i18n, pemakaian `@erpora/ui`, uji komponen, dan anggaran bundle. Contoh acuannya adalah `modules/example/web` dan `modules/exampleaddon/web`.

Keputusan yang melandasinya: [ADR-011](../adr/0011-kontrak-ui-extension-dan-design-system.md) (design system, batas paket, registry), PRD-000 FR-08 dan FR-09, serta spec `.kiro/specs/m3a-frontend-foundation-shell/`. Panduan teks dan kunci terjemahan ada di [`i18n.md`](i18n.md).

## Ringkasan

- Frontend modul ada di `modules/<dir>/web`, package `@erpora/module-<dir>`, dan dirakit oleh app shell `web/` (aturan F3).
- Modul hanya mengimpor `@erpora/ui`, `@erpora/platform-web`, `@erpora/module-sdk`, `@erpora/platform-contracts`, dan `@erpora/module-<x>` yang ada di `manifest.dependencies` (aturan F2). Impor lewat `index.ts`; deep import dilarang.
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
    ├── module.ts       # defineModule(...): navigasi, registerRoutes, offline
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
  navigation: [
    { type: "item", id: "notes", label: msg("example", "nav.notes"), to: "/", icon: "file-text" },
  ],
  registerRoutes,
  offline: defaultOfflinePolicy,
});
```

Bentuk `FrontendModule` (`packages/module-sdk/src/types.ts`):

| Field | Keterangan |
| --- | --- |
| `key` | Sama dengan `manifest.key`, pola `^[a-z0-9]+(\.[a-z0-9-]+)+$` |
| `slug` | Sama dengan `manifest.slug`; basePath `/<slug>` dan namespace i18n |
| `version` | Semver; registry memberi peringatan bila berbeda dari versi backend di bootstrap |
| `title?` | `MessageRef` opsional, crumb dasar modul (Q36, PK-6) |
| `navigation` | Daftar `NavigationItem` (`type: "item"`) atau `NavigationGroup` (`type: "group"`, anak berupa item). Hanya satu tingkat grup |
| `registerRoutes` | `(base: ModuleBaseRoute) => AnyModuleChildRoute[]` |
| `offline` | `OfflinePolicy`, mengikuti `docs/architecture/offline-strategy.md` §3; untuk modul online saja pakai `defaultOfflinePolicy` |

`defineModule` melempar `ModuleDefinitionError` (dengan `key` modul dan `path` bidang yang salah) bila: `key`/`slug`/`version` tidak valid, `id` navigasi ganda dalam modul, `to` tidak diawali `/`, grup tanpa anak, `icon` kosong bila diisi, atau `offline` tidak valid. Objek yang dikembalikan dibekukan (`Object.freeze`).

Registry di shell (`@erpora/platform-web`, `registry/`) mencocokkan modul dengan bootstrap berdasarkan `key`:

- Modul tidak ada di bootstrap: diabaikan dan dicatat sebagai peringatan `not_in_bootstrap`.
- Versi berbeda dari backend: peringatan `version_mismatch`.
- `slug` berbeda dari bootstrap: peringatan `slug_mismatch`; `offline` tidak cocok: `offline_mismatch`.
- Key atau slug ganda: `DuplicateModuleKeyError` / `DuplicateModuleSlugError`.
- Hanya status `enabled` yang memunculkan navigasi (`selectNavigation`); `to` menjadi absolut (`/<slug>` + `to`).

## Route modul (lazy)

Modul tidak pernah menerima `AnyRoute` dan tidak membuat base route. Shell membuat base route `/<slug>` yang sudah berguard, lalu memanggil `registerRoutes(base)`:

```tsx
import { type AnyModuleChildRoute, createRoute, lazyRouteComponent, type ModuleBaseRoute } from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";
import "./i18n/keys.gen";

export function registerRoutes(base: ModuleBaseRoute): readonly AnyModuleChildRoute[] {
  const index = createRoute({
    getParentRoute: () => base,
    path: "/",
    staticData: { crumb: crumb("example", "page.notes.title") },
    component: lazyRouteComponent(() => import("./pages/notes-page"), "NotesPage"),
  });
  return [index];
}
```

- Halaman dimuat dengan `import()` agar menjadi chunk terpisah (R5.4). Modul yang tidak dibuka tidak ikut dimuat.
- `staticData.crumb` adalah sumber tunggal breadcrumb dan judul dokumen. Untuk teks data (mis. nama record) pakai `{ text: "..." }`, bukan `crumb(...)`.
- Gunakan `createRoute` dan `lazyRouteComponent` dari `@erpora/module-sdk`, bukan langsung dari `@tanstack/react-router`.

**Guard.** Shell memasang `beforeLoad` pada base route: bila status modul di bootstrap bukan `enabled`, route melempar `ModuleInactiveError` dan halaman "Modul tidak aktif" tampil **sebelum** chunk halaman dimuat. Saat modul di-disable lewat `/settings/modules`, bootstrap di-refetch (revision berubah), router di-`invalidate`, dan menu hilang tanpa reload.

## Aksi halaman

Halaman mendeklarasikan tombol aksinya sendiri; shell merendernya di navbar dan membersihkannya saat halaman berganti:

```tsx
usePageActions([
  { id: "refresh", label: refresh, icon: "refresh-cw", onSelect: () => void reload(), pinned: true },
]);
```

- `PageAction`: `id`, `label` (sudah diterjemahkan oleh pemanggil), `icon?` (nama di registry `@erpora/ui`), `variant?` (`primary`, `outline`, `ghost`, `destructive`), `onSelect`, `disabled?`, `loading?`, `pinned?`.
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

Aturan lengkap kunci, katalog, dan alur penerjemah ada di [`i18n.md`](i18n.md).

## Pemakaian `@erpora/ui`

- Impor hanya dari barrel `@erpora/ui` (`src/index.ts`). Entry sekunder yang dikecualikan (mis. `@erpora/ui/toaster`) dimuat lazy dan hanya bila perlu.
- Komponen yang tersedia mencakup Button, Badge, BadgeStatus, Avatar, Alert, Checkbox, Label, Separator, Skeleton, Kbd, LoadingIcon, EmptyState, ErrorPage, Dialog, AlertDialog, Sheet, Popover, Tooltip, DropdownMenu, Collapsible, Breadcrumb, Table, dan komponen Sidebar. Daftar lengkap ada di `packages/ui/src/index.ts`.
- Komponen tidak menanam teks. Teks bawaan (mis. tombol tutup dialog) diisi shell lewat `UiLabelsProvider`. Modul tidak perlu mengatur ini.
- Warna dan radius lewat token semantik (`bg-background`, `text-muted-foreground`, `bg-status-success-bg`, `border-border`, dan seterusnya). Nilai palet langsung (`zinc-100`, `red-500`, `#fff`, `rgb(...)`) dilarang di luar `theme.css` dan diuji.
- Ikon: `icon` pada navigasi dan aksi memakai nama kebab dari `iconRegistry` (mis. `file-text`, `bell-ring`). Nama tak dikenal tampil sebagai lingkaran berinisial dan dicatat sekali di development.
- `cn` dari `@erpora/ui` untuk menggabungkan kelas.

## Uji

- Jalankan dari root: `bun test --pass-with-no-tests` (atau `task web:test`). Setiap paket yang punya skrip `test` (`web`, `packages/ui`, `packages/platform-web`, `packages/platform-contracts`, `packages/module-sdk`, `modules/example/web`, `modules/exampleaddon/web`, dan modul baru hasil `make:module --frontend`) punya `bunfig.toml` dengan `[test] preload` ke `web/test/dom.ts` lalu `web/test/setup.ts` (path relatif terhadap paket). `dom.ts` harus dimuat lebih dulu.
- Stack: `bun test`, `@testing-library/react`, `user-event`, `jest-dom`, dan happy-dom. Radix (DropdownMenu, Dialog) dan perilaku ARIA diuji dengan peran dan keyboard.
- Untuk halaman yang membutuhkan bootstrap, pakai `StaticBootstrapProvider` dan `makeBootstrap` (`@erpora/platform-web`). Untuk teks, pakai `createTestI18n({ ns: { "kunci.lengkap": "pesan" } })`, atau `testing.tsx` modul yang memuat katalog JSON nyata.
- Panggilan API di test memakai `fetch` palsu yang merekam permintaan. Test harus membuktikan bahwa body tidak memuat `tenantId` atau `userId`.
- Uji tipe: kunci atau argumen yang salah harus gagal `tsc`. Gunakan berkas `*.check.ts` (dijalankan lewat `typecheck`) dengan `@ts-expect-error`, seperti `packages/platform-web/src/msg/msg.check.ts`.
- End-to-end browser (Playwright) belum tersedia; direncanakan di M3g.

## Anggaran bundle dan performa

- Anggaran entry shell: **200 KB gzip** (NFR-04). Build (`web/build.ts`) gagal bila JS awal melampauinya (`BUDGET` di `web/tools/budget.ts`). Build juga melaporkan JS awal, chunk lazy, CSS, font, dan 15 kontributor terbesar.
- Entry terukur pada 29 Okt 2026: 187,9 KB gzip (Q38). Ruang tersisa sempit.
- Aturan untuk modul:
  - Halaman dan komponen berat dimuat lewat `lazyRouteComponent(() => import(...))`.
  - TanStack DB dimuat malas lewat `useDb()` (K-21); modul yang memakai koleksi tidak boleh mengimpornya statis.
  - Hindari dependensi baru di entry; bila perlu, taruh di chunk halaman.
- Periksa dengan `bun run --filter @erpora/web build`.

## Perintah

| Perintah | Keterangan |
| --- | --- |
| `bun run --filter '*' typecheck` | Typecheck semua paket frontend |
| `bun run --filter '*' lint` | Biome (termasuk larangan `dangerouslySetInnerHTML`) |
| `bun run --filter @erpora/web deps` | dependency-cruiser: aturan F1 sampai F4 |
| `bun test --pass-with-no-tests` | Uji dari root |
| `bun run --filter @erpora/web build` | Build, anggaran bundle, dan csp-check |
| `task web:check` | Gabungan: install beku, typecheck, lint, `i18n:generate --check`, deps, test, build |
| `task web:test` | Hanya uji frontend |
| `task i18n:check` / `task i18n:generate` | Lihat [`i18n.md`](i18n.md) |

## Aturan batas (ringkas)

- F1: `@erpora/ui`, `@erpora/platform-web`, dan `@erpora/module-sdk` tidak bergantung pada `@erpora/module-*`.
- F2: frontend modul hanya mengimpor paket yang diizinkan dan `@erpora/module-<x>` dari `manifest.dependencies`; dependensi `package.json` modul harus cocok dengan manifest (Q27). Antarmodul hanya lewat `index.ts`.
- F3: hanya app shell `web/` yang bergantung pada semua modul.
- F4: `@erpora/ui` adalah daun dan tidak mengimpor `@erpora/*` mana pun.

## Yang belum tersedia

- **`make:page`**: ditunda ke M3d (Q36 PK-5). Halaman baru ditulis manual mengikuti `modules/example/web`.
- **Kontrak UI extension**: belum diputuskan (ADR-011, fase lanjutan).
- **Layar notifikasi dan pengguna nyata**: masih sumber sementara (`emptyNotificationSource`, `anonymousUserSource`) sampai PRD Notifications dan PRD-001.
- **Layar bootstrap gagal** memakai kamus bawaan id/en (PK-4), bukan kunci i18n. Ini satu-satunya pengecualian aturan teks.
