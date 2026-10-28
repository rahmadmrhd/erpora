import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "./collapsible";

function Sample() {
  return (
    <Collapsible>
      <CollapsibleTrigger>Langkah operasi</CollapsibleTrigger>
      <CollapsibleContent>Rincian langkah</CollapsibleContent>
    </Collapsible>
  );
}

describe("Collapsible", () => {
  test("tertutup di awal: aria-expanded=false dan isi tidak tampil", () => {
    render(<Sample />);
    expect(screen.getByRole("button", { name: "Langkah operasi" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.queryByText("Rincian langkah")).toBeNull();
  });

  test.each(["{Enter}", " "])("pemicu %j membuka dan menutup bagian", async (key) => {
    const user = userEvent.setup();
    render(<Sample />);
    const trigger = screen.getByRole("button", { name: "Langkah operasi" });
    trigger.focus();
    await user.keyboard(key);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Rincian langkah")).toBeInTheDocument();
    await user.keyboard(key);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });

  test("klik memicu data-state open", async () => {
    const user = userEvent.setup();
    render(<Sample />);
    await user.click(screen.getByRole("button", { name: "Langkah operasi" }));
    expect(screen.getByRole("button", { name: "Langkah operasi" })).toHaveAttribute(
      "data-state",
      "open",
    );
  });
});
