import { PREF_KEYS, readPref, writePref } from "@erpora/platform-web";

/**
 * Keadaan ciut panel menu (R5.6): disimpan di `localStorage["erpora.sidebar"]` oleh shell. `Sidebar` shadcn persis
 * registry tidak menulis storage (hanya cookie `sidebar_state`), jadi `SidebarProvider` dikendalikan lewat `open`.
 * Nilai selain `collapsed` (termasuk storage diblokir) berarti terbuka.
 */
export function readSidebarOpen(): boolean {
  return readPref(PREF_KEYS.sidebar) !== "collapsed";
}

export function writeSidebarOpen(open: boolean): void {
  writePref(PREF_KEYS.sidebar, open ? "expanded" : "collapsed");
}
