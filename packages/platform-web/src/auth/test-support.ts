// Dukungan uji klien token (bukan API produksi): server palsu untuk /api/core/auth/* dan route terlindungi.
import { expect } from "bun:test";

export interface RecordedCall {
  path: string;
  method: string;
  authorization: string | null;
  credentials: RequestCredentials | undefined;
}

export type RouteHandler = (call: RecordedCall, n: number) => Response | Promise<Response>;

export function json(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });
}

export const session = (accessToken: string, mfaState = "none") =>
  json({ data: { accessToken, expiresIn: 600, mfaState } });

export const apiError = (status: number, code: string) =>
  json({ error: { code, message: "x" } }, status);

/** fetch palsu per path; mencatat setiap panggilan (termasuk `credentials`). */
export function fakeServer(routes: Record<string, RouteHandler>) {
  const calls: RecordedCall[] = [];
  const counts = new Map<string, number>();
  const fetchFn = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(typeof input === "string" ? input : input.toString(), "http://localhost");
    const call: RecordedCall = {
      path: url.pathname,
      method: (init?.method ?? "GET").toUpperCase(),
      authorization: new Headers(init?.headers).get("Authorization"),
      credentials: init?.credentials,
    };
    calls.push(call);
    const n = (counts.get(call.path) ?? 0) + 1;
    counts.set(call.path, n);
    const handler = routes[call.path];
    if (!handler) return apiError(404, "not_found");
    return handler(call, n);
  }) as typeof fetch;
  return {
    fetch: fetchFn,
    calls,
    count: (path: string) => calls.filter((c) => c.path === path).length,
  };
}

/** Menghitung penulisan ke seluruh penyimpanan peramban selama `fn`, dan memastikan `secret` tidak ada di sana. */
export async function expectNoBrowserStorage(secrets: string[], fn: () => Promise<void>) {
  const writes: string[] = [];
  const protoSet = Storage.prototype.setItem;
  Storage.prototype.setItem = function (key: string, value: string) {
    writes.push(`${key}=${value}`);
    return protoSet.call(this, key, value);
  };
  const cookieDesc = Object.getOwnPropertyDescriptor(Document.prototype, "cookie");
  let cookieWrites = 0;
  if (cookieDesc?.get && cookieDesc.set) {
    const { get, set } = cookieDesc;
    Object.defineProperty(document, "cookie", {
      configurable: true,
      get: () => get.call(document),
      set: (v: string) => {
        cookieWrites++;
        set.call(document, v);
      },
    });
  }
  const idbOpen = (globalThis as { indexedDB?: { open: (...a: unknown[]) => unknown } }).indexedDB
    ?.open;
  let idbOpens = 0;
  const idb = (globalThis as { indexedDB?: { open: (...a: unknown[]) => unknown } }).indexedDB;
  if (idb && idbOpen) {
    idb.open = (...a: unknown[]) => {
      idbOpens++;
      return idbOpen.apply(idb, a);
    };
  }
  try {
    await fn();
  } finally {
    Storage.prototype.setItem = protoSet;
    if (cookieDesc) delete (document as { cookie?: unknown }).cookie;
    if (idb && idbOpen) idb.open = idbOpen;
  }
  for (const secret of secrets) {
    expect(writes.join("\n")).not.toContain(secret);
    expect(JSON.stringify({ ...localStorage })).not.toContain(secret);
    expect(JSON.stringify({ ...sessionStorage })).not.toContain(secret);
    expect(document.cookie).not.toContain(secret);
  }
  expect(cookieWrites).toBe(0);
  expect(idbOpens).toBe(0);
}
