import type { ApiError, PlatformPaths, TypedClient } from "@erpora/platform-contracts";

export interface CatalogEntry {
  readonly locale: string;
  readonly namespace: string;
  readonly revision: string;
  /** Kunci lengkap bertitik ke pesan ICU. */
  readonly messages: Readonly<Record<string, string>>;
}

/** Dilempar bila katalog tidak dapat dimuat (404, galat server, respons rusak, atau 304 tanpa entri lama). */
export class CatalogLoadError extends Error {
  override readonly name = "CatalogLoadError";
  readonly status: number;
  readonly error: ApiError;
  constructor(status: number, error: ApiError) {
    super(`katalog gagal dimuat (${status}): ${error.code}`);
    this.status = status;
    this.error = error;
  }
}

function isEntry(v: unknown): v is CatalogEntry {
  if (typeof v !== "object" || v === null) return false;
  const e = v as Record<string, unknown>;
  if (typeof e.locale !== "string" || typeof e.namespace !== "string") return false;
  if (typeof e.revision !== "string") return false;
  const m = e.messages;
  return (
    typeof m === "object" &&
    m !== null &&
    !Array.isArray(m) &&
    Object.values(m).every((x) => typeof x === "string")
  );
}

/**
 * Pemuat katalog `GET /api/platform/i18n/{locale}/{namespace}` ber-ETag: menyimpan `{ etag, entry }` per
 * (locale, namespace), mengirim `If-None-Match`, dan pada `304` mengembalikan entri yang sama (referensi tetap).
 */
export function createCatalogLoader(
  client: TypedClient<PlatformPaths>,
): (locale: string, namespace: string) => Promise<CatalogEntry> {
  const last = new Map<string, { etag: string | undefined; entry: CatalogEntry }>();
  return async (locale, namespace) => {
    const cacheKey = `${locale}\u0000${namespace}`;
    const prev = last.get(cacheKey);
    const res = await client.get("/api/platform/i18n/{locale}/{namespace}", {
      params: { path: { locale, namespace } },
      headers: prev?.etag ? { "If-None-Match": prev.etag } : undefined,
    });
    if (res.ok) {
      if (!isEntry(res.data)) {
        throw new CatalogLoadError(res.status, {
          code: "invalid_response",
          message: "respons katalog tidak valid",
        });
      }
      last.set(cacheKey, { etag: res.etag, entry: res.data });
      return res.data;
    }
    if (res.status === 304 && prev) return prev.entry;
    throw new CatalogLoadError(res.status, res.error);
  };
}
