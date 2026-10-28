// Konformansi Go-TS (design.md P5; Requirement 9): fixture yang sama dengan test Go
// (sdk/i18n/testdata/conformance.json) harus menghasilkan keluaran identik lewat format.ts.
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { type Args, compile, format } from "./format";

interface ConformanceCase {
  name: string;
  locale: string;
  message: string;
  args: Args;
  expected?: string;
  expectError?: boolean;
  goOnly?: boolean;
  goOnlyReason?: string;
}

interface ConformanceFile {
  version: number;
  cases: ConformanceCase[];
}

const fixturePath = fileURLToPath(
  new URL("../../../../sdk/i18n/testdata/conformance.json", import.meta.url),
);
const fixture = JSON.parse(readFileSync(fixturePath, "utf8")) as ConformanceFile;

describe("fixture konformansi", () => {
  test("bentuk fixture terjaga", () => {
    expect(fixture.version).toBe(1);
    expect(fixture.cases.length).toBeGreaterThanOrEqual(40);
    const names = new Set<string>();
    const locales = new Set<string>();
    for (const c of fixture.cases) {
      expect(names.has(c.name)).toBe(false);
      names.add(c.name);
      locales.add(c.locale);
      // Tepat satu dari expected atau expectError.
      expect(c.expected !== undefined).not.toBe(c.expectError === true);
    }
    expect(locales.has("id")).toBe(true);
    expect(locales.has("en")).toBe(true);
  });
});

describe("konformansi Go-TS", () => {
  for (const c of fixture.cases) {
    if (c.goOnly) {
      // Penolakan khusus Go (konstruksi tak didukung, batas ukuran): FormatJS menerimanya.
      test.skip(`${c.name} (goOnly: ${c.goOnlyReason ?? ""})`, () => {});
      continue;
    }
    test(c.name, () => {
      if (c.expectError) {
        expect(() => format(compile(c.locale, c.message), c.args)).toThrow();
        return;
      }
      expect(format(compile(c.locale, c.message), c.args)).toBe(c.expected as string);
    });
  }
});
