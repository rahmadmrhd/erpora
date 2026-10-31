import { describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { UiLabelsProvider } from "../../labels";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "./dialog";

const labels = { "actions.close": "Tutup dialog" } as Record<string, string>;
function Labels({ children }: { children: ReactNode }) {
  return <UiLabelsProvider t={(k) => labels[k] ?? k}>{children}</UiLabelsProvider>;
}

function Sample({ showCloseButton = true }: { showCloseButton?: boolean }) {
  return (
    <Labels>
      <Dialog>
        <DialogTrigger>Buka</DialogTrigger>
        <DialogContent showCloseButton={showCloseButton}>
          <DialogHeader>
            <DialogTitle>Riwayat modul</DialogTitle>
            <DialogDescription>Daftar operasi terakhir.</DialogDescription>
          </DialogHeader>
          <input aria-label="Cari" />
          <DialogFooter>
            <button type="button">Batal</button>
            <button type="button">Simpan</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Labels>
  );
}

describe("Dialog", () => {
  test("berperan dialog, bernama dari judul, dan dideskripsikan oleh deskripsi", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Buka" }));
    const dialog = await screen.findByRole("dialog", { name: "Riwayat modul" });
    expect(dialog).toHaveAccessibleDescription("Daftar operasi terakhir.");
  });

  test("Esc menutup dan fokus kembali ke pemicu", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Buka" }));
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.getByRole("button", { name: "Buka" })).toHaveFocus();
  });

  test("fokus terperangkap: Tab berputar di dalam dialog", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Buka" }));
    const dialog = await screen.findByRole("dialog");
    for (let i = 0; i < 8; i++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
  });

  test("tombol tutup berlabel dari UiLabels (actions.close) dan menutup dialog", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Buka" }));
    await user.click(await screen.findByRole("button", { name: "Tutup dialog" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("showCloseButton={false} menyembunyikan tombol tutup", async () => {
    const user = userEvent.setup();
    render(<Sample showCloseButton={false} />);
    await user.click(screen.getByRole("button", { name: "Buka" }));
    await screen.findByRole("dialog");
    expect(screen.queryByRole("button", { name: "Tutup dialog" })).toBeNull();
  });
});
