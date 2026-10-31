// Pembantu uji halaman (bukan bagian bundel): merakit provider minimum (Query, bootstrap statis, i18n katalog nyata,
// DeskProvider dengan `fetch` palsu) dan router memori berisi rute fixture.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { createAppRootRoute, createRoute } from "@erpora/module-sdk";
import {
  createPlatformClient,
  createTestI18n,
  type Desk,
  DeskProvider,
  makeBootstrap,
  StaticBootstrapProvider,
} from "@erpora/platform-web";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render } from "@testing-library/react";
import type { ReactNode } from "react";

const repoRoot = join(import.meta.dir, "../../../..");

function flatten(obj: unknown, prefix = ""): Record<string, string> {
  if (typeof obj === "string") return { [prefix]: obj };
  if (typeof obj !== "object" || obj === null) return {};
  return Object.assign(
    {},
    ...Object.entries(obj).map(([k, v]) => flatten(v, prefix === "" ? k : `${prefix}.${k}`)),
  );
}

const platformId = flatten(
  JSON.parse(readFileSync(join(repoRoot, "internal/platform/i18n/catalog/id.json"), "utf8")),
);

export const i18n = createTestI18n({
  platform: platformId,
  example: { "desk.title": "Catatan Contoh" },
  alpha: { "desk.title": "Alpha Desk" },
});

export interface HarnessRoute {
  path: string;
  component: () => ReactNode;
}

export interface RenderPageOptions {
  /** Alamat awal (state riwayat dapat diberikan lewat `state`). */
  path?: string;
  state?: Record<string, unknown>;
  desks: readonly Desk[];
  routes: readonly HarnessRoute[];
  /** `fetch` palsu yang menggantikan respons Desk (mis. galat). */
  fetchImpl?: typeof fetch;
}

/** Merender rute fixture di bawah provider; `calls` mencatat jumlah permintaan Desk. */
export function renderPage(opts: RenderPageOptions) {
  const calls = { desks: 0 };
  const fetchImpl =
    opts.fetchImpl ??
    ((async () => {
      calls.desks += 1;
      return new Response(JSON.stringify({ data: { desks: opts.desks, revision: "r1" } }), {
        status: 200,
        headers: { "Content-Type": "application/json", ETag: '"e1"' },
      });
    }) as unknown as typeof fetch);
  const client = createPlatformClient({ fetch: fetchImpl });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const root = createAppRootRoute();
  const routes = opts.routes.map((r) =>
    createRoute({ getParentRoute: () => root, path: r.path, component: r.component }),
  );
  const history = createMemoryHistory({ initialEntries: [opts.path ?? "/"] });
  if (opts.state) history.replace(opts.path ?? "/", opts.state);
  const router = createRouter({
    routeTree: root.addChildren(routes),
    history,
    context: { queryClient },
  });
  const wrap = (children: ReactNode) => (
    <QueryClientProvider client={queryClient}>
      <StaticBootstrapProvider value={makeBootstrap()}>
        <i18n.Provider>
          <DeskProvider client={client}>{children}</DeskProvider>
        </i18n.Provider>
      </StaticBootstrapProvider>
    </QueryClientProvider>
  );
  const result = render(wrap(<RouterProvider router={router} />));
  return { ...result, router, history, calls, queryClient };
}
