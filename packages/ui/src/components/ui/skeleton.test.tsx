import { describe, expect, test } from "bun:test";
import { render } from "@testing-library/react";
import { Skeleton } from "./skeleton";

describe("Skeleton", () => {
  test("blok muted berdenyut yang disembunyikan dari pembaca layar", () => {
    const { container } = render(<Skeleton className="h-7 w-52" />);
    const el = container.firstElementChild as HTMLElement;
    expect(el).toHaveAttribute("aria-hidden", "true");
    expect(el.className).toContain("bg-muted");
    expect(el.className).toContain("animate-pulse");
    expect(el.className).toContain("h-7");
  });

  test("denyut dimatikan pada prefers-reduced-motion (R14.1)", () => {
    const { container } = render(<Skeleton />);
    expect((container.firstElementChild as HTMLElement).className).toContain(
      "motion-reduce:animate-none",
    );
  });

  test("atribut lain diteruskan", () => {
    const { container } = render(<Skeleton data-testid="sk" style={{ width: 10 }} />);
    expect(container.firstElementChild).toHaveAttribute("data-testid", "sk");
  });
});
