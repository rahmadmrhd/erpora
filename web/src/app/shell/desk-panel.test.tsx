import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test";
import { type Desk, type NotificationSource, registerFixedItem } from "@erpora/platform-web";
import {
  makeDashboardItem,
  makeDesk,
  makeDeskGroup,
  makeDeskItem,
} from "@erpora/platform-web/testing";
import { SidebarTrigger } from "@erpora/ui";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DeskPanel, findActiveItem, PanelSheet } from "./desk-panel";
import { renderShell, setViewportWidth } from "./shell-test-support";

const example = makeDesk(
  "example",
  [
    makeDashboardItem("example", "example"),
    makeDeskItem("example", "notes", "/example/notes"),
    makeDeskGroup("example", "reports", [
      makeDeskItem("example", "sales", "/example/reports/sales"),
      makeDeskItem("example", "stock", "/example/reports/stock"),
    ]),
  ],
  { icon: "file-text" },
);
const platform = makeDesk(
  "platform",
  [
    makeDashboardItem("platform", "platform"),
    makeDeskItem("platform", "modules", "/platform/modules"),
  ],
  { source: "platform" },
);
const desks: Desk[] = [platform, example];

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  sessionStorage.setItem("erpora.desk.tab", "example");
});
afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  setViewportWidth(1024);
});

async function ready() {
  return await screen.findByRole("navigation", { name: "Navigasi utama" });
}

describe("findActiveItem", () => {
  test("terpanjang menang, Dashboard hanya persis, segmen-aman", () => {
    expect(findActiveItem(example, "/example")?.id).toBe("dashboard");
    expect(findActiveItem(example, "/example/notes")?.id).toBe("notes");
    expect(findActiveItem(example, "/example/notes/42")?.id).toBe("notes");
    expect(findActiveItem(example, "/example/reports/stock")?.id).toBe("stock");
    expect(findActiveItem(example, "/example/other")).toBeNull();
    expect(findActiveItem(example, "/example2/notes")).toBeNull();
    expect(findActiveItem(example, "/example/notes/")?.id).toBe("notes");
  });
});

describe("DeskPanel: isi dan urutan", () => {
  test("tanpa header Desk; urutan grup tetap, pemisah, label Desk, lalu menu dengan Dashboard pertama", async () => {
    renderShell(<DeskPanel variant="pinned" />, { desks });
    const nav = await ready();
    const first = within(nav).getByRole("button", { name: /Cari/ });
    const todo = within(nav).getByRole("link", { name: /ToDo/ });
    const bell = within(nav).getByRole("button", { name: /Notifikasi/ });
    const order = [first, bell, todo];
    for (let i = 0; i < order.length - 1; i++) {
      const a = order[i] as HTMLElement;
      const b = order[i + 1] as HTMLElement;
      expect(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }
    const separator = nav.querySelector('[data-slot="sidebar-separator"]') as HTMLElement;
    expect(separator).not.toBeNull();
    await waitFor(() => expect(within(nav).getByText("Example")).toBeInTheDocument());
    const label = within(nav).getByText("Example");
    expect(todo.compareDocumentPosition(separator) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      separator.compareDocumentPosition(label) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    const links = within(nav)
      .getAllByRole("link")
      .map((l) => l.getAttribute("href"));
    // ToDo, Dashboard (pertama di menu Desk), Notes
    expect(links.slice(0, 3)).toEqual(["/platform/todo", "/example", "/example/notes"]);
    // Pinned: logo dan pengguna ada di Dock, bukan di panel.
    expect(screen.queryByRole("button", { name: /Menu aplikasi/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Menu akun/ })).toBeNull();
  });

  test("pemisah tidak meluap secara horizontal: lebar penuh + mx-2 ditimpa w-auto (regresi bilah gulir horizontal)", async () => {
    renderShell(<DeskPanel variant="pinned" />, { desks });
    const nav = await ready();
    const separator = nav.querySelector('[data-slot="sidebar-separator"]') as HTMLElement;
    expect(separator.className).toContain("mx-2");
    expect(separator.className).toContain("data-[orientation=horizontal]:w-auto");
    expect(separator.className).not.toContain("data-[orientation=horizontal]:w-full");
  });

  test("Floating dan sheet: logo di kepala dan pengguna di dasar", async () => {
    for (const variant of ["floating", "sheet"] as const) {
      const { unmount } = renderShell(<DeskPanel variant={variant} />, { desks });
      await ready();
      expect(screen.getByRole("button", { name: /Menu aplikasi/ })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /^Menu akun/ })).toBeInTheDocument();
      unmount();
    }
  });

  test("tanpa Desk saat ini (belum ada pilihan): petunjuk memilih Desk di Dock; grup tetap tetap ada", async () => {
    sessionStorage.clear();
    renderShell(<DeskPanel variant="pinned" />, { desks });
    const nav = await ready();
    expect(
      await within(nav).findByText("Pilih Desk di Dock untuk melihat menunya."),
    ).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: /Cari/ })).toBeInTheDocument();
  });
});

describe("DeskPanel: item tetap", () => {
  test("Cari membuka palet dan Notifikasi membuka panel; ToDo menavigasi ke /platform/todo", async () => {
    const openPalette = mock(() => {});
    const openNotifications = mock(() => {});
    const user = userEvent.setup();
    const { router } = renderShell(<DeskPanel variant="pinned" />, {
      desks,
      actions: { openPalette, openNotifications },
    });
    const nav = await ready();
    await user.click(within(nav).getByRole("button", { name: /Cari/ }));
    expect(openPalette).toHaveBeenCalledTimes(1);
    await user.click(within(nav).getByRole("button", { name: /Notifikasi/ }));
    expect(openNotifications).toHaveBeenCalledTimes(1);
    await user.click(within(nav).getByRole("link", { name: /ToDo/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/platform/todo"));
  });

  test("Cari menampilkan petunjuk Ctrl K", async () => {
    renderShell(<DeskPanel variant="pinned" />, { desks });
    const nav = await ready();
    const search = within(nav).getByRole("button", { name: /Cari/ });
    expect(within(search).getByText("Ctrl")).toBeInTheDocument();
    expect(within(search).getByText("K")).toBeInTheDocument();
  });

  test("item yang didaftarkan lewat registerFixedItem muncul di grup tetap tanpa mengubah komponen", async () => {
    const unregister = registerFixedItem({
      id: "inbox",
      label: { ns: "platform", key: "notifications.title" },
      icon: "inbox",
      order: 25,
      action: { kind: "link", to: "/platform/inbox" },
    });
    try {
      renderShell(<DeskPanel variant="pinned" />, { desks });
      const nav = await ready();
      const links = within(nav)
        .getAllByRole("link")
        .map((l) => l.getAttribute("href"));
      expect(links.slice(0, 2)).toEqual(["/platform/inbox", "/platform/todo"]);
    } finally {
      unregister();
    }
  });

  const source = (count: number): NotificationSource => ({
    unreadCount: async () => count,
    list: async () => [],
    markRead: async () => {},
    markAllRead: async () => {},
  });

  test.each([
    [5, "5"],
    [99, "99"],
    [100, "99+"],
  ])("lencana Notifikasi untuk %p belum dibaca menampilkan %p", async (count, text) => {
    renderShell(<DeskPanel variant="pinned" />, { desks, notificationSource: source(count) });
    const nav = await ready();
    const bell = within(nav).getByRole("button", { name: /Notifikasi/ });
    const item = bell.closest("li") as HTMLElement;
    await waitFor(() => expect(within(item).getByText(text)).toBeInTheDocument());
    expect(bell).toHaveAccessibleDescription(text);
  });

  test("tanpa belum dibaca tidak ada lencana", async () => {
    renderShell(<DeskPanel variant="pinned" />, { desks, notificationSource: source(0) });
    const nav = await ready();
    await act(async () => {
      await new Promise((r) => setTimeout(r, 30));
    });
    const bell = within(nav).getByRole("button", { name: /Notifikasi/ });
    expect(bell.closest("li")?.querySelector('[data-slot="sidebar-menu-badge"]')).toBeNull();
  });
});

describe("DeskPanel: menu Desk", () => {
  test("item aktif unik: aria-current hanya pada kecocokan persis; kecocokan awalan hanya menyorot", async () => {
    renderShell(<DeskPanel variant="pinned" />, { desks, path: "/example/notes/42" });
    const nav = await ready();
    const notes = await within(nav).findByRole("link", { name: "Notes" });
    await waitFor(() => expect(notes).toHaveAttribute("data-active", "true"));
    expect(notes).not.toHaveAttribute("aria-current");
    expect(within(nav).getByRole("link", { name: "Dashboard" })).not.toHaveAttribute(
      "data-active",
      "true",
    );
  });

  test("halaman persis: aria-current page", async () => {
    renderShell(<DeskPanel variant="pinned" />, { desks, path: "/example/notes" });
    const nav = await ready();
    const notes = await within(nav).findByRole("link", { name: "Notes" });
    await waitFor(() => expect(notes).toHaveAttribute("aria-current", "page"));
    expect(nav.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });

  test("Dashboard aktif hanya pada /<slug> persis", async () => {
    renderShell(<DeskPanel variant="pinned" />, { desks, path: "/example" });
    const nav = await ready();
    const dash = await within(nav).findByRole("link", { name: "Dashboard" });
    await waitFor(() => expect(dash).toHaveAttribute("aria-current", "page"));
  });

  test("grup tertutup dapat dibuka dengan klik dan terbuka otomatis bila ada anak aktif", async () => {
    const user = userEvent.setup();
    const first = renderShell(<DeskPanel variant="pinned" />, { desks });
    const nav = await ready();
    expect(within(nav).queryByRole("link", { name: "Sales" })).toBeNull();
    await user.click(await within(nav).findByRole("button", { name: "Reports" }));
    expect(await within(nav).findByRole("link", { name: "Sales" })).toBeInTheDocument();
    first.unmount();

    renderShell(<DeskPanel variant="pinned" />, { desks, path: "/example/reports/stock" });
    const nav2 = await ready();
    const stock = await within(nav2).findByRole("link", { name: "Stock" });
    expect(stock).toHaveAttribute("aria-current", "page");
  });

  test("strip ikon (Floating terciut): grup membuka popover berisi anaknya (dimuat malas)", async () => {
    const user = userEvent.setup();
    renderShell(<DeskPanel variant="floating" />, {
      desks,
      sidebar: { open: false, onOpenChange: () => {} },
    });
    const nav = await ready();
    // Chunk popover dimuat malas: tunggu pemicu popover asli menggantikan cadangan sebelum diklik.
    await waitFor(() =>
      expect(within(nav).getByRole("button", { name: "Reports" })).toHaveAttribute(
        "aria-haspopup",
        "dialog",
      ),
    );
    await user.click(within(nav).getByRole("button", { name: "Reports" }));
    expect(await screen.findByRole("link", { name: "Sales" })).toBeInTheDocument();
  });

  test("Desk lain: hanya menu Desk saat ini yang tampil", async () => {
    sessionStorage.setItem("erpora.desk.tab", "platform");
    renderShell(<DeskPanel variant="pinned" />, { desks });
    const nav = await ready();
    expect(await within(nav).findByRole("link", { name: "Modules" })).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Notes" })).toBeNull();
  });
});

describe("PanelSheet (< 1024 px)", () => {
  function Trigger() {
    return <SidebarTrigger aria-label="Buka menu" />;
  }

  test("tertutup secara bawaan; tombol menu membuka sheet berisi panel; navigasi menutupnya", async () => {
    setViewportWidth(900);
    const user = userEvent.setup();
    const { router } = renderShell(
      <>
        <Trigger />
        <PanelSheet />
      </>,
      { desks, defaultSidebarOpen: false },
    );
    await screen.findByRole("button", { name: "Buka menu" });
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Buka menu" }));
    const dialog = await screen.findByRole("dialog", { name: "Menu navigasi" });
    expect(within(dialog).getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: /Menu aplikasi/ })).toBeInTheDocument();
    await user.click(await within(dialog).findByRole("link", { name: "Notes" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/example/notes"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("lebar ponsel (< 768 px) juga memakai sheet", async () => {
    setViewportWidth(600);
    const user = userEvent.setup();
    renderShell(
      <>
        <Trigger />
        <PanelSheet />
      </>,
      { desks, defaultSidebarOpen: false },
    );
    await user.click(await screen.findByRole("button", { name: "Buka menu" }));
    expect(await screen.findByRole("dialog", { name: "Menu navigasi" })).toBeInTheDocument();
  });

  test("tanpa pembatas klik di dalam sheet", async () => {
    setViewportWidth(900);
    const user = userEvent.setup();
    renderShell(
      <>
        <Trigger />
        <PanelSheet />
      </>,
      { desks, defaultSidebarOpen: false },
    );
    await user.click(await screen.findByRole("button", { name: "Buka menu" }));
    await screen.findByRole("dialog");
    expect(screen.queryByRole("separator", { name: /sidebar/i })).toBeNull();
  });
});
