import { describe, expect, test } from "bun:test";
import { createPlatformClient } from "./client";

function recorder() {
  const headers: Headers[] = [];
  const fetchFn = (async (_url: unknown, init?: RequestInit) => {
    headers.push(new Headers(init?.headers));
    return new Response(JSON.stringify({ data: [], page: {} }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as unknown as typeof fetch;
  return { headers, fetchFn };
}

describe("createPlatformClient", () => {
  test("menyisipkan Accept-Language dari getLocale pada setiap request", async () => {
    const r = recorder();
    let locale = "id";
    const client = createPlatformClient({ fetch: r.fetchFn, getLocale: () => locale });
    await client.get("/api/platform/modules");
    locale = "en";
    await client.get("/api/platform/modules");
    expect(r.headers.map((h) => h.get("Accept-Language"))).toEqual(["id", "en"]);
  });

  test("tanpa getLocale atau locale kosong: header tidak dikirim", async () => {
    const r = recorder();
    await createPlatformClient({ fetch: r.fetchFn }).get("/api/platform/modules");
    await createPlatformClient({ fetch: r.fetchFn, getLocale: () => undefined }).get(
      "/api/platform/modules",
    );
    expect(r.headers.every((h) => h.get("Accept-Language") === null)).toBe(true);
  });

  test("header tambahan digabung dan tetap Accept: application/json", async () => {
    const r = recorder();
    const client = createPlatformClient({
      fetch: r.fetchFn,
      getLocale: () => "id",
      headers: () => ({ "X-Test": "1" }),
    });
    await client.get("/api/platform/modules");
    expect(r.headers[0]?.get("X-Test")).toBe("1");
    expect(r.headers[0]?.get("Accept")).toBe("application/json");
  });
});
