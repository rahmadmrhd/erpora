import { beforeEach, describe, expect, spyOn, test } from "bun:test";
import { makeBootstrapModule } from "@erpora/platform-web";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFakeServer, renderApp } from "../test-utils";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

function serverFailing(ns: string) {
  const server = createFakeServer([
    makeBootstrapModule("acme.alpha", "alpha", "enabled"),
    makeBootstrapModule("acme.beta", "beta", "enabled"),
  ]);
  server.failNamespaces.add(ns);
  return server;
}

describe("kegagalan memuat katalog di Dock Pinned terisolasi", () => {
  test("katalog satu Desk 500: ikon itu menjadi galat kecil dengan Coba lagi, shell dan ikon lain tetap hidup", async () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    const warn = spyOn(console, "warn").mockImplementation(() => {});
    const debug = spyOn(console, "debug").mockImplementation(() => {});
    try {
      localStorage.setItem("erpora.dock", "pinned");
      const server = serverFailing("alpha");
      const user = userEvent.setup();
      renderApp({ path: "/platform", server });
      const dock = await screen.findByRole("navigation", { name: "Dock Desk" }, { timeout: 5000 });
      const failed = await within(dock).findByRole(
        "button",
        { name: /tidak dapat dimuat/ },
        { timeout: 5000 },
      );
      // Shell tetap hidup: konten, ikon Desk lain, dan item tetap.
      expect(screen.getByRole("main")).toBeInTheDocument();
      expect(within(dock).getByRole("button", { name: "Platform" })).toBeInTheDocument();
      expect(within(dock).getByRole("button", { name: "Beta Desk" })).toBeInTheDocument();
      expect(within(dock).getByRole("button", { name: /Cari/ })).toBeInTheDocument();
      expect(screen.queryByText(/Something Went Wrong|Terjadi Kesalahan/)).toBeNull();

      // Coba lagi: setelah server pulih, ikon Desk kembali.
      server.failNamespaces.delete("alpha");
      await user.click(failed);
      expect(await within(dock).findByRole("button", { name: "Alpha Desk" })).toBeInTheDocument();
    } finally {
      err.mockRestore();
      warn.mockRestore();
      debug.mockRestore();
    }
  }, 15_000);

  test("katalog gagal untuk Desk saat ini: panel menu menampilkan galat lokal, bukan merobohkan aplikasi", async () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    const warn = spyOn(console, "warn").mockImplementation(() => {});
    const debug = spyOn(console, "debug").mockImplementation(() => {});
    try {
      localStorage.setItem("erpora.dock", "pinned");
      sessionStorage.setItem("erpora.desk.tab", "alpha");
      renderApp({ path: "/platform", server: serverFailing("alpha") });
      const nav = await screen.findByRole(
        "navigation",
        { name: "Navigasi utama" },
        { timeout: 5000 },
      );
      expect(await within(nav).findByText("Menu Desk gagal dimuat.")).toBeInTheDocument();
      expect(screen.getByRole("navigation", { name: "Dock Desk" })).toBeInTheDocument();
      expect(screen.getByRole("main")).toBeInTheDocument();
    } finally {
      err.mockRestore();
      warn.mockRestore();
      debug.mockRestore();
    }
  }, 15_000);
});
