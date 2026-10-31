import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import type { Desk } from "@erpora/platform-web";
import { makeDashboardItem, makeDesk, makeDeskItem } from "@erpora/platform-web/testing";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Dock, fitDockCount } from "./dock";
import { DockFloating } from "./dock-floating";
import { renderShell } from "./shell-test-support";

const desks: Desk[] = [
  makeDesk(
    "platform",
    [
      makeDashboardItem("platform", "platform"),
      makeDeskItem("platform", "modules", "/platform/modules"),
    ],
    { source: "platform", icon: "settings" },
  ),
  makeDesk(
    "example",
    [makeDashboardItem("example", "example"), makeDeskItem("example", "notes", "/example/notes")],
    { icon: "file-text" },
  ),
  makeDesk("billing", [makeDashboardItem("billing", "billing")], { icon: "receipt" }),
];

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

const originalResizeObserver = globalThis.ResizeObserver;
function restoreResizeObserver() {
  globalThis.ResizeObserver = originalResizeObserver;
}
/** Memalsukan `ResizeObserver` agar daftar Desk terukur setinggi `height` px (memaksa +N bila tak cukup). */
function mockHeight(height: number) {
  globalThis.ResizeObserver = class {
    constructor(private cb: ResizeObserverCallback) {}
    observe(target: Element) {
      this.cb(
        [{ target, contentRect: { height } } as unknown as ResizeObserverEntry],
        this as unknown as ResizeObserver,
      );
    }
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

describe("fitDockCount", () => {
  test("semua muat: tanpa +N", () => {
    expect(fitDockCount({ total: 3, availableHeight: 200, itemSize: 40, gap: 4 })).toBe(3);
  });
  test("tidak muat: menyisakan satu slot untuk +N", () => {
    // 2 slot (88 px) untuk 3 item: 1 item + tombol +N
    expect(fitDockCount({ total: 3, availableHeight: 84, itemSize: 40, gap: 4 })).toBe(1);
    expect(fitDockCount({ total: 10, availableHeight: 200, itemSize: 40, gap: 4 })).toBe(3);
  });
  test("tinggi belum terukur atau tak valid: tampilkan semua", () => {
    expect(fitDockCount({ total: 5, availableHeight: 0, itemSize: 40, gap: 4 })).toBe(5);
    expect(fitDockCount({ total: 5, availableHeight: Number.NaN, itemSize: 40, gap: 4 })).toBe(5);
  });
  test("tinggi sangat kecil: nol item, hanya +N", () => {
    expect(fitDockCount({ total: 5, availableHeight: 20, itemSize: 40, gap: 4 })).toBe(0);
  });
  test("tanpa item", () => {
    expect(fitDockCount({ total: 0, availableHeight: 100, itemSize: 40, gap: 4 })).toBe(0);
  });
});

describe("Dock (Pinned)", () => {
  test("navigasi berlabel berisi satu tombol per Desk yang diizinkan, logo, dan menu akun", async () => {
    renderShell(<Dock />, { desks });
    const nav = await screen.findByRole("navigation", { name: "Dock Desk" });
    await waitFor(() =>
      expect(
        within(nav).getAllByRole("button", { name: /^(Platform|Example|Billing)$/ }),
      ).toHaveLength(3),
    );
    expect(within(nav).getByRole("button", { name: /Menu aplikasi/ })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /^Menu akun/ })).toBeInTheDocument();
  });

  test("Desk saat ini ditandai aria-current dan klik berpindah Desk menuju Dashboard-nya", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const user = userEvent.setup();
    const { router } = renderShell(<Dock />, { desks });
    const example = await screen.findByRole("button", { name: "Example" });
    await waitFor(() => expect(example).toHaveAttribute("aria-current", "true"));
    expect(screen.getByRole("button", { name: "Platform" })).not.toHaveAttribute("aria-current");

    await user.click(screen.getByRole("button", { name: "Billing" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/billing"));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Billing" })).toHaveAttribute(
        "aria-current",
        "true",
      ),
    );
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("billing");
  });

  test("sorotan Desk aktif memakai latar (sidebar-accent), tanpa outline atau cincin (K2)", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    renderShell(<Dock />, { desks });
    const example = await screen.findByRole("button", { name: "Example" });
    await waitFor(() => expect(example).toHaveAttribute("aria-current", "true"));
    const classes = example.className.split(/\s+/);
    expect(classes).toContain("data-[active=true]:bg-sidebar-accent");
    expect(
      classes.filter((c) => /(^|:)(ring|outline)(-|$)/.test(c) && c.startsWith("data-")),
    ).toEqual([]);
  });

  test("hanya satu ikon yang dapat difokus (roving); panah bawah/atas, Home, dan End memindahkan fokus", async () => {
    const user = userEvent.setup();
    renderShell(<Dock />, { desks });
    const platform = await screen.findByRole("button", { name: "Platform" });
    const example = screen.getByRole("button", { name: "Example" });
    const billing = screen.getByRole("button", { name: "Billing" });
    expect([platform, example, billing].filter((b) => b.tabIndex === 0)).toHaveLength(1);
    platform.focus();
    await user.keyboard("{ArrowDown}");
    expect(example).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(billing).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(billing).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(example).toHaveFocus();
    await user.keyboard("{Home}");
    expect(platform).toHaveFocus();
    await user.keyboard("{End}");
    expect(billing).toHaveFocus();
    // Enter memilih (perilaku tombol).
    await user.keyboard("{Enter}");
  });

  describe("+N saat tinggi tidak cukup", () => {
    afterEach(restoreResizeObserver);

    test("menampilkan sebagian ikon dan sisanya di menu +N yang berfungsi", async () => {
      mockHeight(84); // 2 slot: 1 ikon + +N
      const user = userEvent.setup();
      const { router } = renderShell(<Dock />, { desks });
      const more = await screen.findByRole("button", { name: "+2 Desk lainnya" });
      expect(more).toHaveTextContent("+2");
      expect(screen.getByRole("button", { name: "Platform" })).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Example" })).toBeNull();
      await user.click(more);
      const item = await screen.findByRole("menuitem", { name: "Example" });
      await user.click(item);
      await waitFor(() => expect(router.state.location.pathname).toBe("/example"));
    });

    test("tanpa kekurangan tinggi tidak ada +N", async () => {
      mockHeight(400);
      renderShell(<Dock />, { desks });
      await screen.findByRole("button", { name: "Billing" });
      expect(screen.queryByRole("button", { name: /Desk lainnya/ })).toBeNull();
    });
  });
});

describe("DockFloating", () => {
  async function renderFloating() {
    const result = renderShell(<DockFloating />, { desks });
    // Daftar Desk dimuat; pil tertutup (tersembunyi dari pohon aksesibilitas).
    await waitFor(() =>
      expect(
        screen.getByRole("navigation", { name: "Dock Desk", hidden: true }),
      ).toBeInTheDocument(),
    );
    return result;
  }
  const pill = () => screen.getByRole("navigation", { name: "Dock Desk", hidden: true });
  const wrapper = () => pill().closest('[data-slot="dock-floating"]') as HTMLElement;
  const press = () =>
    act(() => {
      fireEvent.keyDown(window, { key: "?", code: "Slash", ctrlKey: true, shiftKey: true });
    });

  test("tertutup secara bawaan: tersembunyi dari pohon aksesibilitas dan tidak dapat difokus", async () => {
    await renderFloating();
    expect(wrapper()).toHaveAttribute("aria-hidden", "true");
    expect(wrapper()).toHaveAttribute("inert");
    expect(screen.queryByRole("navigation", { name: "Dock Desk" })).toBeNull();
  });

  test("Ctrl+Shift+/ membuka pil dan memfokuskan Desk saat ini atau pertama; Escape menutup", async () => {
    const user = userEvent.setup();
    await renderFloating();
    press();
    const nav = await screen.findByRole("navigation", { name: "Dock Desk" });
    expect(nav).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Platform" })).toHaveFocus());
    await user.keyboard("{Escape}");
    await waitFor(() => expect(wrapper()).toHaveAttribute("aria-hidden", "true"));
  });

  test("menekan lagi pintasan menutup pil", async () => {
    await renderFloating();
    press();
    await screen.findByRole("navigation", { name: "Dock Desk" });
    press();
    await waitFor(() => expect(wrapper()).toHaveAttribute("aria-hidden", "true"));
  });

  test("zona tepi kiri (hot-edge 12 px) membuka pil saat pointer masuk", async () => {
    await renderFloating();
    const edge = document.querySelector('[data-slot="dock-hot-edge"]');
    expect(edge).not.toBeNull();
    expect(edge?.className).toContain("w-3");
    act(() => {
      fireEvent.mouseEnter(edge as Element);
    });
    expect(await screen.findByRole("navigation", { name: "Dock Desk" })).toBeInTheDocument();
  });

  test("memilih Desk menutup pil dan menuju Dashboard-nya", async () => {
    const user = userEvent.setup();
    const { router } = await renderFloating();
    press();
    await user.click(await screen.findByRole("button", { name: "Example" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/example"));
    await waitFor(() => expect(wrapper()).toHaveAttribute("aria-hidden", "true"));
  });

  test("memilih Desk yang memuat rute saat ini bertahan di rute itu (K10)", async () => {
    const user = userEvent.setup();
    const shared: Desk[] = [
      ...desks,
      makeDesk("addon", [
        makeDashboardItem("addon", "addon"),
        makeDeskItem("addon", "notes", "/example/notes", { ownerDesk: "example" }),
      ]),
    ];
    const { router } = renderShell(<DockFloating />, { desks: shared, path: "/example/notes" });
    await waitFor(() => expect(pill()).toBeInTheDocument());
    press();
    await user.click(await screen.findByRole("button", { name: "Addon" }));
    await waitFor(() => expect(sessionStorage.getItem("erpora.desk.tab")).toBe("addon"));
    expect(router.state.location.pathname).toBe("/example/notes");
    await waitFor(() => expect(wrapper()).toHaveAttribute("aria-hidden", "true"));
  });

  test("fokus keluar dari pil menutupnya", async () => {
    const user = userEvent.setup();
    renderShell(
      <>
        <button type="button">Luar</button>
        <DockFloating />
      </>,
      { desks },
    );
    await waitFor(() => expect(pill()).toBeInTheDocument());
    press();
    await screen.findByRole("navigation", { name: "Dock Desk" });
    await waitFor(() => expect(screen.getByRole("button", { name: "Platform" })).toHaveFocus());
    await user.click(screen.getByRole("button", { name: "Luar" }));
    await waitFor(() => expect(wrapper()).toHaveAttribute("aria-hidden", "true"));
  });

  test("transisi 280 ms dan dimatikan oleh prefers-reduced-motion", async () => {
    await renderFloating();
    const el = wrapper();
    expect(el.className).toContain("duration-280");
    expect(el.className).toContain("motion-reduce:transition-none");
  });

  describe("menu +N di portal (I-4)", () => {
    afterEach(restoreResizeObserver);

    test("blur ke isi menu dan pointer meninggalkan pil tidak menutup pil selama menu terbuka", async () => {
      mockHeight(84); // 1 ikon + +N
      const user = userEvent.setup();
      const { router } = await renderFloating();
      press();
      const more = await screen.findByRole("button", { name: "+2 Desk lainnya" });
      await user.click(more);
      const menu = await screen.findByRole("menu");
      // Isi menu dirender di portal (di luar DOM pil): fokus dan pointer ke sana bukan "keluar" dari pil.
      fireEvent.focusOut(more, { relatedTarget: menu });
      fireEvent.mouseLeave(wrapper());
      await new Promise((r) => setTimeout(r, 400));
      expect(wrapper()).toHaveAttribute("data-state", "open");
      await user.click(await screen.findByRole("menuitem", { name: "Example" }));
      await waitFor(() => expect(router.state.location.pathname).toBe("/example"));
      await waitFor(() => expect(wrapper()).toHaveAttribute("aria-hidden", "true"));
    });

    test("Escape menutup menu +N lebih dulu dan pil tetap terbuka", async () => {
      mockHeight(84);
      const user = userEvent.setup();
      await renderFloating();
      press();
      await user.click(await screen.findByRole("button", { name: "+2 Desk lainnya" }));
      await screen.findByRole("menu");
      await user.keyboard("{Escape}");
      await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
      expect(wrapper()).toHaveAttribute("data-state", "open");
    });
  });
});
