import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Kbd, KbdGroup } from "./kbd";

describe("Kbd", () => {
  test("elemen <kbd> berisi teks tombol dengan token muted", () => {
    render(<Kbd>Ctrl</Kbd>);
    const k = screen.getByText("Ctrl");
    expect(k.tagName).toBe("KBD");
    expect(k.className).toContain("bg-muted");
    expect(k.className).toContain("pointer-events-none");
    expect(k.className).toContain("select-none");
  });

  test("KbdGroup membungkus beberapa Kbd", () => {
    const { container } = render(
      <KbdGroup>
        <Kbd>Ctrl</Kbd>
        <Kbd>B</Kbd>
      </KbdGroup>,
    );
    expect(container.querySelectorAll("kbd")).toHaveLength(2);
    expect((container.firstElementChild as HTMLElement).className).toContain("inline-flex");
  });
});
