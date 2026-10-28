import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { BadgeStatus, type StatusTone } from "./badge-status";

describe("BadgeStatus", () => {
  test.each([
    ["primary", "status-primary"],
    ["success", "status-success"],
    ["warning", "status-warning"],
    ["error", "status-error"],
    ["secondary", "status-secondary"],
  ] as const)("nada %s memakai token %s-bg dan -fg", (tone: StatusTone, token) => {
    render(<BadgeStatus tone={tone}>Label</BadgeStatus>);
    const b = screen.getByText("Label");
    expect(b).toHaveAttribute("data-tone", tone);
    expect(b.className).toContain(`bg-${token}-bg`);
    expect(b.className).toContain(`text-${token}-fg`);
  });

  test("label selalu teks dari children (status tidak bergantung pada warna saja)", () => {
    render(<BadgeStatus tone="success">Disetujui</BadgeStatus>);
    expect(screen.getByText("Disetujui")).toBeInTheDocument();
  });

  test("bentuk pil dengan teks 14/20 medium", () => {
    render(<BadgeStatus tone="primary">x</BadgeStatus>);
    const cls = screen.getByText("x").className;
    expect(cls).toContain("rounded-full");
    expect(cls).toContain("text-sm");
    expect(cls).toContain("font-medium");
  });
});
