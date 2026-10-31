/** Teks lencana: kosong bila tidak ada jumlah, `99+` bila lebih dari 99. */
export function formatBadgeCount(count: number): string {
  if (!Number.isFinite(count) || count <= 0) return "";
  const whole = Math.floor(count);
  if (whole <= 0) return "";
  return whole > 99 ? "99+" : String(whole);
}
