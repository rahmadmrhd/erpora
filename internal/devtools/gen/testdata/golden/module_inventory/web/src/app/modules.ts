// Daftar modul frontend. Baris baru disisipkan `erpora make:module --frontend` di atas penanda. Dipakai shell mulai M3 (FR-09).
import { moduleInfo as inventoryInfo } from "@erpora/module-inventory";
// erp:generated:web-imports
export const frontendModules = [
  inventoryInfo,
  // erp:generated:web-modules
] as const;
