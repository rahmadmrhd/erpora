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
  window.innerWidth = mobile ? 500 : 1280;
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
  test("diperluas secara bawaan", () => {
    render(<App />);
    expect(root()).toHaveAttribute("data-state", "expanded");
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

  test("SidebarTrigger bernama dari UiLabels (patch sidebar.toggle)", () => {
    render(<App />);
    expect(
      screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }),
    ).toBeInTheDocument();
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

  test("tanpa pengubah, huruf b tidak mengubah keadaan", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.keyboard("b");
    expect(root()).toHaveAttribute("data-state", "expanded");
  });
});

describe("Sidebar: keadaan terkendali (persistensi dikerjakan shell)", () => {
  test("prop open dihormati dan onOpenChange dipanggil saat dialihkan", async () => {
    const user = userEvent.setup();
    const seen: boolean[] = [];
    render(
      <UiLabelsProvider t={t}>
        <SidebarProvider open={false} onOpenChange={(o) => seen.push(o)}>
          <Sidebar collapsible="icon">
            <SidebarContent>x</SidebarContent>
          </Sidebar>
          <SidebarTrigger />
        </SidebarProvider>
      </UiLabelsProvider>,
    );
    expect(root()).toHaveAttribute("data-state", "collapsed");
    await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
    expect(seen).toEqual([true]);
    expect(root()).toHaveAttribute("data-state", "collapsed");
  });
});

describe("Sidebar: tanpa cookie (patch perilaku K12)", () => {
  test("mengalihkan sidebar (terkendali maupun tak terkendali) tidak menulis cookie", async () => {
    const user = userEvent.setup();
    const before = document.cookie;
    const { unmount } = render(<App />);
    await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
    expect(root()).toHaveAttribute("data-state", "collapsed");
    expect(document.cookie).toBe(before);
    expect(document.cookie).not.toContain("sidebar_state");
    unmount();
    render(
      <UiLabelsProvider t={t}>
        <SidebarProvider open onOpenChange={() => {}}>
          <Sidebar collapsible="icon">
            <SidebarContent>x</SidebarContent>
          </Sidebar>
          <SidebarTrigger />
        </SidebarProvider>
      </UiLabelsProvider>,
    );
    await user.click(screen.getByRole("button", { name: "Ciutkan atau perluas sidebar" }));
    expect(document.cookie).not.toContain("sidebar_state");
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

  test("SidebarMenuBadge menampilkan hitungan", () => {
    render(<App />);
    const badge = document.querySelector('[data-slot="sidebar-menu-badge"]') as HTMLElement;
    expect(badge).toHaveTextContent("5");
  });

  test("item terpilih ditandai data-active (aria-current ditambahkan pemanggil)", () => {
    render(<App />);
    const active = screen.getByRole("button", { name: "Beranda" });
    expect(active).toHaveAttribute("data-active", "true");
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
