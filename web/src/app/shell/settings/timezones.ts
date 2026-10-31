/**
 * Zona waktu yang ditawarkan di Settings. `Select` tanpa pencarian tidak layak untuk ±400 zona IANA, jadi daftar
 * dipersempit ke zona yang umum (Indonesia lebih dulu). Pilihan tersimpan yang valid tetapi tidak ada di daftar
 * tetap ditampilkan (`timezoneOptions`); pencarian penuh menunggu `Command` (M3c).
 */
export const COMMON_TIMEZONES = [
  "Asia/Jakarta",
  "Asia/Makassar",
  "Asia/Jayapura",
  "UTC",
  "Asia/Singapore",
  "Asia/Kuala_Lumpur",
  "Asia/Bangkok",
  "Asia/Manila",
  "Asia/Hong_Kong",
  "Asia/Shanghai",
  "Asia/Tokyo",
  "Asia/Seoul",
  "Asia/Kolkata",
  "Asia/Dubai",
  "Asia/Riyadh",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "Europe/Amsterdam",
  "Europe/Istanbul",
  "Europe/Moscow",
  "Africa/Cairo",
  "Africa/Johannesburg",
  "Australia/Perth",
  "Australia/Sydney",
  "Pacific/Auckland",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Sao_Paulo",
] as const;

/** `GMT+7`, `GMT+5:30`, `GMT`; kosong bila `Intl` tidak mendukung. */
export function timezoneOffsetLabel(zone: string, locale: string): string {
  try {
    const parts = new Intl.DateTimeFormat(locale, {
      timeZone: zone,
      timeZoneName: "shortOffset",
    }).formatToParts(new Date());
    return parts.find((p) => p.type === "timeZoneName")?.value ?? "";
  } catch {
    return "";
  }
}

/** Daftar opsi: zona umum, dengan zona tersimpan di awal bila belum ada di daftar. */
export function timezoneOptions(selected: string | null): string[] {
  const list: string[] = [...COMMON_TIMEZONES];
  if (selected !== null && !list.includes(selected)) list.unshift(selected);
  return list;
}
