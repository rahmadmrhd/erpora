import { readFileSync } from "node:fs";
import { join } from "node:path";
// Pembantu uji Dock, panel menu, dan menu pengguna (bukan bagian bundel): provider tanpa jaringan dengan katalog
// `platform` dan `ui` nyata dari JSON repo, daftar Desk dari server tiruan, sumber pengguna dan notifikasi, serta
// router memori (rute pemalar `$` agar navigasi ke path mana pun berhasil dan dapat diperiksa lewat `router`).

import { createClient, type PlatformPaths } from "@erpora/platform-contracts";
import {
  anonymousUserSource,
  type CurrentUserSource,
  CurrentUserSourceProvider,
  createTestI18n,
  type Desk,
  DeskProvider,
  emptyNotificationSource,
  makeBootstrap,
  type NotificationSource,
  NotificationSourceProvider,
  StaticBootstrapProvider,
  ThemeProvider,
} from "@erpora/platform-web";
import { SidebarProvider, TooltipProvider } from "@erpora/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { render } from "@testing-library/react";
import type { CSSProperties, ReactNode } from "react";
import { UiLabelsBridge } from "../providers/ui-labels-bridge";
import { type ShellActions, ShellActionsProvider } from "./shell-context";

// Katalog nyata dibaca dari berkas (bukan `import`) agar aturan F2 (tanpa deep import ke packages/*) tidak terpicu.
const repoRoot = join(import.meta.dir, "../../../..");
const readJson = (rel: string): unknown => JSON.parse(readFileSync(join(repoRoot, rel), "utf8"));

function flatten(obj: unknown, prefix = ""): Record<string, string> {
  if (typeof obj === "string") return { [prefix]: obj };
  if (typeof obj !== "object" || obj === null) return {};
  return Object.assign(
    {},
    ...Object.entries(obj).map(([k, v]) => flatten(v, prefix === "" ? k : `${prefix}.${k}`)),
  );
}

const CORE = {
  id: {
    platform: flatten(readJson("internal/platform/i18n/catalog/id.json")),
    ui: flatten(readJson("packages/ui/i18n/id.json")),
  },
  en: {
    platform: flatten(readJson("internal/platform/i18n/catalog/en.json")),
    ui: flatten(readJson("packages/ui/i18n/en.json")),
  },
} as const;

const title = (s: string): string => s.charAt(0).toUpperCase() + s.slice(1);

/** Katalog fixture per Desk: judul Desk `desk.title` dan label menu `nav.<id>` dari id-nya (mis. `notes` jadi "Notes"). */
function deskCatalogs(desks: readonly Desk[]): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  const add = (ns: string, key: string, text: string) => {
    const catalog = out[ns] ?? {};
    catalog[key] = text;
    out[ns] = catalog;
  };
  for (const d of desks) {
    add(d.label.ns, d.label.key, title(d.id));
    for (const m of d.menus) {
      add(m.label.ns, m.label.key, title(m.id));
      if (m.type === "group") for (const c of m.children) add(c.label.ns, c.label.key, title(c.id));
    }
  }
  return out;
}

export interface ShellHarnessOptions {
  desks?: readonly Desk[];
  path?: string;
  locale?: "id" | "en";
  user?: CurrentUserSource;
  notificationSource?: NotificationSource;
  actions?: Partial<ShellActions>;
  /** Keadaan sidebar terkendali; tanpa ini tak terkendali dan terbuka. */
  sidebar?: { open: boolean; onOpenChange: (open: boolean) => void };
  /** Keadaan awal sidebar tak terkendali (bawaan terbuka). */
  defaultSidebarOpen?: boolean;
  /** Menyetel `--dock-width` dan lebar sidebar seperti `AppShell`. */
  style?: CSSProperties;
}

const noopActions: ShellActions = {
  openPalette: () => {},
  openNotifications: () => {},
  openSettings: () => {},
  reload: () => {},
};

/** Merender `ui` di dalam provider shell. `ui` berada di akar router sehingga `Link` dan `useNavigate` bekerja. */
export function renderShell(ui: ReactNode, opts: ShellHarnessOptions = {}) {
  const desks = opts.desks ?? [];
  const locale = opts.locale ?? "id";
  const catalogs: Record<string, Record<string, string>> = { ...CORE[locale] };
  for (const [ns, messages] of Object.entries(deskCatalogs(desks))) {
    catalogs[ns] = { ...catalogs[ns], ...messages };
  }
  const i18n = createTestI18n(catalogs, { locale });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const history = createMemoryHistory({ initialEntries: [opts.path ?? "/"] });
  const rootRoute = createRootRoute({
    component: () => (
      <>
        {ui}
        <Outlet />
      </>
    ),
  });
  const index = createRoute({ getParentRoute: () => rootRoute, path: "/", component: () => null });
  const splat = createRoute({ getParentRoute: () => rootRoute, path: "$", component: () => null });
  const router = createRouter({ routeTree: rootRoute.addChildren([index, splat]), history });
  const client = createClient<PlatformPaths>({
    fetch: (async () =>
      new Response(JSON.stringify({ data: { desks, revision: "r1" } }), {
        status: 200,
        headers: { "Content-Type": "application/json", ETag: '"e"' },
      })) as unknown as typeof fetch,
  });
  const bootstrap = makeBootstrap({
    i18n: {
      defaultLocale: "id",
      locales: [
        { code: "id", name: "Indonesia" },
        { code: "en", name: "English" },
      ],
      revisions: { platform: "p1", ui: "u1" },
    },
  });
  const actions: ShellActions = { ...noopActions, ...opts.actions };
  const result = render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <StaticBootstrapProvider value={bootstrap}>
          <i18n.Provider>
            <UiLabelsBridge>
              <CurrentUserSourceProvider source={opts.user ?? anonymousUserSource}>
                <NotificationSourceProvider
                  source={opts.notificationSource ?? emptyNotificationSource}
                >
                  <DeskProvider client={client}>
                    <TooltipProvider>
                      <SidebarProvider
                        style={opts.style}
                        defaultOpen={opts.defaultSidebarOpen ?? true}
                        open={opts.sidebar?.open}
                        onOpenChange={opts.sidebar?.onOpenChange}
                      >
                        <ShellActionsProvider value={actions}>
                          <RouterProvider router={router} />
                        </ShellActionsProvider>
                      </SidebarProvider>
                    </TooltipProvider>
                  </DeskProvider>
                </NotificationSourceProvider>
              </CurrentUserSourceProvider>
            </UiLabelsBridge>
          </i18n.Provider>
        </StaticBootstrapProvider>
      </ThemeProvider>
    </QueryClientProvider>,
  );
  return { ...result, router, history, queryClient };
}

/** Mengubah lebar jendela uji (happy-dom) dan memberi tahu pendengar `resize`. */
export function setViewportWidth(width: number): void {
  const w = window as unknown as { happyDOM?: { setViewport?: (v: { width: number }) => void } };
  if (w.happyDOM?.setViewport) w.happyDOM.setViewport({ width });
  else Object.defineProperty(window, "innerWidth", { value: width, configurable: true });
  window.dispatchEvent(new Event("resize"));
}
