// Daftar modul frontend. Baris baru disisipkan `erpora make:module --frontend` di atas penanda. Dipakai shell mulai M3 (FR-09).
import { frontendModule as exampleModule } from "@erpora/module-example";
import { frontendModule as exampleaddonModule } from "@erpora/module-exampleaddon";
// erp:generated:web-imports
export const frontendModules = [
  exampleModule,
  exampleaddonModule,
  // erp:generated:web-modules
] as const;
