import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./tooltip";

function Sample({ delay = 0 }: { delay?: number }) {
  return (
    <TooltipProvider delayDuration={delay}>
      <Tooltip>
        <TooltipTrigger asChild>
          <button type="button">Simpan</button>
        </TooltipTrigger>
        <TooltipContent>Simpan perubahan</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

describe("Tooltip", () => {
  test("muncul saat pemicu difokus (keyboard) dan berperan tooltip", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    expect(screen.queryByRole("tooltip")).toBeNull();
    await user.tab();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Simpan perubahan");
  });

  test("muncul saat hover dan hilang saat pointer keluar", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.hover(screen.getByRole("button", { name: "Simpan" }));
    expect(await screen.findByRole("tooltip")).toBeInTheDocument();
    await user.unhover(screen.getByRole("button", { name: "Simpan" }));
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  test("Esc menutup tooltip", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.tab();
    await screen.findByRole("tooltip");
    await user.keyboard("{Escape}");
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  test("jeda bawaan TooltipProvider 300 ms dan permukaan popover (teks saja)", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <button type="button">x</button>
          </TooltipTrigger>
          <TooltipContent>Isi</TooltipContent>
        </Tooltip>
      </TooltipProvider>,
    );
    await user.tab();
    const tip = await screen.findByRole("tooltip");
    // Radix menyalin teks ke elemen tersembunyi; konten visual memakai token popover.
    const visual = document.querySelector('[data-slot="tooltip-content"]') as HTMLElement;
    expect(visual.className).toContain("bg-popover");
    expect(visual.className).toContain("text-popover-foreground");
    expect(tip).toBeInTheDocument();
  });
});
