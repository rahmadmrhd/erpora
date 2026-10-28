import { afterEach, describe, expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { frontendModule } from "../module";
import { installFetch, json, loadCatalogs, renderModuleRoute, TENANT_ID } from "../testing";

const catalogs = loadCatalogs("exampleaddon", "example-addon");

let restore: (() => void) | undefined;
afterEach(() => restore?.());

function open() {
  return renderModuleRoute({ module: frontendModule, path: "/example-addon", catalogs });
}

describe("halaman /example-addon (ringkasan)", () => {
  test("menampilkan judul dan ringkasan noteCount dari API terketik", async () => {
    const f = installFetch(() => json({ data: { noteCount: 3 } }));
    restore = f.restore;
    open();
    expect(await screen.findByRole("heading", { level: 1, name: "Ringkasan" })).toBeInTheDocument();
    expect(await screen.findByText("3 catatan")).toBeInTheDocument();
    expect(f.calls.map((c) => c.url)).toEqual(["/api/example-addon/summary"]);
  });

  test("tanpa tenantId/userId pada permintaan; query key berawalan tenant", async () => {
    const f = installFetch(() => json({ data: { noteCount: 0 } }));
    restore = f.restore;
    const { queryClient } = open();
    expect(await screen.findByText("Tidak ada catatan")).toBeInTheDocument();
    for (const c of f.calls) expect(c.url).not.toMatch(/tenant|userId/i);
    expect(
      queryClient
        .getQueryCache()
        .getAll()
        .map((q) => q.queryKey),
    ).toContainEqual([TENANT_ID, "example-addon", "summary"]);
  });

  test("tautan ke halaman modul example memakai slug dari paket @erpora/module-example", async () => {
    const f = installFetch(() => json({ data: { noteCount: 3 } }));
    restore = f.restore;
    open();
    const link = await screen.findByRole("link", { name: "Lihat catatan" });
    expect(link).toHaveAttribute("href", "/example");
  });

  test("aksi halaman 'Segarkan' memuat ulang ringkasan", async () => {
    let n = 0;
    const f = installFetch(() => json({ data: { noteCount: n++ === 0 ? 3 : 4 } }));
    restore = f.restore;
    const user = userEvent.setup();
    open();
    await screen.findByText("3 catatan");
    const actions = await screen.findByTestId("page-actions");
    await user.click(within(actions).getByRole("button", { name: "Segarkan" }));
    expect(await screen.findByText("4 catatan")).toBeInTheDocument();
    expect(f.calls).toHaveLength(2);
  });

  test("galat 500 menampilkan Alert dengan pesan katalog platform dan tombol coba lagi", async () => {
    let fail = true;
    const f = installFetch(() =>
      fail
        ? json({ error: { code: "internal_error", message: "panic: nil pointer" } }, 500)
        : json({ data: { noteCount: 1 } }),
    );
    restore = f.restore;
    const user = userEvent.setup();
    open();
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Ringkasan tidak dapat dimuat.");
    expect(alert).not.toHaveTextContent("panic");
    fail = false;
    await user.click(within(alert).getByRole("button", { name: "Coba lagi" }));
    expect(await screen.findByText("1 catatan")).toBeInTheDocument();
  });
});
