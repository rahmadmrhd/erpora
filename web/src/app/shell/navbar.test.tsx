import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, test } from "bun:test";
import { act, configure, getConfig, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";
import { installLayoutMock, type LayoutMock } from "./navbar-test-support";

let layout: LayoutMock | null = null;

// Pemuatan awal (modul, katalog i18n) lambat saat mesin sibuk: batas findBy/waitFor 1 dtk membuat tes ini tak stabil.
let previousTimeout = 1000;
beforeAll(() => {
  previousTimeout = getConfig().asyncUtilTimeout;
  configure({ asyncUtilTimeout: 4000 });
});
afterAll(() => configure({ asyncUtilTimeout: previousTimeout }));

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

  test("klik Home membuka halaman Desks tanpa resolve; penanda showDesks dikonsumsi dan tidak tersisa di riwayat", async () => {
    const user = userEvent.setup();
    const { history } = renderApp({ path: "/alpha/home" });
    const bc = await breadcrumb();
    await user.click(await within(bc).findByRole("button", { name: "Beranda" }));
    // Halaman Desks tampil di "/" dan penentuan Desk dilewati: Desk tersimpan (alpha) tidak mengalihkan ke /alpha.
    expect(await screen.findByRole("link", { name: "Alpha Desk" })).toBeInTheDocument(); // kartu Desk
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("alpha");
    await new Promise((r) => setTimeout(r, 100));
    expect(history.location.pathname).toBe("/");
    // R10.5: DeskEntry mengonsumsi penanda showDesks dan menghapusnya dari entri riwayat setelah menampilkan Desks.
    await waitFor(() => expect(history.location.state).not.toMatchObject({ showDesks: true }));
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

describe("Navbar: pembatas toggle sidebar dan breadcrumb (K5)", () => {
  test("Separator vertikal ada tepat di antara tombol collapse dan breadcrumb, bertinggi pas (bukan h-full)", async () => {
    renderApp({ path: "/alpha/home" });
    const header = await screen.findByRole("banner");
    await breadcrumb();
    const leading = header.querySelector('[data-slot="navbar-leading"]');
    expect(leading).not.toBeNull();
    const separator = leading?.querySelector('[data-slot="separator"]');
    expect(separator).not.toBeNull();
    expect(separator).toHaveAttribute("data-orientation", "vertical");
    // Tinggi harus dipasang pada varian orientasi (data-[orientation=vertical]:h-*) agar menggantikan h-full bawaan
    // shadcn; `h-4` polos kalah spesifisitas sehingga garis runtuh menjadi 0 px di dalam flex berinduk tak berukuran.
    expect(separator?.className).toContain("data-[orientation=vertical]:h-5");
    expect(separator?.className.split(/\s+/)).not.toContain("h-4");
    // Urutan DOM: tombol collapse, pembatas, lalu breadcrumb.
    const trigger = within(header).getByRole("button", { name: "Ciutkan atau perluas sidebar" });
    const nav = header.querySelector('nav[data-slot="breadcrumb"]');
    expect(
      trigger.compareDocumentPosition(separator as Element) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      (separator as Element).compareDocumentPosition(nav as Element) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});

describe("Navbar: judul halaman = breadcrumb terakhir (K9)", () => {
  test("breadcrumb terakhir dibungkus h1 tunggal, lebih besar dan tebal, tetap aria-current=page", async () => {
    renderApp({ path: "/alpha/reports/sales" });
    const bc = await breadcrumb();
    await within(bc).findByText("Penjualan");
    const headings = within(screen.getByRole("banner")).getAllByRole("heading", { level: 1 });
    expect(headings).toHaveLength(1);
    const page = within(headings[0] as HTMLElement).getByText("Penjualan");
    expect(page).toHaveAttribute("aria-current", "page");
    expect(page.dataset.slot).toBe("breadcrumb-page");
    expect(page.className).toContain("text-base");
    expect(page.className).toContain("font-semibold");
    expect(page.className).not.toContain("font-normal");
    // Langkah sebelumnya tidak ikut menjadi heading.
    expect(within(bc).getAllByRole("heading")).toHaveLength(1);
  });

  test("terpotong: tetap di dalam h1, elipsis teks dan tooltip judul penuh", async () => {
    layout = installLayoutMock({ navWidth: 60 });
    renderApp({ path: "/alpha/reports/sales" });
    await breadcrumb();
    const page = await waitFor(() => {
      const el = document.querySelector('header h1 [aria-current="page"]') as HTMLElement | null;
      expect(el?.className).toContain("truncate");
      return el as HTMLElement;
    });
    expect(page.className).toContain("font-semibold");
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
