/**
 * Pintasan keyboard global (M3a R18.3). Satu daftar dipakai bersama: bagian Pintasan di Settings dan popover logo
 * ("Bantuan dan pintasan"). Berkas ini sengaja tanpa dependensi (tidak menarik dialog Settings yang dimuat malas).
 * Pendaftaran penanganan tiap pintasan ada di komponen pemiliknya (palet, panel menu, Dock); daftar ini hanya
 * dokumentasi yang tampil. `id` = akhiran kunci `platform:settings.shortcuts.<id>`.
 */
export const GLOBAL_SHORTCUTS = [
  { id: "openPalette", keys: ["Ctrl", "K"] },
  { id: "showShortcuts", keys: ["Shift", "/"] },
  { id: "toggleSidebar", keys: ["Ctrl", "B"] },
  { id: "openDock", keys: ["Ctrl", "Shift", "/"] },
  { id: "toggleTheme", keys: ["Ctrl", "Shift", "L"] },
] as const;

export type ShortcutId = (typeof GLOBAL_SHORTCUTS)[number]["id"];
