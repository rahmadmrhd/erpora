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

const SCOPED_DEFINITION = /\bqueryKey\s*=\s*(use)?[sS]copedKey\(/;

/**
 * Apakah `queryKey` shorthand pada baris indeks `useIndex` berasal dari definisi `queryKey = scopedKey(...)` /
 * `useScopedKey(...)` di lingkup leksikal yang sama? Lingkup didekati lewat indentasi (kode diformat Biome): definisi harus
 * ada di atas pemakaian dan tidak ada baris di antaranya yang berindentasi lebih dangkal (blok definisi sudah ditutup).
 */
function shorthandIsScoped(lines: string[], useIndex: number): boolean {
  for (let i = useIndex - 1; i >= 0; i--) {
    const line = lines[i] ?? "";
    if (!SCOPED_DEFINITION.test(line)) continue;
    const indent = line.length - line.trimStart().length;
    for (let j = i + 1; j < useIndex; j++) {
      const mid = lines[j] ?? "";
      if (mid.trim() === "") continue;
      if (mid.length - mid.trimStart().length < indent) return false;
    }
    return true;
  }
  return false;
}

/**
 * Pelanggaran dalam satu berkas: `queryKey: <ekspresi>` / `queryKey = <ekspresi>` yang tidak berawalan bentuk yang
 * diizinkan (termasuk key dari variabel), dan `queryKey` shorthand (`{ queryKey }`, `{ queryKey, ... }`) kecuali variabelnya
 * didefinisikan dari `scopedKey(...)`/`useScopedKey(...)` di lingkup yang sama (bukan sekadar di berkas yang sama).
 */
function findViolations(text: string): { line: number; expr: string }[] {
  const bad: { line: number; expr: string }[] = [];
  const lines = text.split("\n");
  const lineOf = (index: number | undefined) => text.slice(0, index).split("\n").length;
  for (const m of text.matchAll(/\bqueryKey\s*[:=]\s*([^\n]*)/g)) {
    const expr = (m[1] ?? "").trim();
    if (!ALLOWED.some((a) => a.test(expr))) bad.push({ line: lineOf(m.index), expr });
  }
  for (const m of text.matchAll(/\bqueryKey\b(?=\s*[,}])/g)) {
    const line = lineOf(m.index);
    if (!shorthandIsScoped(lines, line - 1)) bad.push({ line, expr: "queryKey (shorthand)" });
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

  test("shorthand diizinkan bila queryKey didefinisikan dari scopedKey/useScopedKey di lingkup yang sama", () => {
    expect(
      findViolations('const queryKey = scopedKey(tenantId, "x");\nuseQuery({ queryKey });'),
    ).toEqual([]);
    expect(
      findViolations('const queryKey = useScopedKey("x");\nuseQuery({ queryKey, queryFn });'),
    ).toEqual([]);
  });

  test("shorthand di fungsi lain dalam berkas yang sama TIDAK diloloskan oleh definisi scopedKey di fungsi lain", () => {
    const text = [
      "function a() {",
      '  const queryKey = scopedKey(tenantId, "x");',
      "  return useQuery({ queryKey });",
      "}",
      "function b() {",
      '  const queryKey = ["tanpa", "tenant"];',
      "  return useQuery({ queryKey });",
      "}",
    ].join("\n");
    // baris 6 (literal tanpa tenant) dan baris 7 (shorthand di luar lingkup definisi scoped)
    expect(findViolations(text).map((v) => v.line)).toEqual([6, 7]);
  });

  test("shorthand di blok bersarang dari definisi scoped yang sama lolos", () => {
    const text = [
      "function a() {",
      '  const queryKey = scopedKey(tenantId, "x");',
      "  if (cond) {",
      "    return useQuery({ queryKey });",
      "  }",
      "}",
    ].join("\n");
    expect(findViolations(text)).toEqual([]);
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
