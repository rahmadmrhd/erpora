import { describe, expect, test } from "bun:test";
import { join } from "node:path";

// P8 (M3a design): tidak ada `queryKey` selain bootstrap yang didaftarkan tanpa awalan tenant. Pemindai statis atas
// web/src, packages/*/src, dan modules/*/web/src (kode non-test). Bentuk yang diizinkan: `scopedKey(`, `useScopedKey(`,
// `BOOTSTRAP_KEY`, atau literal array berawalan `[tenantId` (mis. katalog i18n: [tenantId, "i18n", locale, ns]).

const root = join(import.meta.dir, "../../../..");
const ALLOWED = [
  /^scopedKey\(/,
  /^useScopedKey\(/,
  /^BOOTSTRAP_KEY\b/,
  /^\[\s*tenantId\b/,
  /^\[\s*\.\.\.BOOTSTRAP_KEY\b/,
];

async function scan(): Promise<{ file: string; line: number; expr: string }[]> {
  const globs = [
    "web/src/**/*.{ts,tsx}",
    "packages/*/src/**/*.{ts,tsx}",
    "modules/*/web/src/**/*.{ts,tsx}",
  ];
  const bad: { file: string; line: number; expr: string }[] = [];
  for (const pattern of globs) {
    for await (const rel of new Bun.Glob(pattern).scan({ cwd: root, onlyFiles: true })) {
      if (/(^|[\\/])(node_modules|dist)[\\/]/.test(rel)) continue;
      if (/\.(test|check|gen)\.tsx?$/.test(rel)) continue;
      const text = await Bun.file(join(root, rel)).text();
      const re = /\bqueryKey\s*[:=]\s*([^\n]*)/g;
      for (const m of text.matchAll(re)) {
        const expr = (m[1] ?? "").trim();
        const line = text.slice(0, m.index).split("\n").length;
        if (!ALLOWED.some((a) => a.test(expr))) bad.push({ file: rel, line, expr });
      }
    }
  }
  return bad;
}

describe("P8: query key berawalan tenant", () => {
  test("tidak ada queryKey (selain bootstrap) tanpa awalan tenant di kode sumber", async () => {
    expect(await scan()).toEqual([]);
  });

  test("pemindai mengenali pelanggaran (kontrol negatif)", () => {
    const sample = 'const o = { queryKey: ["notes", "list"] };';
    const m = /\bqueryKey\s*[:=]\s*([^\n]*)/.exec(sample);
    const expr = (m?.[1] ?? "").trim();
    expect(ALLOWED.some((a) => a.test(expr))).toBe(false);
  });
});
