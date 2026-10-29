import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { pickBootLanguage } from "../providers/boot-screens";
import { createFakeServer, renderApp } from "../test-utils";

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

afterEach(() => {
  localStorage.clear();
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
    const app = renderApp({ path: "/alpha", statuses: { alpha: "disabled" } });
    await screen.findByText("Modul Tidak Aktif");
    app.server.setModuleStatus("acme.alpha", "enabled");
    await app.refreshBootstrap();
    expect(await screen.findByText("Isi Alpha Beranda")).toBeInTheDocument();
    expect(screen.queryByText("Modul Tidak Aktif")).toBeNull();
    expect(app.loads.alphaHome).toBe(1);
  });

  test("di-disable saat halaman terbuka: konten diganti dan menu hilang tanpa reload", async () => {
    const app = renderApp({ path: "/alpha" });
    expect(await screen.findByText("Isi Alpha Beranda")).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Navigasi utama" });
    expect(within(nav).getByRole("link", { name: "Alpha Beranda" })).toBeInTheDocument();
    app.server.setModuleStatus("acme.alpha", "disabled");
    await app.refreshBootstrap();
    await waitFor(() => expect(screen.getByText("Modul Tidak Aktif")).toBeInTheDocument());
    expect(screen.queryByText("Isi Alpha Beranda")).toBeNull();
    expect(within(nav).queryByRole("link", { name: "Alpha Beranda" })).toBeNull();
  });

  test("modul diaktifkan ulang: hanya bootstrap, katalog, dan chunk halaman yang diminta (tanpa permintaan API lain)", async () => {
    const app = renderApp({ path: "/", statuses: { alpha: "disabled" } });
    await screen.findByRole("navigation", { name: "Navigasi utama" });
    app.server.setModuleStatus("acme.alpha", "enabled");
    await app.refreshBootstrap();
    await waitFor(() =>
      expect(screen.getByRole("navigation", { name: "Navigasi utama" }).textContent).toContain(
        "Alpha Beranda",
      ),
    );
    expect(
      app.server.requests.every(
        (p) => p.startsWith("/api/bootstrap") || p.startsWith("/api/platform/i18n/"),
      ),
    ).toBe(true);
    expect(app.server.requests.some((p) => p.endsWith("/alpha"))).toBe(true);
  });
});

describe("katalog modul nonaktif tidak tersaji (M-1, G28)", () => {
  test("reload di /alpha saat nonaktif: 'Modul Tidak Aktif' di DALAM shell (sidebar dan navbar tetap)", async () => {
    const app = renderApp({ path: "/alpha", statuses: { alpha: "disabled" } });
    expect(await screen.findByText("Modul Tidak Aktif")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
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
    const app = renderApp({ path: "/alpha", statuses: { alpha: "disabled" } });
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
    expect(screen.getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
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
      expect(screen.getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
      expect(screen.getByRole("banner")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Coba lagi" })).toBeInTheDocument();
    } finally {
      err.mockRestore();
    }
  });
});

describe("judul, pengumuman, dan fokus (K-33)", () => {
  test("document.title mengikuti crumb terakhir dengan akhiran nama aplikasi", async () => {
    renderApp({ path: "/alpha" });
    await screen.findByText("Isi Alpha Beranda");
    await waitFor(() => expect(document.title).toBe("Alpha Beranda - ERPora"));
  });

  test("tautan 'Lewati ke konten' ada pertama di DOM dan memindahkan fokus ke <main>", async () => {
    const user = userEvent.setup();
    renderApp();
    await screen.findByRole("navigation", { name: "Navigasi utama" });
    const skip = screen.getByRole("link", { name: "Lewati ke konten" });
    expect(document.body.querySelector("button,a,[tabindex]")).toBe(skip);
    expect(skip.getAttribute("href")).toBe("#content");
    await user.click(skip);
    expect(document.activeElement).toBe(screen.getByRole("main"));
  });

  test("pemuatan awal tidak memindahkan fokus; navigasi berikutnya mengumumkan halaman dan memfokuskan <main>", async () => {
    const user = userEvent.setup();
    renderApp({ path: "/alpha" });
    await screen.findByText("Isi Alpha Beranda");
    expect(document.activeElement).not.toBe(screen.getByRole("main"));
    expect(screen.getByTestId("route-announcer")).toHaveTextContent("");
    const nav = screen.getByRole("navigation", { name: "Navigasi utama" });
    await user.click(within(nav).getByRole("link", { name: "Beranda" }));
    await waitFor(() =>
      expect(screen.getByTestId("route-announcer")).toHaveTextContent("Beranda - ERPora"),
    );
    expect(document.activeElement).toBe(screen.getByRole("main"));
  });

  test("area konten bergulir secara bawaan", async () => {
    renderApp({ path: "/alpha" });
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
    expect(await screen.findByRole("navigation", { name: "Main navigation" })).toBeInTheDocument();
  });

  test("pickBootLanguage: id untuk bahasa Indonesia atau kosong, selain itu Inggris", () => {
    expect(pickBootLanguage([])).toBe("id");
    expect(pickBootLanguage(["id-ID"])).toBe("id");
    expect(pickBootLanguage(["en-US", "id"])).toBe("en");
    expect(pickBootLanguage(["fr"])).toBe("en");
  });
});
