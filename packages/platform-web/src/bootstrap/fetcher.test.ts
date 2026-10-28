import { describe, expect, test } from "bun:test";
import { createClient, type PlatformPaths } from "@erpora/platform-contracts";
import { BOOTSTRAP_KEY, createBootstrapFetcher } from "./fetcher";
import { makeBootstrap } from "./testing";

interface Call {
  url: string;
  ifNoneMatch: string | null;
}

function fakeServer(responses: (() => Response)[]) {
  const calls: Call[] = [];
  let i = 0;
  const fetchFn = (async (url: string | URL | Request, init?: RequestInit) => {
    const headers = new Headers(init?.headers);
    calls.push({ url: String(url), ifNoneMatch: headers.get("If-None-Match") });
    const make = responses[Math.min(i, responses.length - 1)];
    i += 1;
    if (!make) throw new Error("tidak ada respons tiruan");
    return make();
  }) as unknown as typeof fetch;
  return { calls, client: createClient<PlatformPaths>({ fetch: fetchFn }) };
}

const ok = (data: unknown, etag: string) => () =>
  new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json", ETag: etag },
  });
const notModified = () => new Response(null, { status: 304, headers: { ETag: '"1"' } });

describe("createBootstrapFetcher", () => {
  test("BOOTSTRAP_KEY adalah satu-satunya key tanpa awalan tenant", () => {
    expect(BOOTSTRAP_KEY).toEqual(["bootstrap"]);
  });

  test("200 menyimpan etag dan mengembalikan data; permintaan berikutnya memakai If-None-Match", async () => {
    const b1 = makeBootstrap({ revision: 1 });
    const server = fakeServer([ok(b1, '"1"'), notModified]);
    const fetcher = createBootstrapFetcher(server.client);
    const first = await fetcher();
    expect(first).toEqual(b1);
    expect(server.calls[0]?.ifNoneMatch).toBeNull();
    await fetcher();
    expect(server.calls[1]?.ifNoneMatch).toBe('"1"');
  });

  test("P5: 304 mengembalikan referensi objek yang sama", async () => {
    const server = fakeServer([
      ok(makeBootstrap({ revision: 1 }), '"1"'),
      notModified,
      notModified,
    ]);
    const fetcher = createBootstrapFetcher(server.client);
    const first = await fetcher();
    expect(await fetcher()).toBe(first);
    expect(await fetcher()).toBe(first);
  });

  test("P5: 200 baru mengganti data dan etag", async () => {
    const b2 = makeBootstrap({ revision: 2 });
    const server = fakeServer([
      ok(makeBootstrap({ revision: 1 }), '"1"'),
      ok(b2, '"2"'),
      notModified,
    ]);
    const fetcher = createBootstrapFetcher(server.client);
    const first = await fetcher();
    const second = await fetcher();
    expect(second).not.toBe(first);
    expect(second).toEqual(b2);
    await fetcher();
    expect(server.calls[2]?.ifNoneMatch).toBe('"2"');
  });

  test("galat HTTP melempar BootstrapError berisi kode dan status", async () => {
    const server = fakeServer([
      () =>
        new Response(
          JSON.stringify({ error: { code: "internal_error", message: "x", requestId: "r1" } }),
          {
            status: 500,
            headers: { "Content-Type": "application/json" },
          },
        ),
    ]);
    const fetcher = createBootstrapFetcher(server.client);
    await expect(fetcher()).rejects.toMatchObject({
      name: "BootstrapError",
      status: 500,
      error: { code: "internal_error", requestId: "r1" },
    });
  });

  test("JSON rusak dan galat jaringan melempar (bukan data kosong)", async () => {
    const bad = fakeServer([
      () => new Response("<html>", { status: 200, headers: { ETag: '"1"' } }),
    ]);
    await expect(createBootstrapFetcher(bad.client)()).rejects.toBeDefined();
    const net = createClient<PlatformPaths>({
      fetch: (async () => {
        throw new TypeError("Failed to fetch");
      }) as unknown as typeof fetch,
    });
    await expect(createBootstrapFetcher(net)()).rejects.toBeInstanceOf(TypeError);
  });

  test("304 sebelum ada data (tak seharusnya terjadi) melempar", async () => {
    const server = fakeServer([notModified]);
    await expect(createBootstrapFetcher(server.client)()).rejects.toBeDefined();
  });

  test("galat tidak menghapus data terakhir: sukses berikutnya tetap berfungsi", async () => {
    const b1 = makeBootstrap({ revision: 1 });
    const server = fakeServer([
      ok(b1, '"1"'),
      () =>
        new Response(JSON.stringify({ error: { code: "internal_error", message: "x" } }), {
          status: 500,
        }),
      notModified,
    ]);
    const fetcher = createBootstrapFetcher(server.client);
    const first = await fetcher();
    await expect(fetcher()).rejects.toBeDefined();
    expect(await fetcher()).toBe(first);
  });
});
