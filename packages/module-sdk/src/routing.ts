import type { QueryClient } from "@tanstack/react-query";
import {
  type AnyRoute,
  createRootRouteWithContext,
  createRoute,
  type ErrorRouteComponent,
  type NotFoundRouteComponent,
  type Route,
  type RouteComponent,
} from "@tanstack/react-router";
import { ModuleDefinitionError } from "./define-module";
import type { Crumb, FrontendModule } from "./types";

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

/** Opsi route akar; diisi shell (kerangka aplikasi, 404, galat). */
export interface AppRootRouteOptions {
  readonly component?: RouteComponent;
  readonly notFoundComponent?: NotFoundRouteComponent;
  readonly errorComponent?: ErrorRouteComponent;
}

/** Route akar aplikasi: dipakai shell; modul tidak membuatnya. */
export function createAppRootRoute(options: AppRootRouteOptions = {}) {
  return createRootRouteWithContext<AppRouterContext>()({
    component: options.component,
    notFoundComponent: options.notFoundComponent,
    errorComponent: options.errorComponent,
  });
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
 * generik TanStack tidak bocor ke pemanggil. Rute indeks (`path: "/"`) ditolak dengan `ModuleDefinitionError`:
 * indeks `/<slug>` adalah Dashboard bawaan shell (M3a revisi R10.8).
 */
export function attachModuleRoutes(
  base: ModuleBaseRoute,
  children: readonly AnyModuleChildRoute[],
): AnyRoute {
  const basePath = (base.options as { path?: string }).path ?? "?";
  for (const child of children) {
    const path = (child.options as { path?: string }).path;
    if (path === "/" || path === "") {
      throw new ModuleDefinitionError(
        basePath,
        "registerRoutes",
        `rute indeks (path "${path}") tidak diizinkan: indeks ${basePath} adalah Dashboard bawaan shell; gunakan halaman bernama (mis. "/notes")`,
      );
    }
  }
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

type RouteLike = { options: { path?: string }; children?: unknown };

function childrenOf(route: RouteLike): RouteLike[] {
  const c = route.children;
  if (Array.isArray(c)) return c as RouteLike[];
  if (c && typeof c === "object") return Object.values(c) as RouteLike[];
  return [];
}

function collectPaths(routes: readonly RouteLike[], prefix: string, out: Set<string>): void {
  for (const r of routes) {
    const own = r.options.path ?? "";
    const full = `${prefix}${own.startsWith("/") || own === "" ? own : `/${own}`}`;
    if (full !== "") out.add(full);
    collectPaths(childrenOf(r), full, out);
  }
}

/**
 * Path halaman yang didaftarkan `registerRoutes` modul, relatif base route (mis. `/notes`, `/notes/archived`),
 * termasuk rute bersarang; tanpa indeks. Dipakai pengujian (`assertDeskRoutes`) dan analisis dampak (path terdampak).
 */
export function listModuleRoutePaths(module: FrontendModule): string[] {
  const base = createModuleBaseRoute(createAppRootRoute(), module.slug);
  const out = new Set<string>();
  collectPaths(module.registerRoutes(base) as unknown as RouteLike[], "", out);
  return [...out].sort();
}
