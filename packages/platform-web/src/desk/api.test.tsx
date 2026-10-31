import { describe, expect, test } from "bun:test";
import { createClient, type PlatformPaths } from "@erpora/platform-contracts";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StaticBootstrapProvider } from "../bootstrap";
import { makeBootstrap } from "../bootstrap/testing";
import { createDesksFetcher, DesksError, parseDeskList, useDesksQuery } from "./api";
import { makeDashboardItem, makeDesk, makeDeskGroup, makeDeskItem } from "./testing";

const list = {
  desks: [
    makeDesk("example", [
      makeDashboardItem("example", "example"),
      makeDeskItem("example", "notes", "/example/notes", { icon: "file-text" }),
      makeDeskGroup("example", "more", [makeDeskItem("example", "tags", "/example/more/tags")]),
    ]),
  ],
  revision: "r1",
};

function harness(responses: (() => Response)[]) {
  let i = 0;
  const seen: { url: string; inm: string | null }[] = [];
  const client = createClient<PlatformPaths>({
    fetch: (async (url: string, init?: RequestInit) => {
      seen.push({ url, inm: new Headers(init?.headers).get("If-None-Match") });
      const r = responses[Math.min(i, responses.length - 1)];
      i += 1;
      if (!r) throw new Error("tanpa respons");
      return r();
    }) as unknown as typeof fetch,
  });
  return { client, raw: client, seen };
}

const ok = (data: unknown, etag = '"e1"') =>
  new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { "Content-Type": "application/json", ETag: etag },
  });

describe("createDesksFetcher", () => {
  test("memanggil GET /api/platform/desks dan mengembalikan DeskList", async () => {
    const h = harness([() => ok(list)]);
    const data = await createDesksFetcher(h.client)();
    expect(h.seen[0]?.url).toBe("/api/platform/desks");
    expect(data.revision).toBe("r1");
    expect(data.desks[0]?.menus).toHaveLength(3);
  });

  test("ETag: kirim If-None-Match, 304 mengembalikan referensi objek yang sama", async () => {
    const h = harness([() => ok(list), () => new Response(null, { status: 304 })]);
    const fetcher = createDesksFetcher(h.client);
    const a = await fetcher();
    const b = await fetcher();
    expect(h.seen[0]?.inm).toBeNull();
    expect(h.seen[1]?.inm).toBe('"e1"');
    expect(b).toBe(a);
  });

  test("galat HTTP melempar DesksError dengan status dan kode", async () => {
    const h = harness([
      () =>
        new Response(JSON.stringify({ error: { code: "forbidden", message: "x" } }), {
          status: 403,
          headers: { "Content-Type": "application/json" },
        }),
    ]);
    const err = await createDesksFetcher(h.client)().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DesksError);
    expect((err as DesksError).status).toBe(403);
    expect((err as DesksError).error.code).toBe("forbidden");
  });

  test("304 tanpa data sebelumnya melempar DesksError", async () => {
    const h = harness([() => new Response(null, { status: 304 })]);
    await expect(createDesksFetcher(h.client)()).rejects.toBeInstanceOf(DesksError);
  });

  test("respons rusak melempar DesksError invalid_response", async () => {
    const h = harness([() => ok({ desks: "bukan array", revision: 1 })]);
    const err = await createDesksFetcher(h.client)().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DesksError);
    expect((err as DesksError).error.code).toBe("invalid_response");
  });
});

describe("parseDeskList", () => {
  test("item dengan `to` tidak aman dibuang; grup yang kosong ikut dibuang", () => {
    const parsed = parseDeskList({
      desks: [
        makeDesk("d", [
          makeDeskItem("d", "ok", "/d/ok"),
          makeDeskItem("d", "ext", "https://evil.example/x"),
          makeDeskItem("d", "proto", "//evil.example"),
          makeDeskItem("d", "js", "javascript:alert(1)"),
          makeDeskGroup("d", "g", [makeDeskItem("d", "bad", "//x")]),
        ]),
      ],
      revision: "r",
    });
    expect(parsed.desks[0]?.menus.map((m) => m.id)).toEqual(["ok"]);
  });

  test("id Desk yang tidak berbentuk id valid ditolak", () => {
    expect(() => parseDeskList({ desks: [makeDesk("Bad Id")], revision: "r" })).toThrow(DesksError);
  });

  test("bukan objek / tanpa revision ditolak", () => {
    for (const v of [null, 1, "x", [], { desks: [] }, { revision: "r" }]) {
      expect(() => parseDeskList(v)).toThrow(DesksError);
    }
  });
});

describe("useDesksQuery", () => {
  test("memuat daftar dengan key berawalan tenant", async () => {
    const h = harness([() => ok(list)]);
    const bootstrap = makeBootstrap();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>
        <StaticBootstrapProvider value={bootstrap}>{children}</StaticBootstrapProvider>
      </QueryClientProvider>
    );
    const { result } = renderHook(() => useDesksQuery(h.raw), { wrapper });
    await waitFor(() => expect(result.current.data?.revision).toBe("r1"));
    expect(queryClient.getQueryData([bootstrap.tenantId, "desks"])).toBeDefined();
  });
});
