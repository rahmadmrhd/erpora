import { describe, expect, test } from "bun:test";
import type { Failure } from "../api";
import { describeFailure } from "./errors";

const fail = (
  status: number,
  code: string,
  extra: Partial<Failure> = {},
  details?: unknown,
): Failure => ({
  status,
  error: { code, message: "teks server tidak boleh tampil", ...(details ? { details } : {}) },
  network: false,
  ...extra,
});

const keys = (f: Failure) => describeFailure(f).messages.map((m) => m.key);

describe("describeFailure", () => {
  test("kode galat utama dipetakan ke kunci katalog; teks server tidak pernah dipakai", () => {
    expect(keys(fail(401, "invalid_credentials"))).toEqual(["auth.errors.invalid_credentials"]);
    expect(keys(fail(403, "account_locked"))).toEqual(["auth.errors.account_locked"]);
    expect(keys(fail(403, "mfa_required"))).toEqual(["auth.errors.mfa_required"]);
    expect(keys(fail(409, "invalid_state"))).toEqual(["auth.errors.invalid_state"]);
    expect(keys(fail(500, "internal_error"))).toEqual(["auth.errors.generic"]);
    expect(keys(fail(418, "kode_tak_dikenal"))).toEqual(["auth.errors.generic"]);
  });

  test("rate_limited memakai Retry-After bila ada", () => {
    expect(describeFailure(fail(429, "rate_limited", { retryAfter: 42 })).messages).toEqual([
      { key: "auth.errors.rate_limited", args: { seconds: 42 } },
    ]);
    expect(keys(fail(429, "rate_limited"))).toEqual(["auth.errors.rate_limited_generic"]);
  });

  test("validation_failed dengan details[].code dikenal memilih pesan detail", () => {
    for (const code of ["wrong_current_password", "invalid_token", "invalid_code"]) {
      expect(keys(fail(422, "validation_failed", {}, [{ field: "x", code }]))).toEqual([
        `detail.${code}`,
      ]);
    }
    expect(keys(fail(422, "validation_failed", {}, [{ code: "lain" }]))).toEqual([
      "auth.errors.validation_failed",
    ]);
    expect(keys(fail(422, "validation_failed"))).toEqual(["auth.errors.validation_failed"]);
  });

  test("password_policy_violation: satu pesan per aturan unik; kode asing menjadi unknown", () => {
    const f = fail(422, "password_policy_violation", {}, [
      { field: "newPassword", code: "too_short" },
      { field: "newPassword", code: "common" },
      { field: "newPassword", code: "aturan_baru" },
      { field: "newPassword", code: "aturan_baru_lain" },
    ]);
    expect(keys(f)).toEqual([
      "password.violation.too_short",
      "password.violation.common",
      "password.violation.unknown",
    ]);
    expect(keys(fail(422, "password_policy_violation"))).toEqual(["password.violation.unknown"]);
  });

  test("kegagalan jaringan", () => {
    expect(keys({ status: 0, error: { code: "network", message: "" }, network: true })).toEqual([
      "auth.errors.network",
    ]);
  });
});
