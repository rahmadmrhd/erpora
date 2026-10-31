import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import type { Desk } from "@erpora/platform-web";
import { makeDashboardItem, makeDesk, makeDeskItem } from "@erpora/platform-web/testing";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Dock, dockChunks } from "./dock";
import { renderShell } from "./shell-test-support";

const desks: Desk[] = [
  makeDesk("example", [
    makeDashboardItem("example", "example"),
    makeDeskItem("example", "notes", "/example/notes"),
  ]),
  makeDesk("billing", [makeDashboardItem("billing", "billing")]),
];

const realLoader = dockChunks.deskMenu;
beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  dockChunks.deskMenu = realLoader;
  dockChunks.reset();
});

describe("ikon Desk Pinned sebelum chunk popover termuat (M-3)", () => {
  test("klik dicatat dan popover terbuka begitu chunk termuat, tanpa berpindah Desk", async () => {
    // Chunk ditahan sampai `release()`: meniru jaringan lambat.
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    dockChunks.deskMenu = () => gate.then(() => realLoader());
    dockChunks.reset();

    const user = userEvent.setup();
    const { router } = renderShell(<Dock />, { desks });
    const icon = await screen.findByRole("button", { name: "Example" });
    expect(icon).not.toHaveAttribute("aria-haspopup");
    await user.click(icon);
    expect(router.state.location.pathname).toBe("/");
    expect(screen.queryByRole("dialog")).toBeNull();
    release();
    const popover = await screen.findByRole("dialog", { name: "Example" });
    expect(within(popover).getByRole("link", { name: "Notes" })).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Example" })).toHaveAttribute(
        "aria-haspopup",
        "dialog",
      ),
    );
  });

  test("fokus pada tombol cadangan dipindahkan ke tombol asli saat chunk termuat", async () => {
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    dockChunks.deskMenu = () => gate.then(() => realLoader());
    dockChunks.reset();

    renderShell(<Dock />, { desks });
    const fallback = await screen.findByRole("button", { name: "Example" });
    fallback.focus();
    expect(fallback).toHaveFocus();
    release();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Example" })).toHaveAttribute(
        "aria-haspopup",
        "dialog",
      ),
    );
    expect(screen.getByRole("button", { name: "Example" })).toHaveFocus();
  });

  test("kegagalan chunk popover lalu coba lagi memuat ulang chunk dan ikon pulih", async () => {
    let attempts = 0;
    dockChunks.deskMenu = () => {
      attempts += 1;
      return attempts <= 2 ? Promise.reject(new Error("chunk gagal")) : realLoader();
    };
    dockChunks.reset();
    const user = userEvent.setup();
    renderShell(<Dock />, { desks });
    // Pra-muat (percobaan gagal) dan muat lazy gagal: ikon menjadi tombol galat kecil.
    const failed = (await screen.findAllByRole("button", { name: /tidak dapat dimuat/ }))[0];
    await user.click(failed as HTMLElement);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Example" })).toHaveAttribute(
        "aria-haspopup",
        "dialog",
      ),
    );
    expect(attempts).toBeGreaterThanOrEqual(3);
  });

  describe("coba lagi hanya me-remount bagian yang gagal (I-2)", () => {
    test("dua ikon gagal, satu dicoba lagi: ikon yang pulih tidak ter-remount saat pindah halaman dan tetap memegang fokus", async () => {
      const err = spyOn(console, "error").mockImplementation(() => {});
      try {
        let ok = false;
        dockChunks.deskMenu = () => (ok ? realLoader() : Promise.reject(new Error("chunk gagal")));
        dockChunks.reset();
        const user = userEvent.setup();
        const { router } = renderShell(<Dock />, { desks });
        await waitFor(() =>
          expect(screen.getAllByRole("button", { name: /tidak dapat dimuat/ })).toHaveLength(2),
        );
        ok = true;
        // Coba lagi pada ikon pertama saja: hanya ia yang pulih.
        await user.click(
          screen.getAllByRole("button", { name: /tidak dapat dimuat/ })[0] as HTMLElement,
        );
        await waitFor(() =>
          expect(screen.getByRole("button", { name: "Example" })).toHaveAttribute(
            "aria-haspopup",
            "dialog",
          ),
        );
        const recovered = screen.getByRole("button", { name: "Example" });
        expect(screen.getAllByRole("button", { name: /tidak dapat dimuat/ })).toHaveLength(1);
        recovered.focus();
        // Berpindah halaman mengulang ikon yang masih gagal (reset chunk), tetapi tidak me-remount ikon yang pulih.
        await act(async () => {
          await router.navigate({ to: "/example/notes" });
        });
        await waitFor(() =>
          expect(screen.getByRole("button", { name: "Billing" })).toBeInTheDocument(),
        );
        const after = screen.getByRole("button", { name: "Example" });
        expect(after).toBe(recovered);
        expect(after).toHaveFocus();
      } finally {
        err.mockRestore();
      }
    });
  });
});
