import { type ApiError, createClient } from "@erpora/platform-contracts";
import { getActiveLocale } from "@erpora/platform-web";
import type { paths } from "../api/contract.gen";

/** Klien modul: kontrak terketik + `Accept-Language` dari bahasa aktif (tidak pernah mengirim tenantId/userId). */
const client = createClient<paths>({ getLocale: getActiveLocale });

/** Galat permintaan API yang dibawa ke TanStack Query; `apiError` dipetakan ke pesan katalog oleh halaman. */
export class SummaryRequestError extends Error {
  override readonly name = "SummaryRequestError";
  readonly apiError: ApiError;
  constructor(apiError: ApiError) {
    super(apiError.code);
    this.apiError = apiError;
  }
}

/** Ringkasan jumlah notes (memakai kontrak publik modul example di backend). */
export async function fetchSummary(): Promise<{ noteCount: number }> {
  const res = await client.get("/api/example-addon/summary");
  if (!res.ok) throw new SummaryRequestError(res.error);
  return res.data;
}

/** Galat apa pun (termasuk jaringan putus) menjadi `ApiError` agar dapat dipetakan ke pesan katalog. */
export function toApiError(error: unknown): ApiError {
  return error instanceof SummaryRequestError
    ? error.apiError
    : { code: "internal_error", message: "" };
}
