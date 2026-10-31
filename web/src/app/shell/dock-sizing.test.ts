import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { buttonVariants, cn } from "@erpora/ui";
import { DOCK_BUTTON_CLASS, DOCK_GAP, DOCK_ITEM_SIZE } from "./dock-sizing";

// Sumber komponen dibaca dari berkas (bukan `import`) agar aturan F2 (tanpa deep import ke packages/*) tidak terpicu.
const repoRoot = join(import.meta.dir, "../../../..");
const sidebarSource = readFileSync(
  join(repoRoot, "packages/ui/src/components/ui/sidebar.tsx"),
  "utf8",
);

describe("DOCK_BUTTON_CLASS (K17)", () => {
  // Kelas yang benar-benar dipakai `Button` ghost: cn(buttonVariants({ ... className })).
  const merged = cn(
    buttonVariants({ variant: "ghost", size: "icon", className: DOCK_BUTTON_CLASS }),
  ).split(/\s+/);

  test("sorotan hover memakai sidebar-accent di tema terang dan gelap (menimpa dark:hover:bg-accent/50 varian ghost)", () => {
    expect(merged).toContain("hover:bg-sidebar-accent");
    expect(merged).toContain("hover:text-sidebar-accent-foreground");
    expect(merged).toContain("dark:hover:bg-sidebar-accent");
    expect(merged).toContain("dark:hover:text-sidebar-accent-foreground");
    expect(merged).not.toContain("dark:hover:bg-accent/50");
    expect(merged).not.toContain("hover:bg-accent");
  });

  test("sorotan aktif berupa latar sidebar-accent dan tidak hilang saat hover", () => {
    expect(merged).toContain("data-[active=true]:bg-sidebar-accent");
    expect(merged).toContain("data-[active=true]:text-sidebar-accent-foreground");
  });
});

describe("ukuran Dock setara strip ikon sidebar Floating (K15)", () => {
  test("DOCK_ITEM_SIZE sama dengan ukuran tombol strip ikon pada SidebarMenuButton shadcn", () => {
    const match = /group-data-\[collapsible=icon\]:size-(\d+)!/.exec(sidebarSource);
    expect(match).not.toBeNull();
    // Skala spasi Tailwind: 1 unit = 4 px.
    expect(Number(match?.[1]) * 4).toBe(DOCK_ITEM_SIZE);
    expect(DOCK_BUTTON_CLASS).toContain(`size-${DOCK_ITEM_SIZE / 4}`);
  });

  test("DOCK_GAP sama dengan jarak antaritem SidebarMenu shadcn", () => {
    const menu = /data-slot="sidebar-menu"[\s\S]*?className=\{cn\("([^"]*)"/.exec(sidebarSource);
    expect(menu).not.toBeNull();
    const gap = /\bgap-(\d+)\b/.exec(menu?.[1] ?? "");
    expect(Number(gap?.[1]) * 4).toBe(DOCK_GAP);
  });
});
