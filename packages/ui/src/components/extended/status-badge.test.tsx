import { describe, expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { StatusBadge, type StatusTone } from "./status-badge";

describe("StatusBadge (pembungkus Badge bernada status)", () => {
  test("label dari children, nada ditandai data-tone, dan dirender sebagai Badge", () => {
    render(<StatusBadge tone="success">Aktif</StatusBadge>);
    const b = screen.getByText("Aktif");
    expect(b).toHaveAttribute("data-tone", "success");
    expect(b).toHaveAttribute("data-slot", "badge");
  });

  test.each(["primary", "success", "warning", "error", "secondary"] as StatusTone[])(
    "nada %s memetakan ke kelas token status-%s",
    (tone) => {
      render(<StatusBadge tone={tone}>x</StatusBadge>);
      const cls = screen.getByText("x").className;
      expect(cls).toContain(`bg-status-${tone}-bg`);
      expect(cls).toContain(`text-status-${tone}-fg`);
    },
  );

  test("className pemanggil digabung", () => {
    render(
      <StatusBadge tone="warning" className="ml-2">
        x
      </StatusBadge>,
    );
    expect(screen.getByText("x").className).toContain("ml-2");
  });
});
