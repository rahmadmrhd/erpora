/**
 * Preferensi UI di `localStorage` (M3a R14.2, revisi): hanya kunci di bawah ini; tidak boleh menyimpan rahasia,
 * token, atau data pengguna. Semua akses dibungkus `try/catch` (akses diblokir/penuh tidak boleh merusak aplikasi).
 * `dock` = mode Dock (revisi R4.6); `desk` = id Desk terakhir (R9.1 tahap 2, lihat `desk/storage`);
 * `timezone` = zona waktu IANA pilihan pengguna (revisi R17.3; tidak ada = ikuti peramban).
 */
export const PREF_KEYS = {
  theme: "erpora.theme",
  sidebar: "erpora.sidebar",
  locale: "erpora.locale",
  dock: "erpora.dock",
  desk: "erpora.desk",
  timezone: "erpora.timezone",
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

/** `true` hanya untuk string berupa zona waktu IANA yang dikenal `Intl` (nilai tersimpan tidak tepercaya). */
export function isValidTimezone(value: unknown): value is string {
  if (typeof value !== "string" || value === "" || value.length > 64) return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value });
    return true;
  } catch {
    return false;
  }
}

/** Zona waktu pilihan pengguna; `null` (otomatis, ikuti peramban) bila tidak ada, tidak valid, atau tidak dapat diakses. */
export function readTimezone(): string | null {
  const v = readPref(PREF_KEYS.timezone);
  return isValidTimezone(v) ? v : null;
}

/** Menyimpan zona waktu; `null` menghapus pilihan (otomatis). Nilai tidak valid diabaikan. */
export function writeTimezone(timezone: string | null): void {
  if (timezone === null) removePref(PREF_KEYS.timezone);
  else if (isValidTimezone(timezone)) writePref(PREF_KEYS.timezone, timezone);
}

/** Zona waktu efektif: pilihan tersimpan, selain itu zona peramban, selain itu `UTC`. Tidak pernah kosong. */
export function resolveTimezone(): string {
  const chosen = readTimezone();
  if (chosen !== null) return chosen;
  try {
    const tz = new Intl.DateTimeFormat().resolvedOptions().timeZone;
    return isValidTimezone(tz) ? tz : "UTC";
  } catch {
    return "UTC";
  }
}

type PrefListener = (key: string | null) => void;
const listeners = new Set<PrefListener>();
let storageHooked = false;

function emit(key: string | null): void {
  for (const l of [...listeners]) {
    try {
      l(key);
    } catch {
      // pelanggan yang gagal tidak boleh menghentikan pelanggan lain atau penulisan preferensi.
    }
  }
}

function onStorage(e: StorageEvent): void {
  emit(e.key);
}

/**
 * Berlangganan perubahan preferensi: dipanggil seketika setelah `writePref`/`removePref` di tab ini dan saat tab lain
 * mengubah `localStorage` (event `storage`; `null` = penyimpanan dikosongkan). Argumen = kunci yang berubah.
 * Mengembalikan fungsi berhenti. Dasar `useDockMode`/`useTimezone` (shell memantau mode Dock tanpa muat ulang).
 */
export function subscribePrefs(listener: PrefListener): () => void {
  listeners.add(listener);
  if (!storageHooked && typeof window !== "undefined") {
    window.addEventListener("storage", onStorage);
    storageHooked = true;
  }
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && storageHooked && typeof window !== "undefined") {
      window.removeEventListener("storage", onStorage);
      storageHooked = false;
    }
  };
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
  emit(key);
}

/** Menghapus preferensi; diam-diam gagal bila penyimpanan diblokir. */
export function removePref(key: string): void {
  try {
    storage()?.removeItem(key);
  } catch {
    // diblokir: tidak ada yang dihapus, aplikasi tetap berjalan.
  }
  emit(key);
}
