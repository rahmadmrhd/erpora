import { describe, expect, test } from "bun:test";
import platformEn from "../../../../internal/platform/i18n/catalog/en.json";
import platformId from "../../../../internal/platform/i18n/catalog/id.json";
import uiEn from "../../../../packages/ui/i18n/en.json";
import uiId from "../../../../packages/ui/i18n/id.json";
import { platformKeyList, uiKeyList } from "./keys";

function flatten(obj: unknown, prefix = ""): string[] {
  if (typeof obj === "string") return [prefix];
  if (typeof obj !== "object" || obj === null) return [];
  return Object.entries(obj).flatMap(([k, v]) => flatten(v, prefix === "" ? k : `${prefix}.${k}`));
}

describe("paritas kunci terketik dengan katalog JSON", () => {
  const cases: [string, unknown, Record<string, true>][] = [
    ["platform/id", platformId, platformKeyList],
    ["platform/en", platformEn, platformKeyList],
    ["ui/id", uiId, uiKeyList],
    ["ui/en", uiEn, uiKeyList],
  ];
  test.each(cases)(
    "%s: himpunan kunci sama persis (tanpa kunci yatim atau hilang)",
    (_name, json, list) => {
      expect(flatten(json).sort()).toEqual(Object.keys(list).sort());
    },
  );
});
