import { describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UiLabelsProvider } from "../../labels";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "./sheet";

function Sample({ side }: { side?: "left" | "right" | "top" | "bottom" }) {
  return (
    <UiLabelsProvider t={(k) => (k === "actions.close" ? "Tutup panel" : k)}>
      <Sheet>
        <SheetTrigger>Menu</SheetTrigger>
        <SheetContent side={side}>
          <SheetHeader>
            <SheetTitle>Navigasi</SheetTitle>
            <SheetDescription>Menu utama</SheetDescription>
          </SheetHeader>
        </SheetContent>
      </Sheet>
    </UiLabelsProvider>
  );
}

describe("Sheet", () => {
  test("side=left: dialog bernama dari judul dengan data-side=left", async () => {
    const user = userEvent.setup();
    render(<Sample side="left" />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    const dialog = await screen.findByRole("dialog", { name: "Navigasi" });
    expect(dialog).toHaveAttribute("data-side", "left");
    expect(dialog.className).toContain("left-0");
  });

  test("bawaan side=right", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    expect(await screen.findByRole("dialog")).toHaveAttribute("data-side", "right");
  });

  test("tombol tutup berlabel dari UiLabels dan menutup panel", async () => {
    const user = userEvent.setup();
    render(<Sample side="left" />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    await user.click(await screen.findByRole("button", { name: "Tutup panel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("Esc menutup panel", async () => {
    const user = userEvent.setup();
    render(<Sample side="left" />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  test("permukaan background dan overlay token", async () => {
    const user = userEvent.setup();
    render(<Sample side="left" />);
    await user.click(screen.getByRole("button", { name: "Menu" }));
    expect((await screen.findByRole("dialog")).className).toContain("bg-background");
    expect(document.querySelector('[data-slot="sheet-overlay"]')?.className).toContain(
      "bg-overlay",
    );
  });
});
