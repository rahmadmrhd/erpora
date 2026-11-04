// Dukungan test modul (bukan bagian API modul): merender gerbang autentikasi dengan klien token berfetch palsu.
import { expect } from "bun:test";
import { type AuthController, AuthProvider, createAuthController } from "@erpora/platform-web";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, configure, render, waitFor } from "@testing-library/react";
import { Suspense } from "react";
import { AuthScreens } from "./gates";

configure({ asyncUtilTimeout: 5000 });

// Chunk gerbang dimuat malas dan transpilasi pertama bisa beberapa detik: muat di sini (saat berkas tes dimuat), bukan di
// dalam batas waktu tes pertama.
await import("./gates/auth-screens");

export interface RecordedCall {
  path: string;
  method: string;
  body: unknown;
  authorization: string | null;
}

export type Route = (call: RecordedCall, n: number) => Response | Promise<Response>;

export function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

export const sessionBody = (accessToken: string, mfaState = "none") => ({
  data: { accessToken, expiresIn: 600, mfaState },
});

export const apiError = (
  status: number,
  code: string,
  details?: unknown,
  headers: Record<string, string> = {},
) =>
  json(
    { error: { code, message: "teks server", ...(details ? { details } : {}) } },
    status,
    headers,
  );

export function fakeApi(routes: Record<string, Route>) {
  const calls: RecordedCall[] = [];
  const counts = new Map<string, number>();
  const fetchFn = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(typeof input === "string" ? input : input.toString(), "http://localhost");
    const raw = init?.body;
    const call: RecordedCall = {
      path: url.pathname,
      method: (init?.method ?? "GET").toUpperCase(),
      body: typeof raw === "string" && raw !== "" ? JSON.parse(raw) : undefined,
      authorization: new Headers(init?.headers).get("Authorization"),
    };
    calls.push(call);
    const n = (counts.get(call.path) ?? 0) + 1;
    counts.set(call.path, n);
    const route = routes[call.path];
    return route ? route(call, n) : apiError(404, "not_found");
  }) as typeof fetch;
  return { fetch: fetchFn, calls, count: (p: string) => calls.filter((c) => c.path === p).length };
}

export interface RenderGateOptions {
  path?: string;
  controller?: AuthController;
  routes?: Record<string, Route>;
  navigate?: (to: string) => void;
}

/** Merender `AuthScreens` di dalam `AuthProvider` (keadaan awal anonymous) dan menunggu chunk malas termuat. */
export async function renderGate(opts: RenderGateOptions = {}) {
  const api = fakeApi(opts.routes ?? {});
  const controller =
    opts.controller ?? createAuthController({ fetch: api.fetch, initial: "anonymous" });
  const queryClient = new QueryClient();
  const navigations: string[] = [];
  const navigate = opts.navigate ?? ((to: string) => navigations.push(to));
  let view!: ReturnType<typeof render>;
  await act(async () => {
    view = render(
      <QueryClientProvider client={queryClient}>
        <AuthProvider controller={controller} boot="skip">
          <Suspense fallback={<p>memuat</p>}>
            <AuthScreens path={opts.path ?? "/login"} navigate={navigate} />
          </Suspense>
        </AuthProvider>
      </QueryClientProvider>,
    );
  });
  // Chunk gerbang dimuat malas: tunggu sampai fallback hilang agar tes dapat langsung mencari elemen.
  await waitFor(() => expect(view.queryByText("memuat")).toBeNull());
  return { ...view, api, controller, queryClient, navigations };
}
