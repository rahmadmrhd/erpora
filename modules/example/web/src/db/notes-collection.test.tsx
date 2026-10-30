import { afterEach, describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { loadDb, makeBootstrap, StaticBootstrapProvider } from "@erpora/platform-web";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { type ReactNode, Suspense } from "react";
import { installFetch, json, TENANT_ID } from "../testing";
import { createNotesCollection, useNotesCollection } from "./notes-collection";

const notes = [
  {
    id: "n1",
    title: "Satu",
    version: 1,
    createdAt: "2026-10-21T07:15:00Z",
    updatedAt: "2026-10-21T07:15:00Z",
  },
  {
    id: "n2",
    title: "Dua",
    version: 2,
    createdAt: "2026-10-22T07:15:00Z",
    updatedAt: "2026-10-22T07:15:00Z",
  },
];

let restore: (() => void) | undefined;
afterEach(() => restore?.());

const list = () =>
  json({ data: notes, page: { page: 1, pageSize: 100, total: 2, hasMore: false } });

describe("createNotesCollection", () => {
  test("koleksi Query berkunci id, dimuat dari API, dan memakai query key berawalan tenant", async () => {
    restore = installFetch(list).restore;
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const collection = createNotesCollection(await loadDb(), { queryClient, tenantId: TENANT_ID });

    const rows = await collection.toArrayWhenReady();
    expect(rows.map((n) => n.title).sort()).toEqual(["Dua", "Satu"]);
    expect(collection.get("n1")?.title).toBe("Satu");
    expect(
      queryClient
        .getQueryCache()
        .getAll()
        .map((q) => q.queryKey),
    ).toContainEqual([TENANT_ID, "example", "notes"]);
  });

  test("tenant kosong ditolak (tidak ada key tanpa scope)", async () => {
    const queryClient = new QueryClient();
    const db = await loadDb();
    expect(() => createNotesCollection(db, { queryClient, tenantId: "" })).toThrow(/tenantId/);
  });
});

describe("useNotesCollection", () => {
  function Probe({ onCollection }: { onCollection: (c: unknown) => void }) {
    onCollection(useNotesCollection());
    return <p>siap</p>;
  }

  function wrap(queryClient: QueryClient, tenantId: string, children: ReactNode) {
    return (
      <QueryClientProvider client={queryClient}>
        <StaticBootstrapProvider value={makeBootstrap({ tenantId })}>
          <Suspense fallback={<p>memuat…</p>}>{children}</Suspense>
        </StaticBootstrapProvider>
      </QueryClientProvider>
    );
  }

  test("koleksi yang sama dipakai ulang untuk (queryClient, tenant) yang sama; tenant berbeda terpisah", async () => {
    restore = installFetch(list).restore;
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const seen: unknown[] = [];
    let view!: ReturnType<typeof render>;
    // Dalam act agar penangguhan Suspense (muat chunk DB lewat use()) dilanjutkan di lingkungan uji.
    await act(async () => {
      view = render(wrap(queryClient, TENANT_ID, <Probe onCollection={(c) => seen.push(c)} />));
    });
    await screen.findByText("siap");
    view.rerender(wrap(queryClient, TENANT_ID, <Probe onCollection={(c) => seen.push(c)} />));
    expect(new Set(seen).size).toBe(1);

    const other: unknown[] = [];
    view.rerender(
      wrap(
        queryClient,
        "0190a1b2-ffff-7e5f-8a9b-0c1d2e3f4a5b",
        <Probe onCollection={(c) => other.push(c)} />,
      ),
    );
    await screen.findByText("siap");
    expect(other[0]).not.toBe(seen[0]);
  });
});

describe("TanStack DB tidak boleh diimpor statis oleh modul (K-21)", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const srcRoot = join(here, "..");

  function sources(dir: string): string[] {
    return readdirSync(dir).flatMap((name) => {
      const full = join(dir, name);
      if (statSync(full).isDirectory()) return sources(full);
      return /\.(ts|tsx)$/.test(name) && !/\.test\.tsx?$/.test(name) ? [full] : [];
    });
  }

  test("tidak ada import (statis maupun dinamis) @tanstack/react-db atau @tanstack/query-db-collection di src modul", () => {
    for (const file of sources(srcRoot)) {
      const text = readFileSync(file, "utf8");
      expect(text, file).not.toMatch(/from\s+["']@tanstack\/(react-db|db|query-db-collection)["']/);
      expect(text, file).not.toMatch(
        /import\(\s*["']@tanstack\/(react-db|db|query-db-collection)["']\s*\)/,
      );
    }
  });

  test("halaman /example memuat DB lewat useDb/useNotesCollection (Suspense), bukan impor langsung", () => {
    const page = readFileSync(join(srcRoot, "pages", "notes-page.tsx"), "utf8");
    expect(page).toMatch(/useNotesCollection\(\)/);
    expect(page).toMatch(/useLiveQuery/);
  });
});
