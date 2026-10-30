/**
 * Preferensi UI di `localStorage` (M3a R14.2): hanya tiga kunci di bawah ini; tidak boleh menyimpan rahasia, token,
 * atau data pengguna. Semua akses dibungkus `try/catch` (akses diblokir/penuh tidak boleh merusak aplikasi).
 */
export const PREF_KEYS = {
  theme: "erpora.theme",
  sidebar: "erpora.sidebar",
  locale: "erpora.locale",
} as const;

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
