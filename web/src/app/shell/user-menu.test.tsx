import { beforeEach, describe, expect, test } from "bun:test";
import type { CurrentUserSource } from "@erpora/platform-web";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
  document.documentElement.className = "";
});

const user = {
  id: "u1",
  displayName: "Rahmad Maulana",
  email: "rahmad@erpora.id",
  avatarUrl: null,
};

function source(over: Partial<CurrentUserSource> = {}): CurrentUserSource {
  return {
    capabilities: { manageAccount: true, signOut: true },
    get: async () => user,
    manageAccountPath: () => "/alpha",
    signOut: async () => {},
    ...over,
  };
}

async function openMenu() {
  const u = userEvent.setup();
  await u.click(await screen.findByRole("button", { name: /^Menu akun/ }));
  return u;
}

describe("UserMenu", () => {
  test("nama aksesibel tombol memuat teks yang terlihat (nama dan email), tanpa aria-label yang menimpa (WCAG 2.5.3)", async () => {
    renderApp({ currentUserSource: source() });
    await screen.findByText("Rahmad Maulana");
    const button = screen.getByRole("button", { name: /^Menu akun/ });
    expect(button.getAttribute("aria-label")).toBe(null);
    expect(screen.getByRole("button", { name: /Rahmad Maulana/ })).toBe(button);
    expect(screen.getByRole("button", { name: /rahmad@erpora\.id/ })).toBe(button);
  });

  test("avatar menampilkan inisial, nama, dan email di dasar sidebar", async () => {
    renderApp({ currentUserSource: source() });
    expect(await screen.findByText("Rahmad Maulana")).toBeInTheDocument();
    expect(screen.getByText("rahmad@erpora.id")).toBeInTheDocument();
    expect(screen.getByText("RM")).toBeInTheDocument();
  });

  test("anonim: nama 'Pengguna' tanpa email; Kelola Akun dan Keluar nonaktif dengan penjelasan", async () => {
    renderApp();
    await openMenu();
    expect(screen.getAllByText("Pengguna").length).toBeGreaterThan(0);
    const manage = await screen.findByRole("menuitem", { name: "Kelola Akun" });
    expect(manage).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("menuitem", { name: "Keluar" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(manage.getAttribute("aria-describedby")).toBe("user-menu-unavailable");
    expect(document.getElementById("user-menu-unavailable")).toHaveTextContent(
      "Belum tersedia di versi ini",
    );
  });

  test("isi menu: Kelola Akun, Bahasa, Tema, Keluar", async () => {
    renderApp({ currentUserSource: source() });
    await openMenu();
    expect(await screen.findByRole("menuitem", { name: "Kelola Akun" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("menuitem", { name: "Bahasa" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Tema" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Keluar" })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    );
  });

  test("Kelola Akun menavigasi ke path internal dari sumber", async () => {
    const app = renderApp({ currentUserSource: source() });
    const u = await openMenu();
    await u.click(await screen.findByRole("menuitem", { name: "Kelola Akun" }));
    await waitFor(() => expect(app.history.location.pathname).toBe("/alpha"));
  });

  test("Keluar memanggil signOut sumber", async () => {
    let called = 0;
    renderApp({
      currentUserSource: source({
        signOut: async () => {
          called += 1;
        },
      }),
    });
    const u = await openMenu();
    await u.click(await screen.findByRole("menuitem", { name: "Keluar" }));
    await waitFor(() => expect(called).toBe(1));
  });

  test("pilihan Tema: Gelap menyimpan erpora.theme dan memasang kelas dark", async () => {
    renderApp({ currentUserSource: source() });
    const u = await openMenu();
    await u.click(await screen.findByRole("menuitem", { name: "Tema" }));
    await screen.findByRole("menuitemradio", { name: "Gelap" });
    // Item submenu dipilih lewat keyboard (klik pointer pada konten submenu portal tidak andal di happy-dom).
    await u.keyboard("{ArrowRight}{ArrowDown}{Enter}");
    await waitFor(() => expect(document.documentElement.classList.contains("dark")).toBe(true));
    expect(localStorage.getItem("erpora.theme")).toBe("dark");
  });

  test("pilihan Bahasa: English memuat katalog en dan mengganti teks tanpa reload", async () => {
    renderApp({ currentUserSource: source() });
    const u = await openMenu();
    await u.click(await screen.findByRole("menuitem", { name: "Bahasa" }));
    expect(await screen.findByRole("menuitemradio", { name: "Indonesia" })).toBeChecked();
    await u.keyboard("{ArrowRight}{ArrowDown}{Enter}");
    await waitFor(() =>
      expect(screen.getByRole("navigation", { name: "Main navigation" })).toBeInTheDocument(),
    );
    expect(localStorage.getItem("erpora.locale")).toBe("en");
    expect(document.documentElement.lang).toBe("en");
    expect(screen.getByRole("link", { name: "Home" })).toBeInTheDocument();
  });

  test("avatarUrl tidak aman (eksternal) tidak dirender sebagai gambar", async () => {
    renderApp({
      currentUserSource: source({
        get: async () => ({ ...user, avatarUrl: "https://evil.example/a.png" }),
      }),
    });
    await screen.findByText("Rahmad Maulana");
    expect(document.querySelector('img[src^="https://evil.example"]')).toBeNull();
  });
});
