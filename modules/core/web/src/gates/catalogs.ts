import { chooseLocale, flattenCatalog, PREF_KEYS, readPref } from "@erpora/platform-web";
import en from "../../../i18n/en.json";
import id from "../../../i18n/id.json";

/**
 * Katalog `core` yang dibundel bersama gerbang autentikasi. Gerbang dirender SEBELUM bootstrap, sedangkan katalog
 * biasa dimuat dari API setelah login (bergantung pada tenant dan bahasa dari bootstrap), jadi layar auth membawa
 * katalognya sendiri (sumbernya sama dengan yang disajikan backend: `modules/core/i18n`).
 */
export const GATE_CATALOGS = {
  id: { core: flattenCatalog(id) },
  en: { core: flattenCatalog(en) },
} as const;

const SUPPORTED = [{ code: "id" }, { code: "en" }];

/** Bahasa awal gerbang: pilihan tersimpan, lalu `navigator.languages`, lalu Indonesia. */
export function pickGateLocale(): string {
  return chooseLocale({
    defaultLocale: "id",
    locales: SUPPORTED,
    stored: readPref(PREF_KEYS.locale),
    languages: typeof navigator === "undefined" ? [] : (navigator.languages ?? []),
  });
}
