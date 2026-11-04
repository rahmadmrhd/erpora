import { afterEach, describe, expect, test } from "bun:test";
import { createClient, setDefaultTransport } from "./client";
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

describe("client: getLocale dan Accept-Language (i18n 12.6)", () => {
  function localeClient(
    getLocale: () => string | undefined,
    headers?: () => Record<string, string>,
  ) {
    const seen: Headers[] = [];
    const client = createClient<paths>({
      baseUrl: "http://api.test",
      getLocale,
      headers,
      fetch: (async (_u: unknown, init?: RequestInit) => {
        seen.push(new Headers(init?.headers));
        return new Response(null, { status: 204 });
      }) as unknown as typeof fetch,
    });
    return { client, seen };
  }

  test("menyisipkan Accept-Language dari getLocale pada setiap request (dibaca ulang tiap kali)", async () => {
    let locale: string | undefined = "id";
    const { client, seen } = localeClient(() => locale);
    await client.get("/api/bootstrap");
    locale = "en";
    await client.get("/api/bootstrap");
    expect(seen.map((h) => h.get("Accept-Language"))).toEqual(["id", "en"]);
  });

  test("getLocale mengembalikan undefined atau kosong: header tidak dikirim", async () => {
    const { client, seen } = localeClient(() => undefined);
    await client.get("/api/bootstrap");
    const empty = localeClient(() => "");
    await empty.client.get("/api/bootstrap");
    expect(seen[0]?.get("Accept-Language")).toBeNull();
    expect(empty.seen[0]?.get("Accept-Language")).toBeNull();
  });

  test("header eksplisit pada init atau headers() menang atas getLocale", async () => {
    const a = localeClient(
      () => "id",
      () => ({ "Accept-Language": "fr" }),
    );
    await a.client.get("/api/bootstrap");
    expect(a.seen[0]?.get("Accept-Language")).toBe("fr");
    const b = localeClient(() => "id");
    await b.client.get("/api/bootstrap", { headers: { "Accept-Language": "en" } });
    expect(b.seen[0]?.get("Accept-Language")).toBe("en");
  });
});

describe("client: Retry-After pada galat (C1.14)", () => {
  const limited = (retryAfter?: string) => () =>
    new Response(JSON.stringify({ error: { code: "rate_limited", message: "x" } }), {
      status: 429,
      headers: {
        "Content-Type": "application/json",
        ...(retryAfter === undefined ? {} : { "Retry-After": retryAfter }),
      },
    });

  test("429 dengan Retry-After bilangan bulat: retryAfter dalam detik", async () => {
    const { client } = harness(limited("42"));
    const res = await client.get("/api/bootstrap");
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.retryAfter).toBe(42);
  });

  test("Retry-After tidak ada, bukan bilangan bulat positif, atau bukan 429: retryAfter tidak ada", async () => {
    for (const value of [undefined, "0", "-3", "abc", "1.5", "Wed, 21 Oct 2026 07:28:00 GMT"]) {
      const { client } = harness(limited(value));
      const res = await client.get("/api/bootstrap");
      expect(res.ok).toBe(false);
      if (!res.ok) expect(res.retryAfter, String(value)).toBeUndefined();
    }
    const { client } = harness(
      () =>
        new Response(JSON.stringify({ error: { code: "internal_error", message: "x" } }), {
          status: 503,
          headers: { "Content-Type": "application/json", "Retry-After": "7" },
        }),
    );
    const res = await client.get("/api/bootstrap");
    if (!res.ok) expect(res.retryAfter).toBeUndefined();
  });
});

describe("client: transport bawaan (C1.14)", () => {
  afterEach(() => setDefaultTransport(undefined));

  test("klien tanpa fetch/headers eksplisit memakai transport bawaan; opsi eksplisit menang", async () => {
    const seen: Array<{ via: string; auth: string | null }> = [];
    const make = (via: string) =>
      (async (_url: string | URL | Request, init?: RequestInit) => {
        seen.push({ via, auth: new Headers(init?.headers).get("Authorization") });
        return new Response(JSON.stringify(bootstrapBody), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }) as unknown as typeof fetch;
    setDefaultTransport({
      fetch: make("default"),
      headers: () => ({ Authorization: "Bearer dari-transport" }),
    });

    await createClient<paths>().get("/api/bootstrap");
    await createClient<paths>({ fetch: make("eksplisit") }).get("/api/bootstrap");
    await createClient<paths>({ headers: () => ({ Authorization: "Bearer eksplisit" }) }).get(
      "/api/bootstrap",
    );
    expect(seen).toEqual([
      { via: "default", auth: "Bearer dari-transport" },
      { via: "eksplisit", auth: "Bearer dari-transport" },
      { via: "default", auth: "Bearer eksplisit" },
    ]);

    setDefaultTransport(undefined);
    seen.length = 0;
    const raw = globalThis.fetch;
    globalThis.fetch = make("global");
    try {
      await createClient<paths>().get("/api/bootstrap");
    } finally {
      globalThis.fetch = raw;
    }
    expect(seen).toEqual([{ via: "global", auth: null }]);
  });
});
