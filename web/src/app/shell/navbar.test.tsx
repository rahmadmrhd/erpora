import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";

const originalMatchMedia = window.matchMedia;

function mockViewport(mobile: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: mobile && query.includes("767"),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
}

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
  mockViewport(false);
});

afterEach(() => {
  window.matchMedia = originalMatchMedia;
  localStorage.clear();
});

async function breadcrumb() {
  return await screen.findByRole("navigation", { name: "Jejak halaman" });
}

describe("Breadcrumbs (P4)", () => {
  test("satu tingkat: hanya halaman aktif sebagai teks (bukan tautan)", async () => {
    renderApp({ path: "/alpha" });
    const bc = await breadcrumb();
    await within(bc).findByText("Alpha Beranda");
    expect(within(bc).queryAllByRole("link")).toHaveLength(0);
    expect(within(bc).getByText("Alpha Beranda")).toHaveAttribute("aria-current", "page");
  });

  test("dua tingkat: tingkat awal tanpa halaman sendiri berupa teks, halaman aktif terakhir", async () => {
    renderApp({ path: "/alpha/reports/sales" });
    const bc = await breadcrumb();
    await within(bc).findByText("Penjualan");
    expect(within(bc).getByText("Laporan")).toBeInTheDocument();
    expect(within(bc).getByText("Penjualan")).toHaveAttribute("aria-current", "page");
    expect(within(bc).queryByRole("link", { name: "Laporan" })).toBeNull();
  });

  test("lebih dari tiga tingkat: pertama, elipsis, terakhir; elipsis membuka tingkat yang dilipat", async () => {
    const user = userEvent.setup();
    renderApp({ path: "/alpha/deep" });
    const bc = await breadcrumb();
    await within(bc).findByText("Dalam");
    expect(within(bc).getByText("Tingkat 1")).toBeInTheDocument();
    expect(within(bc).queryByText("Tingkat 2")).toBeNull();
    expect(within(bc).queryByText("Tingkat 3")).toBeNull();
    await user.click(within(bc).getByRole("button", { name: "Tampilkan jalur lengkap" }));
    expect(await screen.findByRole("menuitem", { name: "Tingkat 2" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Tingkat 3" })).toBeInTheDocument();
  });

  test("layar kecil (compact): elipsis lalu halaman aktif", async () => {
    mockViewport(true);
    renderApp({ path: "/alpha/reports/sales" });
    const bc = await breadcrumb();
    await within(bc).findByText("Penjualan");
    expect(within(bc).queryByText("Laporan")).toBeNull();
    expect(within(bc).getByRole("button", { name: "Tampilkan jalur lengkap" })).toBeInTheDocument();
  });
});

describe("Navbar", () => {
  test("tombol collapse berlabel dan sticky; ada landmark banner", async () => {
    renderApp();
    const header = await screen.findByRole("banner");
    expect(header.className).toContain("sticky");
    expect(
      within(header).getByRole("button", { name: "Ciutkan atau perluas sidebar" }),
    ).toBeInTheDocument();
  });

  test("petunjuk Ctrl B muncul pada tooltip tombol collapse saat difokuskan", async () => {
    const user = userEvent.setup();
    renderApp();
    const button = await screen.findByRole("button", { name: "Ciutkan atau perluas sidebar" });
    await user.tab();
    button.focus();
    await waitFor(() => expect(screen.getAllByText("Ctrl").length).toBeGreaterThan(0));
  });
});

describe("PageActionsView", () => {
  test("layar lebar: semua aksi sebagai tombol di navbar", async () => {
    renderApp({ path: "/alpha" });
    expect(await screen.findByRole("button", { name: "Tambah Alpha" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ekspor Alpha" })).toBeInTheDocument();
  });

  test("layar kecil: aksi pinned jadi tombol ikon dan sisanya masuk menu ⋯", async () => {
    const user = userEvent.setup();
    mockViewport(true);
    renderApp({ path: "/alpha" });
    expect(await screen.findByRole("button", { name: "Tambah Alpha" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ekspor Alpha" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Aksi lainnya" }));
    expect(await screen.findByRole("menuitem", { name: "Ekspor Alpha" })).toBeInTheDocument();
  });

  test("aksi dibersihkan saat berpindah ke halaman yang tidak mendeklarasikannya", async () => {
    const user = userEvent.setup();
    renderApp({ path: "/alpha" });
    await screen.findByRole("button", { name: "Tambah Alpha" });
    const nav = screen.getByRole("navigation", { name: "Navigasi utama" });
    await user.click(within(nav).getByRole("link", { name: "Beranda" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Tambah Alpha" })).toBeNull());
  });
});
