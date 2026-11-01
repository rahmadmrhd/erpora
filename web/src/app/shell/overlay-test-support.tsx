import { readFileSync } from "node:fs";
import { join } from "node:path";
// Pembantu uji overlay shell (panel Notifikasi, dialog Settings; bukan bagian bundel): `I18nProvider` nyata dengan
// `fetch` palsu yang menyajikan katalog `platform` dan `ui` dari JSON repo, bootstrap statis, router memori, dan
// sumber notifikasi.

import {
  createPlatformClient,
  getActiveLocale,
  I18nProvider,
  makeBootstrap,
  type NotificationSource,
  NotificationSourceProvider,
  StaticBootstrapProvider,
  ThemeProvider,
} from "@erpora/platform-web";
import { TooltipProvider } from "@erpora/ui";
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
import { type ReactNode, Suspense } from "react";
import { UiLabelsBridge } from "../providers/ui-labels-bridge";

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

const CATALOGS = {
  id: {
    platform: flatten(readJson("internal/platform/i18n/catalog/id.json")),
    ui: flatten(readJson("packages/ui/i18n/id.json")),
  },
  en: {
    platform: flatten(readJson("internal/platform/i18n/catalog/en.json")),
    ui: flatten(readJson("packages/ui/i18n/en.json")),
  },
} as const;

/** `fetch` palsu untuk `/api/platform/i18n/<locale>/<ns>` (katalog nyata); selain itu 404. */
const fakeFetch = (async (url: string | URL | Request) => {
  const path = new URL(String(url), "http://localhost").pathname;
  const m = /^\/api\/platform\/i18n\/([^/]+)\/([^/]+)$/.exec(path);
  const messages = m
    ? (CATALOGS as Record<string, Record<string, unknown>>)[m[1] ?? ""]?.[m[2] ?? ""]
    : undefined;
  if (!m || !messages) {
    return new Response(JSON.stringify({ error: { code: "not_found", message: "x" } }), {
      status: 404,
    });
  }
  return new Response(
    JSON.stringify({
      data: { locale: m[1], namespace: m[2], revision: "r1", messages },
    }),
    { status: 200, headers: { "Content-Type": "application/json", ETag: '"r1"' } },
  );
}) as unknown as typeof fetch;

export interface OverlayOptions {
  locale?: "id" | "en";
  notificationSource?: NotificationSource;
  path?: string;
}

/**
 * Merender `ui` di dalam provider yang dibutuhkan overlay shell. Elemen `ui` berada di akar router (rute `/alpha`
 * disediakan untuk uji navigasi) sehingga `useNavigate` bekerja; `history` memberi akses ke lokasi saat ini.
 */
export function renderOverlay(ui: ReactNode, opts: OverlayOptions = {}) {
  const locale = opts.locale ?? "id";
  // Bahasa awal ditentukan peramban (`navigator.languages`) bila tak ada pilihan tersimpan: samakan dengan `locale`.
  Object.defineProperty(navigator, "languages", { value: [locale], configurable: true });
  const client = createPlatformClient({ fetch: fakeFetch, getLocale: getActiveLocale });
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
  const alpha = createRoute({
    getParentRoute: () => rootRoute,
    path: "/alpha",
    component: () => <p>Halaman Alpha</p>,
  });
  const router = createRouter({ routeTree: rootRoute.addChildren([index, alpha]), history });
  const bootstrap = makeBootstrap({
    i18n: {
      defaultLocale: locale,
      locales: [
        { code: "id", name: "Indonesia" },
        { code: "en", name: "English" },
      ],
      revisions: { platform: "r1", ui: "r1" },
    },
  });
  const result = render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <StaticBootstrapProvider value={bootstrap}>
          <I18nProvider client={client}>
            <Suspense fallback={null}>
              <UiLabelsBridge>
                <TooltipProvider>
                  <NotificationSourceProvider source={opts.notificationSource ?? emptySource}>
                    <RouterProvider router={router} />
                  </NotificationSourceProvider>
                </TooltipProvider>
              </UiLabelsBridge>
            </Suspense>
          </I18nProvider>
        </StaticBootstrapProvider>
      </ThemeProvider>
    </QueryClientProvider>,
  );
  return { ...result, history, queryClient };
}

const emptySource: NotificationSource = {
  unreadCount: async () => 0,
  list: async () => [],
  markRead: async () => {},
  markAllRead: async () => {},
};
