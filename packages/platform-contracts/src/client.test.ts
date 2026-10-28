import { describe, expect, test } from "bun:test";
import { createClient } from "./client";
import type { paths } from "./platform.gen";

interface Seen {
  url: string;
  method: string;
  headers: Headers;
  body: string | undefined;
}

function harness(respond: () => Response) {
  const seen: Seen[] = [];
  const client = createClient<paths>({
    baseUrl: "http://api.test",
    fetch: (async (url: string | URL | Request, init?: RequestInit) => {
      seen.push({
        url: String(url),
        method: init?.method ?? "GET",
        headers: new Headers(init?.headers),
        body: init?.body as string | undefined,
      });
      return respond();
    }) as unknown as typeof fetch,
  });
  return { client, seen };
}

const bootstrapBody = {
  data: { appVersion: "0.1.0", tenantId: "t", modules: [], permissions: [], revision: 4 },
};

describe("client: ETag dan 304 (M3a 9.3)", () => {
  test("hasil sukses memuat etag dari header ETag", async () => {
    const { client } = harness(
      () =>
        new Response(JSON.stringify(bootstrapBody), {
          status: 200,
          headers: { "Content-Type": "application/json", ETag: '"4"' },
        }),
    );
    const res = await client.get("/api/bootstrap");
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.etag).toBe('"4"');
      expect(res.data.revision).toBe(4);
    }
  });

  test("tanpa header ETag: etag tidak ada", async () => {
    const { client } = harness(
      () =>
        new Response(JSON.stringify(bootstrapBody), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    const res = await client.get("/api/bootstrap");
    expect(res.ok && res.etag).toBeFalsy();
  });

  test("304 menjadi { ok: false, status: 304, error.code: not_modified } tanpa membaca body", async () => {
    const { client } = harness(() => new Response(null, { status: 304, headers: { ETag: '"4"' } }));
    const res = await client.get("/api/bootstrap");
    expect(res).toEqual({
      ok: false,
      status: 304,
      error: { code: "not_modified", message: "" },
    });
  });

  test("status galat lain tetap http_error dengan statusText", async () => {
    const { client } = harness(() => new Response("x", { status: 502, statusText: "Bad Gateway" }));
    const res = await client.get("/api/bootstrap");
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.status).toBe(502);
      expect(res.error.code).toBe("http_error");
    }
  });
});

describe("client: header permintaan dan body (M3a 9.3)", () => {
  test("If-None-Match dan If-Match lewat init.headers terkirim", async () => {
    const { client, seen } = harness(() => new Response(null, { status: 304 }));
    await client.get("/api/bootstrap", { headers: { "If-None-Match": '"4"' } });
    expect(seen[0]?.headers.get("If-None-Match")).toBe('"4"');
    await client.post("/api/platform/modules/{key}/{operation}", {
      params: { path: { key: "acme.example", operation: "enable" } },
      body: {},
      headers: { "If-Match": '"4"' },
    });
    expect(seen[1]?.headers.get("If-Match")).toBe('"4"');
  });

  test("mutasi tanpa body tidak menyetel Content-Type", async () => {
    const { client, seen } = harness(() => new Response(null, { status: 204 }));
    await client.post("/api/platform/modules/{key}/{operation}", {
      params: { path: { key: "acme.example", operation: "enable" } },
    });
    expect(seen[0]?.headers.get("Content-Type")).toBeNull();
    expect(seen[0]?.body).toBeUndefined();
  });

  test("mutasi dengan body {} menyetel Content-Type application/json dan mengirim {}", async () => {
    const { client, seen } = harness(() => new Response(null, { status: 204 }));
    await client.post("/api/platform/modules/{key}/{operation}", {
      params: { path: { key: "acme.example", operation: "enable" } },
      body: {},
    });
    expect(seen[0]?.headers.get("Content-Type")).toBe("application/json");
    expect(seen[0]?.body).toBe("{}");
  });

  test("key di path di-encode", async () => {
    const { client, seen } = harness(() => new Response(null, { status: 204 }));
    await client.get("/api/platform/modules/{key}", { params: { path: { key: "a/b c" } } });
    expect(seen[0]?.url).toBe("http://api.test/api/platform/modules/a%2Fb%20c");
  });
});
