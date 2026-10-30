import { PREF_KEYS, readPref } from "../prefs";

// Pola kode bahasa registri (contracts/module.schema.json): "id", "en", "en-GB". Menolak nilai tersimpan yang aneh.
const LOCALE_CODE = /^[a-z]{2,3}(-[A-Z]{2})?$/;

const base = (tag: string): string => tag.split("-")[0]?.toLowerCase() ?? "";

/**
 * Mencocokkan satu tag bahasa (mis. dari `navigator.languages`) ke kode terdaftar: persis (tidak peka huruf), lalu
 * kode yang sama dengan bahasa dasar tag ("en-US" -> "en"), lalu kode mana pun berbahasa dasar sama ("pt" -> "pt-BR").
 */
export function matchLocale(tag: string, codes: readonly string[]): string | undefined {
  if (tag === "") return undefined;
  const lower = tag.toLowerCase();
  const exact = codes.find((c) => c.toLowerCase() === lower);
  if (exact !== undefined) return exact;
  const b = base(tag);
  if (b === "" || b === "*") return undefined;
  return codes.find((c) => c.toLowerCase() === b) ?? codes.find((c) => base(c) === b);
}

export interface ChooseLocaleInput {
  defaultLocale: string;
  locales: readonly { code: string }[];
  /** Nilai `localStorage["erpora.locale"]` (belum divalidasi). */
  stored: string | null;
  /** `navigator.languages`. */
  languages: readonly string[];
}

/**
 * Bahasa awal: pilihan tersimpan (hanya bila terdaftar), lalu `navigator.languages`, lalu bahasa bawaan.
 * Hasil selalu anggota daftar bahasa atau bahasa bawaan.
 */
export function chooseLocale(input: ChooseLocaleInput): string {
  const codes = input.locales.map((l) => l.code);
  if (codes.length === 0) return input.defaultLocale;
  if (input.stored !== null && codes.includes(input.stored)) return input.stored;
  for (const lang of input.languages) {
    const hit = matchLocale(lang, codes);
    if (hit !== undefined) return hit;
  }
  return input.defaultLocale;
}

let active: string | undefined;

/** Dipanggil `I18nProvider` agar klien API mengirim `Accept-Language` yang sama dengan bahasa UI. */
export function setActiveLocale(locale: string | undefined): void {
  active = locale;
}

/**
 * Bahasa untuk header `Accept-Language` (`createPlatformClient({ getLocale: getActiveLocale })`): bahasa aktif
 * provider; sebelum provider ada (mis. permintaan bootstrap) memakai pilihan tersimpan yang berbentuk kode bahasa;
 * selain itu `undefined` (peramban mengirim bahasanya sendiri).
 */
export function getActiveLocale(): string | undefined {
  if (active !== undefined) return active;
  const stored = readPref(PREF_KEYS.locale);
  return stored !== null && LOCALE_CODE.test(stored) ? stored : undefined;
}
