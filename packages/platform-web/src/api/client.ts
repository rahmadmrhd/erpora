import {
  type ClientOptions,
  createClient,
  type PlatformPaths,
  type TypedClient,
} from "@erpora/platform-contracts";

export type PlatformClientOptions = ClientOptions;

/**
 * Klien API platform bertipe (tipe `data` dari kontrak hasil generate). `getLocale` menambahkan `Accept-Language`
 * pada setiap request (native di klien kontrak). Klien tidak pernah mengirim `tenantId`/`userId` (tenant ditentukan
 * backend; aturan IDOR).
 */
export function createPlatformClient(opts: PlatformClientOptions = {}): TypedClient<PlatformPaths> {
  return createClient<PlatformPaths>(opts);
}
