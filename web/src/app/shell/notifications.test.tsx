import { beforeEach, describe, expect, test } from "bun:test";
import type { AppNotification, NotificationSource } from "@erpora/platform-web";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
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

async function openPanel() {
  const user = userEvent.setup();
  const trigger = await screen.findByRole("button", { name: /^Notifikasi/ });
  await user.click(trigger);
  return user;
}

describe("NotificationsPanel", () => {
  test("sumber kosong: keadaan kosong 'Belum ada notifikasi'", async () => {
    renderApp();
    await openPanel();
    expect(await screen.findByText("Belum ada notifikasi")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tandai semua telah dibaca" })).toBeDisabled();
  });

  test("daftar: judul, pesan, waktu relatif, penanda belum dibaca", async () => {
    const m = memory([mk("a"), mk("b", { read: true })]);
    renderApp({ notificationSource: m.source });
    await openPanel();
    expect(await screen.findByText("Judul a")).toBeInTheDocument();
    expect(screen.getByText("Pesan a")).toBeInTheDocument();
    expect(screen.getAllByText("5 menit yang lalu")).toHaveLength(2);
    const unread = screen.getByText("Judul a").closest("li");
    const read = screen.getByText("Judul b").closest("li");
    expect(unread?.querySelector('[data-unread="true"]')).not.toBeNull();
    expect(read?.querySelector('[data-unread="true"]')).toBeNull();
  });

  test("menandai terbaca lewat klik baris dan 'Tandai semua telah dibaca' memperbarui lencana", async () => {
    const m = memory([mk("a"), mk("b")]);
    renderApp({ notificationSource: m.source });
    const user = await openPanel();
    await screen.findByText("Judul a");
    expect(await screen.findByTestId("notification-badge")).toHaveTextContent("2");
    await user.click(screen.getByText("Judul a"));
    await waitFor(() => expect(screen.getByTestId("notification-badge")).toHaveTextContent("1"));
    expect(m.calls.read).toEqual(["a"]);
    await user.click(screen.getByRole("button", { name: "Tandai semua telah dibaca" }));
    await waitFor(() => expect(screen.queryByTestId("notification-badge")).toBeNull());
    expect(m.calls.all).toBe(1);
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
    renderApp({ notificationSource: source });
    const user = await openPanel();
    expect(await screen.findByText("Notifikasi gagal dimuat.")).toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole("button", { name: "Coba lagi" }));
    expect(await screen.findByText("Belum ada notifikasi")).toBeInTheDocument();
  });

  test("href aman menavigasi; href tidak aman hanya menandai terbaca tanpa navigasi", async () => {
    const m = memory([
      mk("aman", { href: "/alpha" }),
      mk("jahat", { href: "https://evil.example/x" }),
      mk("ganda", { href: "//evil.example" }),
    ]);
    const app = renderApp({ notificationSource: m.source });
    const user = await openPanel();
    await user.click(await screen.findByText("Judul jahat"));
    await user.click(screen.getByText("Judul ganda"));
    expect(app.history.location.pathname).toBe("/");
    expect(m.calls.read).toEqual(["jahat", "ganda"]);
    await user.click(screen.getByText("Judul aman"));
    await waitFor(() => expect(app.history.location.pathname).toBe("/alpha"));
  });

  test("konten notifikasi dirender sebagai teks, bukan HTML", async () => {
    const m = memory([mk("x", { title: "<img src=x onerror=alert(1)>", message: "<b>tebal</b>" })]);
    renderApp({ notificationSource: m.source });
    await openPanel();
    const title = await screen.findByText("<img src=x onerror=alert(1)>");
    expect(title.closest("li")?.querySelector("img")).toBeNull();
    expect(
      within(title.closest("li") as HTMLElement).getByText("<b>tebal</b>"),
    ).toBeInTheDocument();
  });

  test("markAllRead dan markRead yang ditolak sumber menampilkan Alert lokal (tanpa unhandled rejection)", async () => {
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
    renderApp({ notificationSource: source });
    const user = await openPanel();
    await user.click(await screen.findByRole("button", { name: "Tandai semua telah dibaca" }));
    expect(await screen.findByText("Aksi pada notifikasi gagal. Coba lagi.")).toBeInTheDocument();
    await user.click(screen.getByText("Judul a"));
    expect(screen.getByText("Aksi pada notifikasi gagal. Coba lagi.")).toBeInTheDocument();
  });
});
