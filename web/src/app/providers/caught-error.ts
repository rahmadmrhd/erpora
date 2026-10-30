import { ModuleInactiveError } from "../pages/pages";
import { isHandledByShellBoundary } from "../shell/local-boundary";

/**
 * Penanganan galat yang ditangkap batas galat React (`createRoot({ onCaughtError })`). Bawaan React mencatat setiap galat
 * tertangkap sebagai `console.error`; `ModuleInactiveError` (guard modul nonaktif) adalah keadaan yang diharapkan dan
 * menampilkan halaman "Modul Tidak Aktif", jadi hanya dicatat sebagai `debug`. Galat lain tetap `console.error`.
 */
export function handleCaughtError(error: unknown, info: { componentStack?: string }): void {
  // Galat bagian shell (Navbar, pengumuman rute) sudah dicatat sekali oleh batas lokalnya (tanpa stack).
  if (isHandledByShellBoundary(error)) return;
  if (error instanceof ModuleInactiveError) {
    console.debug(`[erpora] ${error.message}`);
    return;
  }
  console.error(error, info.componentStack ?? "");
}
