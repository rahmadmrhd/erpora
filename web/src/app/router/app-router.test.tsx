import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { createRoute, type ModuleBaseRoute, ModuleDefinitionError } from "@erpora/module-sdk";
import { makeBootstrap, makeBootstrapModule, resolveModules } from "@erpora/platform-web";
import { QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { pickBootLanguage } from "../providers/boot-screens";
import { createFakeServer, makeFixtureModules, renderApp } from "../test-utils";
import { buildRouter } from "./router";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("route guard (P17, K-11)", () => {
  test("modul nonaktif: 'Modul Tidak Aktif' tampil dan chunk halaman modul tidak dimuat", async () => {
    const app = renderApp({ path: "/alpha", statuses: { alpha: "disabled" } });
    expect(await screen.findByText("Modul Tidak Aktif")).toBeInTheDocument();
    expect(screen.getByText("Modul ini belum diaktifkan.")).toBeInTheDocument();
    expect(app.loads.alphaHome).toBe(0);
    expect(screen.queryByText("Isi Alpha Beranda")).toBeNull();
  });

  test("teks mengikuti status: dipasang tapi dimatikan, belum dipasang, sementara tidak tersedia", async () => {
    const a = renderApp({ path: "/alpha", statuses: { alpha: "available" } });
    expect(await screen.findByText("Modul ini belum dipasang.")).toBeInTheDocument();
    a.unmount();
    const b = renderApp({ path: "/alpha", statuses: { alpha: "upgrade_pending" } });
    expect(await screen.findByText("Modul ini sementara tidak tersedia.")).toBeInTheDocument();
    b.unmount();
  });

  test("diubah ke enabled lalu bootstrap di-refetch: halaman pulih tanpa reload", async () => {
    const app = renderApp({ path: "/alpha/home", statuses: { alpha: "disabled" } });
    await screen.findByText("Modul Tidak Aktif");
    app.server.setModuleStatus("acme.alpha", "enabled");
    await app.refreshBootstrap();
    expect(await screen.findByText("Isi Alpha Beranda")).toBeInTheDocument();
    expect(screen.queryByText("Modul Tidak Aktif")).toBeNull();
    expect(app.loads.alphaHome).toBe(1);
  });

  test("di-disable saat halaman terbuka: konten diganti dan daftar Desk dimuat ulang tanpa reload", async () => {
    const app = renderApp({ path: "/alpha/home" });
    expect(await screen.findByText("Isi Alpha Beranda")).toBeInTheDocument();
    const desksBefore = app.server.requests.filter((p) => p === "/api/platform/desks").length;
    expect(desksBefore).toBeGreaterThanOrEqual(1);
    app.server.setModuleStatus("acme.alpha", "disabled");
    await app.refreshBootstrap();
    await waitFor(() => expect(screen.getByText("Modul Tidak Aktif")).toBeInTheDocument());
    expect(screen.queryByText("Isi Alpha Beranda")).toBeNull();
    await waitFor(() =>
      expect(app.server.requests.filter((p) => p === "/api/platform/desks").length).toBeGreaterThan(
        desksBefore,
      ),
    );
  });

  test("modul diaktifkan ulang: hanya bootstrap, daftar Desk, dan katalog i18n yang diminta ke server (tanpa permintaan API lain)", async () => {
    const app = renderApp({ path: "/", statuses: { alpha: "disabled" } });
    await screen.findByRole("heading", { name: "Desk" });
    expect(screen.queryByRole("link", { name: /Alpha Desk/ })).toBeNull();
    app.server.setModuleStatus("acme.alpha", "enabled");
    await app.refreshBootstrap();
    expect(await screen.findByRole("link", { name: /Alpha Desk/ })).toBeInTheDocument();
    expect(
      app.server.requests.every(
        (p) =>
          p.startsWith("/api/bootstrap") ||
          p === "/api/platform/desks" ||
          p.startsWith("/api/platform/i18n/"),
      ),
    ).toBe(true);
    // Yang tercatat server tiruan adalah permintaan katalog namespace modul (chunk JS halaman tidak lewat server tiruan).
    expect(
      app.server.requests.some((p) => p.startsWith("/api/platform/i18n/") && p.endsWith("/alpha")),
    ).toBe(true);
  });
});

describe("katalog modul nonaktif tidak tersaji (M-1, G28)", () => {
  test("reload di /alpha saat nonaktif: 'Modul Tidak Aktif' di DALAM shell (sidebar dan navbar tetap)", async () => {
    const app = renderApp({ path: "/alpha", statuses: { alpha: "disabled" } });
    expect(await screen.findByText("Modul Tidak Aktif")).toBeInTheDocument();
    expect(screen.getByRole("main")).toBeInTheDocument();
    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(screen.queryByText("Terjadi Kesalahan")).toBeNull();
    // Server meniru backend: tidak ada permintaan katalog untuk namespace modul nonaktif yang berakhir fatal.
    expect(app.server.requests).not.toContain("/api/platform/i18n/id/alpha");
  });

  test("crumb modul nonaktif memakai fallback slug dan judul dokumen tidak menampilkan kunci mentah", async () => {
    renderApp({ path: "/alpha", statuses: { alpha: "disabled" } });
    await screen.findByText("Modul Tidak Aktif");
    expect(document.title).not.toContain("::");
    expect(document.title).not.toContain("nav.home");
  });

  test("diaktifkan ulang setelah reload nonaktif: halaman dan crumb pulih dengan katalog modul", async () => {
    const app = renderApp({ path: "/alpha/home", statuses: { alpha: "disabled" } });
    await screen.findByText("Modul Tidak Aktif");
    app.server.setModuleStatus("acme.alpha", "enabled");
    await app.refreshBootstrap();
    expect(await screen.findByText("Isi Alpha Beranda")).toBeInTheDocument();
    await waitFor(() => expect(document.title).toBe("Alpha Beranda - ERPora"));
  });
});

describe("404 dan modul tak ada di bootstrap", () => {
  test("modul frontend yang tidak ada di bootstrap: alamatnya 404 dan sidebar tetap", async () => {
    renderApp({ path: "/ghost" });
    expect(await screen.findByText("Halaman Tidak Ditemukan")).toBeInTheDocument();
    expect(screen.getByRole("main")).toBeInTheDocument();
  });

  test("alamat tak dikenal: 404 di dalam kerangka (sidebar dan navbar tetap) dengan tautan ke Beranda", async () => {
    renderApp({ path: "/tidak/ada" });
    expect(await screen.findByText("Halaman Tidak Ditemukan")).toBeInTheDocument();
    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Kembali ke Beranda" })).toHaveAttribute("href", "/");
  });
});

describe("galat halaman (R14.5)", () => {
  test("galat render di halaman modul: kerangka bertahan dan RouteErrorPage tampil dengan Coba Lagi", async () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      renderApp({ path: "/alpha/boom" });
      expect(await screen.findByText("Terjadi Kesalahan")).toBeInTheDocument();
      expect(screen.getByRole("banner")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Coba lagi" })).toBeInTheDocument();
    } finally {
      err.mockRestore();
    }
  });
});

describe("judul, pengumuman, dan fokus (K-33)", () => {
  test("document.title mengikuti crumb terakhir dengan akhiran nama aplikasi", async () => {
    renderApp({ path: "/alpha/home" });
    await screen.findByText("Isi Alpha Beranda");
    await waitFor(() => expect(document.title).toBe("Alpha Beranda - ERPora"));
  });

  test("tautan 'Lewati ke konten' ada pertama di DOM dan memindahkan fokus ke <main>", async () => {
    const user = userEvent.setup();
    renderApp();
    const skip = await screen.findByRole("link", { name: "Lewati ke konten" });
    expect(document.body.querySelector("button,a,[tabindex]")).toBe(skip);
    expect(skip.getAttribute("href")).toBe("#content");
    await user.click(skip);
    expect(document.activeElement).toBe(screen.getByRole("main"));
  });

  test("pemuatan awal tidak memindahkan fokus; navigasi berikutnya mengumumkan halaman dan memfokuskan <main>", async () => {
    const _user = userEvent.setup();
    const app = renderApp({ path: "/alpha/home" });
    await screen.findByText("Isi Alpha Beranda");
    expect(document.activeElement).not.toBe(screen.getByRole("main"));
    expect(screen.getByTestId("route-announcer")).toHaveTextContent("");
    await act(async () => {
      app.history.push("/alpha/reports/sales");
    });
    await waitFor(() =>
      expect(screen.getByTestId("route-announcer")).toHaveTextContent("Penjualan - ERPora"),
    );
    expect(document.activeElement).toBe(screen.getByRole("main"));
  });

  test("area konten bergulir secara bawaan", async () => {
    renderApp({ path: "/alpha/home" });
    await screen.findByText("Isi Alpha Beranda");
    expect(screen.getByRole("main").className).toContain("overflow-auto");
  });
});

describe("bootstrap gagal (PK-4)", () => {
  test("layar galat dengan kamus bawaan dan tombol Coba Lagi yang memuat ulang sampai berhasil", async () => {
    const user = userEvent.setup();
    Object.defineProperty(navigator, "languages", { value: ["en-US"], configurable: true });
    const failing = createFakeServer([]);
    failing.failBootstrap = true;
    renderApp({ server: failing });
    expect(await screen.findByText("The Application Could Not Be Loaded")).toBeInTheDocument();
    failing.failBootstrap = false;
    await user.click(screen.getByRole("button", { name: "Try Again" }));
    expect(await screen.findByRole("heading", { name: "Desks" })).toBeInTheDocument();
  });

  test("pickBootLanguage: id untuk bahasa Indonesia atau kosong, selain itu Inggris", () => {
    expect(pickBootLanguage([])).toBe("id");
    expect(pickBootLanguage(["id-ID"])).toBe("id");
    expect(pickBootLanguage(["en-US", "id"])).toBe("en");
    expect(pickBootLanguage(["fr"])).toBe("en");
  });
});

describe("tabel rute revisi M3a (R10)", () => {
  test("/ dengan Desk tersimpan: diarahkan ke Dashboard Desk itu (replace); indeks /<slug> adalah Dashboard", async () => {
    sessionStorage.setItem("erpora.desk.tab", "alpha");
    const app = renderApp({ path: "/" });
    expect(await screen.findByRole("region", { name: "Widget Dashboard" })).toBeInTheDocument();
    expect(app.history.location.pathname).toBe("/alpha");
    expect(screen.getByRole("heading", { name: "Alpha Desk" })).toBeInTheDocument();
    expect(app.history.length).toBe(1);
  });

  test("/ tanpa Desk valid: halaman Desks dengan kartu Desk yang diizinkan", async () => {
    renderApp({ path: "/" });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Alpha Desk/ })).toHaveAttribute("href", "/alpha");
    expect(screen.getByRole("link", { name: /Platform/ })).toHaveAttribute("href", "/platform");
  });

  test("/platform adalah Dashboard Desk Platform; /platform/todo placeholder; /platform/modules dapat dibuka", async () => {
    const a = renderApp({ path: "/platform" });
    expect(await screen.findByRole("heading", { name: "Platform" })).toBeInTheDocument();
    a.unmount();
    const b = renderApp({ path: "/platform/todo" });
    expect(
      await screen.findByText("Daftar tugas akan hadir pada pembaruan berikutnya."),
    ).toBeInTheDocument();
    b.unmount();
    renderApp({ path: "/platform/modules" });
    await waitFor(() => expect(document.title).toBe("Modul - ERPora"));
  });

  test("/settings/modules dan /desks tidak terdaftar: 404 tanpa pengalihan", async () => {
    const a = renderApp({ path: "/settings/modules" });
    expect(await screen.findByText("Halaman Tidak Ditemukan")).toBeInTheDocument();
    expect(a.history.location.pathname).toBe("/settings/modules");
    a.unmount();
    const b = renderApp({ path: "/desks" });
    expect(await screen.findByText("Halaman Tidak Ditemukan")).toBeInTheDocument();
    expect(b.history.location.pathname).toBe("/desks");
  });

  test("modul yang mendaftarkan rute indeks ditolak saat router dibangun", () => {
    const modules = makeFixtureModules({ alphaHome: 0, alphaSales: 0, beta: 0 }).map((m) =>
      m.slug === "alpha"
        ? {
            ...m,
            registerRoutes: (base: ModuleBaseRoute) => [
              createRoute({ getParentRoute: () => base, path: "/" }),
            ],
          }
        : m,
    );
    const bootstrap = makeBootstrap({
      modules: [makeBootstrapModule("acme.alpha", "alpha", "enabled")],
    });
    const { resolved } = resolveModules(modules, bootstrap, { warn: () => {} });
    expect(() => buildRouter({ modules: resolved, queryClient: new QueryClient() })).toThrow(
      ModuleDefinitionError,
    );
  });
});

describe("validasi Desk pada perpindahan halaman (R9.3)", () => {
  test("membuka halaman milik Desk lain mengoreksi Desk saat ini tanpa muat ulang", async () => {
    sessionStorage.setItem("erpora.desk.tab", "beta");
    const app = renderApp({ path: "/alpha/home" });
    expect(await screen.findByText("Isi Alpha Beranda")).toBeInTheDocument();
    await waitFor(() => expect(sessionStorage.getItem("erpora.desk.tab")).toBe("alpha"));
    expect(localStorage.getItem("erpora.desk")).toBe("alpha");
    expect(app.history.location.pathname).toBe("/alpha/home");
  });

  test("halaman yang tidak masuk Desk mana pun terbuka tanpa mengubah Desk tersimpan", async () => {
    sessionStorage.setItem("erpora.desk.tab", "beta");
    renderApp({ path: "/alpha/deep" });
    expect(await screen.findByText("Isi Dalam")).toBeInTheDocument();
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("beta");
  });
});
