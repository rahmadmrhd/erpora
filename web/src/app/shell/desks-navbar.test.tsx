import { beforeEach, describe, expect, mock, test } from "bun:test";
import type { Desk, NotificationSource } from "@erpora/platform-web";
import { makeDashboardItem, makeDesk, makeDeskItem } from "@erpora/platform-web/testing";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DesksNavbar } from "./desks-navbar";
import { renderShell } from "./shell-test-support";

const desks: Desk[] = [
  makeDesk("platform", [makeDashboardItem("platform", "platform")], { source: "platform" }),
  makeDesk("example", [
    makeDashboardItem("example", "example"),
    makeDeskItem("example", "notes", "/example/notes"),
  ]),
];

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

const source = (count: number): NotificationSource => ({
  unreadCount: async () => count,
  list: async () => [],
  markRead: async () => {},
  markAllRead: async () => {},
});

describe("DesksNavbar (K1)", () => {
  test("kiri logo, tengah tombol Cari dengan petunjuk Ctrl K, kanan lonceng dan menu pengguna; tanpa breadcrumb dan tombol sidebar", async () => {
    renderShell(<DesksNavbar />, { desks });
    const bar = await screen.findByRole("banner");
    const logo = within(bar).getByRole("button", { name: /Menu aplikasi/ });
    const search = within(bar).getByRole("button", { name: /Cari/ });
    const bell = within(bar).getByRole("button", { name: /Notifikasi/ });
    const user = within(bar).getByRole("button", { name: /^Menu akun/ });
    expect(within(search).getByText("Ctrl")).toBeInTheDocument();
    expect(within(search).getByText("K")).toBeInTheDocument();
    expect(search).toHaveAttribute("aria-keyshortcuts", "Control+K");
    const order = [logo, search, bell, user];
    for (let i = 0; i < order.length - 1; i++) {
      const a = order[i] as HTMLElement;
      const b = order[i + 1] as HTMLElement;
      expect(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    }
    expect(within(bar).queryByRole("navigation")).toBeNull();
    expect(within(bar).queryByRole("button", { name: /Ciutkan atau perluas/ })).toBeNull();
  });

  test("Cari membuka palet dan lonceng membuka panel Notifikasi", async () => {
    const openPalette = mock(() => {});
    const openNotifications = mock(() => {});
    const user = userEvent.setup();
    renderShell(<DesksNavbar />, { desks, actions: { openPalette, openNotifications } });
    const bar = await screen.findByRole("banner");
    await user.click(within(bar).getByRole("button", { name: /Cari/ }));
    expect(openPalette).toHaveBeenCalledTimes(1);
    await user.click(within(bar).getByRole("button", { name: /Notifikasi/ }));
    expect(openNotifications).toHaveBeenCalledTimes(1);
  });

  test("lencana lonceng menampilkan jumlah belum dibaca dan menjadi deskripsi tombol", async () => {
    renderShell(<DesksNavbar />, { desks, notificationSource: source(7) });
    const bell = await screen.findByRole("button", { name: /Notifikasi/ });
    await waitFor(() => expect(bell).toHaveAccessibleDescription("7"));
  });

  test("logo membuka popover logo yang sama (Semua Desk menuju / dengan penanda showDesks)", async () => {
    const user = userEvent.setup();
    const { router } = renderShell(<DesksNavbar />, { desks });
    await user.click(await screen.findByRole("button", { name: /Menu aplikasi/ }));
    expect(await screen.findByRole("menuitem", { name: "Pindah Desk" })).toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: "Semua Desk" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
  });

  test("avatar membuka menu pengguna dengan Pengaturan di bawah tombol", async () => {
    const openSettings = mock(() => {});
    const user = userEvent.setup();
    renderShell(<DesksNavbar />, { desks, actions: { openSettings } });
    await user.click(await screen.findByRole("button", { name: /^Menu akun/ }));
    await user.click(await screen.findByRole("menuitem", { name: /Pengaturan/ }));
    expect(openSettings).toHaveBeenCalledTimes(1);
  });
});
