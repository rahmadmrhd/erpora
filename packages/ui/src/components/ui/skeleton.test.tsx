import { describe, expect, test } from "bun:test";
import { render } from "@testing-library/react";
import { Skeleton } from "./skeleton";

describe("Skeleton", () => {
  test("blok placeholder dengan data-slot dan className pemanggil", () => {
    const { container } = render(<Skeleton className="h-7 w-52" />);
    const el = container.firstElementChild as HTMLElement;
    expect(el).toHaveAttribute("data-slot", "skeleton");
    expect(el.className).toContain("h-7");
  });

  test("atribut lain diteruskan", () => {
    const { container } = render(<Skeleton data-testid="sk" style={{ width: 10 }} />);
    expect(container.firstElementChild).toHaveAttribute("data-testid", "sk");
  });
});
