import type { PlatformComponents } from "@erpora/platform-contracts";

type RawBootstrap = PlatformComponents["schemas"]["BootstrapEnvelope"]["data"];

/** Bendera fitur dari bootstrap (aditif; M3a K-19). Bidang yang tidak ada diperlakukan `false`. */
export interface BootstrapFeatures {
  /** API mutasi `/api/platform/modules` aktif (`ERP_ENV=development` atau `ERP_MODULES_ADMIN_API=true`). */
  modulesAdminApi?: boolean;
}

/** Respons `GET /api/bootstrap` (bagian `data`); tipe dasar dari kontrak hasil generate. */
export type Bootstrap = RawBootstrap & { features?: BootstrapFeatures };
export type BootstrapModule = Bootstrap["modules"][number];
export type ModuleStatus = BootstrapModule["status"];
