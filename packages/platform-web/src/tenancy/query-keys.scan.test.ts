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

/**
 * Pelanggaran dalam satu berkas: `queryKey: <ekspresi>` / `queryKey = <ekspresi>` yang tidak berawalan bentuk yang
 * diizinkan (termasuk key dari variabel), dan `queryKey` shorthand (`{ queryKey }`, `{ queryKey, ... }`) kecuali berkas
 * itu mendefinisikan `queryKey = scopedKey(...)` atau `useScopedKey(...)`.
 */
function findViolations(text: string): { line: number; expr: string }[] {
  const bad: { line: number; expr: string }[] = [];
  const lineOf = (index: number | undefined) => text.slice(0, index).split("\n").length;
  for (const m of text.matchAll(/\bqueryKey\s*[:=]\s*([^\n]*)/g)) {
    const expr = (m[1] ?? "").trim();
    if (!ALLOWED.some((a) => a.test(expr))) bad.push({ line: lineOf(m.index), expr });
  }
  const definedScoped = /\bqueryKey\s*=\s*(use)?[sS]copedKey\(/.test(text);
  if (!definedScoped) {
    for (const m of text.matchAll(/\bqueryKey\b(?=\s*[,}])/g)) {
      bad.push({ line: lineOf(m.index), expr: "queryKey (shorthand)" });
    }
  }
  return bad;
}

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
      for (const v of findViolations(await Bun.file(join(root, rel)).text())) {
        bad.push({ file: rel, ...v });
      }
    }
  }
  return bad;
}

describe("P8: query key berawalan tenant", () => {
  test("tidak ada queryKey (selain bootstrap) tanpa awalan tenant di kode sumber", async () => {
    expect(await scan()).toEqual([]);
  });

  test("pemindai mengenali literal tanpa awalan tenant (kontrol negatif)", () => {
    expect(findViolations('const o = { queryKey: ["notes", "list"] };')).toHaveLength(1);
  });

  test("pemindai mengenali key dari variabel yang tidak jelas asalnya", () => {
    expect(findViolations('const k = ["x"];\nuseQuery({ queryKey: k });')).toHaveLength(1);
  });

  test("pemindai mengenali shorthand { queryKey } tanpa definisi scopedKey", () => {
    expect(findViolations('const queryKey = ["x"];\nuseQuery({ queryKey });')).not.toHaveLength(0);
    expect(findViolations("useQuery({ queryKey, queryFn });")).toHaveLength(1);
  });

  test("shorthand diizinkan bila berkas mendefinisikan queryKey dari scopedKey/useScopedKey", () => {
    expect(
      findViolations('const queryKey = scopedKey(tenantId, "x");\nuseQuery({ queryKey });'),
    ).toEqual([]);
    expect(
      findViolations('const queryKey = useScopedKey("x");\nuseQuery({ queryKey, queryFn });'),
    ).toEqual([]);
  });

  test("bentuk yang diizinkan lolos", () => {
    for (const ok of [
      'useQuery({ queryKey: scopedKey(tenantId, "a") })',
      "useQuery({ queryKey: useScopedKey('a') })",
      "queryOptions({ queryKey: BOOTSTRAP_KEY })",
      'useQuery({ queryKey: [tenantId, "i18n", locale] })',
    ]) {
      expect(findViolations(ok)).toEqual([]);
    }
  });
});
