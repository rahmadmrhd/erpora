import { describe, expect, test } from "bun:test";
import { render, screen, waitFor } from "@testing-library/react";
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

  test("muncul saat hover", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.hover(screen.getByRole("button", { name: "Simpan" }));
    expect(await screen.findByRole("tooltip")).toBeInTheDocument();
  });

  test("hilang saat fokus pindah dari pemicu (blur)", async () => {
    const user = userEvent.setup();
    render(
      <>
        <Sample />
        <button type="button">Lain</button>
      </>,
    );
    await user.tab();
    await screen.findByRole("tooltip");
    await user.tab();
    await waitFor(() => expect(screen.queryByRole("tooltip")).toBeNull());
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
});
