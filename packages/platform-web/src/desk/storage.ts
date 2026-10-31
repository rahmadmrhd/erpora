import { PREF_KEYS, readPref, writePref } from "../prefs";

/**
 * Penyimpanan pilihan Desk (M3a R9.4): hanya id Desk (bukan data sensitif). Nilai yang dibaca tidak tepercaya
 * (divalidasi `isValidDeskId`; pemakai tetap wajib mencocokkannya dengan daftar server), dan semua akses dibungkus
 * `try/catch` (mode privat/diblokir tidak boleh merusak aplikasi).
 */
export const DESK_STORAGE_KEYS = {
  /** `sessionStorage`: Desk per tab, bertahan saat muat ulang tab itu. */
  tab: "erpora.desk.tab",
  /** `localStorage`: Desk terakhir lintas tab. */
  remembered: PREF_KEYS.desk,
} as const;

const DESK_ID = /^[a-z0-9:-]{1,64}$/;

/** `true` hanya untuk string berbentuk id Desk (`^[a-z0-9:-]{1,64}$`). */
export function isValidDeskId(value: unknown): value is string {
  return typeof value === "string" && DESK_ID.test(value);
}

function session(): Storage | null {
  try {
    return globalThis.sessionStorage ?? null;
  } catch {
    return null;
  }
}

/** Id Desk di memori tab ini; `null` bila tidak ada, tidak valid, atau penyimpanan tidak dapat diakses. */
export function readTabDesk(): string | null {
  try {
    const v = session()?.getItem(DESK_STORAGE_KEYS.tab) ?? null;
    return isValidDeskId(v) ? v : null;
  } catch {
    return null;
  }
}

/** Id Desk terakhir di `localStorage`; `null` bila tidak ada, tidak valid, atau tidak dapat diakses. */
export function readRememberedDesk(): string | null {
  const v = readPref(DESK_STORAGE_KEYS.remembered);
  return isValidDeskId(v) ? v : null;
}

/** Menyimpan pilihan Desk di tab dan `localStorage`; id tidak valid atau penyimpanan gagal diabaikan diam-diam. */
export function writeDeskChoice(deskId: string): void {
  if (!isValidDeskId(deskId)) return;
  try {
    session()?.setItem(DESK_STORAGE_KEYS.tab, deskId);
  } catch {
    // diblokir atau penuh: pilihan tab tidak tersimpan, aplikasi tetap berjalan.
  }
  writePref(DESK_STORAGE_KEYS.remembered, deskId);
}
