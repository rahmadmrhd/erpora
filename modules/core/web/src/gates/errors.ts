import type { Failure } from "../api";

/** Satu pesan katalog `core` (kunci bertitik) beserta argumennya. */
export interface AuthMessage {
  key: string;
  args?: Record<string, string | number>;
}

export interface FailureInfo {
  /** Satu atau lebih pesan untuk ditampilkan (pelanggaran kebijakan sandi dapat lebih dari satu). */
  messages: AuthMessage[];
  /** Kode galat API (`network` bila tidak sampai ke server). */
  code: string;
  /** Kode `details[]` (mis. `invalid_token`, `invalid_code`). */
  detailCodes: string[];
}

const POLICY_CODES = new Set(["too_short", "too_long", "common"]);
const DETAIL_KEYS = new Set(["wrong_current_password", "invalid_token", "invalid_code"]);
const CODE_KEYS = new Set([
  "invalid_credentials",
  "account_locked",
  "mfa_required",
  "unauthenticated",
  "conflict",
  "invalid_state",
  "validation_failed",
]);

function detailCodes(details: unknown): string[] {
  if (!Array.isArray(details)) return [];
  const out: string[] = [];
  for (const d of details) {
    const code = (d as { code?: unknown } | null)?.code;
    if (typeof code === "string") out.push(code);
  }
  return out;
}

/**
 * Memetakan kegagalan ke pesan katalog `core` per KODE galat (bukan teks server, yang tidak pernah ditampilkan).
 * Login gagal tidak membedakan email atau sandi (`invalid_credentials` seragam, FR-101). Total: tidak melempar.
 */
export function describeFailure(f: Failure): FailureInfo {
  const details = detailCodes(f.error.details);
  const base = { code: f.error.code, detailCodes: details };
  if (f.network) return { ...base, messages: [{ key: "auth.errors.network" }] };

  if (f.status === 429 || f.error.code === "rate_limited") {
    const message =
      f.retryAfter === undefined
        ? { key: "auth.errors.rate_limited_generic" }
        : { key: "auth.errors.rate_limited", args: { seconds: f.retryAfter } };
    return { ...base, messages: [message] };
  }

  if (f.error.code === "password_policy_violation") {
    const keys = new Set<string>();
    for (const c of details) keys.add(POLICY_CODES.has(c) ? c : "unknown");
    if (keys.size === 0) keys.add("unknown");
    return { ...base, messages: [...keys].map((c) => ({ key: `password.violation.${c}` })) };
  }

  if (f.error.code === "validation_failed") {
    const known = details.find((c) => DETAIL_KEYS.has(c));
    if (known) return { ...base, messages: [{ key: `detail.${known}` }] };
  }

  if (CODE_KEYS.has(f.error.code)) {
    return { ...base, messages: [{ key: `auth.errors.${f.error.code}` }] };
  }
  return { ...base, messages: [{ key: "auth.errors.generic" }] };
}
