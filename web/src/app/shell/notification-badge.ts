/**
 * `99+` bila lebih dari 99. Dipisah dari `notification-panel` agar item Notifikasi di panel menu dapat memakainya
 * tanpa menarik panel (dimuat malas) ke bundel awal.
 */
export function badgeText(count: number): string {
  return count > 99 ? "99+" : String(count);
}
