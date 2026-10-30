import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Badge } from "./badge";

describe("Badge", () => {
  test("pil kecil bernama dengan varian default (primary)", () => {
    render(<Badge>Baru</Badge>);
    const b = screen.getByText("Baru");
    expect(b).toHaveAttribute("data-variant", "default");
    expect(b.className).toContain("bg-primary");
    expect(b.className).toContain("rounded-full");
  });

  test.each([
    ["secondary", "bg-secondary"],
    ["destructive", "bg-destructive"],
    ["outline", "border-border"],
  ] as const)("varian %s memakai token %s", (variant, token) => {
    render(<Badge variant={variant}>x</Badge>);
    expect(screen.getByText("x").className).toContain(token);
  });

  test("asChild merender elemen anak", () => {
    render(
      <Badge asChild>
        <a href="/x">Tautan</a>
      </Badge>,
    );
    expect(screen.getByRole("link", { name: "Tautan" })).toBeInTheDocument();
  });
});
