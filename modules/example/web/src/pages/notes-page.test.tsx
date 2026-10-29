import { afterEach, describe, expect, test } from "bun:test";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { frontendModule } from "../module";
import { installFetch, json, loadCatalogs, renderModuleRoute, TENANT_ID } from "../testing";

const catalogs = loadCatalogs("example", "example");

const notes = [
  {
    id: "0190a1b2-0000-7e5f-8a9b-0c1d2e3f4a01",
    title: "Catatan pertama",
    version: 1,
    createdAt: "2026-10-21T07:15:00Z",
    updatedAt: "2026-10-21T07:15:00Z",
  },
  {
    id: "0190a1b2-0000-7e5f-8a9b-0c1d2e3f4a02",
    title: "Catatan kedua",
    version: 3,
    createdAt: "2026-10-22T07:15:00Z",
    updatedAt: "2026-10-23T09:00:00Z",
  },
];

const listBody = (data: unknown[]) => ({
  data,
  page: { page: 1, pageSize: 100, total: data.length, hasMore: false },
});

let restore: (() => void) | undefined;
afterEach(() => restore?.());

function open() {
  return renderModuleRoute({ module: frontendModule, path: "/example", catalogs });
}

describe("halaman /example (daftar catatan)", () => {
  test("memuat catatan lewat API terketik dan menampilkan judul, kolom, baris, dan ringkasan jumlah", async () => {
    const f = installFetch(() => json(listBody(notes)));
    restore = f.restore;
    await open();

    expect(await screen.findByRole("heading", { level: 1, name: "Catatan" })).toBeInTheDocument();
    const table = await screen.findByRole("table");
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Judul", "Versi", "Diperbarui"]);
    expect(within(table).getByText("Catatan pertama")).toBeInTheDocument();
    expect(within(table).getByText("Catatan kedua")).toBeInTheDocument();
    expect(screen.getByText("2 catatan")).toBeInTheDocument();
    expect(f.calls.map((c) => c.url)).toContain("/api/example/notes?page=1&pageSize=100");
  });

  test("tidak pernah mengirim tenantId atau userId (tenant dari backend; aturan IDOR)", async () => {
    const f = installFetch(() => json(listBody(notes)));
    restore = f.restore;
    await open();
    await screen.findByRole("table");
    for (const c of f.calls) expect(c.url).not.toMatch(/tenant|userId/i);
  });

  test("query key berawalan tenant: [tenantId, 'example', 'notes']", async () => {
    const f = installFetch(() => json(listBody(notes)));
    restore = f.restore;
    const { queryClient } = await open();
    await screen.findByRole("table");
    const keys = queryClient
      .getQueryCache()
      .getAll()
      .map((q) => q.queryKey);
    expect(keys).toContainEqual([TENANT_ID, "example", "notes"]);
  });

  test("mengikuti paginasi sampai hasMore=false", async () => {
    const f = installFetch((url) => {
      const page = Number(url.searchParams.get("page"));
      return json({
        data: [notes[page - 1]],
        page: { page, pageSize: 100, total: 2, hasMore: page < 2 },
      });
    });
    restore = f.restore;
    await open();
    expect(await screen.findByText("Catatan kedua")).toBeInTheDocument();
    expect(screen.getByText("Catatan pertama")).toBeInTheDocument();
    expect(f.calls.map((c) => c.url)).toEqual([
      "/api/example/notes?page=1&pageSize=100",
      "/api/example/notes?page=2&pageSize=100",
    ]);
  });

  test("daftar kosong menampilkan keadaan kosong", async () => {
    const f = installFetch(() => json(listBody([])));
    restore = f.restore;
    await open();
    expect(await screen.findByText("Belum ada catatan.")).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  test("galat API dipetakan ke pesan katalog platform (bukan teks server) dan dapat dicoba lagi", async () => {
    let fail = true;
    const f = installFetch(() =>
      fail
        ? json({ error: { code: "permission_denied", message: "SQL: secret detail" } }, 403)
        : json(listBody(notes)),
    );
    restore = f.restore;
    const user = userEvent.setup();
    await open();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Catatan tidak dapat dimuat.");
    expect(alert).toHaveTextContent("Anda tidak memiliki izin untuk aksi ini.");
    expect(alert).not.toHaveTextContent("SQL");
    fail = false;
    await user.click(within(alert).getByRole("button", { name: "Coba lagi" }));
    expect(await screen.findByText("Catatan pertama")).toBeInTheDocument();
  });

  test("aksi halaman 'Segarkan' terdaftar dan memuat ulang data", async () => {
    let n = 0;
    const f = installFetch(() => json(listBody(n++ === 0 ? [notes[0]] : notes)));
    restore = f.restore;
    const user = userEvent.setup();
    await open();
    await screen.findByText("Catatan pertama");
    expect(screen.queryByText("Catatan kedua")).toBeNull();

    const actions = await screen.findByTestId("page-actions");
    await user.click(within(actions).getByRole("button", { name: "Segarkan" }));
    expect(await screen.findByText("Catatan kedua")).toBeInTheDocument();
    expect(f.calls).toHaveLength(2);
  });

  test("aksi halaman dibersihkan saat halaman ditinggalkan (unmount)", async () => {
    const f = installFetch(() => json(listBody(notes)));
    restore = f.restore;
    const { router } = await open();
    await screen.findByText("Catatan pertama");
    expect(within(screen.getByTestId("page-actions")).getByRole("button")).toBeInTheDocument();
    await act(async () => {
      await router.navigate({ to: "/tidak-ada" });
    });
    await waitFor(() =>
      expect(within(screen.getByTestId("page-actions")).queryByRole("button")).toBeNull(),
    );
  });
});
