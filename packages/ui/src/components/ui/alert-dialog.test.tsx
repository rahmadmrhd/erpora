import { describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "./alert-dialog";

function Sample({ onAction }: { onAction?: () => void }) {
  return (
    <AlertDialog>
      <AlertDialogTrigger>Nonaktifkan</AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Nonaktifkan modul?</AlertDialogTitle>
          <AlertDialogDescription>Menu dan route modul akan hilang.</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Batal</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onAction}>
            Nonaktifkan
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

describe("AlertDialog", () => {
  test("berperan alertdialog bernama dari judul; fokus awal pada Batal", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Nonaktifkan modul?" });
    expect(dialog).toHaveAccessibleDescription("Menu dan route modul akan hilang.");
    await waitFor(() => expect(screen.getByRole("button", { name: "Batal" })).toHaveFocus());
  });

  test("Esc TIDAK menutup (hanya Batal atau aksi)", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    await screen.findByRole("alertdialog");
    await user.keyboard("{Escape}");
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  test("klik di luar tidak menutup", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    await screen.findByRole("alertdialog");
    const overlay = document.querySelector('[data-slot="alert-dialog-overlay"]') as HTMLElement;
    await user.click(overlay);
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });

  test("tidak ada tombol tutup silang", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    await screen.findByRole("alertdialog");
    expect(screen.getAllByRole("button").map((b) => b.textContent)).toEqual([
      "Batal",
      "Nonaktifkan",
    ]);
  });

  test("Batal menutup tanpa aksi; Aksi memanggil handler lalu menutup", async () => {
    const user = userEvent.setup();
    let n = 0;
    render(<Sample onAction={() => n++} />);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    await user.click(await screen.findByRole("button", { name: "Batal" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(n).toBe(0);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    await user.click(
      await screen
        .findByRole("button", { name: "Nonaktifkan", hidden: false })
        .then(() => screen.getAllByRole("button", { name: "Nonaktifkan" }).at(-1) as HTMLElement),
    );
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(n).toBe(1);
  });

  test("fokus terperangkap di dalam dialog", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    const dialog = await screen.findByRole("alertdialog");
    for (let i = 0; i < 5; i++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
  });

  test("aksi merusak memakai varian destructive pada tombol", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Nonaktifkan" }));
    await screen.findByRole("alertdialog");
    const action = screen.getAllByRole("button", { name: "Nonaktifkan" }).at(-1) as HTMLElement;
    expect(action).toHaveAttribute("data-variant", "destructive");
  });
});
