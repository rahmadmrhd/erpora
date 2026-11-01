// Pelari korpus vektor bersama (dipakai vectors.test.ts dan parity.test.ts); bukan test.
import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import {
  addDecimal,
  compareDecimal,
  type Decimal,
  divDecimal,
  evaluateCondition,
  evaluateExpr,
  formatDecimal,
  formatReason,
  mulDecimal,
  parseDecimal,
  parseLabelTemplate,
  type RoundMode,
  renderLabel,
  roundDecimal,
  subDecimal,
} from "./index";

// Korpus vektor bersama (Requirement 4.4): berkas yang sama dijalankan evaluator Go (sdk/metadata).
export function loadVectors(rel: string): Record<string, unknown>[] {
  const url = new URL(`../../../../contracts/metadata-vectors/${rel}`, import.meta.url);
  const doc = JSON.parse(readFileSync(fileURLToPath(url), "utf8")) as {
    cases: Record<string, unknown>[];
  };
  if (!Array.isArray(doc.cases) || doc.cases.length === 0) throw new Error(`korpus ${rel} kosong`);
  return doc.cases;
}

// biome-ignore lint/suspicious/noExplicitAny: kasus korpus JSON bersifat dinamis
export type Case = Record<string, any>;
export const short = (s: unknown) =>
  String(s).length > 120 ? `${String(s).slice(0, 120)}...` : String(s);

export function runConditionCases(cases: Case[]) {
  for (const c of cases) {
    test(short(c.name), () => {
      const got = evaluateCondition(c.condition, c.types, c.record);
      expect(got.value).toBe(c.expect.result);
      expect(got.reasons.map(formatReason)).toEqual(c.expect.reasons);
    });
  }
}

export function runExprCases(cases: Case[]) {
  for (const c of cases) {
    test(short(c.name), () => {
      const got = evaluateExpr(c.expr, c.types, c.record);
      expect(got.type).toBe(c.expect.type);
      expect(got.value).toEqual(c.expect.value);
      expect(got.reasons.map(formatReason)).toEqual(c.expect.reasons);
    });
  }
}

export function runLabelCases(cases: Case[]) {
  for (const c of cases) {
    test(short(c.name), () => {
      const p = parseLabelTemplate(c.template);
      if (c.expect.error !== undefined) {
        expect(p.ok).toBe(false);
        if (!p.ok) expect(p.reason.code).toBe(c.expect.error);
        return;
      }
      expect(p.ok).toBe(true);
      if (!p.ok) return;
      const got = renderLabel(p.value, c.types, c.record);
      expect(got.text).toBe(c.expect.text);
      expect(got.reasons.map(formatReason)).toEqual(c.expect.reasons);
    });
  }
}

export function runDecimalCases(cases: Case[]) {
  const dec = (s: unknown): Decimal => {
    const d = parseDecimal(s as string);
    if (d === null) throw new Error(`parseDecimal(${s}) ditolak`);
    return d;
  };
  for (const c of cases) {
    test(short(c.name), () => {
      switch (c.op) {
        case "parse": {
          const d = parseDecimal(c.in);
          expect(d === null ? null : formatDecimal(d)).toBe(c.out);
          break;
        }
        case "cmp":
          expect(compareDecimal(dec(c.a), dec(c.b))).toBe(c.out);
          break;
        case "add":
          expect(formatDecimal(addDecimal(dec(c.a), dec(c.b)))).toBe(c.out);
          break;
        case "sub":
          expect(formatDecimal(subDecimal(dec(c.a), dec(c.b)))).toBe(c.out);
          break;
        case "mul":
          expect(formatDecimal(mulDecimal(dec(c.a), dec(c.b)))).toBe(c.out);
          break;
        case "round":
          expect(formatDecimal(roundDecimal(dec(c.a), c.scale, c.mode as RoundMode))).toBe(c.out);
          break;
        case "div": {
          const q = divDecimal(dec(c.a), dec(c.b), c.scale, c.mode as RoundMode);
          expect(q === null ? null : formatDecimal(q)).toBe(c.out);
          break;
        }
        default:
          throw new Error(`op tak dikenal: ${c.op}`);
      }
    });
  }
}
