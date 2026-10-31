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
  for (const el of document.querySelectorAll("[data-fake-chrome]")) el.remove();
});

/** Elemen tiruan bagian kerangka (container sidebar, Dock, navbar): hanya tepi yang diukur panel yang dipalsukan. */
function fakeChrome(attr: string, rect: { right?: number; bottom?: number }): HTMLElement {
  const el = document.createElement("div");
  const [name, value] = attr.split("=");
  el.setAttribute(name ?? "", value ?? "");
  el.setAttribute("data-fake-chrome", "");
  el.getBoundingClientRect = () => ({ right: 0, bottom: 0, ...rect }) as DOMRect;
  document.body.append(el);
  return el;
}

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

/** Pemicu seperti lonceng navbar di halaman Desks: panel diminta tampil sebagai kartu mengambang. */
function DesksHost() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen((o) => !o)}>
        Pemicu
      </button>
      <NotificationPanel open={open} onOpenChange={setOpen} placement="desks" />
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

  test("tanpa panel menu dan Dock (mis. layar ponsel) dimulai dari tepi kiri", async () => {
    await openPanel();
    expect(screen.getByRole("dialog", { name: "Notifikasi" }).style.left).toBe("0px");
  });

  test("Pinned terbuka: dimulai di tepi kanan container panel menu", async () => {
    fakeChrome("data-slot=dock", { right: 56 });
    fakeChrome("data-slot=sidebar-container", { right: 312 });
    await openPanel();
    expect(screen.getByRole("dialog", { name: "Notifikasi" }).style.left).toBe("312px");
  });

  test("Pinned diciutkan (container di luar layar): dimulai di tepi kanan Dock, bukan menutupinya (Minor 1)", async () => {
    fakeChrome("data-slot=dock", { right: 56 });
    fakeChrome("data-slot=sidebar-container", { right: 0 });
    await openPanel();
    expect(screen.getByRole("dialog", { name: "Notifikasi" }).style.left).toBe("56px");
  });

  test("Floating (strip ikon): dimulai di tepi kanan strip tanpa Dock", async () => {
    fakeChrome("data-slot=sidebar-container", { right: 52 });
    await openPanel();
    expect(screen.getByRole("dialog", { name: "Notifikasi" }).style.left).toBe("52px");
  });

  test("offset mengikuti panel menu yang berubah (atribut data-state) saat panel Notifikasi terbuka", async () => {
    fakeChrome("data-slot=dock", { right: 56 });
    let right = 284;
    const container = fakeChrome("data-slot=sidebar-container", {});
    container.getBoundingClientRect = () => ({ right, bottom: 0 }) as DOMRect;
    await openPanel();
    const dialog = screen.getByRole("dialog", { name: "Notifikasi" });
    expect(dialog.style.left).toBe("284px");
    right = 0;
    container.setAttribute("data-state", "collapsed");
    await waitFor(() => expect(dialog.style.left).toBe("56px"));
  });

  test("animasi: geser pendek (bukan seluruh lebar) dengan pudar, sehingga tidak melintasi Dock atau panel menu", async () => {
    await openPanel();
    const dialog = screen.getByRole("dialog", { name: "Notifikasi" });
    expect(dialog.style.getPropertyValue("--tw-enter-translate-x")).toBe("-0.75rem");
    expect(dialog.style.getPropertyValue("--tw-exit-translate-x")).toBe("-0.75rem");
    expect(dialog.className).toContain("data-[state=open]:fade-in-0");
    expect(dialog.className).toContain("data-[state=closed]:fade-out-0");
    expect(dialog.dataset.placement).toBe("sidebar");
  });
});

describe("NotificationPanel: penempatan di halaman Desks", () => {
  test("placement desks: kartu mengambang di kanan atas di bawah navbar, tanpa offset sidebar", async () => {
    fakeChrome("data-slot=navbar", { bottom: 48 });
    const app = renderOverlay(<DesksHost />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Pemicu" }));
    const dialog = await screen.findByRole("dialog", { name: "Notifikasi" });
    expect(dialog.dataset.placement).toBe("desks");
    expect(dialog.style.left).toBe("auto");
    expect(dialog.style.right).toBe("1rem");
    expect(dialog.style.top).toBe("56px");
    expect(dialog.style.getPropertyValue("--tw-enter-translate-x")).toBe("0");
    expect(dialog.className).toContain("rounded-lg");
    expect(app.history.location.pathname).toBe("/");
  });

  test("terdeteksi otomatis lewat penanda data-shell-chrome desks", async () => {
    const marker = document.createElement("div");
    marker.setAttribute("data-shell-chrome", "desks");
    marker.setAttribute("data-fake-chrome", "");
    document.body.append(marker);
    await openPanel();
    expect(screen.getByRole("dialog", { name: "Notifikasi" }).dataset.placement).toBe("desks");
  });

  test("kartu memuat tab Semua/Peristiwa, tombol tandai semua dan tutup, serta keadaan kosong", async () => {
    const user = userEvent.setup();
    renderOverlay(<DesksHost />);
    await user.click(await screen.findByRole("button", { name: "Pemicu" }));
    const dialog = await screen.findByRole("dialog", { name: "Notifikasi" });
    expect(
      within(dialog)
        .getAllByRole("tab")
        .map((t) => t.textContent),
    ).toEqual(["Semua", "Peristiwa"]);
    expect(
      within(dialog).getByRole("button", { name: "Tandai semua telah dibaca" }),
    ).toBeInTheDocument();
    expect(await within(dialog).findByText("Belum ada notifikasi")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Tutup" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: "Pemicu" })).toHaveFocus();
  });
});

describe("NotificationPanel: jebakan fokus (K11)", () => {
  test("fokus awal berada di dalam panel", async () => {
    await openPanel();
    const dialog = screen.getByRole("dialog", { name: "Notifikasi" });
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
  });

  test("Tab dari elemen terakhir berputar ke elemen pertama dan Shift+Tab sebaliknya (fokus tidak keluar)", async () => {
    const m = memory([mk("a"), mk("b")]);
    const { user } = await openPanel({ notificationSource: m.source });
    const dialog = screen.getByRole("dialog", { name: "Notifikasi" });
    await screen.findByText("Judul a");
    const markAll = screen.getByRole("button", { name: "Tandai semua telah dibaca" });
    const lastRow = screen.getByText("Judul b").closest("button") as HTMLElement;
    act(() => lastRow.focus());
    await user.tab();
    expect(markAll).toHaveFocus();
    await user.tab({ shift: true });
    expect(lastRow).toHaveFocus();
    for (let i = 0; i < 8; i++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
    expect(screen.getByRole("dialog", { name: "Notifikasi" })).toBeInTheDocument();
  });

  test("klik di luar dan Escape tetap menutup meski fokus terjebak", async () => {
    const { user } = await openPanel();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: "Pemicu" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "Pemicu" }));
    await screen.findByRole("dialog", { name: "Notifikasi" });
    await user.click(document.body);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("a11y: dialog non-modal bernama dengan deskripsi; konten lain tidak disembunyikan dari teknologi bantu", async () => {
    await openPanel();
    const dialog = screen.getByRole("dialog", { name: "Notifikasi" });
    expect(dialog).toHaveAccessibleDescription();
    expect(screen.getByRole("button", { name: "Pemicu" })).toBeVisible();
    expect(document.querySelector('[data-slot="sheet-overlay"]')).toBeNull();
  });
});
