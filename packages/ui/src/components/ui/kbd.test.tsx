import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Kbd, KbdGroup } from "./kbd";

describe("Kbd", () => {
  test("merender elemen <kbd> dengan teks pintasan", () => {
    render(<Kbd>Ctrl</Kbd>);
    const k = screen.getByText("Ctrl");
    expect(k.tagName).toBe("KBD");
    expect(k).toHaveAttribute("data-slot", "kbd");
  });

  test("KbdGroup mengelompokkan beberapa Kbd", () => {
    render(
      <KbdGroup>
        <Kbd>Ctrl</Kbd>
        <Kbd>K</Kbd>
      </KbdGroup>,
    );
    const group = document.querySelector('[data-slot="kbd-group"]');
    expect(group?.querySelectorAll("kbd")).toHaveLength(2);
  });
});
