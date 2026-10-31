import { beforeEach, describe, expect, mock, test } from "bun:test";
import type { Desk } from "@erpora/platform-web";
import { makeDashboardItem, makeDesk, makeDeskItem } from "@erpora/platform-web/testing";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LogoPopover } from "./logo-popover";
import { renderShell, type ShellHarnessOptions } from "./shell-test-support";

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

async function open(
  variant: "dock" | "panel" | "navbar" = "panel",
  opts: ShellHarnessOptions = {},
) {
  const user = userEvent.setup();
  const result = renderShell(<LogoPopover variant={variant} />, { desks, ...opts });
  const trigger = await screen.findByRole("button", { name: /Menu aplikasi/ });
  await user.click(trigger);
  return { user, trigger, ...result };
}

describe("LogoPopover", () => {
  test("varian panel menampilkan nama aplikasi; varian dock hanya logo", async () => {
    const first = renderShell(<LogoPopover variant="panel" />, { desks });
    expect(await screen.findByText("ERPora")).toBeInTheDocument();
    first.unmount();
    renderShell(<LogoPopover variant="dock" />, { desks });
    await screen.findByRole("button", { name: /Menu aplikasi/ });
    expect(screen.queryByText("ERPora")).toBeNull();
  });

  test("memuat lima item: Pindah Desk, Semua Desk, Edit sidebar, Bantuan dan pintasan, Tentang", async () => {
    await open();
    const menu = await screen.findByRole("menu");
    const names = within(menu)
      .getAllByRole("menuitem")
      .map((i) => i.textContent?.trim());
    expect(names).toHaveLength(5);
    expect(names[0]).toBe("Pindah Desk");
    expect(names[1]).toBe("Semua Desk");
    expect(names[2]).toContain("Edit sidebar");
    expect(names[3]).toBe("Bantuan dan pintasan");
    expect(names[4]).toBe("Tentang");
  });

  test("varian navbar (halaman Desks) tidak memuat Pindah Desk dan Semua Desk; sisanya tetap (U4)", async () => {
    await open("navbar");
    const menu = await screen.findByRole("menu");
    const names = within(menu)
      .getAllByRole("menuitem")
      .map((i) => i.textContent?.trim());
    expect(names).toHaveLength(3);
    expect(names[0]).toContain("Edit sidebar");
    expect(names[1]).toBe("Bantuan dan pintasan");
    expect(names[2]).toBe("Tentang");
    expect(screen.queryByRole("menuitem", { name: "Pindah Desk" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Semua Desk" })).toBeNull();
  });

  test("Edit sidebar nonaktif dengan keterangan M3b", async () => {
    await open();
    const edit = await screen.findByRole("menuitem", { name: /Edit sidebar/ });
    expect(edit).toHaveAttribute("aria-disabled", "true");
    const hintId = edit.getAttribute("aria-describedby");
    expect(hintId).toBeTruthy();
    expect(document.getElementById(hintId as string)).toHaveTextContent(/M3b/);
  });

  test("Pindah Desk: submenu berisi Desk yang diizinkan; memilih menetapkan Desk dan membuka Dashboard-nya", async () => {
    const { user, router } = await open();
    await user.click(await screen.findByRole("menuitem", { name: "Pindah Desk" }));
    await screen.findByRole("menuitemradio", { name: "Example" });
    expect(screen.getByRole("menuitemradio", { name: "Platform" })).toBeInTheDocument();
    // Item submenu dipilih lewat keyboard (klik pointer pada konten submenu portal tidak andal di happy-dom).
    await user.keyboard("{ArrowRight}{ArrowDown}{Enter}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/example"));
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
  });

  describe("Pindah Desk mempertahankan rute bila ada di Desk tujuan (K10)", () => {
    // `addon` memuat rute `/example/notes` milik `example` (menu bersama), jadi pindah ke sana tidak meninggalkan halaman.
    const shared: Desk[] = [
      ...desks,
      makeDesk("addon", [
        makeDashboardItem("addon", "addon"),
        makeDeskItem("addon", "notes", "/example/notes", { ownerDesk: "example" }),
      ]),
    ];

    test("rute ada di Desk tujuan: tetap di rute, hanya Desk saat ini berganti", async () => {
      const { user, router } = await open("panel", { desks: shared, path: "/example/notes" });
      await user.click(await screen.findByRole("menuitem", { name: "Pindah Desk" }));
      await screen.findByRole("menuitemradio", { name: "Addon" });
      await user.keyboard("{ArrowRight}{ArrowDown}{ArrowDown}{Enter}");
      await waitFor(() => expect(sessionStorage.getItem("erpora.desk.tab")).toBe("addon"));
      expect(router.state.location.pathname).toBe("/example/notes");
    });

    test("rute tidak ada di Desk tujuan: menuju Dashboard Desk tujuan", async () => {
      const { user, router } = await open("panel", { desks: shared, path: "/platform" });
      await user.click(await screen.findByRole("menuitem", { name: "Pindah Desk" }));
      await screen.findByRole("menuitemradio", { name: "Addon" });
      await user.keyboard("{ArrowRight}{ArrowDown}{ArrowDown}{Enter}");
      await waitFor(() => expect(router.state.location.pathname).toBe("/addon"));
      expect(sessionStorage.getItem("erpora.desk.tab")).toBe("addon");
    });
  });

  test("Semua Desk menuju / dengan penanda state showDesks", async () => {
    const { user, router } = await open();
    await user.click(await screen.findByRole("menuitem", { name: "Semua Desk" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect((router.state.location.state as { showDesks?: boolean }).showDesks).toBe(true);
  });

  test("Bantuan dan pintasan membuka Settings pada bagian pintasan", async () => {
    const openSettings = mock(() => {});
    const { user } = await open("panel", { actions: { openSettings } });
    await user.click(await screen.findByRole("menuitem", { name: "Bantuan dan pintasan" }));
    expect(openSettings).toHaveBeenCalledWith("shortcuts");
  });

  test("Tentang menampilkan versi aplikasi dari bootstrap", async () => {
    const { user } = await open();
    await user.click(await screen.findByRole("menuitem", { name: "Tentang" }));
    expect(await screen.findByText("Versi 0.1.0")).toBeInTheDocument();
  });

  test("keyboard: Escape menutup menu dan fokus kembali ke logo", async () => {
    const { user, trigger } = await open();
    await screen.findByRole("menu");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
    expect(trigger).toHaveFocus();
  });
});
