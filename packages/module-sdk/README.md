# module-sdk

Paket TypeScript untuk modul frontend (`@erpora/module-sdk`). Tidak bergantung pada `@erpora/platform-web` maupun `@erpora/ui`;
hanya pada tipe `@tanstack/react-router` dan `@tanstack/react-query` (`peerDependencies`, versi dipin).

## Isi

- **`ModuleManifest`**: tipe hasil generate dari `contracts/module.schema.json` (`src/manifest.gen.ts`, JANGAN DIEDIT; perbarui
  lewat `erpora contracts:generate`). `src/manifest.check.ts` memastikan tipe itu menerima manifest `example` dan menolak nilai
  yang melanggar schema.
- **`defineModule()` dan `FrontendModule`**: deklarasi modul frontend (PRD-000 FR-09). Field: `key`, `slug`, `version`
  (sama dengan manifest; `key`/`slug`/`version` divalidasi dengan pola yang sama dengan schema), `title?`, `navigation[]`,
  `registerRoutes`, dan `offline`. `defineModule` memvalidasi dan mengembalikan salinan beku; definisi yang tidak valid
  melempar `ModuleDefinitionError` yang menyebut bidangnya (mis. `navigation[1].to`). `slug` disimpulkan sebagai tipe literal.
- **`NavigationEntry`**: item (`{ type: "item", id, label, to, icon? }`) atau grup satu tingkat (`{ type: "group", ..., children }`).
  `to` relatif terhadap basePath modul (`"/"` menjadi `/<slug>`, `"/notes"` menjadi `/<slug>/notes`). `label` bertipe
  `MessageRef { ns, key }`; buat dengan `msg("<slug>", "nav.notes")` dari `@erpora/platform-web` agar kunci terketik.
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
import { defineModule, defaultOfflinePolicy, createRoute } from "@erpora/module-sdk";
import { msg, crumb } from "@erpora/platform-web";

export const frontendModule = defineModule({
  key: "acme.example",
  slug: "example",
  version: "1.0.0",
  navigation: [
    { type: "item", id: "notes", label: msg("example", "nav.notes"), to: "/", icon: "file-text" },
  ],
  registerRoutes: (base) => [
    createRoute({
      getParentRoute: () => base,
      path: "/",
      staticData: { crumb: crumb("example", "nav.notes") },
    }).lazy(() => import("./pages/home").then((m) => m.Route)),
  ],
  offline: defaultOfflinePolicy,
});
```

## Pengujian

`bun test` (dari root atau paket) memakai preload `web/test` (DOM happy-dom). `src/routing.check.ts` adalah fixture tipe (positif
dan negatif, `@ts-expect-error`) yang dijalankan lewat `bun run typecheck`.
