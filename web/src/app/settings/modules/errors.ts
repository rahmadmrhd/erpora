import type { ApiError } from "@erpora/platform-contracts";
import { describeApiError } from "@erpora/platform-web";
import type { MessageDescriptor, ModulesKey } from "./keys";

/** Kode detail lifecycle di `error.details[].code` (ADR-0007; kontrak `runModuleOperation`). */
const LIFECYCLE_DETAILS = [
  "invalid_transition",
  "operation_in_progress",
  "module_required",
  "dependency_not_ready",
  "dependents_active",
  "module_incompatible",
  "downgrade_not_supported",
  "module_not_found",
] as const;
const DETAIL_SET = new Set<string>(LIFECYCLE_DETAILS);

/** Kode kegagalan operasi (`operation.failure.code`) yang punya pesan sendiri. */
const FAILURE_CODES = [
  "migration_failed",
  "interrupted",
  "health_check_failed",
  "hook_failed",
  "dependency_not_ready",
  "downgrade_not_supported",
  "internal_error",
] as const;
const FAILURE_SET = new Set<string>(FAILURE_CODES);

export interface MappedError {
  /** Pesan untuk pengguna (kunci katalog + argumen); tidak pernah memuat teks atau `details` mentah dari server. */
  messages: readonly MessageDescriptor[];
  /** Status server berubah (409/412): daftar harus dimuat ulang. */
  reloadList: boolean;
  /** API mutasi tidak aktif di lingkungan ini (404 pada mutasi): halaman beralih ke mode hanya-baca. */
  switchToReadOnly: boolean;
}

function detailCodes(details: unknown): string[] {
  if (!Array.isArray(details)) return [];
  const out: string[] = [];
  for (const d of details) {
    if (typeof d !== "object" || d === null) continue;
    const item = d as { code?: unknown; field?: unknown };
    // Detail level field menjelaskan satu isian, bukan galat keseluruhan.
    if (typeof item.code === "string" && item.field === undefined) out.push(item.code);
  }
  return out;
}

/**
 * Memetakan galat API modul ke pesan katalog (R13.7). Urutan: detail lifecycle yang dikenal (satu pesan per detail,
 * tanpa duplikat), status khusus (412 `version_mismatch`, 404 pada mutasi), lalu `describeApiError`. Kode tak dikenal
 * menjadi `errors.unknown` ditambah `requestId` untuk korelasi.
 */
export function mapModuleError(
  status: number,
  error: ApiError,
  context: "read" | "mutation",
): MappedError {
  const reloadList = status === 409 || status === 412;
  const known = [...new Set(detailCodes(error?.details).filter((c) => DETAIL_SET.has(c)))];
  if (known.length > 0) {
    return {
      messages: known.map((c) => ({ key: `modules.error.${c}` as ModulesKey })),
      reloadList,
      switchToReadOnly: false,
    };
  }
  if (status === 412) {
    return {
      messages: [{ key: "modules.error.version_mismatch" }],
      reloadList: true,
      switchToReadOnly: false,
    };
  }
  if (status === 404 && context === "mutation") {
    return {
      messages: [{ key: "modules.error.mutation_disabled" }],
      reloadList: false,
      switchToReadOnly: true,
    };
  }
  const d = describeApiError(error);
  const messages: MessageDescriptor[] = [{ key: d.key }];
  if (d.key === "errors.unknown" && d.params?.requestId) {
    messages.push({ key: "modules.error.requestId", args: { requestId: d.params.requestId } });
  }
  return { messages, reloadList, switchToReadOnly: false };
}

/** Pesan untuk `operation.failure.code`; kode tak dikenal menjadi `modules.failure.unknown`. */
export function mapFailureCode(code: string): { key: ModulesKey } {
  return {
    key: (FAILURE_SET.has(code)
      ? `modules.failure.${code}`
      : "modules.failure.unknown") as ModulesKey,
  };
}
