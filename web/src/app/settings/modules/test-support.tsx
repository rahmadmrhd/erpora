// Pembantu uji halaman /settings/modules (bukan bagian bundel): server tiruan stateful untuk API modul, katalog nyata,
// dan perakitan provider minimal (tanpa router/shell).
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { defaultOfflinePolicy, defineModule, type FrontendModule } from "@erpora/module-sdk";
import {
  createTestI18n,
  MessageTranslatorProvider,
  ModuleRegistryProvider,
  makeBootstrap,
  PageChromeProvider,
  StaticBootstrapProvider,
  type UseTranslate,
  usePageChrome,
  useTranslation,
} from "@erpora/platform-web";
import { TooltipProvider } from "@erpora/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, configure, render } from "@testing-library/react";
import type { ReactNode } from "react";
import type { EventView, ModuleView, OperationView } from "./types";

configure({ asyncUtilTimeout: 5000 });

const repoRoot = join(import.meta.dir, "../../../../..");
const readJson = (rel: string): unknown => JSON.parse(readFileSync(join(repoRoot, rel), "utf8"));

function flatten(obj: unknown, prefix = ""): Record<string, string> {
  if (typeof obj === "string") return { [prefix]: obj };
  if (typeof obj !== "object" || obj === null) return {};
  return Object.assign(
    {},
    ...Object.entries(obj).map(([k, v]) => flatten(v, prefix === "" ? k : `${prefix}.${k}`)),
  );
}

export const catalogs = {
  platform: flatten(readJson("internal/platform/i18n/catalog/id.json")),
  ui: flatten(readJson("packages/ui/i18n/id.json")),
};

export const TENANT = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";

export function mod(key: string, over: Partial<ModuleView> = {}): ModuleView {
  const slug = key.split(".").at(-1) ?? key;
  return {
    key,
    slug,
    name: slug[0]?.toUpperCase() + slug.slice(1),
    binaryVersion: "1.0.0",
    installedVersion: "1.0.0",
    status: "enabled",
    required: false,
    compatible: true,
    incompatibleCodes: [],
    dependencies: [{ module: "platform.core", version: "^0.1", optional: false }],
    dependents: [],
    lastOperationId: null,
    ...over,
  };
}

export function operation(over: Partial<OperationView> = {}): OperationView {
  return {
    id: "op-1",
    moduleKey: "acme.shop",
    operation: "disable",
    fromVersion: "1.0.0",
    toVersion: "1.0.0",
    status: "succeeded",
    steps: [
      {
        name: "dependents",
        status: "succeeded",
        startedAt: "2026-10-29T07:00:00Z",
        durationMs: 4,
        error: null,
      },
      {
        name: "hook:disable",
        status: "succeeded",
        startedAt: "2026-10-29T07:00:01Z",
        durationMs: 15,
        error: null,
      },
    ],
    failure: null,
    actor: "api:dev",
    startedAt: "2026-10-29T07:00:00Z",
    finishedAt: "2026-10-29T07:00:02Z",
    ...over,
  } as OperationView;
}

export interface ServerOptions {
  /** Operasi POST: "done" (200) atau "accepted" (202 lalu polling). */
  mode?: "done" | "accepted";
  /** Hasil operasi pada mode done; bawaan succeeded. */
  failure?: { code: string; message: string };
  listStatus?: number;
  /** Status HTTP GET operations/{id} (bawaan 200); selain 200 membalas galat. */
  operationStatus?: number;
  /** Status HTTP GET operations/{id} per id (menimpa `operationStatus`). */
  operationStatuses?: Record<string, number>;
  /** Id operasi yang dibalas POST 202 secara berurutan (bawaan selalu `op-1`). */
  acceptedIds?: string[];
  postStatus?: number;
  postError?: { code: string; details?: unknown[]; message?: string; requestId?: string };
  events?: EventView[];
  operations?: Record<string, OperationView>;
}

export interface ModulesServer {
  fetch: typeof fetch;
  modules: ModuleView[];
  revision: number;
  calls: Array<{
    method: string;
    path: string;
    search: string;
    headers: Headers;
    body: string | undefined;
  }>;
  options: ServerOptions;
  /** Jumlah GET operations/{id} per id (untuk polling). */
  opFetches: Record<string, number>;
}

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });

const NEXT: Record<string, ModuleView["status"]> = {
  install: "installed",
  enable: "enabled",
  disable: "disabled",
  uninstall: "uninstalled",
  upgrade: "enabled",
  retry: "enabled",
};

export function createModulesServer(
  initial: ModuleView[],
  options: ServerOptions = {},
): ModulesServer {
  const s: ModulesServer = {
    modules: initial,
    revision: 1,
    calls: [],
    options,
    opFetches: {},
    fetch: (async (url: string | URL | Request, init?: RequestInit) => {
      const u = new URL(String(url), "http://localhost");
      const method = (init?.method ?? "GET").toUpperCase();
      const headers = new Headers(init?.headers);
      s.calls.push({
        method,
        path: u.pathname,
        search: u.search,
        headers,
        body: typeof init?.body === "string" ? init.body : undefined,
      });
      const path = u.pathname;
      if (method === "GET" && path === "/api/platform/modules") {
        if (s.options.listStatus && s.options.listStatus !== 200) {
          return json(
            {
              error: {
                code: s.options.listStatus === 403 ? "permission_denied" : "internal_error",
                message: "x",
              },
            },
            s.options.listStatus,
          );
        }
        return json(
          {
            data: s.modules,
            page: { page: 1, pageSize: 200, total: s.modules.length, hasMore: false },
          },
          200,
          { ETag: `"${s.revision}"` },
        );
      }
      let m = /^\/api\/platform\/modules\/operations\/([^/]+)$/.exec(path);
      if (method === "GET" && m) {
        const id = decodeURIComponent(m[1] ?? "");
        s.opFetches[id] = (s.opFetches[id] ?? 0) + 1;
        const forced = s.options.operationStatuses?.[id] ?? s.options.operationStatus;
        if (forced && forced !== 200) {
          return json({ error: { code: "not_found", message: "x" } }, forced);
        }
        const op = s.options.operations?.[id] ?? operation({ id });
        return json({ data: op });
      }
      m = /^\/api\/platform\/modules\/([^/]+)\/history$/.exec(path);
      if (method === "GET" && m) {
        const events = s.options.events ?? [];
        return json({
          data: events,
          page: { page: 1, pageSize: 25, total: events.length, hasMore: false },
        });
      }
      m =
        /^\/api\/platform\/modules\/([^/]+)\/(install|enable|disable|upgrade|uninstall|retry)$/.exec(
          path,
        );
      if (method === "POST" && m) {
        if (s.options.postError) {
          return json(
            { error: { message: "teks server rahasia", ...s.options.postError } },
            s.options.postStatus ?? 422,
          );
        }
        const key = decodeURIComponent(m[1] ?? "");
        const op = m[2] as string;
        const target = s.modules.find((x) => x.key === key);
        if (s.options.mode === "accepted") {
          const id = s.options.acceptedIds?.shift() ?? "op-1";
          return json({ data: { operationId: id, operationIds: [id], status: "running" } }, 202);
        }
        if (target && !s.options.failure) {
          s.modules = s.modules.map((x) =>
            x.key === key ? { ...x, status: NEXT[op] ?? x.status } : x,
          );
          s.revision += 1;
        }
        const operations = s.options.failure
          ? [
              operation({
                id: "op-f",
                moduleKey: key,
                operation: op as never,
                status: "failed",
                failure: s.options.failure,
              }),
            ]
          : [operation({ moduleKey: key, operation: op as never })];
        return json({
          data: {
            module: s.modules.find((x) => x.key === key),
            operations,
            operationIds: operations.map((o) => o.id),
          },
        });
      }
      return json({ error: { code: "not_found", message: "x" } }, 404);
    }) as typeof fetch,
  };
  return s;
}

/** Menampilkan aksi halaman yang dideklarasikan halaman aktif (padanan Navbar). */
function ChromeProbe() {
  const chrome = usePageChrome();
  return (
    <ul data-testid="page-actions">
      {chrome.actions.map((a) =>
        "node" in a ? null : (
          <li key={a.id}>
            <button type="button" onClick={a.onSelect} disabled={a.disabled}>
              {a.label}
            </button>
          </li>
        ),
      )}
    </ul>
  );
}

export function fixtureFrontend(): FrontendModule[] {
  return [
    defineModule({
      key: "acme.shop",
      slug: "shop",
      version: "1.0.0",
      navigation: [{ type: "item", id: "home", label: { ns: "shop", key: "nav.home" }, to: "/" }],
      registerRoutes: () => [],
      offline: defaultOfflinePolicy,
    }),
  ];
}

/**
 * Meniru backend nyata: katalog namespace modul hanya disajikan bila modul `enabled`, selain itu 404 sehingga memuatnya
 * melempar. Hanya `platform` dan `ui` yang tersedia; pemakaian namespace lain (mis. label menu modul) membuat test gagal.
 */
const useStrictTranslate: UseTranslate = (ns) => {
  const { t } = useTranslation(ns as never);
  if (ns !== "platform" && ns !== "ui") {
    throw new Error(`katalog namespace "${ns}" tidak tersedia (modul nonaktif: 404)`);
  }
  return t as never;
};

export interface RenderOptions {
  server: ModulesServer;
  /** `permissions[]` bootstrap (kosong = belum ada informasi, optimistis). */
  permissions?: string[];
  adminApi?: boolean;
  frontend?: FrontendModule[];
  queryClient?: QueryClient;
  /** Menimpa teks katalog `platform` (kunci rata) untuk menguji bahwa label benar-benar lewat i18n. */
  platformCatalog?: Record<string, string>;
  children: ReactNode;
}

/** Merender anak dengan provider minimal; default `fetch` global diarahkan ke server tiruan. */
export async function renderWithServer({
  server,
  adminApi = true,
  permissions = [],
  frontend,
  queryClient,
  platformCatalog,
  children,
}: RenderOptions) {
  const original = globalThis.fetch;
  globalThis.fetch = server.fetch;
  const qc = queryClient ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const i18n = createTestI18n({
    ...catalogs,
    platform: { ...catalogs.platform, ...platformCatalog },
  });
  const modules = frontend ?? fixtureFrontend();
  let view!: ReturnType<typeof render>;
  await act(async () => {
    view = render(
      <QueryClientProvider client={qc}>
        <StaticBootstrapProvider
          value={makeBootstrap({
            tenantId: TENANT,
            permissions,
            features: { modulesAdminApi: adminApi },
            modules: [
              {
                key: "acme.shop",
                slug: "shop",
                version: "1.0.0",
                status: "enabled",
                basePath: "/shop",
                offline: false,
              },
            ],
          })}
        >
          <ModuleRegistryProvider modules={modules}>
            <i18n.Provider>
              <TooltipProvider>
                <MessageTranslatorProvider useTranslate={useStrictTranslate}>
                  <PageChromeProvider>
                    <ChromeProbe />
                    {children}
                  </PageChromeProvider>
                </MessageTranslatorProvider>
              </TooltipProvider>
            </i18n.Provider>
          </ModuleRegistryProvider>
        </StaticBootstrapProvider>
      </QueryClientProvider>,
    );
  });
  return {
    ...view,
    queryClient: qc,
    restore: () => {
      globalThis.fetch = original;
    },
  };
}
