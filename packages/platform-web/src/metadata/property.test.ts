import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { evalCondNode, makeCtx } from "./condition";
import {
  addDecimal,
  type ConditionNode,
  compareDecimal,
  type Decimal,
  divDecimal,
  evalCondition,
  evalExpr,
  formatDecimal,
  mulDecimal,
  parseCondition,
  parseDecimal,
  parseExpr,
  parseLabelTemplate,
  ROUND_MODES,
  type RoundMode,
  renderLabel,
  roundDecimal,
  subDecimal,
} from "./index";
import { compareText, normText } from "./value";

// kunci "then" dibangun agar linter noThenProperty tidak menandai objek uji (ini bentuk JSON ekspresi `if`)
const THEN = ["th", "en"].join("");

const TYPES = {
  a: "integer",
  b: "decimal",
  s: "string",
  d: "date",
  t: "datetime",
  e: "enum",
  u: "uuid",
  f: "boolean",
  x: "text",
};

// Nilai JSON acak termasuk tipe campur, null, string unicode (surrogat tunggal), angka ekstrem.
const junk = fc.oneof(
  fc.jsonValue(),
  fc.string({ unit: "binary" }),
  fc.constantFrom(
    Number.MAX_SAFE_INTEGER + 2,
    Number.POSITIVE_INFINITY,
    -0,
    Number.NaN,
    "2026-01-01",
    "12.50",
    "-0.00",
    "1e5",
  ),
);

const KEYS = [
  "all",
  "any",
  "not",
  "field",
  "op",
  "value",
  "ref",
  "extra",
  "__proto__",
  "constructor",
];
const OPS = ["eq", "ne", "gt", "gte", "lt", "lte", "in", "contains", "isnull", "bogus"];
const FIELDS = [...Object.keys(TYPES), "zz", "toString", "__proto__"];

const rawCondition: fc.Arbitrary<unknown> = fc.letrec((tie) => ({
  tree: fc.oneof(
    { depthSize: "small", withCrossShrink: true },
    tie("leaf"),
    fc.record({ all: fc.array(tie("tree"), { minLength: 0, maxLength: 4 }) }),
    fc.record({ any: fc.array(tie("tree"), { minLength: 0, maxLength: 4 }) }),
    fc.record({ not: tie("tree") }),
    fc.dictionary(fc.constantFrom(...KEYS), tie("tree"), { maxKeys: 3 }),
    junk,
  ),
  leaf: fc.record(
    {
      field: fc.constantFrom(...FIELDS),
      op: fc.constantFrom(...OPS),
      value: fc.oneof(junk, fc.array(junk, { maxLength: 4 })),
      ref: fc.constantFrom(...FIELDS),
    },
    { requiredKeys: ["field", "op"] },
  ),
})).tree;

const record = fc.dictionary(
  fc.constantFrom(...Object.keys(TYPES), "__proto__", "toString"),
  junk,
  {
    maxKeys: 9,
  },
);

describe("property: evaluator total", () => {
  test("kondisi: parse dan eval tidak pernah melempar", () => {
    fc.assert(
      fc.property(rawCondition, record, (raw, rec) => {
        // sengaja memanggil lapisan dalam (tanpa try/catch pembungkus) agar lemparan internal terdeteksi
        const p = parseCondition(raw);
        if (!p.ok) return p.reason.path.startsWith("$");
        const r = evalCondition(p.value, TYPES, rec);
        return !(r.value && r.reasons.length > 0);
      }),
      { numRuns: 3000 },
    );
  });

  test("ekspresi dan templat: tidak pernah melempar", () => {
    fc.assert(
      fc.property(rawCondition, junk, record, fc.string(), (cond, any, rec, tpl) => {
        for (const e of [
          any,
          { concat: [cond, any] },
          { coalesce: [any, { field: "b" }] },
          { if: cond, [THEN]: any, else: { lit: "x" } },
          { add: [any, { field: "b" }], scale: 2, round: "half_up" },
        ]) {
          const p = parseExpr(e);
          if (p.ok) evalExpr(p.value, TYPES, rec);
        }
        const t = parseLabelTemplate(tpl);
        if (t.ok) renderLabel(t.value, TYPES, rec);
        return true;
      }),
      { numRuns: 3000 },
    );
  });

  test("struktur raksasa ditolak tanpa dieksekusi", () => {
    const leaf = { field: "a", op: "eq", value: 1 };
    let deep: unknown = leaf;
    for (let i = 0; i < 100_000; i++) deep = { not: deep };
    const wide = { all: Array.from({ length: 1_000_000 }, () => leaf) };
    for (const raw of [deep, wide]) {
      const p = parseCondition(raw);
      expect(p.ok).toBe(false);
    }
  });

  test("jumlah simpul dikunjungi sama dengan jumlah simpul (linear)", () => {
    const count = (c: ConditionNode): number =>
      c.kind === "leaf" ? 1 : 1 + c.children.reduce((n, k) => n + count(k), 0);
    const validLeaf = fc.record({
      field: fc.constantFrom(...Object.keys(TYPES)),
      op: fc.constantFrom("eq", "ne", "gt", "lt", "isnull"),
      value: fc.oneof(fc.integer(), fc.string(), fc.boolean()),
    });
    const tree = fc.letrec((tie) => ({
      n: fc.oneof(
        { depthSize: "small" },
        validLeaf,
        fc.record({ all: fc.array(tie("n"), { minLength: 1, maxLength: 3 }) }),
        fc.record({ any: fc.array(tie("n"), { minLength: 1, maxLength: 3 }) }),
        fc.record({ not: tie("n") }),
      ),
    })).n;
    let checked = 0;
    fc.assert(
      fc.property(tree, record, (raw, rec) => {
        const p = parseCondition(raw);
        if (!p.ok) return true;
        checked++;
        const ctx = makeCtx(TYPES, rec);
        evalCondNode(ctx, p.value, "$");
        return ctx.steps === count(p.value);
      }),
      { numRuns: 3000 },
    );
    expect(checked).toBeGreaterThan(300);
  });
});

// ---- desimal terhadap oracle rasional BigInt (formulasi lantai/pecahan, berbeda dari roundQuotient) ----

const decimalString = fc
  .tuple(
    fc.boolean(),
    fc.oneof(fc.constant("0"), fc.stringMatching(/^[1-9][0-9]{0,19}$/)),
    fc.option(fc.stringMatching(/^[0-9]{1,7}$/), { nil: undefined }),
  )
  .map(([neg, int, frac]) => `${neg ? "-" : ""}${int}${frac === undefined ? "" : `.${frac}`}`);

const p10 = (n: number) => 10n ** BigInt(n);
const dec = (s: string): Decimal => parseDecimal(s) as Decimal;

/** Membulatkan pecahan num/den ke skala dengan lantai dan sisa. */
function refRound(num: bigint, den: bigint, scale: number, mode: RoundMode): bigint {
  if (den < 0n) {
    num = -num;
    den = -den;
  }
  const y = num * p10(scale); // nilai × 10^scale = y/den
  let fl = y / den;
  if (y % den !== 0n && y < 0n) fl -= 1n; // lantai (BigInt memotong ke nol)
  const rem = y - fl * den; // 0 <= rem < den
  const ce = rem > 0n ? fl + 1n : fl;
  const neg = y < 0n;
  const away = neg ? fl : ce;
  const toward = neg ? ce : fl;
  switch (mode) {
    case "floor":
      return fl;
    case "ceiling":
      return ce;
    case "down":
      return toward;
    case "up":
      return away;
  }
  const c = 2n * rem === den ? 0 : 2n * rem > den ? 1 : -1;
  if (c > 0) return ce;
  if (c < 0) return fl;
  if (mode === "half_up") return away;
  if (mode === "half_down") return toward;
  return fl % 2n === 0n ? fl : ce;
}

describe("property: desimal", () => {
  test("round dan div sesuai oracle; add/sub/mul/cmp eksak", () => {
    fc.assert(
      fc.property(
        decimalString,
        decimalString,
        fc.integer({ min: 0, max: 6 }),
        fc.constantFrom(...ROUND_MODES),
        (as, bs, scale, mode) => {
          const a = dec(as);
          const b = dec(bs);
          const sa = a.scale;
          const sb = b.scale;
          // nilai a = a.coef / 10^sa
          const r = roundDecimal(a, scale, mode);
          if (r.scale !== scale || r.coef !== refRound(a.coef, p10(sa), scale, mode)) return false;
          const q = divDecimal(a, b, scale, mode);
          if (b.coef === 0n) return q === null;
          if (q === null) return false;
          // a/b = (a.coef × 10^sb) / (b.coef × 10^sa)
          if (
            q.scale !== scale ||
            q.coef !== refRound(a.coef * p10(sb), b.coef * p10(sa), scale, mode)
          )
            return false;
          const sum = addDecimal(a, b);
          const dif = subDecimal(a, b);
          const prod = mulDecimal(a, b);
          const m = Math.max(sa, sb);
          const al = a.coef * p10(m - sa);
          const bl = b.coef * p10(m - sb);
          return (
            sum.coef === al + bl &&
            sum.scale === m &&
            dif.coef === al - bl &&
            prod.coef === a.coef * b.coef &&
            prod.scale === sa + sb &&
            compareDecimal(a, b) === (al === bl ? 0 : al < bl ? -1 : 1)
          );
        },
      ),
      { numRuns: 4000 },
    );
  });

  test("format dan parse bolak-balik; tidak ada nol bertanda minus", () => {
    fc.assert(
      fc.property(decimalString, (s) => {
        const d = dec(s);
        const f = formatDecimal(d);
        const back = dec(f);
        return (
          back.coef === d.coef &&
          back.scale === d.scale &&
          f === formatDecimal(back) &&
          !/^-0(\.0+)?$/.test(f)
        );
      }),
      { numRuns: 2000 },
    );
  });

  test("masukan tidak kanonik ditolak", () => {
    for (const s of ["", "-", "+1", "1.", ".5", "01", "1e5", " 1", "1 ", "NaN", "0x1", "1_0"]) {
      expect(parseDecimal(s)).toBeNull();
    }
    expect(parseDecimal("1".repeat(65))).toBeNull();
    expect(parseDecimal("1".repeat(64))).not.toBeNull();
  });
});

describe("property: teks", () => {
  test("compareText = urutan code point setelah normalisasi", () => {
    const cps = (s: string) => Array.from(normText(s), (c) => c.codePointAt(0) as number);
    fc.assert(
      fc.property(fc.string({ unit: "binary" }), fc.string({ unit: "binary" }), (a, b) => {
        const x = cps(a);
        const y = cps(b);
        let want = 0;
        for (let i = 0; i < Math.min(x.length, y.length) && want === 0; i++) {
          want =
            (x[i] as number) < (y[i] as number) ? -1 : (x[i] as number) > (y[i] as number) ? 1 : 0;
        }
        if (want === 0) want = x.length === y.length ? 0 : x.length < y.length ? -1 : 1;
        return compareText(normText(a), normText(b)) === want;
      }),
      { numRuns: 4000 },
    );
  });
});
