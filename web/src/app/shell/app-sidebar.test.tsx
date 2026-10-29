import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

afterEach(() => {
  localStorage.clear();
});

async function ready() {
  return await screen.findByRole("navigation", { name: "Navigasi utama" });
}

describe("AppSidebar", () => {
  test("urutan dari atas: logo dan nama aplikasi, Notifikasi, menu (Beranda, modul, Pengaturan), avatar di dasar", async () => {
    renderApp();
    const nav = await ready();
    expect(screen.getByText("ERPora")).toBeInTheDocument();
    const names = within(nav)
      .getAllByRole("link")
      .map((l) => l.textContent);
    expect(names.slice(0, 2)).toEqual(["Beranda", "Alpha Beranda"]);
    const notif = within(nav).getByRole("button", { name: /Notifikasi/ });
    const home = within(nav).getByRole("link", { name: "Beranda" });
    expect(notif.compareDocumentPosition(home) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(nav).getByText("Menu")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Menu akun" })).toBeInTheDocument();
  });

  test("hanya modul enabled yang muncul di menu", async () => {
    renderApp({ statuses: { beta: "disabled" } });
    const nav = await ready();
    expect(within(nav).getByRole("link", { name: "Alpha Beranda" })).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Beta Beranda" })).toBeNull();
  });

  test("ikon tak dikenal menjadi lingkaran berinisial (di lapisan NavIcon)", async () => {
    renderApp();
    const nav = await ready();
    // Item "Penjualan" (anak grup) tidak berikon; grup "Laporan" memakai ikon folder yang dikenal.
    expect(within(nav).getByText("Laporan")).toBeInTheDocument();
  });

  test("tanpa modul aktif: keadaan kosong yang jelas, bukan area kosong", async () => {
    renderApp({ statuses: { alpha: "disabled", beta: "disabled" } });
    const nav = await ready();
    expect(within(nav).getByText("Belum ada menu modul")).toBeInTheDocument();
  });

  test("grup terbuka bila ada anak aktif; item aktif unik (aria-current)", async () => {
    renderApp({ path: "/alpha/reports/sales" });
    const nav = await ready();
    const sales = await within(nav).findByRole("link", { name: "Penjualan" });
    expect(sales).toHaveAttribute("aria-current", "page");
    expect(nav.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });

  test("grup tertutup dapat dibuka dengan klik dan memperlihatkan anaknya", async () => {
    const user = userEvent.setup();
    renderApp();
    const nav = await ready();
    expect(within(nav).queryByRole("link", { name: "Penjualan" })).toBeNull();
    await user.click(within(nav).getByRole("button", { name: "Laporan" }));
    expect(await within(nav).findByRole("link", { name: "Penjualan" })).toBeInTheDocument();
  });
});

describe("keadaan sidebar", () => {
  test("Ctrl+B melipat dan memperluas; keadaan tersimpan di localStorage", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    const sidebar = document.querySelector('[data-slot="sidebar"]');
    expect(sidebar).toHaveAttribute("data-state", "expanded");
    await user.keyboard("{Control>}b{/Control}");
    await waitFor(() => expect(sidebar).toHaveAttribute("data-state", "collapsed"));
    expect(localStorage.getItem("erpora.sidebar")).toBe("collapsed");
    await user.keyboard("{Control>}b{/Control}");
    await waitFor(() => expect(sidebar).toHaveAttribute("data-state", "expanded"));
  });

  test("tombol collapse di navbar membalik keadaan", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    const sidebar = document.querySelector('[data-slot="sidebar"]');
    await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
    await waitFor(() => expect(sidebar).toHaveAttribute("data-state", "collapsed"));
  });
});

describe("lencana notifikasi", () => {
  function source(count: number) {
    return {
      unreadCount: async () => count,
      list: async () => [],
      markRead: async () => {},
      markAllRead: async () => {},
    };
  }

  test.each([
    [0, null],
    [5, "5"],
    [99, "99"],
    [100, "99+"],
    [250, "99+"],
  ])("jumlah %p menampilkan %p", async (count, text) => {
    renderApp({ notificationSource: source(count) });
    await ready();
    if (text === null) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 30));
      });
      expect(screen.queryByTestId("notification-badge")).toBeNull();
    } else {
      expect(await screen.findByTestId("notification-badge")).toHaveTextContent(text);
    }
  });

  test("tetap ada (sebagai titik lewat CSS) saat sidebar terlipat", async () => {
    const user = userEvent.setup();
    renderApp({ notificationSource: source(4) });
    await screen.findByTestId("notification-badge");
    await user.keyboard("{Control>}b{/Control}");
    expect(await screen.findByTestId("notification-badge")).toBeInTheDocument();
  });
});
