import { describe, expect, test } from "bun:test";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CollapseEdge } from "./collapse-edge";
import { renderShell } from "./shell-test-support";

function setup(mode: "pinned" | "floating", initial = true) {
  let current = initial;
  const states: boolean[] = [];
  const result = renderShell(<CollapseEdge mode={mode} />, {
    sidebar: {
      open: initial,
      onOpenChange: (open) => {
        current = open;
        states.push(open);
      },
    },
  });
  return { ...result, states, current: () => current };
}

describe("CollapseEdge", () => {
  test("pemisah vertikal yang dapat difokus dengan nama, aria-controls, dan nilai (terbuka = 100)", async () => {
    setup("pinned");
    const edge = await screen.findByRole("separator");
    expect(edge).toHaveAttribute("aria-orientation", "vertical");
    expect(edge).toHaveAttribute("tabindex", "0");
    expect(edge).toHaveAttribute("aria-label", "Ciutkan atau perluas sidebar");
    expect(edge).toHaveAttribute("aria-controls", "desk-panel");
    expect(edge).toHaveAttribute("aria-valuenow", "100");
  });

  test("area klik 16 px (w-4) dengan kursor resize", async () => {
    setup("pinned");
    const edge = await screen.findByRole("separator");
    expect(edge.className).toContain("w-4");
    expect(edge.className).toContain("cursor-w-resize");
  });

  test("klik, Enter, dan Spasi mengalihkan keadaan", async () => {
    const user = userEvent.setup();
    const { states } = setup("floating");
    const edge = await screen.findByRole("separator");
    await user.click(edge);
    expect(states).toEqual([false]);
    edge.focus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(states).toEqual([false, false, false]);
  });

  test("keadaan tertutup: nilai 0 dan kursor berubah ke arah buka", async () => {
    setup("pinned", false);
    const edge = await screen.findByRole("separator");
    expect(edge).toHaveAttribute("aria-valuenow", "0");
    expect(edge.className).toContain("cursor-e-resize");
  });

  test.each([
    ["pinned", true, "calc(var(--dock-width) + var(--sidebar-width))"],
    ["pinned", false, "var(--dock-width)"],
    ["floating", true, "var(--sidebar-width)"],
    ["floating", false, "var(--sidebar-width-icon)"],
  ] as const)("posisi %s terbuka=%p di %s", async (mode, open, left) => {
    setup(mode, open);
    const edge = await screen.findByRole("separator");
    expect(edge.style.left).toBe(left);
  });

  test("tombol lain tidak memengaruhi (fokus tidak mencuri Spasi dari elemen lain)", async () => {
    const { states } = setup("pinned");
    const edge = await screen.findByRole("separator");
    fireEvent.keyDown(edge, { key: "a" });
    await waitFor(() => expect(states).toEqual([]));
  });
});
