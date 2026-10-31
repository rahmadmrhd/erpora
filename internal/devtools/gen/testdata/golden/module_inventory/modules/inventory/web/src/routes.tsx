import {
  type AnyModuleChildRoute,
  createRoute,
  lazyRouteComponent,
  type ModuleBaseRoute,
} from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";
import "./i18n/keys.gen";

/**
 * Route modul: anak langsung base route `/inventory` (sudah berguard oleh shell). Indeks (`/`) adalah Dashboard
 * bawaan shell dan dilarang didaftarkan modul; halaman modul didaftarkan sebagai anak (mis. `/items`) dan dirujuk
 * oleh `desk.menus[].to` di module.json. Komponen halaman dimuat malas (`import()` di sini) sehingga kodenya
 * menjadi chunk terpisah dan tidak ada di JS awal.
 */
export function registerRoutes(base: ModuleBaseRoute): readonly AnyModuleChildRoute[] {
  const items = createRoute({
    getParentRoute: () => base,
    path: "/items",
    staticData: { crumb: crumb("inventory", "page.items.title") },
    component: lazyRouteComponent(() => import("./pages/items-page"), "ItemsPage"),
  });
  return [items];
}
