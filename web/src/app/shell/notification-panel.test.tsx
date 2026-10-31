import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import type { AppNotification, NotificationSource } from "@erpora/platform-web";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { NotificationPanel } from "./notification-panel";
import { renderOverlay } from "./overlay-test-support";

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  for (const el of document.querySelectorAll("[data-fake-sidebar]")) el.remove();
});

const iso = (msAgo: number) => new Date(Date.now() - msAgo).toISOString();
const mk = (id: string, over: Partial<AppNotification> = {}): AppNotification => ({
  id,
  title: `Judul ${id}`,
  message: `Pesan ${id}`,
  createdAt: iso(5 * 60_000),
  read: false,
  ...over,
});

function memory(initial: AppNotification[]) {
  let items = initial;
  const calls = { read: [] as string[], all: 0 };
  const source: NotificationSource = {
    unreadCount: async () => items.filter((n) => !n.read).length,
    list: async () => items,
    markRead: async (id) => {
      calls.read.push(id);
      items = items.map((n) => (n.id === id ? { ...n, read: true } : n));
    },
    markAllRead: async () => {
      calls.all += 1;
      items = items.map((n) => ({ ...n, read: true }));
    },
  };
  return { source, calls };
}

/** Pemicu seperti item Notifikasi di panel menu: tombol yang membalik `open`. */
function Host({ initialOpen = false }: { initialOpen?: boolean }) {
  const [open, setOpen] = useState(initialOpen);
  return (
    <>
      <button type="button" onClick={() => setOpen((o) => !o)}>
        Pemicu
      </button>
      <NotificationPanel open={open} onOpenChange={setOpen} />
    </>
  );
}

async function openPanel(opts: Parameters<typeof renderOverlay>[1] = {}) {
  const app = renderOverlay(<Host />, opts);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Pemicu" }));
  await screen.findByRole("dialog", { name: "Notifikasi" });
  return { user, app };
}

describe("NotificationPanel", () => {
  test("tertutup: tidak merender dialog", async () => {
    renderOverlay(<Host />);
    await screen.findByRole("button", { name: "Pemicu" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("terbuka: dialog bernama Notifikasi, dua tab, tanpa overlay penuh", async () => {
    await openPanel();
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["Semua", "Peristiwa"]);
    expect(screen.getByRole("tab", { name: "Semua" })).toHaveAttribute("aria-selected", "true");
    expect(document.querySelector('[data-slot="sheet-overlay"]')).toBeNull();
  });

  test("sumber kosong: Empty 'Belum ada notifikasi' dan tombol tandai semua nonaktif", async () => {
    await openPanel();
    expect(await screen.findByText("Belum ada notifikasi")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tandai semua telah dibaca" })).toBeDisabled();
  });

  test("tab Peristiwa menampilkan keadaan kosong tersendiri dan dapat dioperasikan dengan keyboard", async () => {
    const { user } = await openPanel();
    act(() => screen.getByRole("tab", { name: "Semua" }).focus());
    await user.keyboard("{ArrowRight}");
    const events = screen.getByRole("tab", { name: "Peristiwa" });
    expect(events).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByText("Belum ada peristiwa")).toBeInTheDocument();
  });

  test("daftar: judul, pesan, waktu relatif, penanda belum dibaca", async () => {
    const m = memory([mk("a"), mk("b", { read: true })]);
    await openPanel({ notificationSource: m.source });
    expect(await screen.findByText("Judul a")).toBeInTheDocument();
    expect(screen.getByText("Pesan a")).toBeInTheDocument();
    expect(screen.getAllByText("5 menit yang lalu")).toHaveLength(2);
    const unread = screen.getByText("Judul a").closest("li");
    const read = screen.getByText("Judul b").closest("li");
    expect(unread?.querySelector('[data-unread="true"]')).not.toBeNull();
    expect(read?.querySelector('[data-unread="true"]')).toBeNull();
    expect(screen.getByRole("button", { name: "Tandai semua telah dibaca" })).toBeEnabled();
  });

  test("klik baris menandai terbaca; 'Tandai semua telah dibaca' memanggil sumber", async () => {
    const m = memory([mk("a"), mk("b")]);
    const { user } = await openPanel({ notificationSource: m.source });
    await user.click(await screen.findByText("Judul a"));
    await waitFor(() => expect(m.calls.read).toEqual(["a"]));
    await user.click(screen.getByRole("button", { name: "Tandai semua telah dibaca" }));
    await waitFor(() => expect(m.calls.all).toBe(1));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Tandai semua telah dibaca" })).toBeDisabled(),
    );
  });

  test("galat sumber: pesan galat dengan tombol Coba lagi yang memuat ulang", async () => {
    let fail = true;
    const source: NotificationSource = {
      unreadCount: async () => {
        if (fail) throw new Error("gagal");
        return 0;
      },
      list: async () => {
        if (fail) throw new Error("gagal");
        return [];
      },
      markRead: async () => {},
      markAllRead: async () => {},
    };
    const { user } = await openPanel({ notificationSource: source });
    expect(await screen.findByText("Notifikasi gagal dimuat.")).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Coba lagi" }));
    expect(await screen.findByText("Belum ada notifikasi")).toBeInTheDocument();
  });

  test("href aman menavigasi dan menutup panel; href tidak aman hanya menandai terbaca", async () => {
    const m = memory([
      mk("aman", { href: "/alpha" }),
      mk("jahat", { href: "https://evil.example/x" }),
      mk("ganda", { href: "//evil.example" }),
    ]);
    const { user, app } = await openPanel({ notificationSource: m.source });
    await user.click(await screen.findByText("Judul jahat"));
    await user.click(screen.getByText("Judul ganda"));
    expect(app.history.location.pathname).toBe("/");
    expect(screen.getByRole("dialog", { name: "Notifikasi" })).toBeInTheDocument();
    expect(m.calls.read).toEqual(["jahat", "ganda"]);
    await user.click(screen.getByText("Judul aman"));
    await waitFor(() => expect(app.history.location.pathname).toBe("/alpha"));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("konten notifikasi dirender sebagai teks, bukan HTML", async () => {
    const m = memory([mk("x", { title: "<img src=x onerror=alert(1)>", message: "<b>tebal</b>" })]);
    await openPanel({ notificationSource: m.source });
    const title = await screen.findByText("<img src=x onerror=alert(1)>");
    expect(title.closest("li")?.querySelector("img")).toBeNull();
    expect(
      within(title.closest("li") as HTMLElement).getByText("<b>tebal</b>"),
    ).toBeInTheDocument();
  });

  test("aksi yang ditolak sumber menampilkan Alert lokal (tanpa unhandled rejection)", async () => {
    const m = memory([mk("a")]);
    const source: NotificationSource = {
      ...m.source,
      markAllRead: async () => {
        throw new Error("gagal");
      },
      markRead: async () => {
        throw new Error("gagal");
      },
    };
    const { user } = await openPanel({ notificationSource: source });
    await user.click(await screen.findByRole("button", { name: "Tandai semua telah dibaca" }));
    expect(await screen.findByText("Aksi pada notifikasi gagal. Coba lagi.")).toBeInTheDocument();
    await user.click(screen.getByText("Judul a"));
    expect(screen.getByText("Aksi pada notifikasi gagal. Coba lagi.")).toBeInTheDocument();
  });
});

describe("NotificationPanel: fokus, penutupan, dan posisi", () => {
  test("Escape menutup dan fokus kembali ke pemicu", async () => {
    const { user } = await openPanel();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: "Pemicu" })).toHaveFocus();
  });

  test("tombol Tutup menutup dan fokus kembali ke pemicu", async () => {
    const { user } = await openPanel();
    await user.click(screen.getByRole("button", { name: "Tutup" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: "Pemicu" })).toHaveFocus();
  });

  test("menekan pemicu saat terbuka menutup panel (tidak menutup lalu membuka lagi)", async () => {
    const { user } = await openPanel();
    await user.click(screen.getByRole("button", { name: "Pemicu" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await new Promise((r) => setTimeout(r, 30));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("klik di luar panel menutupnya", async () => {
    const { user } = await openPanel();
    await user.click(document.body);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("dimulai di tepi kanan panel menu (diukur dari kontainer sidebar)", async () => {
    const fake = document.createElement("div");
    fake.setAttribute("data-slot", "sidebar-container");
    fake.setAttribute("data-fake-sidebar", "");
    fake.getBoundingClientRect = () => ({ right: 312 }) as DOMRect;
    document.body.append(fake);
    await openPanel();
    const dialog = screen.getByRole("dialog", { name: "Notifikasi" });
    expect(dialog.style.left).toBe("312px");
  });

  test("tanpa panel menu (mis. layar ponsel) dimulai dari tepi kiri", async () => {
    await openPanel();
    expect(screen.getByRole("dialog", { name: "Notifikasi" }).style.left).toBe("0px");
  });
});
