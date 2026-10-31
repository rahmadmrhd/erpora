/**
 * Preferensi UI di `localStorage` (M3a R14.2, revisi): hanya kunci di bawah ini; tidak boleh menyimpan rahasia,
 * token, atau data pengguna. Semua akses dibungkus `try/catch` (akses diblokir/penuh tidak boleh merusak aplikasi).
 * `dock` = mode Dock (revisi R4.6); `desk` = id Desk terakhir (R9.1 tahap 2, lihat `desk/storage`).
 */
export const PREF_KEYS = {
  theme: "erpora.theme",
  sidebar: "erpora.sidebar",
  locale: "erpora.locale",
  dock: "erpora.dock",
  desk: "erpora.desk",
} as const;

/** Mode Dock: `floating` (bawaan, pil melayang) atau `pinned` (kolom tetap). */
export type DockMode = "pinned" | "floating";

/** Mode Dock tersimpan; nilai tidak dikenal atau penyimpanan tidak dapat diakses menghasilkan `floating`. */
export function readDockMode(): DockMode {
  return readPref(PREF_KEYS.dock) === "pinned" ? "pinned" : "floating";
}

export function writeDockMode(mode: DockMode): void {
  writePref(PREF_KEYS.dock, mode);
}

function storage(): Storage | null {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

/** Membaca preferensi; `null` bila tidak ada atau penyimpanan tidak dapat diakses. */
export function readPref(key: string): string | null {
  try {
    return storage()?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

/** Menulis preferensi; diam-diam gagal bila penyimpanan diblokir atau penuh. */
export function writePref(key: string, value: string): void {
  try {
    storage()?.setItem(key, value);
  } catch {
    // diblokir atau kuota habis: preferensi tidak tersimpan, aplikasi tetap berjalan.
  }
}
