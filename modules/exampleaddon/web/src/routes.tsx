import {
  type AnyModuleChildRoute,
  createRoute,
  lazyRouteComponent,
  type ModuleBaseRoute,
} from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";
import "./i18n/keys.gen";

/** Route modul: anak langsung base route `/example-addon`; komponen halaman dimuat malas (chunk terpisah, R5.4). */
export function registerRoutes(base: ModuleBaseRoute): readonly AnyModuleChildRoute[] {
  const index = createRoute({
    getParentRoute: () => base,
    path: "/",
    staticData: { crumb: crumb("example-addon", "page.summary.title") },
    component: lazyRouteComponent(() => import("./pages/summary-page"), "SummaryPage"),
  });
  return [index];
}
