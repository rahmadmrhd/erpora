import { describe, expect, test } from "bun:test";
import { API_ERROR_KEYS } from "@erpora/platform-web";
import { mapFailureCode, mapModuleError } from "./errors";
import { MODULE_KEYS } from "./keys";

const err = (code: string, details?: unknown, requestId?: string) => ({
  code,
  message: "SQL: secret detail dari server",
  ...(details === undefined ? {} : { details }),
  ...(requestId ? { requestId } : {}),
});

const knownKeys = new Set<string>([...Object.keys(MODULE_KEYS), ...API_ERROR_KEYS]);
const keysOf = (r: ReturnType<typeof mapModuleError>) => r.messages.map((m) => m.key);

describe("mapModuleError: kode detail lifecycle", () => {
  test.each([
    ["invalid_transition", 409, "invalid_state"],
    ["operation_in_progress", 409, "request_in_progress"],
    ["module_required", 422, "business_rule_violated"],
    ["dependency_not_ready", 422, "business_rule_violated"],
    ["dependents_active", 422, "business_rule_violated"],
    ["module_incompatible", 422, "business_rule_violated"],
    ["downgrade_not_supported", 422, "business_rule_violated"],
    ["module_not_found", 404, "not_found"],
  ])("detail %s (%i) menjadi modules.error.%s", (detail, status, code) => {
    const r = mapModuleError(status, err(code, [{ code: detail }]), "mutation");
    expect<string[]>(keysOf(r)).toEqual([`modules.error.${detail}`]);
  });

  test("422 dengan beberapa detail menghasilkan satu pesan per detail (tanpa duplikat, urutan terjaga)", () => {
    const r = mapModuleError(
      422,
      err("business_rule_violated", [
        { code: "dependents_active" },
        { code: "module_required" },
        { code: "dependents_active" },
      ]),
      "mutation",
    );
    expect<string[]>(keysOf(r)).toEqual([
      "modules.error.dependents_active",
      "modules.error.module_required",
    ]);
  });

  test("detail tingkat field diabaikan (bukan galat keseluruhan)", () => {
    const r = mapModuleError(
      422,
      err("validation_failed", [{ code: "unknown_field", field: "foo" }]),
      "mutation",
    );
    expect<string[]>(keysOf(r)).toEqual(["errors.validation_failed"]);
  });
});

describe("mapModuleError: status HTTP", () => {
  test("409 dan 412 memuat ulang daftar; 412 menjadi 'status modul berubah'", () => {
    expect(mapModuleError(409, err("invalid_state"), "mutation").reloadList).toBe(true);
    const r = mapModuleError(412, err("version_mismatch"), "mutation");
    expect<string[]>(keysOf(r)).toEqual(["modules.error.version_mismatch"]);
    expect(r.reloadList).toBe(true);
  });

  test("404 pada mutasi berarti API mutasi mati: pesan khusus dan beralih ke mode hanya-baca", () => {
    const r = mapModuleError(404, err("not_found"), "mutation");
    expect<string[]>(keysOf(r)).toEqual(["modules.error.mutation_disabled"]);
    expect(r.switchToReadOnly).toBe(true);
  });

  test("404 pada bacaan memakai errors.not_found tanpa beralih mode", () => {
    const r = mapModuleError(404, err("not_found"), "read");
    expect<string[]>(keysOf(r)).toEqual(["errors.not_found"]);
    expect(r.switchToReadOnly).toBe(false);
  });

  test("415 dan kode tak dikenal: pesan umum + requestId, tanpa menyalin teks server", () => {
    const r = mapModuleError(415, err("kode_aneh", undefined, "req-9"), "mutation");
    expect<string[]>(keysOf(r)).toEqual(["errors.unknown", "modules.error.requestId"]);
    expect(r.messages[1]?.args).toEqual({ requestId: "req-9" });
    expect(JSON.stringify(r)).not.toContain("SQL");
    expect(JSON.stringify(r)).not.toContain("secret");
  });

  test("tanpa requestId tidak ada pesan requestId", () => {
    expect<string[]>(keysOf(mapModuleError(500, err("internal_error"), "mutation"))).toEqual([
      "errors.internal_error",
    ]);
  });

  test("setiap kunci yang dihasilkan ada di katalog (modules.* atau errors.*)", () => {
    for (const status of [400, 403, 404, 409, 412, 415, 422, 500, 503]) {
      for (const ctx of ["read", "mutation"] as const) {
        for (const d of [undefined, [{ code: "module_required" }], [{ code: "zzz" }]]) {
          const r = mapModuleError(status, err("whatever", d, "r"), ctx);
          for (const m of r.messages) expect(knownKeys.has(m.key), m.key).toBe(true);
        }
      }
    }
  });
});

describe("mapFailureCode (kegagalan operasi)", () => {
  test.each([
    "migration_failed",
    "interrupted",
    "health_check_failed",
    "hook_failed",
    "dependency_not_ready",
    "downgrade_not_supported",
    "internal_error",
  ])("%s menjadi modules.failure.%s", (code) => {
    expect(mapFailureCode(code).key).toBe(`modules.failure.${code}`);
  });

  test("kode tak dikenal jatuh ke modules.failure.unknown", () => {
    expect(mapFailureCode("kode_baru").key).toBe("modules.failure.unknown");
    expect(mapFailureCode("").key).toBe("modules.failure.unknown");
  });
});
