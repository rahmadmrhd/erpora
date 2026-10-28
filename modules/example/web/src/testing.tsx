// Dukungan test modul (bukan bagian API modul): merender rute modul dengan router memori, bootstrap statis,
// penerjemah berbasis katalog JSON nyata (modul, `ui`, `platform`), dan `fetch` palsu perekam.
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  attachModuleRoutes,
  createAppRootRoute,
  createModuleBaseRoute,
  type FrontendModule,
} from "@erpora/module-sdk";
import {
  compile,
  format,
  MessageTranslatorProvider,
  makeBootstrap,
  PageChromeProvider,
  StaticBootstrapProvider,
  type TranslateFn,
  usePageChrome,
} from "@erpora/platform-web";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import { Suspense } from "react";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..", "..", "..");

export const TENANT_ID = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";

/** Meratakan katalog JSON bersarang menjadi kunci bertitik (format katalog i18n-platform). */
export function flattenCatalog(node: unknown, prefix = ""): Record<string, string> {
  if (typeof node === "string") return { [prefix]: node };
  if (node === null || typeof node !== "object") return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(node)) {
    Object.assign(out, flattenCatalog(v, prefix === "" ? k : `${prefix}.${k}`));
  }
  return out;
}

export function readCatalog(path: string): Record<string, string> {
  return flattenCatalog(JSON.parse(readFileSync(path, "utf8")));
}

/** Katalog bahasa Indonesia nyata: modul, `ui`, dan `platform` (kunci galat). */
export function loadCatalogs(
  moduleDir: string,
  slug: string,
): Record<string, Record<string, string>> {
  return {
    [slug]: readCatalog(join(repoRoot, "modules", moduleDir, "i18n", "id.json")),
    ui: readCatalog(join(repoRoot, "packages", "ui", "i18n", "id.json")),
    platform: readCatalog(join(repoRoot, "internal", "platform", "i18n", "catalog", "id.json")),
  };
}

/** Penerjemah uji: kunci yang tidak ada dikembalikan apa adanya (sehingga asersi teks gagal bila kunci hilang). */
export function translatorFrom(catalogs: Record<string, Record<string, string>>) {
  return (ns: string): TranslateFn =>
    (key, args) => {
      const message = catalogs[ns]?.[key];
      return message === undefined ? key : format(compile("id", message), args);
    };
}

export interface FetchCall {
  url: string;
  method: string;
}

export type Responder = (url: URL) => Response | Promise<Response>;

/** `fetch` palsu: merekam panggilan dan menjawab lewat `responder`. Dipasang ke `globalThis.fetch`. */
export function installFetch(responder: Responder): { calls: FetchCall[]; restore: () => void } {
  const original = globalThis.fetch;
  const calls: FetchCall[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(typeof input === "string" ? input : input.toString(), "http://localhost");
    calls.push({
      url: `${url.pathname}${url.search}`,
      method: (init?.method ?? "GET").toUpperCase(),
    });
    return responder(url);
  }) as typeof fetch;
  return {
    calls,
    restore: () => {
      globalThis.fetch = original;
    },
  };
}

export function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
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

export interface RenderOptions {
  module: FrontendModule;
  /** Path awal, mis. "/example". */
  path: string;
  catalogs: Record<string, Record<string, string>>;
  /** Ikut dirender di luar router (mis. tautan modul lain). */
  queryClient?: QueryClient;
}

export function renderModuleRoute({ module, path, catalogs, queryClient }: RenderOptions) {
  const client = queryClient ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const root = createAppRootRoute();
  const base = createModuleBaseRoute(root, module.slug);
  const tree = root.addChildren([attachModuleRoutes(base, module.registerRoutes(base))]);
  const router = createRouter({
    routeTree: tree,
    history: createMemoryHistory({ initialEntries: [path] }),
    context: { queryClient: client },
  });
  const translate = translatorFrom(catalogs);
  const view = render(
    <QueryClientProvider client={client}>
      <StaticBootstrapProvider value={makeBootstrap({ tenantId: TENANT_ID })}>
        <MessageTranslatorProvider useTranslate={translate}>
          <PageChromeProvider>
            <ChromeProbe />
            <Suspense fallback={<p>memuat…</p>}>
              <RouterProvider router={router} />
            </Suspense>
          </PageChromeProvider>
        </MessageTranslatorProvider>
      </StaticBootstrapProvider>
    </QueryClientProvider>,
  );
  return { ...view, router, queryClient: client };
}
