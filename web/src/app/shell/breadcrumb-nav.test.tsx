import { beforeEach, describe, expect, mock, test } from "bun:test";
import { type FitPlan, fitBreadcrumb } from "@erpora/ui";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BreadcrumbNav, type BreadcrumbSegment } from "./breadcrumb-nav";
import { renderShell } from "./shell-test-support";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

function seg(key: string, text: string, extra: Partial<BreadcrumbSegment> = {}): BreadcrumbSegment {
  return { key, kind: "crumb", text, label: text, ...extra };
}

function plan(segments: BreadcrumbSegment[], available: number, mobile = false): FitPlan {
  return fitBreadcrumb({
    segments: segments.map((s) => s.key),
    widths: segments.map((s) => s.text.length * 8),
    available,
    mobile,
    separatorWidth: 34,
    ellipsisWidth: 24,
  });
}

function path(onHome = () => {}, onDesk = () => {}): BreadcrumbSegment[] {
  return [
    seg("home", "Beranda", { kind: "home", onSelect: onHome }),
    seg("desk", "Alpha Desk", { kind: "desk", onSelect: onDesk }),
    seg("a", "Laporan", { to: "/alpha/reports" }),
    seg("b", "Penjualan", { to: "/alpha/reports/sales" }),
    seg("c", "Ringkasan Bulanan", { to: "/alpha/reports/sales/summary" }),
    seg("d", "Detail"),
  ];
}

describe("BreadcrumbNav: tahap utuh", () => {
  test("Home, Desk, langkah berurutan; halaman saat ini bukan tautan dan bertanda aria-current", async () => {
    const segments = path();
    renderShell(<BreadcrumbNav segments={segments} plan={plan(segments, 10_000)} narrow={false} />);
    const nav = await screen.findByRole("navigation", { name: "Jejak halaman" });
    expect(within(nav).getByRole("button", { name: "Beranda" })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: "Alpha Desk" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Laporan" })).toHaveAttribute(
      "href",
      "/alpha/reports",
    );
    const current = within(nav).getByText("Detail");
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current.closest("a")).toBeNull();
    // utuh: tidak ada elipsis
    expect(within(nav).queryByRole("button", { name: "Tampilkan jalur lengkap" })).toBeNull();
  });

  test("Home dan Desk memanggil onSelect", async () => {
    const user = userEvent.setup();
    const onHome = mock(() => {});
    const onDesk = mock(() => {});
    const segments = path(onHome, onDesk);
    renderShell(<BreadcrumbNav segments={segments} plan={plan(segments, 10_000)} narrow={false} />);
    await user.click(await screen.findByRole("button", { name: "Beranda" }));
    await user.click(screen.getByRole("button", { name: "Alpha Desk" }));
    expect(onHome).toHaveBeenCalledTimes(1);
    expect(onDesk).toHaveBeenCalledTimes(1);
  });

  test("klik langkah halaman menavigasi lewat router", async () => {
    const user = userEvent.setup();
    const segments = path();
    const { router } = renderShell(
      <BreadcrumbNav segments={segments} plan={plan(segments, 10_000)} narrow={false} />,
    );
    await user.click(await screen.findByRole("link", { name: "Penjualan" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/alpha/reports/sales"));
  });

  test("tanpa segmen: tidak merender apa pun", async () => {
    renderShell(<BreadcrumbNav segments={[]} plan={plan([], 100)} narrow={false} />);
    await new Promise((r) => setTimeout(r, 50));
    expect(document.querySelector("nav")).toBeNull();
  });
});

describe("BreadcrumbNav: tahap diringkas", () => {
  test("Opsi A: Home, Desk, elipsis, dua langkah terakhir", async () => {
    const segments = path();
    // utuh = 6 segmen; sempit sedikit
    const p = plan(segments, 500);
    expect(p.stage).toBe("optionA");
    renderShell(<BreadcrumbNav segments={segments} plan={p} narrow={false} />);
    const nav = await screen.findByRole("navigation", { name: "Jejak halaman" });
    expect(within(nav).getByRole("button", { name: "Beranda" })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: "Alpha Desk" })).toBeInTheDocument();
    expect(within(nav).queryByText("Laporan")).toBeNull();
    expect(within(nav).queryByText("Penjualan")).toBeNull();
    expect(within(nav).getByRole("link", { name: "Ringkasan Bulanan" })).toBeInTheDocument();
    expect(within(nav).getByText("Detail")).toHaveAttribute("aria-current", "page");
    expect(
      within(nav).getByRole("button", { name: "Tampilkan jalur lengkap" }),
    ).toBeInTheDocument();
  });

  test("elipsis (>= 480 px) membuka dropdown jalur lengkap; item bertautan menavigasi", async () => {
    const user = userEvent.setup();
    const segments = path();
    const { router } = renderShell(
      <BreadcrumbNav segments={segments} plan={plan(segments, 500)} narrow={false} />,
    );
    await user.click(await screen.findByRole("button", { name: "Tampilkan jalur lengkap" }));
    const menu = await screen.findByRole("menu");
    const items = within(menu).getAllByRole("menuitem");
    expect(items.map((i) => i.textContent)).toEqual([
      "Beranda",
      "Alpha Desk",
      "Laporan",
      "Penjualan",
      "Ringkasan Bulanan",
      "Detail",
    ]);
    // halaman saat ini: tidak aktif dan bertanda
    expect(within(menu).getByRole("menuitem", { name: "Detail" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await user.click(within(menu).getByRole("menuitem", { name: "Laporan" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/alpha/reports"));
  });

  test("item Home dan Desk di dropdown memanggil onSelect", async () => {
    const user = userEvent.setup();
    const onHome = mock(() => {});
    const segments = path(onHome);
    renderShell(<BreadcrumbNav segments={segments} plan={plan(segments, 500)} narrow={false} />);
    await user.click(await screen.findByRole("button", { name: "Tampilkan jalur lengkap" }));
    await user.click(await screen.findByRole("menuitem", { name: "Beranda" }));
    expect(onHome).toHaveBeenCalledTimes(1);
  });

  test("Opsi B: Desk ikut terlipat, Home dan dua langkah terakhir terlihat", async () => {
    const segments = path();
    const p = plan(segments, 400);
    expect(p.stage).toBe("optionB");
    renderShell(<BreadcrumbNav segments={segments} plan={p} narrow={false} />);
    const nav = await screen.findByRole("navigation", { name: "Jejak halaman" });
    expect(within(nav).getByRole("button", { name: "Beranda" })).toBeInTheDocument();
    expect(within(nav).queryByRole("button", { name: "Alpha Desk" })).toBeNull();
    expect(within(nav).getByRole("link", { name: "Ringkasan Bulanan" })).toBeInTheDocument();
  });

  test("tahap terpotong: halaman saat ini diberi lebar maksimum, elipsis teks, dan tooltip judul penuh", async () => {
    const user = userEvent.setup();
    const segments = path();
    const p = plan(segments, 70);
    expect(p.stage).toBe("truncated");
    renderShell(<BreadcrumbNav segments={segments} plan={p} narrow={false} />);
    const current = await screen.findByText("Detail");
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current.className).toContain("truncate");
    expect(current.style.maxWidth).toBe(`${p.lastMaxWidth}px`);
    await user.hover(current);
    expect((await screen.findAllByText("Detail")).length).toBeGreaterThan(1); // isi tooltip
  });
});

describe("BreadcrumbNav: navbar sempit (lembar bawah)", () => {
  test("elipsis membuka lembar bawah berjudul 'Jalur halaman' berisi jalur lengkap; tautan menutup lembar dan menavigasi", async () => {
    const user = userEvent.setup();
    const segments = path();
    const p = plan(segments, 10_000, true);
    expect(p.stage).toBe("lastOnly");
    const { router } = renderShell(<BreadcrumbNav segments={segments} plan={p} narrow />);
    const nav = await screen.findByRole("navigation", { name: "Jejak halaman" });
    expect(within(nav).queryByRole("button", { name: "Beranda" })).toBeNull(); // Home di dalam elipsis
    expect(within(nav).getByText("Detail")).toHaveAttribute("aria-current", "page");
    await user.click(within(nav).getByRole("button", { name: "Tampilkan jalur lengkap" }));
    const sheet = await screen.findByRole("dialog", { name: "Jalur halaman" });
    expect(sheet).toHaveAttribute("data-slot", "sheet-content");
    expect(sheet.className).toContain("inset-x-0"); // sisi bawah
    const list = within(sheet).getByRole("list");
    expect(within(list).getAllByRole("listitem")).toHaveLength(6);
    expect(within(list).getByText("Detail")).toHaveAttribute("aria-current", "page");
    await user.click(within(sheet).getByRole("link", { name: "Penjualan" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/alpha/reports/sales"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("Home di lembar bawah memanggil onSelect dan menutup lembar; Escape menutup dan fokus kembali ke pemicu", async () => {
    const user = userEvent.setup();
    const onHome = mock(() => {});
    const segments = path(onHome);
    renderShell(<BreadcrumbNav segments={segments} plan={plan(segments, 10_000, true)} narrow />);
    const trigger = await screen.findByRole("button", { name: "Tampilkan jalur lengkap" });
    await user.click(trigger);
    const sheet = await screen.findByRole("dialog", { name: "Jalur halaman" });
    await user.click(within(sheet).getByRole("button", { name: "Beranda" }));
    expect(onHome).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(trigger);
    await screen.findByRole("dialog", { name: "Jalur halaman" });
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
