import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test";
import type { CurrentUserSource } from "@erpora/platform-web";
import { Toaster } from "@erpora/ui/toaster";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderShell, type ShellHarnessOptions } from "./shell-test-support";
import { UserMenu } from "./user-menu";

beforeEach(() => {
  localStorage.clear();
});

// Toast sonner bersifat global antar tes: bersihkan agar galat satu tes tidak terbaca tes berikutnya.
afterEach(async () => {
  const { toast } = await import("@erpora/ui/toaster");
  toast.dismiss();
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
  const result = renderShell(
    <>
      <UserMenu variant={variant} />
      <Toaster />
    </>,
    opts,
  );
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

  test("Ubah sandi: tautan ke halaman akun dari sumber pengguna (C1.14); tanpa path tidak tampil", async () => {
    const { user, router } = await openMenu("panel", {
      user: source({ manageAccountPath: () => "/core/change-password" }),
    });
    const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent?.trim());
    expect(items).toEqual(["Pengaturan", "Ubah sandi", "Muat ulang", "Keluar"]);
    await user.click(screen.getByRole("menuitem", { name: "Ubah sandi" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/core/change-password"));
  });

  test("Ubah sandi tidak tampil bila kemampuan manageAccount mati atau path bukan path internal aman", async () => {
    const off = await openMenu("panel", {
      user: source({
        capabilities: { manageAccount: false, signOut: true },
        manageAccountPath: () => "/core/change-password",
      }),
    });
    expect(screen.queryByRole("menuitem", { name: "Ubah sandi" })).toBeNull();
    off.unmount();
    await openMenu("panel", { user: source({ manageAccountPath: () => "https://evil.test/x" }) });
    expect(screen.queryByRole("menuitem", { name: "Ubah sandi" })).toBeNull();
  });

  test("Keluar aktif memanggil sumber pengguna bila didukung", async () => {
    const signOut = mock(async () => {});
    const { user } = await openMenu("panel", { user: source({ signOut }) });
    await user.click(await screen.findByRole("menuitem", { name: "Keluar" }));
    expect(signOut).toHaveBeenCalledTimes(1);
  });
});

describe("UserMenu: Keluar gagal (I-1 review C1.14)", () => {
  test("signOut ditolak: galat i18n tampil dengan Coba lagi; Coba lagi memanggil signOut lagi dan sukses tidak menampilkan galat", async () => {
    let n = 0;
    const signOut = mock(async () => {
      n++;
      if (n === 1) throw new Error("signout_failed");
    });
    const { user } = await openMenu("panel", { user: source({ signOut }) });
    await user.click(await screen.findByRole("menuitem", { name: "Keluar" }));
    expect(
      await screen.findByText("Gagal keluar. Sesi Anda masih aktif; coba lagi."),
    ).toBeInTheDocument();
    expect(signOut).toHaveBeenCalledTimes(1);
    await user.click(await screen.findByRole("button", { name: "Coba lagi" }));
    await waitFor(() => expect(signOut).toHaveBeenCalledTimes(2));
  });

  test("signOut berhasil tidak menampilkan galat", async () => {
    const signOut = mock(async () => {});
    const { user } = await openMenu("panel", { user: source({ signOut }) });
    await user.click(await screen.findByRole("menuitem", { name: "Keluar" }));
    await waitFor(() => expect(signOut).toHaveBeenCalledTimes(1));
    expect(screen.queryByText(/Gagal keluar/)).toBeNull();
  });
});

describe("UserMenu varian dock (K16)", () => {
  test("avatar 32 px berkontras (latar secondary, cincin tipis), mengisi tombol terpusat, dan tombol dapat difokus", async () => {
    renderShell(<UserMenu variant="dock" />, { user: source() });
    const trigger = await screen.findByRole("button", { name: /^Menu akun/ });
    const classes = trigger.className.split(/\s+/);
    expect(classes).toContain("size-8");
    expect(classes).toContain("hover:bg-sidebar-accent");
    const avatar = trigger.querySelector('[data-slot="avatar"]') as HTMLElement;
    expect(avatar).toHaveAttribute("data-size", "default");
    expect(avatar.className).toContain("ring-1");
    const fallback = trigger.querySelector('[data-slot="avatar-fallback"]') as HTMLElement;
    expect(fallback.className).toContain("bg-secondary");
    expect(fallback.className).toContain("text-secondary-foreground");
    trigger.focus();
    expect(trigger).toHaveFocus();
    expect(trigger.tabIndex).toBe(0);
  });
});

describe("UserMenu varian panel di strip ikon Floating (K21)", () => {
  test("diciutkan: avatar 32 px berkontras seperti Dock; terbuka: avatar kecil tanpa cincin", async () => {
    const collapsed = renderShell(<UserMenu variant="panel" />, {
      user: source(),
      sidebar: { open: false, onOpenChange: () => {} },
    });
    const trigger = await screen.findByRole("button", { name: /Menu akun/ });
    const avatar = trigger.querySelector('[data-slot="avatar"]') as HTMLElement;
    expect(avatar).toHaveAttribute("data-size", "default");
    expect(avatar.className).toContain("ring-1");
    const fallback = trigger.querySelector('[data-slot="avatar-fallback"]') as HTMLElement;
    expect(fallback.className).toContain("bg-secondary");
    collapsed.unmount();

    renderShell(<UserMenu variant="panel" />, { user: source() });
    const open = await screen.findByRole("button", { name: /Menu akun/ });
    const small = open.querySelector('[data-slot="avatar"]') as HTMLElement;
    expect(small).toHaveAttribute("data-size", "sm");
    expect(small.className).not.toContain("ring-1");
  });
});
