import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UiLabelsProvider } from "../../labels";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "./sidebar";

const labels: Record<string, string> = {
  "sidebar.label": "Navigasi utama",
  "sidebar.toggle": "Ciutkan atau perluas sidebar",
  "sidebar.title": "Menu navigasi",
  "sidebar.description": "Menu navigasi utama aplikasi",
  "actions.close": "Tutup",
};
const t = (k: string) => labels[k] ?? k;

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

function App({ onNavigate }: { onNavigate?: (to: string) => void }) {
  return (
    <UiLabelsProvider t={t}>
      <SidebarProvider>
        <Sidebar collapsible="icon" side="left">
          <SidebarHeader>ERPora</SidebarHeader>
          <SidebarContent>
            <SidebarGroup>
              <SidebarGroupLabel>Menu</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton tooltip="Beranda" isActive onClick={() => onNavigate?.("/")}>
                      <span>Beranda</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton tooltip="Notifikasi" onClick={() => onNavigate?.("/notif")}>
                      <span>Notifikasi</span>
                    </SidebarMenuButton>
                    <SidebarMenuBadge>5</SidebarMenuBadge>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </SidebarContent>
          <SidebarFooter>Pengguna</SidebarFooter>
        </Sidebar>
        <SidebarInset>
          <SidebarTrigger />
          <input aria-label="Catatan" />
          <p>Konten</p>
        </SidebarInset>
      </SidebarProvider>
    </UiLabelsProvider>
  );
}

const root = () => document.querySelector('[data-slot="sidebar"]') as HTMLElement;

beforeEach(() => {
  localStorage.clear();
  mockViewport(false);
});
afterEach(() => {
  window.matchMedia = originalMatchMedia;
  localStorage.clear();
});

describe("Sidebar: tiga keadaan", () => {
  test("diperluas secara bawaan; <nav> bernama dari UiLabels", () => {
    render(<App />);
    expect(root()).toHaveAttribute("data-state", "expanded");
    expect(screen.getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
  });

  test("SidebarTrigger (berlabel UiLabels) melipat ke ikon lalu memperluas lagi", async () => {
    const user = userEvent.setup();
    render(<App />);
    const trigger = screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" });
    await user.click(trigger);
    expect(root()).toHaveAttribute("data-state", "collapsed");
    expect(root()).toHaveAttribute("data-collapsible", "icon");
    await user.click(trigger);
    expect(root()).toHaveAttribute("data-state", "expanded");
  });

  test("SidebarTrigger sudah bernama (aria-label dari UiLabels): tidak ada peringatan dev size=icon", () => {
    const warn = spyOn(console, "warn").mockImplementation(() => {});
    try {
      render(<App />);
      const trigger = screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" });
      expect(trigger.getAttribute("aria-label")).toBe("Ciutkan atau perluas sidebar");
      expect(warn.mock.calls.some((c) => String(c[0]).includes('size="icon"'))).toBe(false);
    } finally {
      warn.mockRestore();
    }
  });

  test("lebar dari token: 14,25 rem diperluas dan 3,25 rem terlipat", () => {
    render(<App />);
    const html = document.body.innerHTML;
    expect(html).toContain("w-(--sidebar-width)");
    expect(html).toContain("w-(--sidebar-width-icon)");
  });

  test("layar < 768 px: sidebar menjadi Sheet kiri bernama dan terdeskripsi dari UiLabels", async () => {
    mockViewport(true);
    const user = userEvent.setup();
    render(<App />);
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
    const sheet = await screen.findByRole("dialog", { name: "Menu navigasi" });
    expect(sheet).toHaveAccessibleDescription("Menu navigasi utama aplikasi");
    expect(sheet).toHaveAttribute("data-side", "left");
    expect(screen.getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Beranda" })).toBeInTheDocument();
  });
});

describe("Sidebar: pintasan Ctrl+B / Cmd+B", () => {
  test.each([
    ["Ctrl+B", "{Control>}b{/Control}"],
    ["Cmd+B", "{Meta>}b{/Meta}"],
  ])("%s membalik keadaan", async (_name, keys) => {
    const user = userEvent.setup();
    render(<App />);
    await user.keyboard(keys);
    expect(root()).toHaveAttribute("data-state", "collapsed");
    await user.keyboard(keys);
    expect(root()).toHaveAttribute("data-state", "expanded");
  });

  test("tidak aktif saat mengetik di kolom isian (Ctrl+B = tebal di editor)", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("textbox", { name: "Catatan" }));
    await user.keyboard("{Control>}b{/Control}");
    expect(root()).toHaveAttribute("data-state", "expanded");
  });

  test("tanpa pengubah, huruf b tidak mengubah keadaan", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.keyboard("b");
    expect(root()).toHaveAttribute("data-state", "expanded");
  });
});

describe("Sidebar: persistensi localStorage['erpora.sidebar']", () => {
  test("menyimpan keadaan dan memulihkannya saat dimuat ulang", async () => {
    const user = userEvent.setup();
    const first = render(<App />);
    await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
    expect(localStorage.getItem("erpora.sidebar")).toBe("collapsed");
    first.unmount();
    render(<App />);
    expect(root()).toHaveAttribute("data-state", "collapsed");
  });

  test("nilai rusak diabaikan: kembali diperluas", () => {
    localStorage.setItem("erpora.sidebar", "{rusak");
    render(<App />);
    expect(root()).toHaveAttribute("data-state", "expanded");
  });

  test("localStorage melempar (diblokir): tetap diperluas dan aksi tidak merusak", async () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("diblokir");
    });
    const set = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("diblokir");
    });
    try {
      const user = userEvent.setup();
      render(<App />);
      expect(root()).toHaveAttribute("data-state", "expanded");
      await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
      expect(root()).toHaveAttribute("data-state", "collapsed");
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });
});

describe("Sidebar: tooltip, lencana, dan keyboard", () => {
  test("terlipat: fokus pada item menampilkan tooltip nama; diperluas: tidak ada tooltip", async () => {
    const user = userEvent.setup();
    render(<App />);
    screen.getByRole("button", { name: "Beranda" }).focus();
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("tooltip")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
    screen.getByRole("button", { name: "Notifikasi" }).focus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Notifikasi");
  });

  test("SidebarMenuBadge tampil sebagai hitungan dan menjadi titik saat terlipat", () => {
    render(<App />);
    const badge = document.querySelector('[data-slot="sidebar-menu-badge"]') as HTMLElement;
    expect(badge).toHaveTextContent("5");
    expect(badge.className).toContain("bg-notification-badge-bg");
    expect(badge.className).toContain("group-data-[collapsible=icon]:size-2");
  });

  test("item terpilih: data-active dan aria-current, gaya sidebar-accent (K-16)", () => {
    render(<App />);
    const active = screen.getByRole("button", { name: "Beranda" });
    expect(active).toHaveAttribute("data-active", "true");
    expect(active).toHaveAttribute("aria-current", "page");
    expect(active.className).toContain("data-[active=true]:bg-sidebar-accent");
    expect(active.className).toContain("data-[active=true]:text-sidebar-accent-foreground");
    expect(screen.getByRole("button", { name: /Notifikasi/ })).not.toHaveAttribute(
      "data-active",
      "true",
    );
  });

  test("navigasi keyboard penuh: Tab berurutan dan Enter/Spasi mengaktifkan item", async () => {
    const user = userEvent.setup();
    const went: string[] = [];
    render(<App onNavigate={(to) => went.push(to)} />);
    await user.tab();
    expect(screen.getByRole("button", { name: "Beranda" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: /Notifikasi/ })).toHaveFocus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(went).toEqual(["/notif", "/notif"]);
  });
});

describe("useSidebar", () => {
  test("di luar SidebarProvider melempar galat yang jelas", () => {
    function Bad() {
      useSidebar();
      return null;
    }
    const spy = spyOn(console, "error").mockImplementation(() => {});
    try {
      expect(() => render(<Bad />)).toThrow(/SidebarProvider/);
    } finally {
      spy.mockRestore();
    }
  });

  test("menyediakan state, open, dan isMobile", async () => {
    let snapshot: ReturnType<typeof useSidebar> | undefined;
    function Probe() {
      snapshot = useSidebar();
      return null;
    }
    render(
      <SidebarProvider>
        <Probe />
      </SidebarProvider>,
    );
    await waitFor(() => expect(snapshot).toBeDefined());
    expect(snapshot?.state).toBe("expanded");
    expect(snapshot?.open).toBe(true);
    expect(snapshot?.isMobile).toBe(false);
  });
});
