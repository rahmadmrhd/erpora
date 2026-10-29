import {
  type AnyModuleChildRoute,
  createRoute,
  lazyRouteComponent,
  type ModuleBaseRoute,
} from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";
import "./i18n/keys.gen";

/**
 * Route modul: anak langsung base route `/example` (sudah berguard oleh shell). Komponen halaman dimuat malas
 * (`import()` di sini) sehingga kodenya menjadi chunk terpisah dan tidak ada di JS awal (R5.4).
 */
export function registerRoutes(base: ModuleBaseRoute): readonly AnyModuleChildRoute[] {
  const index = createRoute({
    getParentRoute: () => base,
    path: "/",
    staticData: { crumb: crumb("example", "page.notes.title") },
    component: lazyRouteComponent(() => import("./pages/notes-page"), "NotesPage"),
  });
  return [index];
}
