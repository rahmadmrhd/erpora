import type { QueryClient } from "@tanstack/react-query";
import {
  type AnyRoute,
  createRootRouteWithContext,
  createRoute,
  type ErrorRouteComponent,
  type Route,
  type RouteComponent,
} from "@tanstack/react-router";
import type { Crumb } from "./types";

/**
 * Satu-satunya tempat tipe route didefinisikan untuk modul (M3a K-28): modul tidak pernah menerima `AnyRoute`
 * dan tidak ada deklarasi `Register` global (pohon modul dirakit saat runtime dari registry).
 */

/** Konteks router aplikasi; dipakai `beforeLoad` guard dan loader halaman. */
export interface AppRouterContext {
  readonly queryClient: QueryClient;
}

declare module "@tanstack/react-router" {
  interface StaticDataRouteOption {
    /** Breadcrumb halaman: satu tingkat atau beberapa. Sumber tunggal breadcrumb dan judul dokumen (R5.5). */
    crumb?: Crumb | readonly Crumb[];
  }
}

/** Route akar aplikasi: dipakai shell; modul tidak membuatnya. */
export function createAppRootRoute() {
  return createRootRouteWithContext<AppRouterContext>()();
}
export type AppRootRoute = ReturnType<typeof createAppRootRoute>;

/** Opsi base route modul; diisi shell (guard, galat). Modul tidak memanggil ini. */
export interface ModuleBaseRouteOptions {
  readonly component?: RouteComponent;
  readonly errorComponent?: ErrorRouteComponent;
  readonly beforeLoad?: (ctx: { context: AppRouterContext }) => void | Promise<void>;
  readonly staticData?: { crumb?: Crumb | readonly Crumb[] };
}

/** Membuat base route modul di `/<slug>` di bawah root aplikasi. Dipanggil shell, bukan modul. */
export function createModuleBaseRoute(
  root: AppRootRoute,
  slug: string,
  options: ModuleBaseRouteOptions = {},
) {
  return createRoute({
    getParentRoute: () => root,
    path: `/${slug}`,
    component: options.component,
    errorComponent: options.errorComponent,
    beforeLoad: options.beforeLoad,
    staticData: options.staticData,
  });
}
export type ModuleBaseRoute = ReturnType<typeof createModuleBaseRoute>;

// biome-ignore lint/suspicious/noExplicitAny: bentuk generik TanStack Router sengaja dibiarkan terbuka
type Open = any;

/**
 * Route yang `getParentRoute()`-nya base route modul. Parameter generik lain dibiarkan terbuka sehingga tipe
 * parameter/loader tiap halaman tetap lokal pada objek route modul.
 */
export type AnyModuleChildRoute = Route<
  Open,
  ModuleBaseRoute,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open,
  Open
>;

/**
 * Menempelkan anak modul ke base route (dipakai shell setelah `registerRoutes`). Satu pintu sehingga tipe route
 * generik TanStack tidak bocor ke pemanggil.
 */
export function attachModuleRoutes(
  base: ModuleBaseRoute,
  children: readonly AnyModuleChildRoute[],
): AnyRoute {
  return (base as AnyRoute).addChildren(children as unknown as AnyRoute[]);
}

export {
  createLazyRoute,
  createRoute,
  Link,
  lazyRouteComponent,
  Outlet,
  useNavigate,
} from "@tanstack/react-router";
