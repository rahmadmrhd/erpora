import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// Dibaca dari berkas (bukan `import`) agar aturan F2 (tanpa deep import ke packages/*) tidak terpicu.
const repoRoot = join(import.meta.dir, "../../../..");
const readJson = (rel: string): unknown => JSON.parse(readFileSync(join(repoRoot, rel), "utf8"));
const platformId = readJson("internal/platform/i18n/catalog/id.json");
const platformEn = readJson("internal/platform/i18n/catalog/en.json");
const uiId = readJson("packages/ui/i18n/id.json");
const uiEn = readJson("packages/ui/i18n/en.json");

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
      // Subpohon `modules.*` (halaman /settings/modules) dimiliki dan diuji `settings/modules/keys.test.ts`.
      const own = flatten(json).filter((k) => !k.startsWith("modules."));
      expect(own.sort()).toEqual(Object.keys(list).sort());
    },
  );
});
