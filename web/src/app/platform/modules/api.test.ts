import { describe, expect, test } from "bun:test";
import { createPlatformClient } from "@erpora/platform-web";
import {
  fetchHistory,
  fetchModules,
  fetchOperation,
  MODULES_PAGE_SIZE,
  ModulesApiError,
  runModuleOperation,
} from "./api";
import type { ModuleView } from "./types";

interface Call {
  url: string;
  method: string;
  headers: Headers;
  body: string | undefined;
}

function recorder(handler: (c: Call) => Response) {
  const calls: Call[] = [];
  const fetchImpl = (async (url: string | URL | Request, init?: RequestInit) => {
    const call: Call = {
      url: String(url),
      method: (init?.method ?? "GET").toUpperCase(),
      headers: new Headers(init?.headers),
      body: typeof init?.body === "string" ? init.body : undefined,
    };
    calls.push(call);
    return handler(call);
  }) as typeof fetch;
  return { calls, client: createPlatformClient({ fetch: fetchImpl }) };
}

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", ...headers },
  });

const moduleView = (key: string, over: Partial<ModuleView> = {}): ModuleView => ({
  key,
  slug: key,
  name: key,
  binaryVersion: "1.0.0",
  installedVersion: "1.0.0",
  status: "enabled",
  required: false,
  compatible: true,
  incompatibleCodes: [],
  dependencies: [],
  dependents: [],
  lastOperationId: null,
  ...over,
});

describe("fetchModules", () => {
  test("pageSize=200, mengikuti hasMore, dan mengembalikan ETag halaman pertama", async () => {
    const { calls, client } = recorder((c) => {
      const page = Number(new URL(c.url, "http://x").searchParams.get("page"));
      return json(
        {
          data: [moduleView(`m${page}`)],
          page: { page, pageSize: 200, total: 3, hasMore: page < 3 },
        },
        200,
        { ETag: `"rev-${page}"` },
      );
    });
    const r = await fetchModules(client);
    expect(MODULES_PAGE_SIZE).toBe(200);
    expect(r.modules.map((m) => m.key)).toEqual(["m1", "m2", "m3"]);
    expect(r.etag).toBe('"rev-1"');
    expect(calls.map((c) => c.url)).toEqual([
      "/api/platform/modules?page=1&pageSize=200",
      "/api/platform/modules?page=2&pageSize=200",
      "/api/platform/modules?page=3&pageSize=200",
    ]);
  });

  test("galat HTTP dilempar sebagai ModulesApiError dengan status dan apiError", async () => {
    const { client } = recorder(() =>
      json({ error: { code: "permission_denied", message: "x", requestId: "r1" } }, 403),
    );
    const err = await fetchModules(client).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ModulesApiError);
    expect((err as ModulesApiError).status).toBe(403);
    expect((err as ModulesApiError).apiError.code).toBe("permission_denied");
  });
});

describe("runModuleOperation", () => {
  const okBody = {
    data: { module: moduleView("acme.x"), operations: [], operationIds: ["op-1"] },
  };

  test("POST dengan body {} dan Content-Type application/json; If-Match dari ETag daftar", async () => {
    const { calls, client } = recorder(() => json(okBody));
    const r = await runModuleOperation(client, {
      key: "acme.x",
      op: "disable",
      ifMatch: '"rev-7"',
    });
    const c = calls[0] as Call;
    expect(c.method).toBe("POST");
    expect(c.url).toBe("/api/platform/modules/acme.x/disable");
    expect(c.body).toBe("{}");
    expect(c.headers.get("Content-Type")).toBe("application/json");
    expect(c.headers.get("If-Match")).toBe('"rev-7"');
    expect(r.kind).toBe("done");
  });

  test("install mengirim withDeps dan enable; operasi lain tidak", async () => {
    const { calls, client } = recorder(() => json(okBody));
    await runModuleOperation(client, {
      key: "acme.x",
      op: "install",
      withDeps: true,
      enable: false,
    });
    await runModuleOperation(client, { key: "acme.x", op: "enable", withDeps: true, enable: true });
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual({ withDeps: true, enable: false });
    expect(calls[1]?.body).toBe("{}");
  });

  test("tanpa ifMatch tidak mengirim header If-Match", async () => {
    const { calls, client } = recorder(() => json(okBody));
    await runModuleOperation(client, { key: "acme.x", op: "enable" });
    expect(calls[0]?.headers.has("If-Match")).toBe(false);
  });

  test("202 menjadi kind accepted dengan seluruh operationIds", async () => {
    const { client } = recorder(() =>
      json(
        { data: { operationId: "op-1", operationIds: ["op-1", "op-2"], status: "running" } },
        202,
      ),
    );
    expect(await runModuleOperation(client, { key: "acme.x", op: "install" })).toEqual({
      kind: "accepted",
      operationIds: ["op-1", "op-2"],
    });
  });

  test("202 tanpa operationIds memakai operationId tunggal", async () => {
    const { client } = recorder(() =>
      json({ data: { operationId: "op-9", status: "running" } }, 202),
    );
    expect(await runModuleOperation(client, { key: "acme.x", op: "retry" })).toEqual({
      kind: "accepted",
      operationIds: ["op-9"],
    });
  });

  test.each([404, 409, 412, 415, 422])(
    "status %i dilempar sebagai ModulesApiError",
    async (status) => {
      const { client } = recorder(() => json({ error: { code: "x", message: "m" } }, status));
      const err = await runModuleOperation(client, { key: "acme.x", op: "enable" }).catch(
        (e: unknown) => e,
      );
      expect(err).toBeInstanceOf(ModulesApiError);
      expect((err as ModulesApiError).status).toBe(status);
    },
  );
});

describe("fetchHistory dan fetchOperation", () => {
  test("riwayat: halaman 25 dan hasMore", async () => {
    const { calls, client } = recorder(() =>
      json({ data: [], page: { page: 2, pageSize: 25, total: 60, hasMore: true } }),
    );
    const r = await fetchHistory(client, "acme.x", 2);
    expect(calls[0]?.url).toBe("/api/platform/modules/acme.x/history?page=2&pageSize=25");
    expect(r).toEqual({ events: [], hasMore: true });
  });

  test("operasi: GET operations/{id}", async () => {
    const op = { id: "op-1", status: "running", steps: [] };
    const { calls, client } = recorder(() => json({ data: op }));
    expect(await fetchOperation(client, "op-1")).toEqual(op as never);
    expect(calls[0]?.url).toBe("/api/platform/modules/operations/op-1");
  });
});

describe("P11: kebersihan request", () => {
  test("tidak ada tenantId atau userId di URL, header, maupun body pada seluruh alur", async () => {
    const { calls, client } = recorder((c) => {
      if (c.method === "POST")
        return json({ data: { module: moduleView("acme.x"), operations: [], operationIds: [] } });
      if (c.url.includes("/operations/"))
        return json({ data: { id: "op-1", status: "succeeded", steps: [] } });
      if (c.url.includes("/history"))
        return json({ data: [], page: { page: 1, pageSize: 25, total: 0, hasMore: false } });
      return json({
        data: [moduleView("acme.x")],
        page: { page: 1, pageSize: 200, total: 1, hasMore: false },
      });
    });
    await fetchModules(client);
    await fetchHistory(client, "acme.x", 1);
    await fetchOperation(client, "op-1");
    await runModuleOperation(client, {
      key: "acme.x",
      op: "install",
      withDeps: true,
      enable: true,
      ifMatch: '"1"',
    });
    await runModuleOperation(client, { key: "acme.x", op: "disable" });
    for (const c of calls) {
      expect(c.url).not.toMatch(/tenant|userId/i);
      expect([...c.headers.keys()].join(",")).not.toMatch(/tenant|user-?id/i);
      expect(c.body ?? "").not.toMatch(/tenant|userId/i);
      if (c.method === "POST") {
        expect(c.headers.get("Content-Type")).toBe("application/json");
        expect(typeof JSON.parse(c.body ?? "null")).toBe("object");
      }
    }
  });
});
