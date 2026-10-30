import { useBootstrap } from "../bootstrap";

/**
 * Petunjuk UX saja: backend tetap otoritas dan menolak aksi tak berhak (M3a K-30, R14.3). `permissions` bootstrap
 * kosong berarti "belum ada informasi" (sebelum PRD-001) sehingga hasilnya optimistis `true`; bila berisi, hanya
 * permission yang tercantum (pencocokan persis, tanpa wildcard) yang `true`.
 */
export function usePermission(permission: string): boolean {
  const { permissions } = useBootstrap();
  if (permissions.length === 0) return true;
  return permissions.includes(permission);
}
