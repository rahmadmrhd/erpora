import { readFileSync } from "node:fs";
import { join } from "node:path";
// Pembantu uji shell (bukan bagian bundel): server tiruan (bootstrap ber-ETag dan katalog i18n dari JSON nyata),
// modul fixture, dan `renderApp` yang merakit `Providers` penuh dengan `fetch` palsu dan riwayat memori.

import {
  createRoute,
  defaultOfflinePolicy,
  defineModule,
  type FrontendModule,
  lazyRouteComponent,
} from "@erpora/module-sdk";
import type {
  Bootstrap,
  BootstrapModule,
  CurrentUserSource,
  NotificationSource,
} from "@erpora/platform-web";
import {
  createPlatformClient,
  getActiveLocale,
  makeBootstrap,
  makeBootstrapModule,
  usePageActions,
} from "@erpora/platform-web";
import { focusManager, QueryClient } from "@tanstack/react-query";
import { createMemoryHistory } from "@tanstack/react-router";
import { act, render } from "@testing-library/react";
import { Providers } from "./providers/providers";

// Katalog nyata dibaca dari berkas (bukan `import`) agar aturan F2 (tanpa deep import ke packages/*) tidak terpicu.
const repoRoot = join(import.meta.dir, "../../..");
const readJson = (rel: string): unknown => JSON.parse(readFileSync(join(repoRoot, rel), "utf8"));
const platformId = readJson("internal/platform/i18n/catalog/id.json");
const platformEn = readJson("internal/platform/i18n/catalog/en.json");
const uiId = readJson("packages/ui/i18n/id.json");
const uiEn = readJson("packages/ui/i18n/en.json");

export const TENANT = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";

export function flatten(obj: unknown, prefix = ""): Record<string, string> {
  if (typeof obj === "string") return { [prefix]: obj };
  if (typeof obj !== "object" || obj === null) return {};
  return Object.assign(
    {},
    ...Object.entries(obj).map(([k, v]) => flatten(v, prefix === "" ? k : `${prefix}.${k}`)),
  );
}

type Catalog = { revision: string; messages: Record<string, string> };

const FIXTURE_CATALOGS: Record<string, Record<string, Record<string, string>>> = {
  alpha: {
    id: {
      "nav.home": "Alpha Beranda",
      "nav.reports": "Laporan",
      "nav.sales": "Penjualan",
      "page.deep": "Dalam",
      "page.title": "Judul Alpha",
    },
    en: {
      "nav.home": "Alpha Home",
      "nav.reports": "Reports",
      "nav.sales": "Sales",
      "page.deep": "Deep",
      "page.title": "Alpha Title",
    },
  },
  beta: {
    id: { "nav.home": "Beta Beranda" },
    en: { "nav.home": "Beta Home" },
  },
};

export interface FakeServer {
  fetch: typeof fetch;
  bootstrap: Bootstrap;
  /** Memperbarui bootstrap (revisi naik); terlihat pada refetch berikutnya. */
  update(patch: Partial<Bootstrap> | ((b: Bootstrap) => Partial<Bootstrap>)): void;
  setModuleStatus(key: string, status: BootstrapModule["status"]): void;
  failBootstrap: boolean;
  /** Namespace katalog yang dijawab 500 (galat non-404 yang fatal). */
  failNamespaces: Set<string>;
  requests: string[];
}

const CORE_NS = new Set(["platform", "ui"]);

/** Meniru backend: katalog namespace modul hanya untuk modul `enabled`, dan `revisions` hanya namespace aktif. */
function withActiveRevisions(b: Bootstrap): Bootstrap {
  if (!b.i18n) return b;
  const revisions: Record<string, string> = { platform: "p1", ui: "u1" };
  for (const m of b.modules) if (m.status === "enabled") revisions[m.slug] = `${m.slug[0]}1`;
  return { ...b, i18n: { ...b.i18n, revisions } };
}

export function createFakeServer(
  modules: BootstrapModule[],
  over: Partial<Bootstrap> = {},
): FakeServer {
  const server: FakeServer = {
    bootstrap: withActiveRevisions(
      makeBootstrap({
        tenantId: TENANT,
        modules,
        revision: 1,
        features: { modulesAdminApi: false },
        i18n: {
          defaultLocale: "id",
          locales: [
            { code: "id", name: "Indonesia" },
            { code: "en", name: "English" },
          ],
          revisions: { platform: "p1", ui: "u1", alpha: "a1", beta: "b1" },
        },
        ...over,
      }),
    ),
    failBootstrap: false,
    failNamespaces: new Set(),
    requests: [],
    update(patch) {
      const next = typeof patch === "function" ? patch(server.bootstrap) : patch;
      server.bootstrap = withActiveRevisions({
        ...server.bootstrap,
        ...next,
        revision: server.bootstrap.revision + 1,
      });
    },
    setModuleStatus(key, status) {
      server.update((b) => ({
        modules: b.modules.map((m) => (m.key === key ? { ...m, status } : m)),
      }));
    },
    fetch: (async (url: string | URL | Request, init?: RequestInit) => {
      const path = new URL(String(url), "http://localhost").pathname;
      server.requests.push(path);
      const headers = new Headers(init?.headers);
      if (path === "/api/bootstrap") {
        if (server.failBootstrap) {
          return new Response(JSON.stringify({ error: { code: "internal_error", message: "x" } }), {
            status: 500,
          });
        }
        const etag = `"${server.bootstrap.revision}"`;
        if (headers.get("If-None-Match") === etag) return new Response(null, { status: 304 });
        return new Response(JSON.stringify({ data: server.bootstrap }), {
          status: 200,
          headers: { "Content-Type": "application/json", ETag: etag },
        });
      }
      const m = /^\/api\/platform\/i18n\/([^/]+)\/([^/]+)$/.exec(path);
      if (m) {
        const locale = decodeURIComponent(m[1] ?? "");
        const ns = decodeURIComponent(m[2] ?? "");
        if (server.failNamespaces.has(ns)) {
          return new Response(JSON.stringify({ error: { code: "internal_error", message: "x" } }), {
            status: 500,
          });
        }
        const mod = server.bootstrap.modules.find((x) => x.slug === ns);
        const active = CORE_NS.has(ns) || mod?.status === "enabled";
        const cat = active ? catalogFor(locale, ns) : undefined;
        if (!cat) {
          return new Response(JSON.stringify({ error: { code: "not_found", message: "x" } }), {
            status: 404,
          });
        }
        const etag = `"${cat.revision}"`;
        if (headers.get("If-None-Match") === etag) return new Response(null, { status: 304 });
        return new Response(
          JSON.stringify({
            data: { locale, namespace: ns, revision: cat.revision, messages: cat.messages },
          }),
          { status: 200, headers: { "Content-Type": "application/json", ETag: etag } },
        );
      }
      return new Response(JSON.stringify({ error: { code: "not_found", message: "x" } }), {
        status: 404,
      });
    }) as unknown as typeof fetch,
  };
  return server;
}

function catalogFor(locale: string, ns: string): Catalog | undefined {
  const real: Record<string, Record<string, unknown>> = {
    platform: { id: platformId, en: platformEn },
    ui: { id: uiId, en: uiEn },
  };
  const json = real[ns]?.[locale];
  if (json) return { revision: `${ns[0]}1`, messages: flatten(json) };
  const fx = FIXTURE_CATALOGS[ns]?.[locale];
  if (fx) return { revision: `${ns[0]}1`, messages: fx };
  return undefined;
}

/** Hitungan pemuatan chunk halaman per modul fixture (P17: tidak bertambah saat modul tidak aktif). */
export interface FixtureLoads {
  alphaHome: number;
  alphaSales: number;
  beta: number;
}

function AlphaHome() {
  usePageActions([
    { id: "new", label: "Tambah Alpha", variant: "primary", pinned: true, onSelect: () => {} },
    { id: "export", label: "Ekspor Alpha", onSelect: () => {} },
  ]);
  return <p>Isi Alpha Beranda</p>;
}

function Boom(): never {
  throw new Error("halaman rusak");
}

export function makeFixtureModules(loads: FixtureLoads): FrontendModule[] {
  const alpha = defineModule({
    key: "acme.alpha",
    slug: "alpha",
    version: "1.0.0",
    navigation: [
      {
        type: "item",
        id: "home",
        label: { ns: "alpha", key: "nav.home" },
        to: "/",
        icon: "package",
      },
      {
        type: "group",
        id: "reports",
        label: { ns: "alpha", key: "nav.reports" },
        icon: "folder",
        children: [
          {
            type: "item",
            id: "sales",
            label: { ns: "alpha", key: "nav.sales" },
            to: "/reports/sales",
          },
        ],
      },
    ],
    registerRoutes: (base) => [
      createRoute({
        getParentRoute: () => base,
        path: "/",
        staticData: { crumb: { ref: { ns: "alpha", key: "nav.home" } } },
        component: lazyRouteComponent(() => {
          loads.alphaHome += 1;
          return Promise.resolve({ default: AlphaHome });
        }),
      }),
      createRoute({
        getParentRoute: () => base,
        path: "/reports/sales",
        staticData: {
          crumb: [
            { ref: { ns: "alpha", key: "nav.reports" } },
            { ref: { ns: "alpha", key: "nav.sales" } },
          ],
        },
        component: lazyRouteComponent(() => {
          loads.alphaSales += 1;
          return Promise.resolve({ default: () => <p>Isi Penjualan</p> });
        }),
      }),
      createRoute({
        getParentRoute: () => base,
        path: "/deep",
        staticData: {
          crumb: [
            { text: "Tingkat 1" },
            { text: "Tingkat 2" },
            { text: "Tingkat 3" },
            { ref: { ns: "alpha", key: "page.deep" } },
          ],
        },
        component: () => <p>Isi Dalam</p>,
      }),
      createRoute({
        getParentRoute: () => base,
        path: "/crumbfail",
        staticData: { crumb: { ref: { ns: "gamma", key: "x" } } },
        component: () => <p>Isi Crumb Gagal</p>,
      }),
      createRoute({
        getParentRoute: () => base,
        path: "/boom",
        staticData: { crumb: { text: "Rusak" } },
        component: Boom,
      }),
    ],
    offline: defaultOfflinePolicy,
  });
  const beta = defineModule({
    key: "acme.beta",
    slug: "beta",
    version: "1.0.0",
    navigation: [{ type: "item", id: "home", label: { ns: "beta", key: "nav.home" }, to: "/" }],
    registerRoutes: (base) => [
      createRoute({
        getParentRoute: () => base,
        path: "/",
        staticData: { crumb: { ref: { ns: "beta", key: "nav.home" } } },
        component: lazyRouteComponent(() => {
          loads.beta += 1;
          return Promise.resolve({ default: () => <p>Isi Beta</p> });
        }),
      }),
    ],
    offline: defaultOfflinePolicy,
  });
  const ghost = defineModule({
    key: "acme.ghost",
    slug: "ghost",
    version: "1.0.0",
    navigation: [{ type: "item", id: "home", label: { ns: "ghost", key: "nav.home" }, to: "/" }],
    registerRoutes: () => [],
    offline: defaultOfflinePolicy,
  });
  return [alpha, beta, ghost];
}

export interface RenderAppOptions {
  path?: string;
  /** Status modul pada bootstrap (bawaan: alpha dan beta enabled; ghost tidak ada). */
  statuses?: Partial<Record<"alpha" | "beta", BootstrapModule["status"]>>;
  notificationSource?: NotificationSource;
  currentUserSource?: CurrentUserSource;
  server?: FakeServer;
  bootstrap?: Partial<Bootstrap>;
}

/** Merakit `Providers` penuh dengan server tiruan; `refreshBootstrap` memicu refetch (fokus jendela). */
export function renderApp(opts: RenderAppOptions = {}) {
  const loads: FixtureLoads = { alphaHome: 0, alphaSales: 0, beta: 0 };
  const modules = makeFixtureModules(loads);
  const server =
    opts.server ??
    createFakeServer(
      [
        makeBootstrapModule("acme.alpha", "alpha", opts.statuses?.alpha ?? "enabled"),
        makeBootstrapModule("acme.beta", "beta", opts.statuses?.beta ?? "enabled"),
      ],
      opts.bootstrap,
    );
  const client = createPlatformClient({ fetch: server.fetch, getLocale: getActiveLocale });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const history = createMemoryHistory({ initialEntries: [opts.path ?? "/"] });
  const result = render(
    <Providers
      modules={modules}
      client={client}
      queryClient={queryClient}
      history={history}
      notificationSource={opts.notificationSource}
      currentUserSource={opts.currentUserSource}
      bootstrapOptions={{ retry: false, staleTime: 0 }}
    />,
  );
  const refreshBootstrap = async () => {
    await act(async () => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      await new Promise((r) => setTimeout(r, 20));
    });
  };
  return { ...result, server, loads, queryClient, history, refreshBootstrap };
}
