// Daftar modul frontend. Baris baru disisipkan `erpora make:module --frontend` di atas penanda. Dipakai shell mulai M3 (FR-09).
import { moduleInfo as stockOpnameInfo } from "@erpora/module-stockopname";
// erp:generated:web-imports
export const frontendModules = [
  stockOpnameInfo,
  // erp:generated:web-modules
] as const;
