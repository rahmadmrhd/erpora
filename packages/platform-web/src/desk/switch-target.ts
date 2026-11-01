import { deskContains, deskDashboardPath } from "./match-menu";
import type { Desk } from "./types";

/**
 * Tujuan navigasi saat pengguna memindahkan Desk (ikon Dock Floating, "Pindah Desk", palet; K10): bila rute saat
 * ini juga ada di Desk tujuan, tetap di rute itu (hanya Desk saat ini yang berganti); selain itu Dashboard Desk
 * tujuan. Desk tanpa menu mengembalikan `currentPath` agar tidak ada navigasi ke rute yang tidak ada.
 */
export function resolveDeskSwitchTarget(currentPath: string, targetDesk: Desk): string {
  if (deskContains(targetDesk, currentPath)) return currentPath;
  return deskDashboardPath(targetDesk) ?? currentPath;
}
