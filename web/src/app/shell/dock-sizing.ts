/**
 * Satu sumber ukuran Dock (U1): tombol, ikon, dan jarak yang sama dengan item `SidebarMenuButton` di strip ikon
 * Floating (tombol 32 px = `size-8`, ikon 16 px, jarak `gap-1` = 4 px), dan lebar rel mengikuti lebar strip ikon.
 */

/** Ukuran tombol Dock (`size-8`) dan jarak antartombol (`gap-1`), dalam px; dipakai `fitDockCount`. */
export const DOCK_ITEM_SIZE = 32;
export const DOCK_GAP = 4;

/** Nilai `--dock-width` yang dipasang `AppShell`: sama dengan lebar strip ikon sidebar. */
export const SHELL_DOCK_WIDTH = "var(--sidebar-width-icon)";

/**
 * Kelas tombol Dock: ukuran seragam, ikon 16 px, sorotan hover seperti item sidebar, dan sorotan aktif berupa latar
 * (`data-active`, tanpa cincin; K2). Indikator fokus keyboard tetap dari `Button`.
 */
export const DOCK_BUTTON_CLASS =
  "size-8 rounded-md p-0 text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground data-[active=true]:bg-sidebar-accent data-[active=true]:text-sidebar-accent-foreground [&_svg]:size-4";
