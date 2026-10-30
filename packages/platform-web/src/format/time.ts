const SECOND = 1;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const MONTH = 30 * DAY;
const YEAR = 365 * DAY;

function relativeFormat(locale: string): Intl.RelativeTimeFormat {
  try {
    return new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  } catch {
    return new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  }
}

/**
 * Waktu relatif ("5 menit yang lalu") dari ISO 8601 UTC; satuan dipilih menurut selisih. Masukan yang bukan tanggal
 * valid dikembalikan apa adanya (tidak melempar); locale tidak valid memakai "en".
 */
export function formatRelativeTime(iso: string, locale: string, now: number = Date.now()): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  const rtf = relativeFormat(locale);
  const diff = Math.round((t - now) / 1000);
  const abs = Math.abs(diff);
  if (abs < 10) return rtf.format(0, "second");
  if (abs < MINUTE) return rtf.format(diff, "second");
  if (abs < HOUR) return rtf.format(Math.round(diff / MINUTE), "minute");
  if (abs < DAY) return rtf.format(Math.round(diff / HOUR), "hour");
  if (abs < MONTH) return rtf.format(Math.round(diff / DAY), "day");
  if (abs < YEAR) return rtf.format(Math.round(diff / MONTH), "month");
  return rtf.format(Math.round(diff / YEAR), "year");
}

/** Tanggal dan jam lokal ringkas dari ISO 8601 UTC; masukan tidak valid dikembalikan apa adanya. */
export function formatDateTime(iso: string, locale: string): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  try {
    return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(t);
  } catch {
    return new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(t);
  }
}
