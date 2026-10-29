import { afterEach, describe, expect, test } from "bun:test";
import { BOOTSTRAP_KEY } from "@erpora/platform-web";
import type { QueryClient } from "@tanstack/react-query";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ModulesPage } from "./page";
import {
  createModulesServer,
  type ModulesServer,
  mod,
  operation,
  renderWithServer,
  type ServerOptions,
} from "./test-support";
import type { ModuleView } from "./types";

let restore: (() => void) | undefined;
afterEach(() => restore?.());

const shop = (over: Partial<ModuleView> = {}) =>
  mod("acme.shop", {
    name: "Shop",
    dependencies: [{ module: "platform.core", version: "^0.1", optional: false }],
    ...over,
  });

async function open(
  modules: ModuleView[],
  opts: ServerOptions = {},
  extra: { adminApi?: boolean; permissions?: string[]; queryClient?: QueryClient } = {},
) {
  const server = createModulesServer(modules, opts);
  const view = await renderWithServer({ server, ...extra, children: <ModulesPage /> });
  restore = view.restore;
  return { server, ...view };
}

async function pickMenu(user: ReturnType<typeof userEvent.setup>, name: string, item: string) {
  await user.click(await screen.findByRole("button", { name: `Aksi untuk ${name}` }));
  await user.click(await screen.findByRole("menuitem", { name: item }));
}

describe("tabel modul (R13.1)", () => {
  test("kolom, nama dengan key monospasi, versi binary vs ter-install, dan nada status", async () => {
    await open([
      shop({ status: "enabled" }),
      mod("acme.crm", {
        name: "Crm",
        status: "upgrade_pending",
        binaryVersion: "1.2.0",
        installedVersion: "1.0.0",
      }),
      mod("acme.fin", { name: "Fin", status: "failed", required: true }),
      mod("acme.new", { name: "New", status: "available", installedVersion: null }),
      mod("acme.gone", { name: "Gone", status: "missing" }),
    ]);
    const table = await screen.findByRole("table");
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Nama", "Versi", "Status", "Dependensi", "Dependen", "Aksi"]);
    expect(within(table).getByText("acme.shop").tagName).toBe("CODE");
    const crm = within(table).getByText("Crm").closest("tr") as HTMLElement;
    expect(within(crm).getByText("Terpasang 1.0.0")).toBeInTheDocument();
    expect(within(crm).getByText("Binary 1.2.0")).toBeInTheDocument();
    expect(within(crm).getByText("Upgrade tersedia")).toBeInTheDocument();
    expect(within(table).getByText("Aktif")).toHaveAttribute("data-tone", "success");
    expect(within(table).getByText("Menunggu upgrade")).toHaveAttribute("data-tone", "warning");
    expect(within(table).getByText("Gagal")).toHaveAttribute("data-tone", "error");
    expect(within(table).getByText("Tersedia")).toHaveAttribute("data-tone", "secondary");
    expect(within(table).getByText("Hilang")).toHaveAttribute("data-tone", "error");
    const fin = within(table).getByText("Fin").closest("tr") as HTMLElement;
    expect(within(fin).getByText("Wajib")).toBeInTheDocument();
    expect(within(table).getByText("Belum terpasang")).toBeInTheDocument();
  });

  test("dependensi opsional ditandai; dependen tampil", async () => {
    await open([
      shop({
        dependencies: [
          { module: "acme.core", version: "^1", optional: false },
          { module: "acme.extra", version: "^1", optional: true },
        ],
        dependents: ["acme.crm"],
      }),
    ]);
    expect(await screen.findByText("acme.core")).toBeInTheDocument();
    expect(screen.getByText("acme.extra (opsional)")).toBeInTheDocument();
    expect(screen.getByText("acme.crm")).toBeInTheDocument();
  });

  test("daftar kosong menampilkan keadaan kosong", async () => {
    await open([]);
    expect(await screen.findByText("Belum ada modul.")).toBeInTheDocument();
  });

  test("query key daftar berawalan tenant", async () => {
    const { queryClient } = await open([shop()]);
    await screen.findByRole("table");
    expect(
      queryClient
        .getQueryCache()
        .getAll()
        .map((q) => q.queryKey[0]),
    ).toContain("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b");
  });
});

describe("mode hanya-baca dan galat daftar (R13.2)", () => {
  test("tanpa features.modulesAdminApi: tanpa kolom Aksi dan ada Alert CLI", async () => {
    await open([shop()], {}, { adminApi: false });
    const table = await screen.findByRole("table");
    expect(within(table).queryByRole("columnheader", { name: "Aksi" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Aksi untuk/ })).toBeNull();
    const alert = screen.getByRole("status", { name: "" });
    expect(alert).toHaveTextContent("Mode hanya-baca");
    expect(alert).toHaveTextContent("erpora modules");
  });

  test("M-8: tanpa permission platform.modules.manage (permissions terisi) halaman hanya-baca walau API mutasi aktif", async () => {
    await open([shop()], {}, { permissions: ["platform.modules.read"] });
    const table = await screen.findByRole("table");
    expect(within(table).queryByRole("columnheader", { name: "Aksi" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Aksi untuk/ })).toBeNull();
    const alert = screen.getByRole("status", { name: "" });
    expect(alert).toHaveTextContent("Mode hanya-baca");
    expect(alert).toHaveTextContent("platform.modules.manage");
    expect(alert).not.toHaveTextContent("erpora modules");
  });

  test("M-8: dengan permission platform.modules.manage aksi ditawarkan", async () => {
    await open([shop()], {}, { permissions: ["platform.modules.read", "platform.modules.manage"] });
    expect(await screen.findByRole("button", { name: "Aksi untuk Shop" })).toBeInTheDocument();
    expect(screen.queryByText("Mode hanya-baca")).toBeNull();
  });

  test("M-8: permissions kosong (belum ada informasi) tetap optimistis (K-30)", async () => {
    await open([shop()], {}, { permissions: [] });
    expect(await screen.findByRole("button", { name: "Aksi untuk Shop" })).toBeInTheDocument();
  });

  test("403 pada daftar menampilkan ErrorPage 403 di dalam halaman", async () => {
    await open([shop()], { listStatus: 403 });
    expect(
      await screen.findByRole("heading", { level: 1, name: "403: Tidak diizinkan" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  test("galat lain pada daftar menampilkan Alert dengan pesan katalog dan Coba lagi", async () => {
    await open([shop()], { listStatus: 500 });
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Daftar modul tidak dapat dimuat.");
    expect(within(alert).getByRole("button", { name: "Coba lagi" })).toBeInTheDocument();
  });

  test("aksi halaman 'Segarkan' memuat ulang daftar", async () => {
    const user = userEvent.setup();
    const { server } = await open([shop()]);
    await screen.findByRole("table");
    await user.click(
      within(screen.getByTestId("page-actions")).getByRole("button", { name: "Segarkan" }),
    );
    await waitFor(() =>
      expect(
        server.calls.filter((c) => c.method === "GET" && c.path === "/api/platform/modules"),
      ).toHaveLength(2),
    );
  });
});

describe("menu aksi per status (cermin ADR-0007)", () => {
  test.each([
    ["enabled", ["Nonaktifkan", "Riwayat"]],
    ["disabled", ["Aktifkan", "Uninstall", "Riwayat"]],
    ["available", ["Install", "Riwayat"]],
    ["failed", ["Coba ulang", "Riwayat"]],
    ["upgrade_pending", ["Upgrade", "Riwayat"]],
    ["missing", ["Riwayat"]],
  ] as const)("%s menawarkan %j", async (status, items) => {
    const user = userEvent.setup();
    await open([shop({ status })]);
    await user.click(await screen.findByRole("button", { name: "Aksi untuk Shop" }));
    const menu = await screen.findByRole("menu");
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((i) => i.textContent),
    ).toEqual(items as unknown as string[]);
  });

  test("modul required tidak menawarkan Nonaktifkan/Uninstall", async () => {
    const user = userEvent.setup();
    await open([shop({ required: true })]);
    await user.click(await screen.findByRole("button", { name: "Aksi untuk Shop" }));
    expect(
      within(await screen.findByRole("menu"))
        .getAllByRole("menuitem")
        .map((i) => i.textContent),
    ).toEqual(["Riwayat"]);
  });
});

describe("dialog konfirmasi dan eksekusi (R13.3, R13.5)", () => {
  test("menampilkan dampak dari navigasi modul frontend; Esc tidak menutup; Batal menutup", async () => {
    const user = userEvent.setup();
    await open([shop()]);
    await pickMenu(user, "Shop", "Nonaktifkan");
    const dialog = await screen.findByRole("alertdialog", { name: "Nonaktifkan: Shop" });
    expect(dialog).toHaveTextContent("Menu disembunyikan: Shop (acme.shop)");
    expect(dialog).toHaveTextContent("Halaman tidak aktif: /shop");
    expect(dialog).toHaveTextContent("API /api/shop menolak permintaan (module_disabled)");
    expect(dialog).toHaveTextContent("Data tidak dihapus");
    await user.keyboard("{Escape}");
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Batal" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  });

  test("M-1: Aktifkan modul nonaktif tidak memuat katalog namespace modul (404 di backend); label dari nama/key API", async () => {
    const user = userEvent.setup();
    await open([shop({ status: "disabled" })]);
    await pickMenu(user, "Shop", "Aktifkan");
    const dialog = await screen.findByRole("alertdialog", { name: "Aktifkan: Shop" });
    expect(dialog).toHaveTextContent("Menu ditampilkan: Shop (acme.shop)");
    expect(dialog).toHaveTextContent("Halaman aktif: /shop");
    expect(dialog).not.toHaveTextContent("nav.home");
  });

  test("blocker menonaktifkan tombol konfirmasi dan menjelaskan penyebab", async () => {
    const user = userEvent.setup();
    const base = shop({ dependents: ["acme.req"] });
    const req = mod("acme.req", {
      name: "Req",
      dependencies: [{ module: "acme.shop", version: "^1", optional: false }],
    });
    await open([base, req]);
    await pickMenu(user, "Shop", "Nonaktifkan");
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveTextContent("Modul lain masih bergantung dan aktif: acme.req");
    expect(within(dialog).getByRole("button", { name: "Lanjutkan" })).toBeDisabled();
  });

  test("install menawarkan opsi dependensi dan aktivasi yang ikut dikirim", async () => {
    const user = userEvent.setup();
    const { server } = await open([
      shop({
        status: "available",
        installedVersion: null,
        dependencies: [{ module: "acme.core", version: "^1", optional: false }],
      }),
      mod("acme.core", { name: "Core", status: "available", installedVersion: null }),
    ]);
    await pickMenu(user, "Shop", "Install");
    const dialog = await screen.findByRole("alertdialog");
    const withDeps = within(dialog).getByRole("checkbox", { name: "Sertakan dependensi" });
    const enable = within(dialog).getByRole("checkbox", { name: "Aktifkan setelah install" });
    expect(withDeps).toHaveAttribute("aria-checked", "true");
    await user.click(enable);
    await user.click(within(dialog).getByRole("button", { name: "Lanjutkan" }));
    await waitFor(() => expect(server.calls.some((c) => c.method === "POST")).toBe(true));
    const post = server.calls.find((c) => c.method === "POST");
    expect(post?.path).toBe("/api/platform/modules/acme.shop/install");
    expect(JSON.parse(post?.body ?? "")).toEqual({ withDeps: true, enable: true });
  });

  test("200: kirim If-Match dari ETag daftar, tampilkan hasil, invalidasi bootstrap, dan daftar menyegarkan status", async () => {
    const user = userEvent.setup();
    const { server, queryClient } = await open([shop()]);
    const invalidated: unknown[] = [];
    const original = queryClient.invalidateQueries.bind(queryClient);
    queryClient.invalidateQueries = ((f: { queryKey?: unknown }) => {
      invalidated.push(f?.queryKey);
      return original(f as never);
    }) as typeof queryClient.invalidateQueries;
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    const post = await waitFor(() => {
      const c = server.calls.find((x) => x.method === "POST");
      if (!c) throw new Error("belum");
      return c;
    });
    expect(post.headers.get("If-Match")).toBe('"1"');
    expect(post.body).toBe("{}");
    expect(await screen.findByText("Berhasil")).toBeInTheDocument();
    expect(invalidated).toContainEqual(BOOTSTRAP_KEY);
    await waitFor(() => expect(screen.queryByText("Aktif")).toBeNull());
    expect(screen.getAllByText("Nonaktif").length).toBeGreaterThan(0);
  });

  test("operasi gagal pada 200: pesan kegagalan dari katalog, bukan teks server", async () => {
    const user = userEvent.setup();
    await open([shop()], {
      failure: { code: "migration_failed", message: "ERROR: relation x does not exist (SQL)" },
    });
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    const dialog = await screen.findByRole("alertdialog");
    expect(await within(dialog).findByText("Migrasi database gagal.")).toBeInTheDocument();
    expect(dialog).not.toHaveTextContent("SQL");
    expect(within(dialog).getByRole("button", { name: "Tutup" })).toBeInTheDocument();
  });

  test("412: pesan 'status modul berubah' dan daftar dimuat ulang", async () => {
    const user = userEvent.setup();
    const { server } = await open([shop()], {
      postStatus: 412,
      postError: { code: "version_mismatch" },
    });
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    expect(await screen.findByText(/Status modul berubah/)).toBeInTheDocument();
    await waitFor(() =>
      expect(
        server.calls.filter((c) => c.method === "GET" && c.path === "/api/platform/modules").length,
      ).toBeGreaterThanOrEqual(2),
    );
  });

  test("M-3: 404 pada mutasi saat API mutasi aktif = 'modul tidak ditemukan': tetap mode aksi, daftar dan bootstrap dimuat ulang", async () => {
    const user = userEvent.setup();
    const { server, queryClient } = await open([shop()], {
      postStatus: 404,
      postError: { code: "not_found" },
    });
    const invalidated: unknown[] = [];
    const original = queryClient.invalidateQueries.bind(queryClient);
    queryClient.invalidateQueries = ((f: { queryKey?: unknown }) => {
      invalidated.push(f?.queryKey);
      return original(f as never);
    }) as typeof queryClient.invalidateQueries;
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    expect(await screen.findByText(/Modul tidak ditemukan/)).toBeInTheDocument();
    expect(screen.queryByText(/Manajemen modul tidak aktif/)).toBeNull();
    await user.click(screen.getByRole("button", { name: "Tutup" }));
    expect(await screen.findByRole("button", { name: "Aksi untuk Shop" })).toBeInTheDocument();
    expect(screen.queryByText("Mode hanya-baca")).toBeNull();
    expect(invalidated).toContainEqual(BOOTSTRAP_KEY);
    await waitFor(() =>
      expect(
        server.calls.filter((c) => c.method === "GET" && c.path === "/api/platform/modules").length,
      ).toBeGreaterThanOrEqual(2),
    );
  });

  test("202: memantau operasi lewat polling sampai selesai lalu menampilkan hasil dan invalidasi", async () => {
    const user = userEvent.setup();
    const ops: Record<string, ReturnType<typeof operation>> = {
      "op-1": operation({ id: "op-1", status: "succeeded" }),
    };
    const { server } = await open([shop()], { mode: "accepted", operations: ops });
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    expect(await screen.findByText("Berhasil")).toBeInTheDocument();
    expect(server.opFetches["op-1"]).toBeGreaterThanOrEqual(1);
  });
  test("M-2: menutup dialog saat polling 202 tidak menghentikan invalidasi bootstrap saat operasi selesai (R13.5)", async () => {
    const user = userEvent.setup();
    const ops: Record<string, ReturnType<typeof operation>> = {
      "op-1": operation({ id: "op-1", status: "running" }),
    };
    const { server, queryClient } = await open([shop()], { mode: "accepted", operations: ops });
    const invalidated: unknown[] = [];
    const original = queryClient.invalidateQueries.bind(queryClient);
    queryClient.invalidateQueries = ((f: { queryKey?: unknown }) => {
      invalidated.push(f?.queryKey);
      return original(f as never);
    }) as typeof queryClient.invalidateQueries;
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    await waitFor(() => expect(server.opFetches["op-1"]).toBeGreaterThanOrEqual(1));
    await user.click(await screen.findByRole("button", { name: "Tutup" }));
    expect(screen.queryByRole("alertdialog")).toBeNull();
    const before = invalidated.filter((k) => k === BOOTSTRAP_KEY).length;
    // Operasi selesai setelah dialog ditutup: polling di level halaman tetap hidup dan menyegarkan bootstrap.
    ops["op-1"] = operation({ id: "op-1", status: "succeeded" });
    await waitFor(
      () => expect(invalidated.filter((k) => k === BOOTSTRAP_KEY).length).toBeGreaterThan(before),
      { timeout: 4500 },
    );
  }, 15_000);

  test("M-2: polling berhenti saat halaman unmount", async () => {
    const user = userEvent.setup();
    const ops: Record<string, ReturnType<typeof operation>> = {
      "op-1": operation({ id: "op-1", status: "running" }),
    };
    const { server, unmount } = await open([shop()], { mode: "accepted", operations: ops });
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    await waitFor(() => expect(server.opFetches["op-1"]).toBeGreaterThanOrEqual(1));
    unmount();
    const n = server.opFetches["op-1"];
    await new Promise((r) => setTimeout(r, 2500));
    expect(server.opFetches["op-1"]).toBe(n);
  }, 15_000);

  test("M-7: galat permanen saat polling (404) menghentikan polling dan menampilkan pesan, bukan percobaan terus-menerus", async () => {
    const user = userEvent.setup();
    const { server } = await open([shop()], { mode: "accepted", operationStatus: 404 });
    await pickMenu(user, "Shop", "Nonaktifkan");
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Lanjutkan" }),
    );
    expect(await screen.findByText(/Pemantauan operasi dihentikan/)).toBeInTheDocument();
    const n = server.opFetches["op-1"];
    expect(n).toBe(1);
    await new Promise((r) => setTimeout(r, 2500));
    expect(server.opFetches["op-1"]).toBe(n);
  }, 15_000);
});

describe("riwayat (R13.4)", () => {
  test("daftar event dengan aktor; langkah operasi dimuat saat baris dibuka; details sebagai teks", async () => {
    const user = userEvent.setup();
    const { server } = await open([shop()], {
      events: events as never,
      operations: { "op-1": operation({ id: "op-1" }) },
    });
    await pickMenu(user, "Shop", "Riwayat");
    const dialog = await screen.findByRole("dialog", { name: "Riwayat Shop" });
    expect(await within(dialog).findByText("Diaktifkan")).toBeInTheDocument();
    expect(within(dialog).getByText("Direkonsiliasi")).toBeInTheDocument();
    expect(within(dialog).getByText("oleh cli:admin")).toBeInTheDocument();
    expect(server.opFetches["op-1"]).toBeUndefined();

    await user.click(within(dialog).getByRole("button", { name: /Langkah operasi/ }));
    expect(await within(dialog).findByText("hook:disable: succeeded (15 ms)")).toBeInTheDocument();
    expect(server.opFetches["op-1"]).toBe(1);

    await user.click(within(dialog).getAllByRole("button", { name: /Detail/ })[0] as HTMLElement);
    const pre = dialog.querySelector("pre") as HTMLElement;
    expect(pre.textContent).toContain("<b>tebal</b>");
    expect(dialog.querySelector("pre b")).toBeNull();
  });

  test("riwayat kosong menampilkan keadaan kosong", async () => {
    const user = userEvent.setup();
    await open([shop()], { events: [] });
    await pickMenu(user, "Shop", "Riwayat");
    expect(await screen.findByText("Belum ada riwayat.")).toBeInTheDocument();
  });
});

export type { ModulesServer };
