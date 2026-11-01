import {
  type AnyModuleChildRoute,
  createRoute,
  lazyRouteComponent,
  type ModuleBaseRoute,
} from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";
import "./i18n/keys.gen";

/** Route modul: anak langsung base route `/example-addon` (halaman `/example-addon/summary`; indeks adalah Dashboard bawaan shell); komponen halaman dimuat malas (chunk terpisah, R5.4). */
export function registerRoutes(base: ModuleBaseRoute): readonly AnyModuleChildRoute[] {
  const summary = createRoute({
    getParentRoute: () => base,
    path: "/summary",
    staticData: { crumb: crumb("example-addon", "page.summary.title") },
    component: lazyRouteComponent(() => import("./pages/summary-page"), "SummaryPage"),
  });
  return [summary];
}
