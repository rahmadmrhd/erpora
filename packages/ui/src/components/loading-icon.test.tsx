import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { LoadingIcon } from "./loading-icon";

describe("LoadingIcon", () => {
  test("menggambar delapan titik lingkaran", () => {
    const { container } = render(<LoadingIcon />);
    expect(container.querySelectorAll("svg circle")).toHaveLength(8);
  });

  test("animasi CSS: tanpa elemen SMIL <animate>/<animateTransform> (agar patuh reduced-motion)", () => {
    const { container } = render(<LoadingIcon />);
    expect(container.querySelector("animate")).toBeNull();
    expect(container.querySelector("animateTransform")).toBeNull();
    const circles = [...container.querySelectorAll("svg circle")];
    for (const c of circles) expect(c.getAttribute("class")).toContain("erpora-loading-dot");
    const delays = circles.map((c) => (c as SVGElement).style.animationDelay);
    expect(new Set(delays).size).toBe(8);
  });

  test("aria-label diteruskan dan memberi peran img; tanpa label disembunyikan dari pembaca layar", () => {
    const { rerender } = render(<LoadingIcon aria-label="Memuat" />);
    expect(screen.getByRole("img", { name: "Memuat" })).toBeInTheDocument();
    rerender(<LoadingIcon />);
    expect(screen.queryByRole("img")).toBeNull();
    expect(document.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });

  test("className digabung dan memakai currentColor", () => {
    const { container } = render(<LoadingIcon className="size-6 text-primary" />);
    const svg = container.querySelector("svg");
    expect(svg?.getAttribute("class")).toContain("size-6");
    expect(svg?.getAttribute("class")).toContain("text-primary");
    expect(container.querySelector("svg circle")?.getAttribute("fill")).toBe("currentColor");
  });
});
