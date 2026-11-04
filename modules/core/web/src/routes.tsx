import {
  type AnyModuleChildRoute,
  createRoute,
  lazyRouteComponent,
  type ModuleBaseRoute,
} from "@erpora/module-sdk";
import { crumb } from "@erpora/platform-web";
import "./i18n/keys.gen";

/**
 * Route modul core: `/core/change-password` (halaman modul biasa di dalam shell; indeks `/core` adalah Dashboard bawaan
 * shell). Layar login, MFA, lupa, dan reset sandi BUKAN route modul: itu gerbang pra-bootstrap yang dipasang shell
 * (PRD-001 Q33 butir 2) dan diekspor sebagai `AuthScreens`. Komponen halaman dimuat malas.
 */
export function registerRoutes(base: ModuleBaseRoute): readonly AnyModuleChildRoute[] {
  const changePassword = createRoute({
    getParentRoute: () => base,
    path: "/change-password",
    staticData: { crumb: crumb("core", "changePassword.title") },
    component: lazyRouteComponent(
      () => import("./pages/change-password-page"),
      "ChangePasswordPage",
    ),
  });
  return [changePassword];
}
