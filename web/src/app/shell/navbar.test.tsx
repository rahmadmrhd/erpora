import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";
import { installLayoutMock, type LayoutMock } from "./navbar-test-support";

let layout: LayoutMock | null = null;

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

afterEach(() => {
  layout?.restore();
  layout = null;
  localStorage.clear();
  sessionStorage.clear();
});

async function breadcrumb() {
  return await screen.findByRole("navigation", { name: "Jejak halaman" });
}

describe("Navbar: breadcrumb Home › Desk › halaman", () => {
  test("banner lengket; Home, Desk dari penentuan Desk, dan halaman saat ini (bukan tautan)", async () => {
    renderApp({ path: "/alpha/home" });
    const bc = await breadcrumb();
    const header = screen.getByRole("banner");
    expect(header.className).toContain("sticky");
    expect(header.contains(bc)).toBe(true);
    await within(bc).findByText("Alpha Beranda");
    expect(within(bc).getByRole("button", { name: "Beranda" })).toBeInTheDocument();
    expect(within(bc).getByRole("button", { name: "Alpha Desk" })).toBeInTheDocument();
    const current = within(bc).getByText("Alpha Beranda");
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current.closest("a")).toBeNull();
  });

  test("dua langkah halaman: langkah awal teks, halaman aktif terakhir", async () => {
    renderApp({ path: "/alpha/reports/sales" });
    const bc = await breadcrumb();
    await within(bc).findByText("Penjualan");
    expect(within(bc).getByText("Laporan")).toBeInTheDocument();
    expect(within(bc).getByText("Penjualan")).toHaveAttribute("aria-current", "page");
    expect(within(bc).queryByRole("link", { name: "Laporan" })).toBeNull();
  });

  test("klik Desk memilih Desk itu dan membuka Dashboard-nya", async () => {
    const user = userEvent.setup();
    const { history } = renderApp({ path: "/alpha/reports/sales" });
    const bc = await breadcrumb();
    await user.click(await within(bc).findByRole("button", { name: "Alpha Desk" }));
    await waitFor(() => expect(history.location.pathname).toBe("/alpha"));
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("alpha");
  });

  test("klik Home membuka halaman Desks tanpa resolve (state showDesks, bukan URL)", async () => {
    const user = userEvent.setup();
    const { history } = renderApp({ path: "/alpha/home" });
    const bc = await breadcrumb();
    await user.click(await within(bc).findByRole("button", { name: "Beranda" }));
    await waitFor(() => expect(history.location.pathname).toBe("/"));
    expect(history.location.state).toMatchObject({ showDesks: true });
  });

  test("halaman yang tidak masuk Desk mana pun: Home › halaman tanpa segmen Desk (R9.6)", async () => {
    renderApp({ path: "/alpha/deep" });
    const bc = await breadcrumb();
    await within(bc).findByText("Dalam");
    expect(within(bc).getByRole("button", { name: "Beranda" })).toBeInTheDocument();
    expect(within(bc).queryByRole("button", { name: "Alpha Desk" })).toBeNull();
  });

  test("lebih dari tiga tingkat tetap utuh selama ruang cukup", async () => {
    renderApp({ path: "/alpha/deep" });
    const bc = await breadcrumb();
    await within(bc).findByText("Dalam");
    for (const t of ["Tingkat 1", "Tingkat 2", "Tingkat 3"]) {
      expect(within(bc).getByText(t)).toBeInTheDocument();
    }
    expect(within(bc).queryByRole("button", { name: "Tampilkan jalur lengkap" })).toBeNull();
  });
});

describe("Navbar: tombol collapse bawaan (leading)", () => {
  test("tombol collapse berlabel ada di navbar dan petunjuk Ctrl B muncul saat fokus", async () => {
    const user = userEvent.setup();
    renderApp();
    const header = await screen.findByRole("banner");
    const button = await within(header).findByRole("button", {
      name: "Ciutkan atau perluas sidebar",
    });
    await user.tab();
    button.focus();
    await waitFor(() => expect(screen.getAllByText("Ctrl").length).toBeGreaterThan(0));
  });
});

describe("Navbar: pengukuran lebar dan aksi halaman (ResizeObserver)", () => {
  // /alpha/home: Home 16 + "Alpha Desk" 80 + "Alpha Beranda" 104 + 2 pemisah 34 = 268;
  // aksi 136 + 136 + 8 = 280 (k=0) atau 136 + 36 + 8 = 180 (k=1); leading 36 + jarak 10.
  const PATH = "/alpha/home";

  test("lebar lega: semua aksi sebagai tombol dan breadcrumb utuh", async () => {
    layout = installLayoutMock({ navWidth: 1280 });
    renderApp({ path: PATH });
    expect(await screen.findByRole("button", { name: "Tambah Alpha" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ekspor Alpha" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aksi lainnya" })).toBeNull();
    const bc = await breadcrumb();
    expect(within(bc).getByRole("button", { name: "Alpha Desk" })).toBeInTheDocument();
  });

  test("ruang menyempit: aksi sekunder masuk menu ⋯ lebih dulu, breadcrumb tetap utuh; aksi utama tetap tampil", async () => {
    const user = userEvent.setup();
    layout = installLayoutMock({ navWidth: 550 });
    renderApp({ path: PATH });
    expect(await screen.findByRole("button", { name: "Tambah Alpha" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ekspor Alpha" })).toBeNull();
    const bc = await breadcrumb();
    expect(within(bc).getByRole("button", { name: "Alpha Desk" })).toBeInTheDocument();
    expect(within(bc).queryByRole("button", { name: "Tampilkan jalur lengkap" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Aksi lainnya" }));
    expect(await screen.findByRole("menuitem", { name: "Ekspor Alpha" })).toBeInTheDocument();
  });

  test("lebih sempit lagi: baru breadcrumb diringkas (Home, elipsis, halaman)", async () => {
    layout = installLayoutMock({ navWidth: 500 });
    renderApp({ path: PATH });
    expect(await screen.findByRole("button", { name: "Tambah Alpha" })).toBeInTheDocument();
    const bc = await breadcrumb();
    await waitFor(() =>
      expect(
        within(bc).getByRole("button", { name: "Tampilkan jalur lengkap" }),
      ).toBeInTheDocument(),
    );
    expect(within(bc).queryByRole("button", { name: "Alpha Desk" })).toBeNull();
    expect(within(bc).getByRole("button", { name: "Beranda" })).toBeInTheDocument();
    expect(within(bc).getByText("Alpha Beranda")).toHaveAttribute("aria-current", "page");
  });

  test("navbar < 480 px: aksi utama berupa tombol ikon bernama, sekunder di ⋯, breadcrumb `… › halaman`, elipsis membuka lembar bawah", async () => {
    const user = userEvent.setup();
    layout = installLayoutMock({ navWidth: 400 });
    renderApp({ path: PATH });
    const primary = await screen.findByRole("button", { name: "Tambah Alpha" });
    expect(primary.textContent).toBe(""); // hanya ikon
    expect(screen.queryByRole("button", { name: "Ekspor Alpha" })).toBeNull();
    const bc = await breadcrumb();
    expect(within(bc).queryByRole("button", { name: "Beranda" })).toBeNull();
    expect(within(bc).getByText("Alpha Beranda")).toHaveAttribute("aria-current", "page");
    await user.click(within(bc).getByRole("button", { name: "Tampilkan jalur lengkap" }));
    const sheet = await screen.findByRole("dialog", { name: "Jalur halaman" });
    expect(within(sheet).getByRole("button", { name: "Alpha Desk" })).toBeInTheDocument();
    expect(within(sheet).getByRole("button", { name: "Beranda" })).toBeInTheDocument();
  });

  test("perubahan ukuran navbar (ResizeObserver) menata ulang tanpa muat ulang", async () => {
    layout = installLayoutMock({ navWidth: 1280 });
    renderApp({ path: PATH });
    expect(await screen.findByRole("button", { name: "Ekspor Alpha" })).toBeInTheDocument();
    act(() => {
      layout?.setNavWidth(550);
      layout?.fireResize();
    });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Ekspor Alpha" })).toBeNull());
    expect(screen.getByRole("button", { name: "Aksi lainnya" })).toBeInTheDocument();
    act(() => {
      layout?.setNavWidth(1280);
      layout?.fireResize();
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Ekspor Alpha" })).toBeInTheDocument(),
    );
  });

  test("aksi dibersihkan saat berpindah ke halaman yang tidak mendeklarasikannya", async () => {
    const user = userEvent.setup();
    layout = installLayoutMock({ navWidth: 1280 });
    renderApp({ path: PATH });
    await screen.findByRole("button", { name: "Tambah Alpha" });
    // pindah lewat riwayat: Dashboard Desk tidak mendeklarasikan aksi
    const bc = await breadcrumb();
    await user.click(within(bc).getByRole("button", { name: "Alpha Desk" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Tambah Alpha" })).toBeNull());
  });
});
