import { describe, expect, test } from "bun:test";
import { join } from "node:path";
import { QueryClient } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { Suspense } from "react";
import { createDbLoader, type DbImporters, loadDb, useDb } from "./index";

describe("loadDb", () => {
  test("memberi createCollection, useLiveQuery, dan queryCollectionOptions", async () => {
    const api = await loadDb();
    expect(typeof api.createCollection).toBe("function");
    expect(typeof api.useLiveQuery).toBe("function");
    expect(typeof api.queryCollectionOptions).toBe("function");
  });

  test("hanya dimuat sekali: promise dan hasil yang sama", async () => {
    const a = loadDb();
    const b = loadDb();
    expect(a).toBe(b);
    expect(await a).toBe(await b);
  });

  test("createDbLoader memanggil importer sekali walau dipanggil berkali-kali", async () => {
    let dbCalls = 0;
    let queryCalls = 0;
    const real = {
      db: await import("@tanstack/react-db"),
      query: await import("@tanstack/query-db-collection"),
    };
    const importers: DbImporters = {
      db: async () => {
        dbCalls += 1;
        return real.db;
      },
      query: async () => {
        queryCalls += 1;
        return real.query;
      },
    };
    const load = createDbLoader(importers);
    await Promise.all([load(), load(), load()]);
    await load();
    expect([dbCalls, queryCalls]).toEqual([1, 1]);
  });

  test("pemuatan gagal membuang cache sehingga percobaan berikutnya berhasil", async () => {
    const real = {
      db: await import("@tanstack/react-db"),
      query: await import("@tanstack/query-db-collection"),
    };
    let attempt = 0;
    const load = createDbLoader({
      db: async () => {
        attempt += 1;
        if (attempt === 1) throw new Error("chunk gagal dimuat");
        return real.db;
      },
      query: async () => real.query,
    });
    await expect(load()).rejects.toThrow("chunk gagal dimuat");
    const api = await load();
    expect(typeof api.createCollection).toBe("function");
    expect(attempt).toBe(2);
  });

  test("koleksi dari Query dapat dibuat dan dibaca (bukti integrasi, memori saja)", async () => {
    const { createCollection, queryCollectionOptions } = await loadDb();
    const queryClient = new QueryClient();
    const collection = createCollection(
      queryCollectionOptions({
        queryKey: ["tenant-1", "example", "notes"],
        queryFn: async () => [
          { id: "1", title: "Satu" },
          { id: "2", title: "Dua" },
        ],
        queryClient,
        getKey: (n: { id: string; title: string }) => n.id,
      }),
    );
    const rows = await collection.toArrayWhenReady();
    expect(rows.map((r) => r.title)).toEqual(["Satu", "Dua"]);
  });
});

describe("useDb", () => {
  test("komponen dalam Suspense menerima API DB", async () => {
    function Probe() {
      const db = useDb();
      return <span>{typeof db.createCollection}</span>;
    }
    await act(async () => {
      render(
        <Suspense fallback={<span>memuat</span>}>
          <Probe />
        </Suspense>,
      );
    });
    expect(await screen.findByText("function")).toBeInTheDocument();
  });
});

describe("tanpa impor statis (K-21)", () => {
  test("hanya db/index.ts yang menyebut paket DB, dan hanya lewat import() dinamis atau tipe", async () => {
    const root = join(import.meta.dir, "../../../..");
    const offenders: string[] = [];
    const globs = [
      "web/src/**/*.{ts,tsx}",
      "packages/*/src/**/*.{ts,tsx}",
      "modules/*/web/src/**/*.{ts,tsx}",
    ];
    for (const pattern of globs) {
      for await (const rel of new Bun.Glob(pattern).scan({ cwd: root, onlyFiles: true })) {
        if (/(^|[\\/])node_modules[\\/]/.test(rel) || /\.(test|check)\.tsx?$/.test(rel)) continue;
        const text = await Bun.file(join(root, rel)).text();
        // import/export statis nilai (bukan `import type`) dari paket DB.
        const staticImport =
          /^\s*(?:import|export)\s+(?!type\b)[^;]*?from\s+["']@tanstack\/(?:react-db|query-db-collection|db)["']/m;
        const sideEffect = /^\s*import\s+["']@tanstack\/(?:react-db|query-db-collection|db)["']/m;
        if (staticImport.test(text) || sideEffect.test(text)) offenders.push(rel);
      }
    }
    expect(offenders).toEqual([]);
  });

  test("pemindai mengenali impor statis (kontrol negatif)", () => {
    const staticImport =
      /^\s*(?:import|export)\s+(?!type\b)[^;]*?from\s+["']@tanstack\/(?:react-db|query-db-collection|db)["']/m;
    expect(staticImport.test(`import { createCollection } from "@tanstack/react-db";`)).toBe(true);
    expect(staticImport.test(`import type { Collection } from "@tanstack/react-db";`)).toBe(false);
    expect(staticImport.test(`const m = await import("@tanstack/react-db");`)).toBe(false);
  });
});
