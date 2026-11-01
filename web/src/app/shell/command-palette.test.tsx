import { afterEach, beforeEach, describe, expect, mock, spyOn, test } from "bun:test";
import type { Desk } from "@erpora/platform-web";
import {
  makeDashboardItem,
  makeDesk,
  makeDeskGroup,
  makeDeskItem,
} from "@erpora/platform-web/testing";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CommandPalette } from "./command-palette";
import { renderShell } from "./shell-test-support";

const desks: Desk[] = [
  makeDesk(
    "platform",
    [
      makeDashboardItem("platform", "platform"),
      makeDeskItem("platform", "modules", "/platform/modules"),
    ],
    { source: "platform", icon: "settings" },
  ),
  makeDesk(
    "example",
    [
      makeDashboardItem("example", "example"),
      makeDeskItem("example", "notes", "/example/notes"),
      makeDeskGroup("example", "reports", [
        makeDeskItem("example", "sales", "/example/reports/sales"),
      ]),
    ],
    { icon: "file-text" },
  ),
  makeDesk("billing", [makeDashboardItem("billing", "billing")], { icon: "receipt" }),
];

beforeEach(() => {
  localStorage.clear();
  sessionStorage.setItem("erpora.desk.tab", "example");
});
afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

/** Opsi menurut teks (nama aksesibel opsi bersorot terpecah oleh elemen <mark> di happy-dom). */
function optionByText(container: HTMLElement, text: RegExp): HTMLElement {
  const found = within(container)
    .getAllByRole("option")
    .find((o) => text.test(o.textContent ?? ""));
  if (found === undefined) throw new Error(`opsi ${text} tidak ditemukan`);
  return found;
}

async function openPalette(onOpenChange: (open: boolean) => void = () => {}) {
  const user = userEvent.setup();
  const view = renderShell(<CommandPalette open onOpenChange={onOpenChange} />, {
    desks,
    path: "/example/notes",
  });
  const dialog = await screen.findByRole("dialog", { name: "Palet perintah" });
  // menunggu daftar Desk dari server tiruan
  await within(dialog).findByRole("option", { name: /Billing/ });
  return { user, dialog, ...view };
}

describe("CommandPalette: isi dan sumber lokal", () => {
  test("Command dalam Dialog bernama; kolom cari berlabel; sumber: menu Desk saat ini dan semua Desk", async () => {
    const { dialog } = await openPalette();
    expect(
      within(dialog).getByRole("combobox", { name: "Cari atau ketik perintah" }),
    ).toBeInTheDocument();
    const pages = within(dialog).getByRole("group", { name: "Halaman" });
    expect(
      within(pages)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Dashboard", "Notes", "SalesReports"]);
    const deskGroup = within(dialog).getByRole("group", { name: "Desk" });
    expect(
      within(deskGroup)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Platform", "Example", "Billing"]);
    // halaman Desk lain tidak tampil sebelum ada kueri
    expect(within(dialog).queryByRole("option", { name: /Modules/ })).toBeNull();
  });

  test("footer memuat petunjuk pintasan (Kbd)", async () => {
    const { dialog } = await openPalette();
    const kbds = Array.from(dialog.querySelectorAll("kbd")).map((k) => k.textContent);
    expect(kbds).toEqual(expect.arrayContaining(["↑", "↓", "↵", "Esc"]));
    expect(within(dialog).getByText("Pilih")).toBeInTheDocument();
    expect(within(dialog).getByText("Buka")).toBeInTheDocument();
    expect(within(dialog).getByText("Tutup")).toBeInTheDocument();
  });

  test("tanpa kueri backend selama membuka dan mengetik", async () => {
    const fetchSpy = spyOn(globalThis, "fetch");
    try {
      const { user, dialog } = await openPalette();
      const before = fetchSpy.mock.calls.length;
      await user.type(within(dialog).getByRole("combobox"), "note");
      expect(fetchSpy.mock.calls.length).toBe(before);
    } finally {
      fetchSpy.mockRestore();
    }
  });
});

describe("CommandPalette: pencarian dan sorotan", () => {
  test("mengetik menyaring dan menyorot teks yang cocok", async () => {
    const { user, dialog } = await openPalette();
    await user.type(within(dialog).getByRole("combobox"), "not");
    const options = within(dialog).getAllByRole("option");
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent("Notes");
    const marks = Array.from(options[0]?.querySelectorAll("mark") ?? []);
    expect(marks.map((m) => m.textContent)).toEqual(["Not"]);
  });

  test("dengan kueri, halaman Desk lain ikut dicari dan dikelompokkan menurut nama Desk", async () => {
    const { user, dialog } = await openPalette();
    await user.type(within(dialog).getByRole("combobox"), "modul");
    const group = await within(dialog).findByRole("group", { name: "Platform" });
    const option = optionByText(group, /Modules/);
    expect(Array.from(option.querySelectorAll("mark")).map((m) => m.textContent)).toEqual([
      "Modul",
    ]);
  });

  test("semua kata kueri harus cocok (urutan bebas) dan tidak peka huruf besar-kecil", async () => {
    const { user, dialog } = await openPalette();
    await user.type(within(dialog).getByRole("combobox"), "REPORTS sal");
    const options = within(dialog).getAllByRole("option");
    expect(options).toHaveLength(1);
    expect(options[0]).toHaveTextContent("Sales");
  });

  test("tanpa hasil: pesan kosong", async () => {
    const { user, dialog } = await openPalette();
    await user.type(within(dialog).getByRole("combobox"), "zzzz");
    expect(within(dialog).getByText("Tidak ada hasil.")).toBeInTheDocument();
    expect(within(dialog).queryAllByRole("option")).toHaveLength(0);
  });
});

describe("CommandPalette: pemilihan", () => {
  test("halaman di Desk saat ini: navigasi dan tutup, Desk tidak berubah", async () => {
    const onOpenChange = mock(() => {});
    const { user, dialog, router } = await openPalette(onOpenChange);
    await user.click(within(dialog).getByRole("option", { name: /Sales/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/example/reports/sales"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
  });

  test("halaman di Desk lain: setCurrentDesk dulu, lalu navigasi", async () => {
    const { user, dialog, router } = await openPalette();
    await user.type(within(dialog).getByRole("combobox"), "modul");
    await within(dialog).findByRole("group", { name: "Platform" });
    await user.click(optionByText(dialog, /Modules/));
    await waitFor(() => expect(router.state.location.pathname).toBe("/platform/modules"));
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("platform");
  });

  test("entri Desk: menjadikannya Desk saat ini dan membuka Dashboard-nya", async () => {
    const { user, dialog, router } = await openPalette();
    await user.click(within(dialog).getByRole("option", { name: /Billing/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/billing"));
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("billing");
  });

  test("keyboard: panah bawah lalu Enter memilih item", async () => {
    const { user, dialog, router } = await openPalette();
    await user.click(within(dialog).getByRole("combobox"));
    await user.keyboard("{ArrowDown}{Enter}");
    await waitFor(() => expect(router.state.location.pathname).toBe("/example/notes"));
  });

  test("Escape menutup", async () => {
    const onOpenChange = mock(() => {});
    const { user } = await openPalette(onOpenChange);
    await user.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  test("kueri direset saat palet dibuka kembali", async () => {
    const user = userEvent.setup();
    const view = renderShell(<CommandPalette open onOpenChange={() => {}} />, {
      desks,
      path: "/example/notes",
    });
    const dialog = await screen.findByRole("dialog", { name: "Palet perintah" });
    await within(dialog).findByRole("option", { name: /Billing/ });
    await user.type(within(dialog).getByRole("combobox"), "bill");
    expect(within(dialog).getByRole("combobox")).toHaveValue("bill");
    view.unmount();
    renderShell(<CommandPalette open onOpenChange={() => {}} />, { desks, path: "/example/notes" });
    const again = await screen.findByRole("dialog", { name: "Palet perintah" });
    expect(within(again).getByRole("combobox")).toHaveValue("");
  });
});

describe("CommandPalette: pindah Desk menurut K10 (tetap di rute bila ada di Desk tujuan)", () => {
  // `shared` memuat halaman /example/notes juga (menu Desk kustom/timpaan); `billing` tidak.
  const k10Desks: Desk[] = [
    ...desks,
    makeDesk("shared", [
      makeDashboardItem("shared", "shared"),
      makeDeskItem("shared", "notes", "/example/notes"),
    ]),
  ];

  async function open(path: string) {
    const user = userEvent.setup();
    const view = renderShell(<CommandPalette open onOpenChange={() => {}} />, {
      desks: k10Desks,
      path,
    });
    const dialog = await screen.findByRole("dialog", { name: "Palet perintah" });
    await within(dialog).findByRole("option", { name: /Billing/ });
    return { user, dialog, ...view };
  }

  test("entri Desk yang juga memuat rute saat ini: tetap di rute itu, hanya Desk saat ini berganti", async () => {
    const { user, dialog, router } = await open("/example/notes");
    const deskGroup = within(dialog).getByRole("group", { name: "Desk" });
    await user.click(optionByText(deskGroup, /Shared/));
    await waitFor(() => expect(sessionStorage.getItem("erpora.desk.tab")).toBe("shared"));
    await new Promise((r) => setTimeout(r, 100));
    expect(router.state.location.pathname).toBe("/example/notes");
  });

  test("pindah Desk ke rute yang sama menjaga query dan hash serta tidak menambah entri riwayat (M-1)", async () => {
    const { user, dialog, router, history } = await open("/example/notes?page=3#x");
    const lengthBefore = history.length;
    const deskGroup = within(dialog).getByRole("group", { name: "Desk" });
    await user.click(optionByText(deskGroup, /Shared/));
    await waitFor(() => expect(sessionStorage.getItem("erpora.desk.tab")).toBe("shared"));
    await new Promise((r) => setTimeout(r, 100));
    expect(router.state.location.pathname).toBe("/example/notes");
    expect(router.state.location.searchStr).toBe("?page=3");
    expect(router.state.location.hash).toBe("x");
    expect(history.length).toBe(lengthBefore);
  });

  test("entri Desk yang tidak memuat rute saat ini: Dashboard Desk tujuan", async () => {
    const { user, dialog, router } = await open("/example/notes");
    await user.click(within(dialog).getByRole("option", { name: /Billing/ }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/billing"));
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("billing");
  });

  test("entri Desk saat ini sendiri tidak memindahkan halaman", async () => {
    const { user, dialog, router } = await open("/example/reports/sales");
    const deskGroup = within(dialog).getByRole("group", { name: "Desk" });
    await user.click(optionByText(deskGroup, /Example/));
    await new Promise((r) => setTimeout(r, 100));
    expect(router.state.location.pathname).toBe("/example/reports/sales");
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
  });

  test("halaman spesifik (hasil pencarian) tetap menavigasi ke halaman itu walau ada di beberapa Desk", async () => {
    const { user, dialog, router } = await open("/billing");
    await user.type(within(dialog).getByRole("combobox"), "modul");
    await within(dialog).findByRole("group", { name: "Platform" });
    await user.click(optionByText(dialog, /Modules/));
    await waitFor(() => expect(router.state.location.pathname).toBe("/platform/modules"));
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("platform");
  });
});
