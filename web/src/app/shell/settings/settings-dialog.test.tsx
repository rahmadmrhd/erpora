import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { renderOverlay } from "../overlay-test-support";
import { installMatchMedia, restoreMatchMedia } from "./match-media-test-support";
import { SettingsDialog, type SettingsSection } from "./settings-dialog";

beforeEach(() => {
  localStorage.clear();
  document.documentElement.classList.remove("light", "dark");
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

const DESKTOP_WIDTH = 1280;
const PHONE_WIDTH = 375;

function setViewportWidth(width: number) {
  installMatchMedia(width);
}

afterEach(() => {
  localStorage.clear();
  restoreMatchMedia();
});

function Host({ initialSection }: { initialSection?: SettingsSection }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Pemicu
      </button>
      <SettingsDialog open={open} onOpenChange={setOpen} initialSection={initialSection} />
    </>
  );
}

async function openSettings(initialSection?: SettingsSection, locale: "id" | "en" = "id") {
  renderOverlay(<Host initialSection={initialSection} />, { locale });
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Pemicu" }));
  const dialog = await screen.findByRole("dialog");
  return { user, dialog };
}

describe("SettingsDialog: struktur", () => {
  test("tertutup: tidak merender dialog", async () => {
    renderOverlay(<Host />);
    await screen.findByRole("button", { name: "Pemicu" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  test("dialog bernama Pengaturan dengan navigasi kiri Tampilan, Preferensi, Pintasan", async () => {
    await openSettings();
    expect(screen.getByRole("dialog", { name: "Pengaturan" })).toBeInTheDocument();
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(["Tampilan", "Preferensi", "Pintasan"]);
    expect(screen.getByRole("tab", { name: "Tampilan" })).toHaveAttribute("aria-selected", "true");
  });

  test("bagian dibuka lewat initialSection", async () => {
    await openSettings("shortcuts");
    expect(screen.getByRole("tab", { name: "Pintasan" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Buka palet perintah")).toBeInTheDocument();
  });

  test("garis pembatas navigasi/konten ada di kolom navigasi (setinggi penuh), bukan di daftar tab yang setinggi isinya (K7)", async () => {
    await openSettings();
    const list = screen.getByRole("tablist");
    const column = list.parentElement as HTMLElement;
    expect(column.dataset.slot).toBe("settings-nav");
    expect(column.className).toContain("md:border-r");
    expect(list.className).not.toContain("border-r");
    expect(column.parentElement).toBe(
      screen.getByRole("dialog").querySelector('[data-slot="tabs"]'),
    );
  });

  test("desktop: dialog besar, navigasi tab vertikal di kiri dan konten kanan menggulir sendiri", async () => {
    setViewportWidth(DESKTOP_WIDTH);
    await openSettings();
    const dialog = screen.getByRole("dialog");
    expect(dialog.className).toContain("sm:max-w-4xl");
    expect(dialog.className).toContain("xl:max-w-5xl");
    expect(dialog.className).toContain("h-[min(40rem,calc(100svh-2rem))]");
    const list = screen.getByRole("tablist");
    expect(list).toHaveAttribute("aria-orientation", "vertical");
    const tabs = dialog.querySelector('[data-slot="tabs"]') as HTMLElement;
    expect(tabs.dataset.orientation).toBe("vertical");
    expect(tabs.className).toContain("md:grid-cols-[14rem_1fr]");
    expect(screen.getByRole("tabpanel").parentElement?.className).toContain("overflow-y-auto");
  });

  test("ponsel: navigasi menjadi tab horizontal di atas konten (dapat digulir), dialog hampir layar penuh", async () => {
    setViewportWidth(PHONE_WIDTH);
    const { user } = await openSettings();
    const dialog = screen.getByRole("dialog");
    const list = screen.getByRole("tablist");
    // Tata letak horizontal sudah berlaku pada render pertama dialog (tanpa kedip vertikal).
    expect(list).toHaveAttribute("aria-orientation", "horizontal");
    const column = list.parentElement as HTMLElement;
    expect(column.dataset.slot).toBe("settings-nav");
    expect(column.className).toContain("overflow-x-auto");
    expect(column.className).toContain("border-b");
    expect(column.className).not.toContain("border-r");
    expect(dialog.className).toContain("max-w-[calc(100%-1rem)]");
    expect(dialog.className).toContain("h-[calc(100svh-1rem)]");
    const tabs = dialog.querySelector('[data-slot="tabs"]') as HTMLElement;
    expect(tabs.dataset.orientation).toBe("horizontal");
    // Panah kiri/kanan berpindah bagian (orientasi horizontal).
    act(() => screen.getByRole("tab", { name: "Tampilan" }).focus());
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("tab", { name: "Preferensi" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  test("keyboard: panah berpindah antar bagian", async () => {
    const { user } = await openSettings();
    act(() => screen.getByRole("tab", { name: "Tampilan" }).focus());
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("tab", { name: "Preferensi" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  test("Escape menutup dan fokus kembali ke pemicu", async () => {
    const { user } = await openSettings();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: "Pemicu" })).toHaveFocus();
  });
});

describe("SettingsDialog: Tampilan", () => {
  test("Tema: tiga kartu (Otomatis, Terang, Gelap) dengan Otomatis terpilih bawaan", async () => {
    await openSettings();
    const group = screen.getByRole("radiogroup", { name: "Tema" });
    expect(within(group).getAllByRole("radio")).toHaveLength(3);
    expect(within(group).getByRole("radio", { name: "Otomatis" })).toBeChecked();
    expect(within(group).getByRole("radio", { name: "Terang" })).not.toBeChecked();
    expect(within(group).getByRole("radio", { name: "Gelap" })).not.toBeChecked();
  });

  test("memilih tema Gelap tersimpan seketika dan berlaku seketika", async () => {
    const { user } = await openSettings();
    await user.click(screen.getByRole("radio", { name: "Gelap" }));
    expect(localStorage.getItem("erpora.theme")).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(screen.getByRole("radio", { name: "Gelap" })).toBeChecked();
    await user.click(screen.getByRole("radio", { name: "Terang" }));
    expect(localStorage.getItem("erpora.theme")).toBe("light");
    expect(document.documentElement.classList.contains("light")).toBe(true);
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  test("tema tersimpan sebelumnya dibaca sebagai pilihan awal", async () => {
    localStorage.setItem("erpora.theme", "dark");
    await openSettings();
    expect(screen.getByRole("radio", { name: "Gelap" })).toBeChecked();
  });

  test("Dock: Melayang bawaan; memilih Tersemat tersimpan seketika", async () => {
    const { user } = await openSettings();
    const group = screen.getByRole("radiogroup", { name: "Dock" });
    expect(within(group).getByRole("radio", { name: "Melayang" })).toBeChecked();
    await user.click(within(group).getByRole("radio", { name: "Tersemat" }));
    expect(localStorage.getItem("erpora.dock")).toBe("pinned");
    expect(within(group).getByRole("radio", { name: "Tersemat" })).toBeChecked();
    await user.click(within(group).getByRole("radio", { name: "Melayang" }));
    expect(localStorage.getItem("erpora.dock")).toBe("floating");
  });

  test("perubahan mode Dock dari tab lain (event storage) ikut tampil", async () => {
    await openSettings();
    act(() => {
      localStorage.setItem("erpora.dock", "pinned");
      window.dispatchEvent(new StorageEvent("storage", { key: "erpora.dock" }));
    });
    expect(screen.getByRole("radio", { name: "Tersemat" })).toBeChecked();
  });

  test("pratinjau kartu disembunyikan dari teknologi bantu (nama radio hanya labelnya)", async () => {
    await openSettings();
    const radio = screen.getByRole("radio", { name: "Terang" });
    const card = radio.closest("label") as HTMLElement;
    expect(card.querySelector('[aria-hidden="true"]')).not.toBeNull();
    expect(card).toHaveTextContent("Terang");
  });

  test("tanpa pengaturan Layout (ditunda ke M3d)", async () => {
    await openSettings();
    expect(screen.queryByText(/Compact|Full Width|Layout/i)).toBeNull();
  });
});

describe("SettingsDialog: Preferensi", () => {
  async function openPreferences() {
    const r = await openSettings();
    await r.user.click(screen.getByRole("tab", { name: "Preferensi" }));
    return r;
  }

  test("Bahasa dan Zona waktu tersedia", async () => {
    await openPreferences();
    expect(screen.getByRole("combobox", { name: "Bahasa" })).toHaveTextContent("Indonesia");
    expect(screen.getByRole("combobox", { name: "Zona waktu" })).toHaveTextContent("Otomatis");
  });

  test("mengganti Bahasa memanggil runtime i18n (label ikut berubah)", async () => {
    const { user } = await openPreferences();
    await user.click(screen.getByRole("combobox", { name: "Bahasa" }));
    await user.click(await screen.findByRole("option", { name: "English" }));
    expect(await screen.findByRole("combobox", { name: "Language" })).toHaveTextContent("English");
  });

  test("memilih Zona waktu tersimpan di erpora.timezone dan Otomatis menghapusnya", async () => {
    const { user } = await openPreferences();
    await user.click(screen.getByRole("combobox", { name: "Zona waktu" }));
    await user.click(await screen.findByRole("option", { name: /Asia\/Jakarta/ }));
    expect(localStorage.getItem("erpora.timezone")).toBe("Asia/Jakarta");
    expect(screen.getByRole("combobox", { name: "Zona waktu" })).toHaveTextContent("Asia/Jakarta");
    await user.click(screen.getByRole("combobox", { name: "Zona waktu" }));
    await user.click(await screen.findByRole("option", { name: /Otomatis/ }));
    expect(localStorage.getItem("erpora.timezone")).toBeNull();
  });

  test("zona waktu tersimpan yang valid tetapi tidak ada di daftar tetap ditampilkan", async () => {
    localStorage.setItem("erpora.timezone", "Atlantic/Reykjavik");
    await openPreferences();
    expect(screen.getByRole("combobox", { name: "Zona waktu" })).toHaveTextContent(
      "Atlantic/Reykjavik",
    );
  });

  test("zona waktu tersimpan tidak valid diabaikan (Otomatis)", async () => {
    localStorage.setItem("erpora.timezone", "Mars/Olympus");
    await openPreferences();
    expect(screen.getByRole("combobox", { name: "Zona waktu" })).toHaveTextContent("Otomatis");
  });
});

describe("SettingsDialog: Pintasan", () => {
  test("daftar pintasan global: palet, tampilkan pintasan, ciutkan panel, buka Dock, ganti tema", async () => {
    await openSettings("shortcuts");
    const rows = screen.getAllByRole("listitem");
    const text = rows.map((r) => r.textContent ?? "");
    expect(
      text.some((t) => t.includes("Buka palet perintah") && t.includes("Ctrl") && t.includes("K")),
    ).toBe(true);
    expect(text.some((t) => t.includes("Tampilkan pintasan"))).toBe(true);
    expect(text.some((t) => t.includes("Ciutkan atau perluas panel") && t.includes("B"))).toBe(
      true,
    );
    expect(text.some((t) => t.includes("Buka Dock") && t.includes("Shift"))).toBe(true);
    expect(text.some((t) => t.includes("Ganti tema"))).toBe(true);
  });
});

describe("SettingsDialog: bahasa Inggris", () => {
  test("judul dan bagian mengikuti katalog en", async () => {
    await openSettings(undefined, "en");
    expect(screen.getByRole("dialog", { name: "Settings" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Appearance" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Preferences" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Shortcuts" })).toBeInTheDocument();
  });
});
