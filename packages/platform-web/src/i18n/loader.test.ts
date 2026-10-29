import { describe, expect, test } from "bun:test";
import { createClient, type PlatformPaths } from "@erpora/platform-contracts";
import { CatalogLoadError, createCatalogLoader } from "./loader";

interface Call {
  url: string;
  ifNoneMatch: string | null;
}

function server(handlers: (() => Response)[]) {
  const calls: Call[] = [];
  let i = 0;
  const client = createClient<PlatformPaths>({
    fetch: (async (url: string | URL | Request, init?: RequestInit) => {
      calls.push({
        url: String(url),
        ifNoneMatch: new Headers(init?.headers).get("If-None-Match"),
      });
      const h = handlers[Math.min(i, handlers.length - 1)];
      i += 1;
      if (!h) throw new Error("tanpa handler");
      return h();
    }) as unknown as typeof fetch,
  });
  return { client, calls };
}

const ok = (revision: string, messages: Record<string, string>) => () =>
  new Response(
    JSON.stringify({ data: { locale: "id", namespace: "example", revision, messages } }),
    {
      status: 200,
      headers: { "Content-Type": "application/json", ETag: `"${revision}"` },
    },
  );
const notModified = () => new Response(null, { status: 304 });

describe("createCatalogLoader", () => {
  test("memuat katalog per (locale, namespace) dari /api/platform/i18n/{locale}/{namespace}", async () => {
    const s = server([ok("r1", { "a.b": "Halo" })]);
    const entry = await createCatalogLoader(s.client)("id", "example");
    expect(entry).toEqual({
      locale: "id",
      namespace: "example",
      revision: "r1",
      messages: { "a.b": "Halo" },
    });
    expect(s.calls[0]?.url).toBe("/api/platform/i18n/id/example");
    expect(s.calls[0]?.ifNoneMatch).toBeNull();
  });

  test("locale dan namespace di-encode", async () => {
    const s = server([ok("r1", {})]);
    await createCatalogLoader(s.client)("en-GB", "a/b c");
    expect(s.calls[0]?.url).toBe("/api/platform/i18n/en-GB/a%2Fb%20c");
  });

  test("permintaan ulang membawa If-None-Match dari ETag; 304 mengembalikan entri yang sama", async () => {
    const s = server([ok("r1", { k: "v" }), notModified]);
    const load = createCatalogLoader(s.client);
    const first = await load("id", "example");
    const again = await load("id", "example");
    expect(again).toBe(first);
    expect(s.calls[1]?.ifNoneMatch).toBe('"r1"');
  });

  test("200 baru mengganti entri dan ETag", async () => {
    const s = server([ok("r1", { k: "v1" }), ok("r2", { k: "v2" }), notModified]);
    const load = createCatalogLoader(s.client);
    await load("id", "example");
    const second = await load("id", "example");
    expect(second.messages).toEqual({ k: "v2" });
    await load("id", "example");
    expect(s.calls[2]?.ifNoneMatch).toBe('"r2"');
  });

  test("ETag disimpan per (locale, namespace): bahasa lain tidak memakai ETag bahasa pertama", async () => {
    const s = server([ok("r1", {}), ok("r9", {})]);
    const load = createCatalogLoader(s.client);
    await load("id", "example");
    await load("en", "example");
    expect(s.calls[1]?.ifNoneMatch).toBeNull();
  });

  test("galat HTTP melempar CatalogLoadError (404 tanpa membedakan penyebab)", async () => {
    const s = server([
      () =>
        new Response(
          JSON.stringify({ error: { code: "not_found", message: "x", requestId: "r" } }),
          {
            status: 404,
          },
        ),
    ]);
    await expect(createCatalogLoader(s.client)("id", "nope")).rejects.toMatchObject({
      name: "CatalogLoadError",
      status: 404,
      error: { code: "not_found" },
    });
    expect(CatalogLoadError).toBeDefined();
  });

  test("respons rusak (bukan katalog) melempar", async () => {
    const s = server([() => new Response("<html>", { status: 200, headers: { ETag: '"x"' } })]);
    await expect(createCatalogLoader(s.client)("id", "example")).rejects.toBeInstanceOf(
      CatalogLoadError,
    );
    const s2 = server([
      () =>
        new Response(JSON.stringify({ data: { locale: "id", messages: { k: 1 } } }), {
          status: 200,
        }),
    ]);
    await expect(createCatalogLoader(s2.client)("id", "example")).rejects.toBeInstanceOf(
      CatalogLoadError,
    );
  });

  test("304 tanpa entri sebelumnya melempar", async () => {
    const s = server([notModified]);
    await expect(createCatalogLoader(s.client)("id", "example")).rejects.toBeInstanceOf(
      CatalogLoadError,
    );
  });
});
