import { beforeEach, describe, expect, mock, test } from "bun:test";
import type { CurrentUserSource } from "@erpora/platform-web";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderShell, type ShellHarnessOptions } from "./shell-test-support";
import { UserMenu } from "./user-menu";

beforeEach(() => {
  localStorage.clear();
});

const person = {
  id: "u1",
  displayName: "Rahmad Maulana",
  email: "rahmad@erpora.id",
  avatarUrl: null,
};

function source(over: Partial<CurrentUserSource> = {}): CurrentUserSource {
  return {
    capabilities: { manageAccount: true, signOut: true },
    get: async () => person,
    signOut: async () => {},
    ...over,
  };
}

async function openMenu(variant: "dock" | "panel" = "panel", opts: ShellHarnessOptions = {}) {
  const user = userEvent.setup();
  const result = renderShell(<UserMenu variant={variant} />, opts);
  await user.click(await screen.findByRole("button", { name: /^Menu akun/ }));
  return { user, ...result };
}

describe("UserMenu", () => {
  test("varian panel: nama aksesibel memuat teks yang terlihat (nama dan email), WCAG 2.5.3", async () => {
    renderShell(<UserMenu variant="panel" />, { user: source() });
    await screen.findByText("Rahmad Maulana");
    const button = screen.getByRole("button", { name: /^Menu akun/ });
    expect(button.getAttribute("aria-label")).toBeNull();
    expect(screen.getByRole("button", { name: /Rahmad Maulana/ })).toBe(button);
    expect(screen.getByRole("button", { name: /rahmad@erpora\.id/ })).toBe(button);
    expect(screen.getByText("RM")).toBeInTheDocument();
  });

  test("varian dock: hanya avatar (inisial); nama tetap ada untuk pembaca layar", async () => {
    renderShell(<UserMenu variant="dock" />, { user: source() });
    const button = await screen.findByRole("button", { name: /Rahmad Maulana/ });
    expect(button).toHaveAccessibleName(/^Menu akun/);
    expect(screen.getByText("RM")).toBeInTheDocument();
    expect(screen.queryByText("rahmad@erpora.id")).toBeNull();
  });

  test("isi menu: Pengaturan, Muat ulang, Keluar; tanpa My Space, Kelola Akun, Bahasa, atau Tema", async () => {
    await openMenu("panel", { user: source() });
    const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent?.trim());
    expect(items).toEqual(["Pengaturan", "Muat ulang", "Keluar"]);
    expect(screen.queryByText(/My Space/i)).toBeNull();
  });

  test("Pengaturan membuka dialog Settings dan Muat ulang memanggil reload", async () => {
    const openSettings = mock(() => {});
    const reload = mock(() => {});
    const { user } = await openMenu("panel", { actions: { openSettings, reload } });
    await user.click(await screen.findByRole("menuitem", { name: "Pengaturan" }));
    expect(openSettings).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: /^Menu akun/ }));
    await user.click(await screen.findByRole("menuitem", { name: "Muat ulang" }));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  test("anonim: nama Pengguna tanpa email; Keluar nonaktif dengan penjelasan", async () => {
    await openMenu("panel");
    expect(screen.getAllByText("Pengguna").length).toBeGreaterThan(0);
    const out = await screen.findByRole("menuitem", { name: "Keluar" });
    expect(out).toHaveAttribute("aria-disabled", "true");
    const hintId = out.getAttribute("aria-describedby");
    expect(document.getElementById(hintId as string)).toHaveTextContent(
      "Belum tersedia di versi ini",
    );
  });

  test("Keluar aktif memanggil sumber pengguna bila didukung", async () => {
    const signOut = mock(async () => {});
    const { user } = await openMenu("panel", { user: source({ signOut }) });
    await user.click(await screen.findByRole("menuitem", { name: "Keluar" }));
    expect(signOut).toHaveBeenCalledTimes(1);
  });
});
