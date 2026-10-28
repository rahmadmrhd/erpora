import {
  type ClientOptions,
  createClient,
  type PlatformPaths,
  type TypedClient,
} from "@erpora/platform-contracts";

export interface PlatformClientOptions extends Omit<ClientOptions, "headers"> {
  /** Bahasa aktif untuk `Accept-Language` (mis. dari runtime i18n); kosong berarti header tidak dikirim. */
  getLocale?: () => string | undefined;
  /** Header tambahan per request. */
  headers?: () => Record<string, string>;
}

/**
 * Klien API platform bertipe (tipe `data` dari kontrak hasil generate). Menambahkan `Accept-Language` dari
 * `getLocale`. Klien tidak pernah mengirim `tenantId`/`userId` (tenant ditentukan backend; aturan IDOR).
 */
export function createPlatformClient(opts: PlatformClientOptions = {}): TypedClient<PlatformPaths> {
  const { getLocale, headers, ...rest } = opts;
  return createClient<PlatformPaths>({
    ...rest,
    headers: () => {
      const locale = getLocale?.();
      return {
        ...(locale ? { "Accept-Language": locale } : {}),
        ...headers?.(),
      };
    },
  });
}
