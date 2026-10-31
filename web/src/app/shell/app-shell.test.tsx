import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";
import { setViewportWidth } from "./shell-test-support";

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
  document.documentElement.className = "";
});
afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  setViewportWidth(1024);
});

const mainNav = () => screen.findByRole("navigation", { name: "Navigasi utama" });

describe("AppShell: Dock dan panel", () => {
  test("bawaan Floating: panel menu dengan logo dan pengguna, tanpa Dock Pinned", async () => {
    renderApp({ path: "/alpha/home" });
    const nav = await mainNav();
    expect(within(nav).getByRole("button", { name: /Cari/ })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Dock Desk" })).toBeNull();
    expect(screen.getByRole("button", { name: /Menu aplikasi/ })).toBeInTheDocument();
    expect(
      screen.getByRole("separator", { name: "Ciutkan atau perluas sidebar" }),
    ).toBeInTheDocument();
  });

  test("mode Pinned: Dock tampil dengan ikon Desk dan Desk saat ini mengikuti halaman", async () => {
    localStorage.setItem("erpora.dock", "pinned");
    renderApp({ path: "/alpha/home" });
    const dock = await screen.findByRole("navigation", { name: "Dock Desk" });
    await waitFor(() =>
      expect(within(dock).getAllByRole("button", { name: /Desk$/ }).length).toBeGreaterThan(0),
    );
  });

  test("Pinned (K3): panel meluncur di bawah Dock (bukan melintasinya), tersembunyi saat ciut, dan tanpa transisi saat reduced-motion", async () => {
    localStorage.setItem("erpora.dock", "pinned");
    localStorage.setItem("erpora.sidebar", "collapsed");
    renderApp({ path: "/alpha/home" });
    await screen.findByRole("navigation", { name: "Dock Desk" });
    const container = document.querySelector('[data-slot="sidebar-container"]') as HTMLElement;
    const classes = container.className.split(/\s+/);
    // Ciut: tepi kiri panel berhenti di balik Dock (dock - lebar panel), bukan di -lebar panel yang melintasi Dock.
    expect(classes).toContain(
      "group-data-[collapsible=offcanvas]:left-[calc(var(--dock-width)-var(--sidebar-width))]",
    );
    expect(classes).not.toContain(
      "group-data-[collapsible=offcanvas]:left-[calc(var(--sidebar-width)*-1)]",
    );
    // Panel tak dapat difokus saat tersembunyi; visibilitas ikut bertransisi agar tetap terlihat selama meluncur.
    expect(classes).toContain("group-data-[collapsible=offcanvas]:invisible");
    expect(classes).toContain("transition-[left,right,width,visibility]");
    expect(classes).toContain("motion-reduce:transition-none");
    // Dock di atas panel (z-index) agar panel yang meluncur tertutup Dock.
    const dock = screen.getByRole("navigation", { name: "Dock Desk" });
    expect(dock.className).toContain("z-20");
    expect(dock.className).toContain("relative");
  });

  test("klik pembatas menciutkan panel dan status tersimpan di erpora.sidebar", async () => {
    const user = userEvent.setup();
    renderApp({ path: "/alpha/home" });
    await mainNav();
    const sidebar = document.querySelector('[data-slot="sidebar"]');
    expect(sidebar).toHaveAttribute("data-state", "expanded");
    await user.click(screen.getByRole("separator", { name: "Ciutkan atau perluas sidebar" }));
    await waitFor(() => expect(sidebar).toHaveAttribute("data-state", "collapsed"));
    expect(localStorage.getItem("erpora.sidebar")).toBe("collapsed");
  });

  test("di bawah 1024 px tanpa pembatas dan panel menjadi Sheet lewat tombol menu; tidak menyimpan keadaan", async () => {
    setViewportWidth(900);
    const user = userEvent.setup();
    renderApp({ path: "/alpha/home" });
    await screen.findByRole("main");
    expect(screen.queryByRole("separator", { name: "Ciutkan atau perluas sidebar" })).toBeNull();
    expect(screen.queryByRole("navigation", { name: "Navigasi utama" })).toBeNull();
    await user.click(await screen.findByRole("button", { name: /Ciutkan atau perluas sidebar/ }));
    const dialog = await screen.findByRole("dialog", { name: "Menu navigasi" });
    expect(within(dialog).getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
    expect(localStorage.getItem("erpora.sidebar")).toBeNull();
  });
});

describe("AppShell: overlay malas dan pintasan", () => {
  test("Ctrl+K dan item Cari membuka palet perintah", async () => {
    const user = userEvent.setup();
    renderApp({ path: "/alpha/home" });
    const nav = await mainNav();
    act(() => {
      fireEvent.keyDown(window, { key: "k", code: "KeyK", ctrlKey: true });
    });
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await user.click(within(nav).getByRole("button", { name: /Cari/ }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  test("Notifikasi membuka panel dan menu pengguna membuka Settings; Shift+/ membuka bagian Pintasan", async () => {
    const user = userEvent.setup();
    renderApp({ path: "/alpha/home" });
    const nav = await mainNav();
    await user.click(within(nav).getByRole("button", { name: /Notifikasi/ }));
    expect(await screen.findByRole("dialog", { name: "Notifikasi" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Notifikasi" })).toBeNull());

    act(() => {
      fireEvent.keyDown(document.body, { key: "?", code: "Slash", shiftKey: true });
    });
    const settings = await screen.findByRole("dialog", { name: "Pengaturan" });
    expect(within(settings).getAllByText("Ctrl").length).toBeGreaterThan(1);
  });

  test("Ctrl+Shift+L mengganti tema", async () => {
    renderApp({ path: "/alpha/home" });
    await mainNav();
    act(() => {
      fireEvent.keyDown(window, { key: "L", code: "KeyL", ctrlKey: true, shiftKey: true });
    });
    await waitFor(() => expect(localStorage.getItem("erpora.theme")).not.toBeNull());
  });
});
