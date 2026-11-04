import { describe, expect, test } from "bun:test";
import * as entry from "../entries/metadata";
import * as main from "../index";

// Anggaran bundle awal (Requirement 18.4): paket metadata hanya lewat subpath, tidak pernah barrel utama.
describe("entry sekunder @erpora/platform-web/metadata", () => {
  test("barrel utama tidak mengekspor paket metadata", () => {
    for (const name of [
      "validateRecord",
      "evaluateCondition",
      "fieldTypeRegistry",
      "resourceMetaLoader",
      "parseDecimal",
    ]) {
      expect(name in main).toBe(false);
    }
  });
  test("subpath mengekspor API publik", () => {
    for (const name of [
      "validateRecord",
      "evaluateCondition",
      "evaluateExpr",
      "fieldTypeRegistry",
      "createFieldTypeRegistry",
      "resourceMetaLoader",
      "createResourceMetaLoader",
      "formatDecimalString",
      "labelRef",
      "checkClientMeta",
      "parseDecimal",
    ]) {
      expect(typeof (entry as Record<string, unknown>)[name]).not.toBe("undefined");
    }
  });
});
