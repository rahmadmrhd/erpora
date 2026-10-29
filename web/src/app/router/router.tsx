import {
  attachModuleRoutes,
  createAppRootRoute,
  createModuleBaseRoute,
  createRoute,
  lazyRouteComponent,
  Outlet,
} from "@erpora/module-sdk";
import {
  BOOTSTRAP_KEY,
  type Bootstrap,
  crumb,
  type ResolvedModule,
  useModuleRegistry,
} from "@erpora/platform-web";
import type { QueryClient } from "@tanstack/react-query";
import { createRouter, type RouterHistory } from "@tanstack/react-router";
import {
  DashboardPage,
  ModuleInactiveError,
  ModuleInactivePage,
  NotFoundPage,
  RouteErrorPage,
} from "../pages/pages";
import { AppShell } from "../shell/app-shell";

/** Guard pertama (`beforeLoad`): modul bukan `enabled` pada bootstrap saat ini menolak sebelum chunk halaman dimuat. */
export function assertModuleEnabled(queryClient: QueryClient, moduleKey: string): void {
  const bootstrap = queryClient.getQueryData<Bootstrap>(BOOTSTRAP_KEY);
  const status = bootstrap?.modules.find((m) => m.key === moduleKey)?.status;
  if (status !== "enabled") throw new ModuleInactiveError(moduleKey, status);
}

/** Guard kedua (reaktif): men-disable modul saat halamannya terbuka langsung menggantikan kontennya tanpa reload. */
function ModuleBase({ moduleKey }: { moduleKey: string }) {
  const registry = useModuleRegistry();
  if (!registry.isEnabled(moduleKey))
    return <ModuleInactivePage status={registry.statusOf(moduleKey)} />;
  return <Outlet />;
}

export interface BuildRouterArgs {
  modules: readonly ResolvedModule[];
  queryClient: QueryClient;
  history?: RouterHistory;
}

/**
 * Router aplikasi, dibangun sekali setelah bootstrap (K-11): `/`, `/settings/modules` (lazy), dan satu base route per
 * modul frontend yang ada di bootstrap dengan guard dua lapis. Modul frontend yang tidak ada di bootstrap tidak
 * mendaftarkan route sehingga alamatnya menjadi 404 (R6.3).
 */
export function buildRouter({ modules, queryClient, history }: BuildRouterArgs) {
  const root = createAppRootRoute({
    component: AppShell,
    notFoundComponent: NotFoundPage,
    errorComponent: ({ error, reset }) => <RouteErrorPage error={error} reset={reset} />,
  });

  const home = createRoute({
    getParentRoute: () => root,
    path: "/",
    component: DashboardPage,
    staticData: { crumb: crumb("platform", "nav.home") },
  });

  const settingsModules = createRoute({
    getParentRoute: () => root,
    path: "/settings/modules",
    component: lazyRouteComponent(() => import("../settings/modules/page"), "ModulesPage"),
    staticData: {
      crumb: [crumb("platform", "nav.settings"), crumb("platform", "nav.settingsModules")],
    },
  });

  const moduleRoutes = modules.map(({ module }) => {
    const base = createModuleBaseRoute(root, module.slug, {
      component: () => <ModuleBase moduleKey={module.key} />,
      beforeLoad: ({ context }) => assertModuleEnabled(context.queryClient, module.key),
      errorComponent: ({ error, reset }) =>
        error instanceof ModuleInactiveError ? (
          <ModuleInactivePage status={error.status} />
        ) : (
          <RouteErrorPage error={error} reset={reset} />
        ),
      staticData: module.title ? { crumb: { ref: module.title } } : undefined,
    });
    return attachModuleRoutes(base, module.registerRoutes(base));
  });

  return createRouter({
    routeTree: root.addChildren([home, settingsModules, ...moduleRoutes]),
    context: { queryClient },
    defaultPreload: "intent",
    history,
  });
}

export type AppRouterInstance = ReturnType<typeof buildRouter>;
