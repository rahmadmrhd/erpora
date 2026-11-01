import { useSidebar, useUiLabel } from "@erpora/ui";
import type { KeyboardEvent } from "react";

/** Id panel menu yang dikendalikan pembatas (`aria-controls`). */
export const DESK_PANEL_ID = "desk-panel";

// Posisi tepi pembatas mengikuti transisi sidebar shadcn (200 ms linear) agar bergerak bersama panel.
const LEFT = {
  pinned: {
    open: "calc(var(--dock-width) + var(--sidebar-width))",
    collapsed: "var(--dock-width)",
  },
  floating: { open: "var(--sidebar-width)", collapsed: "var(--sidebar-width-icon)" },
} as const;

/**
 * Pembatas panel menu (R5.5, R5.6): `role="separator"` fokusable (pola window splitter yang dapat diciutkan) dengan
 * area klik 16 px dan kursor resize. Klik, Enter, atau Spasi menciutkan atau membuka. Pinned: panel saja yang
 * hilang (Dock tetap); Floating: panel menjadi strip ikon (kendali `collapsible` ada di `Sidebar`). Tidak
 * dirender di bawah 1024 px (panel berupa Sheet).
 */
export function CollapseEdge({ mode }: { mode: "pinned" | "floating" }) {
  const { open, toggleSidebar } = useSidebar();
  const t = useUiLabel();
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      toggleSidebar();
    }
  };
  return (
    // biome-ignore lint/a11y/useSemanticElements: pembatas yang dapat difokus (window splitter) tidak punya padanan elemen semantik; <hr> tidak interaktif
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={t("sidebar.toggle")}
      aria-controls={DESK_PANEL_ID}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={open ? 100 : 0}
      tabIndex={0}
      data-slot="collapse-edge"
      data-state={open ? "expanded" : "collapsed"}
      style={{ left: open ? LEFT[mode].open : LEFT[mode].collapsed }}
      onClick={toggleSidebar}
      onKeyDown={onKeyDown}
      className={[
        "group/edge fixed inset-y-0 z-20 hidden w-4 -translate-x-1/2 outline-none transition-[left] duration-200 ease-linear motion-reduce:transition-none md:block",
        open ? "cursor-w-resize" : "cursor-e-resize",
        "after:absolute after:inset-y-0 after:left-1/2 after:w-0.5 after:-translate-x-1/2 hover:after:bg-sidebar-border focus-visible:after:bg-sidebar-ring",
      ].join(" ")}
    />
  );
}
