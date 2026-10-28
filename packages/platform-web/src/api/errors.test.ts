import { describe, expect, test } from "bun:test";
import type { ApiError } from "@erpora/platform-contracts";
import fc from "fast-check";
import { API_ERROR_KEYS, API_ERROR_UNKNOWN, describeApiError } from "./errors";

// Semua kode di api-guidelines §8 (dan Error Handling M3a design) harus terpetakan ke kunci `errors.<code>`.
const KNOWN_CODES = [
  "bad_request",
  "invalid_filter",
  "invalid_sort",
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

const err = (over: Partial<ApiError> & { code: string }): ApiError => ({
  message: "pesan server",
  ...over,
});

describe("describeApiError", () => {
  test.each(KNOWN_CODES)("kode %s terpetakan ke errors.%s", (code) => {
    const d = describeApiError(err({ code }));
    expect(d.key).toBe(`errors.${code}`);
    expect(API_ERROR_KEYS).toContain(d.key);
  });

  test("kode asing, kosong, atau http_error jatuh ke errors.unknown", () => {
    for (const code of ["", "aneh", "http_error", "__proto__", "constructor"]) {
      expect(describeApiError(err({ code })).key).toBe(API_ERROR_UNKNOWN);
    }
  });

  test("details tingkat global (tanpa field) yang dikenal lebih spesifik daripada code", () => {
    const d = describeApiError(
      err({ code: "bad_request", details: [{ code: "unsupported_media_type", message: "x" }] }),
    );
    expect(d.key).toBe("errors.unsupported_media_type");
  });

  test("details level field diabaikan (kode top-level yang dipakai)", () => {
    const d = describeApiError(
      err({
        code: "validation_failed",
        details: [{ field: "lines[2].itemId", code: "not_found", message: "x" }],
      }),
    );
    expect(d.key).toBe("errors.validation_failed");
  });

  test("details berupa objek konteks (mis. 412) tidak mengganggu", () => {
    const d = describeApiError(
      err({ code: "version_mismatch", details: { expected: 3, actual: 4 } }),
    );
    expect(d.key).toBe("errors.version_mismatch");
  });

  test("requestId diteruskan sebagai parameter; tidak ada bila tidak ada", () => {
    expect(describeApiError(err({ code: "internal_error", requestId: "01JREQ" })).params).toEqual({
      requestId: "01JREQ",
    });
    expect(describeApiError(err({ code: "internal_error" })).params).toBeUndefined();
  });

  test("retryable hanya untuk galat sementara", () => {
    for (const code of [
      "internal_error",
      "module_unavailable",
      "not_ready",
      "rate_limited",
      "request_in_progress",
    ]) {
      expect(describeApiError(err({ code })).retryable).toBe(true);
    }
    for (const code of ["permission_denied", "not_found", "validation_failed", "aneh"]) {
      expect(describeApiError(err({ code })).retryable).toBe(false);
    }
  });

  test("P10: total untuk ApiError apa pun dan tidak membocorkan details mentah", () => {
    const secret = "SELECT * FROM users WHERE goroutine 1 [running]";
    fc.assert(
      fc.property(
        fc.string(),
        fc.string(),
        fc.option(fc.jsonValue(), { nil: undefined }),
        fc.option(fc.string(), { nil: undefined }),
        (code, message, details, requestId) => {
          const e: ApiError = { code, message, details, requestId };
          const d = describeApiError(e);
          expect(API_ERROR_KEYS).toContain(d.key);
          const out = JSON.stringify(d);
          if (typeof message === "string" && message.length > 8) expect(out).not.toContain(message);
          expect(out).not.toContain(secret);
        },
      ),
      { numRuns: 300 },
    );
  });

  test("P10: details beracun (stack/SQL) tidak pernah muncul di hasil", () => {
    const e = err({
      code: "internal_error",
      details: [{ code: "x", message: "SELECT * FROM users goroutine 1 [running]" }],
    });
    const out = JSON.stringify(describeApiError(e));
    expect(out).not.toContain("SELECT");
    expect(out).not.toContain("goroutine");
  });

  test("masukan sangat cacat (cast) tidak melempar", () => {
    for (const bad of [null, undefined, {}, { code: 1 }, { code: "not_found", details: "x" }]) {
      expect(() => describeApiError(bad as unknown as ApiError)).not.toThrow();
      expect(API_ERROR_KEYS).toContain(describeApiError(bad as unknown as ApiError).key);
    }
  });
});
