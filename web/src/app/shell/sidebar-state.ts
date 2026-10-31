import { PREF_KEYS, readPref, writePref } from "@erpora/platform-web";

/**
 * Keadaan ciut panel menu (R5.6): disimpan di `localStorage["erpora.sidebar"]` oleh shell lewat `onOpenChange`
 * `SidebarProvider`, yang dikendalikan lewat `open`. `Sidebar` shadcn registry menulis cookie `sidebar_state`; patch
 * perilaku terdaftar K12 (`shadcn.patches.json`) menghapus penulisan cookie itu, jadi `erpora.sidebar` satu-satunya
 * penyimpanan. Nilai selain `collapsed` (termasuk storage diblokir) berarti terbuka.
 */
export function readSidebarOpen(): boolean {
  return readPref(PREF_KEYS.sidebar) !== "collapsed";
}

export function writeSidebarOpen(open: boolean): void {
  writePref(PREF_KEYS.sidebar, open ? "expanded" : "collapsed");
}
