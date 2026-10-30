import { describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";

function Sample() {
  return (
    <>
      <button type="button">Di luar</button>
      <Popover>
        <PopoverTrigger>Buka panel</PopoverTrigger>
        <PopoverContent>
          <p>Isi panel</p>
          <button type="button">Aksi dalam</button>
        </PopoverContent>
      </Popover>
    </>
  );
}

describe("Popover", () => {
  test("dibuka dengan klik; pemicu mengumumkan aria-expanded", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    const trigger = screen.getByRole("button", { name: "Buka panel" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    await user.click(trigger);
    expect(await screen.findByText("Isi panel")).toBeInTheDocument();
    expect(trigger).toHaveAttribute("aria-expanded", "true");
  });

  test("ditutup dengan Esc dan fokus kembali ke pemicu", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Buka panel" }));
    await screen.findByText("Isi panel");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByText("Isi panel")).toBeNull());
    expect(screen.getByRole("button", { name: "Buka panel" })).toHaveFocus();
  });

  test("ditutup dengan klik di luar", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Buka panel" }));
    await screen.findByText("Isi panel");
    await user.click(screen.getByRole("button", { name: "Di luar" }));
    await waitFor(() => expect(screen.queryByText("Isi panel")).toBeNull());
  });

  test("permukaan token popover, lebar 288 px bawaan", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Buka panel" }));
    await screen.findByText("Isi panel");
    const panel = document.querySelector('[data-slot="popover-content"]') as HTMLElement;
    expect(panel.className).toContain("bg-popover");
    expect(panel.className).toContain("w-72");
  });
});
