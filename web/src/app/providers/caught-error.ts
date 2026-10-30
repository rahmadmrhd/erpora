import { ModuleInactiveError } from "../pages/pages";

/**
 * Penanganan galat yang ditangkap batas galat React (`createRoot({ onCaughtError })`). Bawaan React mencatat setiap galat
 * tertangkap sebagai `console.error`; `ModuleInactiveError` (guard modul nonaktif) adalah keadaan yang diharapkan dan
 * menampilkan halaman "Modul Tidak Aktif", jadi hanya dicatat sebagai `debug`. Galat lain tetap `console.error`.
 */
export function handleCaughtError(error: unknown, info: { componentStack?: string }): void {
  if (error instanceof ModuleInactiveError) {
    console.debug(`[erpora] ${error.message}`);
    return;
  }
  console.error(error, info.componentStack ?? "");
}
