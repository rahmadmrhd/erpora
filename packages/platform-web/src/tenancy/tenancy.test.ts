import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { scopedKey } from "./index";

describe("scopedKey", () => {
  test("selalu [tenantId, ...bagian]", () => {
    expect(scopedKey("t-1", "modules", "list")).toEqual(["t-1", "modules", "list"]);
    expect(scopedKey("t-1")).toEqual(["t-1"]);
  });

  test("P8: elemen pertama selalu tenantId dan sisanya tidak berubah", () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 1 }),
        fc.array(fc.jsonValue(), { maxLength: 6 }),
        (tenantId, parts) => {
          const key = scopedKey(tenantId, ...parts);
          expect(key[0]).toBe(tenantId);
          expect(key.slice(1)).toEqual(parts);
          expect(key.length).toBe(parts.length + 1);
        },
      ),
      { numRuns: 200 },
    );
  });

  test("tenantId kosong ditolak (key tanpa scope tidak boleh terbentuk diam-diam)", () => {
    expect(() => scopedKey("", "modules")).toThrow();
  });
});
