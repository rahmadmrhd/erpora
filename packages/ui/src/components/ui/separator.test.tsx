import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Separator } from "./separator";

describe("Separator", () => {
  test("dekoratif secara bawaan: tidak diekspos sebagai separator", () => {
    const { container } = render(<Separator />);
    expect(screen.queryByRole("separator")).toBeNull();
    expect(container.firstElementChild).toHaveAttribute("data-orientation", "horizontal");
  });

  test("decorative={false} berperan separator dengan orientasi", () => {
    render(<Separator decorative={false} orientation="vertical" />);
    const s = screen.getByRole("separator");
    expect(s).toHaveAttribute("aria-orientation", "vertical");
    expect(s).toHaveAttribute("data-orientation", "vertical");
  });

  test("memakai token border dan className digabung", () => {
    const { container } = render(<Separator className="my-2" />);
    const el = container.firstElementChild as HTMLElement;
    expect(el.className).toContain("bg-border");
    expect(el.className).toContain("my-2");
  });
});
