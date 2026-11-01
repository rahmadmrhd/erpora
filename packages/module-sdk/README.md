# module-sdk

Paket TypeScript untuk modul frontend (`@erpora/module-sdk`). Tidak bergantung pada `@erpora/platform-web` maupun `@erpora/ui`;
hanya pada tipe `@tanstack/react-router` dan `@tanstack/react-query` (`peerDependencies`, versi dipin).

## Isi

- **`ModuleManifest`**: tipe hasil generate dari `contracts/module.schema.json` (`src/manifest.gen.ts`, JANGAN DIEDIT; perbarui
  lewat `erpora contracts:generate`). `src/manifest.check.ts` memastikan tipe itu menerima manifest `example` dan menolak nilai
  yang melanggar schema.
- **`defineModule()` dan `FrontendModule`**: deklarasi modul frontend (PRD-000 FR-09). Field: `key`, `slug`, `version`
  (sama dengan manifest; `key`/`slug`/`version` divalidasi dengan pola yang sama dengan schema), `title?`, `registerRoutes`,
  dan `offline`. `defineModule` memvalidasi dan mengembalikan salinan beku; definisi yang tidak valid melempar
  `ModuleDefinitionError` yang menyebut bidangnya. `slug` disimpulkan sebagai tipe literal.
- **Menu bukan bagian `FrontendModule`** (revisi M3a): menu dan Desk modul dideklarasikan di manifest (`module.json`
  properti `desk`) dan datang dari server (`GET /api/platform/desks`). `registerRoutes` hanya mendaftarkan halaman
  `/<slug>/<halaman>`; rute indeks (`path: "/"`) ditolak `attachModuleRoutes` dengan `ModuleDefinitionError`, karena indeks
  `/<slug>` adalah Dashboard bawaan shell.
- **`@erpora/module-sdk/testing`**: `assertDeskRoutes(module, manifest.desk)` memastikan setiap `to` menu di manifest punya rute
  terdaftar di `registerRoutes` (dan tidak ada `to` indeks). Pakai di tes tiap modul.
- **`OfflinePolicy` dan `defaultOfflinePolicy`**: persis `docs/architecture/offline-strategy.md` §3 (sisi UI; manifest backend
  adalah otoritas).
- **Routing** (`src/routing.ts`): satu-satunya tempat tipe route didefinisikan. `registerRoutes(base)` menerima
  `ModuleBaseRoute` (bukan `AnyRoute`) dan mengembalikan `AnyModuleChildRoute[]`: route yang `getParentRoute()`-nya base itu.
  Anak dibuat dengan `createRoute({ getParentRoute: () => base, path: "/notes", staticData: { crumb: ... } })` dan halaman dimuat
  malas lewat `.lazy(() => import("./pages/notes").then((m) => m.Route))` atau `lazyRouteComponent`. `staticData.crumb`
  (`Crumb`: `{ ref: MessageRef }` atau `{ text }`) adalah sumber tunggal breadcrumb dan judul dokumen. Primitif `createRoute`,
  `createLazyRoute`, `lazyRouteComponent`, `Link`, `Outlet`, `useNavigate` diekspor ulang sehingga modul tidak mengimpor
  TanStack Router langsung. Shell memakai `createAppRootRoute`, `createModuleBaseRoute`, dan `attachModuleRoutes`.
  Tidak ada deklarasi `Register` global (keamanan tipe parameter/loader ada pada objek route lokal modul).

```ts
import {
  defineModule,
  defaultOfflinePolicy,
  createRoute,
  lazyRouteComponent,
} from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";

export const frontendModule = defineModule({
  key: "acme.example",
  slug: "example",
  version: "1.0.0",
  registerRoutes: (base) => [
    createRoute({
      getParentRoute: () => base,
      path: "/notes", // menu "notes" -> "/notes" dideklarasikan di module.json (desk.menus)
      staticData: { crumb: crumb("example", "page.notes.title") },
      component: lazyRouteComponent(() => import("./pages/notes-page"), "NotesPage"),
    }),
  ],
  offline: defaultOfflinePolicy,
});
```

## Pengujian

`bun test` (dari root atau paket) memakai preload `web/test` (DOM happy-dom). `src/routing.check.ts` adalah fixture tipe (positif
dan negatif, `@ts-expect-error`) yang dijalankan lewat `bun run typecheck`.
