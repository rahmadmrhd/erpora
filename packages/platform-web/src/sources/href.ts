// Spasi, karakter kontrol (termasuk \u0000-\u001f, \u007f), dan backslash tidak boleh ada: peramban memperlakukan
// "/\host" seperti "//host" dan kontrol di awal dapat melewati pemeriksaan skema.
const UNSAFE = /[\s\\\p{Cc}]/u;

/**
 * Mengembalikan `href` hanya bila berupa path internal aman (diawali tepat satu "/", bukan "//", tanpa skema,
 * spasi, backslash, atau karakter kontrol); selain itu `null`. Total untuk masukan apa pun (P9). Dipakai sebelum
 * navigasi dari data dinamis (mis. notifikasi) agar tidak terjadi open redirect atau `javascript:`.
 */
export function safeInternalHref(href: unknown): string | null {
  if (typeof href !== "string") return null;
  if (!href.startsWith("/") || href.startsWith("//")) return null;
  if (UNSAFE.test(href)) return null;
  return href;
}
