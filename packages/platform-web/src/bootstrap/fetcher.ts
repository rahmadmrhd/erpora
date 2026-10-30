import type { ApiError, PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import { queryOptions } from "@tanstack/react-query";
import type { Bootstrap } from "./types";

/**
 * Satu-satunya query key tanpa awalan tenant: bootstrap adalah sumber `tenantId` itu sendiri (M3a R7.1).
 * Semua key lain wajib lewat `scopedKey`.
 */
export const BOOTSTRAP_KEY = ["bootstrap"] as const;

/** Dilempar fetcher bootstrap bila server membalas galat, respons rusak, atau 304 tanpa data sebelumnya. */
export class BootstrapError extends Error {
  override readonly name = "BootstrapError";
  readonly status: number;
  readonly error: ApiError;
  constructor(status: number, error: ApiError) {
    super(`bootstrap gagal (${status}): ${error.code}`);
    this.status = status;
    this.error = error;
  }
}

function isBootstrap(v: unknown): v is Bootstrap {
  if (typeof v !== "object" || v === null) return false;
  const b = v as Record<string, unknown>;
  return (
    typeof b.tenantId === "string" &&
    typeof b.revision === "number" &&
    Array.isArray(b.modules) &&
    Array.isArray(b.permissions)
  );
}

/**
 * Fetcher bootstrap ber-`ETag`: menyimpan `{ etag, data }` terakhir, mengirim `If-None-Match`, dan pada `304`
 * mengembalikan **referensi objek yang sama** sehingga tidak memicu render ulang (P5). Galat melempar.
 */
export function createBootstrapFetcher(
  client: TypedClient<PlatformPaths>,
): () => Promise<Bootstrap> {
  let last: { etag: string | undefined; data: Bootstrap } | undefined;
  return async () => {
    const res = await client.get("/api/bootstrap", {
      headers: last?.etag ? { "If-None-Match": last.etag } : undefined,
    });
    if (res.ok) {
      if (!isBootstrap(res.data)) {
        throw new BootstrapError(res.status, {
          code: "invalid_response",
          message: "respons bootstrap tidak valid",
        });
      }
      last = { etag: res.etag, data: res.data };
      return res.data;
    }
    if (res.status === 304 && last) return last.data;
    throw new BootstrapError(res.status, res.error);
  };
}

export interface BootstrapQueryTuning {
  /** Bawaan 2 (percobaan ulang untuk galat sementara). */
  retry?: number | false;
  /** Bawaan 5000 ms. */
  staleTime?: number;
}

export function bootstrapQueryOptions(
  fetcher: () => Promise<Bootstrap>,
  tuning: BootstrapQueryTuning = {},
) {
  return queryOptions({
    queryKey: BOOTSTRAP_KEY,
    queryFn: fetcher,
    staleTime: tuning.staleTime ?? 5_000,
    refetchOnWindowFocus: true,
    gcTime: Number.POSITIVE_INFINITY,
    retry: tuning.retry ?? 2,
  });
}
