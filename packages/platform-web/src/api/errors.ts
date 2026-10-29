import type { ApiError } from "@erpora/platform-contracts";

/**
 * Semua kode galat stabil di `docs/architecture/api-guidelines.md` §8. Tiap kode terpetakan ke kunci pesan
 * `errors.<code>` di namespace katalog `platform` (satu kunci per kode).
 */
const KNOWN_CODES = [
  "bad_request",
  "invalid_filter",
  "invalid_sort",
  "invalid_i18n_override",
  "tenant_unresolved",
  "unauthenticated",
  "token_expired",
  "permission_denied",
  "field_forbidden",
  "mfa_required",
  "module_disabled",
  "not_found",
  "module_not_found",
  "invalid_state",
  "request_in_progress",
  "conflict",
  "version_mismatch",
  "validation_failed",
  "business_rule_violated",
  "idempotency_key_reused",
  "rate_limited",
  "unsupported_media_type",
  "internal_error",
  "not_implemented",
  "method_not_allowed",
  "module_unavailable",
  "not_ready",
] as const;

export type KnownApiErrorCode = (typeof KNOWN_CODES)[number];
export type ApiErrorKey = `errors.${KnownApiErrorCode}` | "errors.unknown";

/** Awalan kunci pesan galat di katalog `platform` (sama dengan `errors.<code>` katalog backend; keputusan G14). */
export const API_ERROR_KEY_PREFIX = "errors.";
export const API_ERROR_UNKNOWN: ApiErrorKey = "errors.unknown";

/** Himpunan lengkap kunci yang dapat dikembalikan `describeApiError` (semua wajib ada di katalog `platform`). */
export const API_ERROR_KEYS: readonly ApiErrorKey[] = [
  ...KNOWN_CODES.map((c) => `${API_ERROR_KEY_PREFIX}${c}` as ApiErrorKey),
  API_ERROR_UNKNOWN,
];

const KNOWN = new Set<string>(KNOWN_CODES);
const RETRYABLE = new Set<string>([
  "internal_error",
  "module_unavailable",
  "not_ready",
  "rate_limited",
  "request_in_progress",
]);

export interface ApiErrorDescription {
  key: ApiErrorKey;
  /** Parameter untuk pesan (saat ini hanya `requestId` untuk korelasi). */
  params?: Record<string, string>;
  /** Galat sementara yang masuk akal dicoba ulang. */
  retryable: boolean;
}

function globalDetailCodes(details: unknown): string[] {
  if (!Array.isArray(details)) return [];
  const out: string[] = [];
  for (const d of details) {
    if (typeof d !== "object" || d === null) continue;
    const item = d as { code?: unknown; field?: unknown };
    // Detail level field (punya `field`) menjelaskan satu isian, bukan galat keseluruhan: diabaikan.
    if (typeof item.code === "string" && item.field === undefined) out.push(item.code);
  }
  return out;
}

/**
 * Memetakan `ApiError` ke kunci pesan terjemahan. Total: tidak pernah melempar. Urutan: detail global yang dikenal
 * (lebih spesifik), lalu `error.code`, lalu `errors.unknown`. Teks `message` dan `details` server tidak pernah
 * disalin ke hasil (tidak ada kebocoran stack/SQL ke UI); UI memakai pesan dari katalog.
 */
export function describeApiError(error: ApiError): ApiErrorDescription {
  const e = (typeof error === "object" && error !== null ? error : {}) as Partial<ApiError>;
  const candidates = [...globalDetailCodes(e.details), typeof e.code === "string" ? e.code : ""];
  const code = candidates.find((c) => KNOWN.has(c));
  const key = (code ? `${API_ERROR_KEY_PREFIX}${code}` : API_ERROR_UNKNOWN) as ApiErrorKey;
  const requestId = typeof e.requestId === "string" && e.requestId !== "" ? e.requestId : undefined;
  return {
    key,
    ...(requestId ? { params: { requestId } } : {}),
    retryable: code !== undefined && RETRYABLE.has(code),
  };
}
