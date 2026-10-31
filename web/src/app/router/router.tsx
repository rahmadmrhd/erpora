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
  useDeskValidation,
  useModuleRegistry,
} from "@erpora/platform-web";
import type { QueryClient } from "@tanstack/react-query";
import { createRouter, type RouterHistory, useRouterState } from "@tanstack/react-router";
import { DashboardPage } from "../pages/dashboard-page";
import { DeskEntry } from "../pages/desk-entry";
import {
  ModuleInactiveError,
  ModuleInactivePage,
  NotFoundPage,
  RouteErrorPage,
} from "../pages/pages";
import { TodoPlaceholderPage } from "../pages/todo-placeholder";
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

/**
 * Validasi Desk pada tiap perubahan lokasi (R9.3): dipasang sekali di bawah router, di samping kerangka. Tidak
 * merender apa pun; hanya menyelaraskan Desk saat ini dengan halaman yang dibuka, tanpa memuat ulang.
 */
function DeskValidator() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  useDeskValidation(pathname);
  return null;
}

function RootLayout() {
  return (
    <>
      <DeskValidator />
      <AppShell />
    </>
  );
}

export interface BuildRouterArgs {
  modules: readonly ResolvedModule[];
  queryClient: QueryClient;
  history?: RouterHistory;
}

/**
 * Router aplikasi, dibangun sekali setelah bootstrap (K-11), sesuai tabel rute design revisi M3a: `/` (pintu masuk
 * Desk), `/platform` (Dashboard Desk Platform), `/platform/modules` (lazy) dan `/platform/todo`, dan satu base route
 * per modul frontend yang ada di bootstrap dengan guard dua lapis: indeks `/<slug>` adalah Dashboard bawaan (milik
 * shell) dan anak modul di `/<slug>/<halaman>`. Modul frontend yang tidak ada di bootstrap tidak mendaftarkan route
 * sehingga alamatnya menjadi 404 (R6.3). `/desks` sengaja tidak terdaftar (namespace tercadang untuk M3b).
 */
export function buildRouter({ modules, queryClient, history }: BuildRouterArgs) {
  const root = createAppRootRoute({
    component: RootLayout,
    notFoundComponent: NotFoundPage,
    errorComponent: ({ error, reset }) => <RouteErrorPage error={error} reset={reset} />,
  });

  const entry = createRoute({
    getParentRoute: () => root,
    path: "/",
    component: DeskEntry,
    staticData: { crumb: crumb("platform", "desks.title") },
  });

  const platformDashboard = createRoute({
    getParentRoute: () => root,
    path: "/platform",
    component: () => <DashboardPage deskId="platform" />,
    staticData: { crumb: crumb("platform", "desk.dashboard") },
  });

  const platformModules = createRoute({
    getParentRoute: () => root,
    path: "/platform/modules",
    component: lazyRouteComponent(() => import("../platform/modules/page"), "ModulesPage"),
    staticData: { crumb: crumb("platform", "desk.modules") },
  });

  const platformTodo = createRoute({
    getParentRoute: () => root,
    path: "/platform/todo",
    component: TodoPlaceholderPage,
    staticData: { crumb: crumb("platform", "fixedItems.todo") },
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
    });
    // Indeks `/<slug>` = Dashboard bawaan milik shell; modul tidak boleh mendaftarkannya (ditolak attachModuleRoutes).
    const index = createRoute({
      getParentRoute: () => base,
      path: "/",
      component: () => <DashboardPage deskId={module.slug} />,
      staticData: { crumb: crumb("platform", "desk.dashboard") },
    });
    return attachModuleRoutes(base, module.registerRoutes(base), { index });
  });

  return createRouter({
    routeTree: root.addChildren([
      entry,
      platformDashboard,
      platformModules,
      platformTodo,
      ...moduleRoutes,
    ]),
    context: { queryClient },
    defaultPreload: "intent",
    history,
  });
}

export type AppRouterInstance = ReturnType<typeof buildRouter>;
