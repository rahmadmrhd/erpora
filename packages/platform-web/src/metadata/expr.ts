// Evaluator ekspresi accessor (Requirement 5.2): gabung teks, coalesce, aritmetika desimal dengan skala dan mode
// pembulatan eksplisit, dan `if` memakai kondisi. Fungsi total; semantik identik dengan sdk/metadata (expr.go).

import {
  type Budget,
  type ConditionNode,
  type Ctx,
  evalCondNode,
  fieldValue,
  isIdentLen,
  isPlainObject,
  makeCtx,
  parseCondNode,
} from "./condition";
import {
  addDecimal,
  type Decimal,
  decimalFromInt,
  divDecimal,
  formatDecimal,
  isRoundMode,
  mulDecimal,
  parseDecimal,
  type RoundMode,
  roundDecimal,
  subDecimal,
} from "./decimal";
import {
  MAX_ARITH_ARGS,
  MAX_CONCAT_ARGS,
  MAX_EXPR_DEPTH,
  MAX_EXPR_NODES,
  MAX_SCALE,
} from "./limits";
import {
  type FieldTypes,
  type Parsed,
  REASON,
  type Reason,
  type ReasonCode,
  type RecordData,
  type ValueType,
} from "./types";
import { mkValue, NULL_VALUE, normText, tooLong, type Value, valueText } from "./value";

type ArithKind = "add" | "sub" | "mul" | "div";

/** Simpul ekspresi hasil penguraian. */
export type ExprNode =
  | { readonly kind: "lit"; readonly lit: null | boolean | string | number }
  | { readonly kind: "dec"; readonly dec: Decimal }
  | { readonly kind: "field"; readonly field: string }
  | { readonly kind: "concat" | "coalesce"; readonly args: readonly ExprNode[] }
  | {
      readonly kind: ArithKind;
      readonly args: readonly ExprNode[];
      readonly scale: number;
      readonly round: RoundMode;
    }
  | {
      readonly kind: "if";
      readonly cond: ConditionNode;
      readonly whenTrue: ExprNode;
      readonly whenFalse: ExprNode;
    };

/**
 * Hasil ekspresi. `type: "null"` dengan `value: null` berarti null; bila `reasons` tidak kosong, hasil tak tentu.
 * `value`: null, boolean, string (desimal selalu string), atau number (integer).
 */
export interface ExprResult {
  readonly type: ValueType;
  readonly value: null | boolean | string | number;
  readonly reasons: readonly Reason[];
}

const FORMS = [
  "lit",
  "dec",
  "field",
  "concat",
  "coalesce",
  "add",
  "sub",
  "mul",
  "div",
  "if",
] as const;
type Form = (typeof FORMS)[number];

type PR = { readonly node: ExprNode } | { readonly reason: Reason };

function safeInt(v: unknown): number | null {
  return typeof v === "number" && Number.isSafeInteger(v) ? v : null;
}

function parseExprNode(raw: unknown, path: string, depth: number, b: Budget): PR {
  if (depth > MAX_EXPR_DEPTH) return { reason: { code: REASON.tooDeep, path } };
  b.nodes++;
  if (b.nodes > b.max) return { reason: { code: REASON.tooManyNodes, path } };
  const bad: PR = { reason: { code: REASON.invalidExpression, path } };
  if (!isPlainObject(raw)) return bad;
  const m = raw;
  let form: Form | null = null;
  let forms = 0;
  for (const f of FORMS) {
    if (Object.hasOwn(m, f)) {
      form = f;
      forms++;
    }
  }
  if (forms !== 1 || form === null) return bad;
  const want =
    form === "add" || form === "sub" || form === "mul" || form === "div" || form === "if" ? 3 : 1;
  if (Object.keys(m).length !== want) return bad;
  const val = m[form];

  switch (form) {
    case "lit": {
      if (val === null || typeof val === "boolean") return { node: { kind: "lit", lit: val } };
      if (typeof val === "string") {
        return tooLong(val) ? bad : { node: { kind: "lit", lit: val } };
      }
      const n = safeInt(val);
      return n === null ? bad : { node: { kind: "lit", lit: n } };
    }
    case "dec": {
      if (typeof val !== "string") return bad;
      const d = parseDecimal(val);
      return d === null ? bad : { node: { kind: "dec", dec: d } };
    }
    case "field":
      return isIdentLen(val) ? { node: { kind: "field", field: val } } : bad;
    case "concat":
    case "coalesce":
    case "add":
    case "sub":
    case "mul":
    case "div": {
      if (!Array.isArray(val)) return bad;
      let scale = 0;
      let round: RoundMode = "half_up";
      let min = 1;
      let max = MAX_CONCAT_ARGS;
      if (form === "add" || form === "sub" || form === "mul" || form === "div") {
        const s = safeInt(m.scale);
        if (s === null || s < 0 || s > MAX_SCALE || !isRoundMode(m.round)) return bad;
        scale = s;
        round = m.round;
        min = 2;
        max = form === "add" || form === "mul" ? MAX_ARITH_ARGS : 2;
      }
      if (val.length < min || val.length > max) return bad;
      const args: ExprNode[] = [];
      for (let i = 0; i < val.length; i++) {
        const r = parseExprNode(val[i], `${path}.${form}[${i}]`, depth + 1, b);
        if ("reason" in r) return r;
        args.push(r.node);
      }
      if (form === "concat" || form === "coalesce") return { node: { kind: form, args } };
      return { node: { kind: form, args, scale, round } };
    }
    default: {
      // if
      if (!Object.hasOwn(m, "then") || !Object.hasOwn(m, "else")) return bad;
      const c = parseCondNode(val, `${path}.if`, 1, b);
      if ("reason" in c) return c;
      const t = parseExprNode(m.then, `${path}.then`, depth + 1, b);
      if ("reason" in t) return t;
      const e = parseExprNode(m.else, `${path}.else`, depth + 1, b);
      if ("reason" in e) return e;
      return { node: { kind: "if", cond: c.node, whenTrue: t.node, whenFalse: e.node } };
    }
  }
}

/** Menguraikan ekspresi mentah (JSON hasil dekode). Fungsi total. */
export function parseExpr(raw: unknown): Parsed<ExprNode> {
  const r = parseExprNode(raw, "$", 1, { nodes: 0, max: MAX_EXPR_NODES });
  return "reason" in r ? { ok: false, reason: r.reason } : { ok: true, value: r.node };
}

type ER = readonly [Value, readonly Reason[]];

function evalExprNode(ctx: Ctx, e: ExprNode, path: string): ER {
  ctx.steps++;
  const unk = (code: ReasonCode): ER => [NULL_VALUE, [{ code, path }]];
  switch (e.kind) {
    case "lit": {
      const x = e.lit;
      if (typeof x === "boolean") return [mkValue({ k: "bool", ft: "boolean", b: x }), []];
      if (typeof x === "string") return [mkValue({ k: "text", ft: "string", s: normText(x) }), []];
      if (typeof x === "number") {
        return [mkValue({ k: "num", ft: "integer", num: decimalFromInt(x), isInt: true }), []];
      }
      return [NULL_VALUE, []];
    }
    case "dec":
      return [mkValue({ k: "num", ft: "decimal", num: e.dec }), []];
    case "field": {
      const v = fieldValue(ctx, e.field);
      return typeof v === "string" ? unk(v) : [v, []];
    }
    case "if": {
      const [t, rs] = evalCondNode(ctx, e.cond, `${path}.if`);
      if (t === 2) return [NULL_VALUE, rs];
      return evalExprNode(
        ctx,
        t === 1 ? e.whenTrue : e.whenFalse,
        `${path}.${t === 1 ? "then" : "else"}`,
      );
    }
  }

  const vals: Value[] = [];
  const unknown: Reason[] = [];
  for (let i = 0; i < e.args.length; i++) {
    const [v, rs] = evalExprNode(ctx, e.args[i] as ExprNode, `${path}.${e.kind}[${i}]`);
    unknown.push(...rs);
    vals.push(v);
  }
  if (unknown.length > 0) return [NULL_VALUE, unknown];

  if (e.kind === "concat" || e.kind === "coalesce") {
    if (e.kind === "coalesce") return [vals.find((v) => v.k !== "null") ?? NULL_VALUE, []];
    const s = vals.map(valueText).join("");
    return tooLong(s) ? unk(REASON.valueTooLong) : [mkValue({ k: "text", ft: "string", s }), []];
  }

  // aritmetika desimal
  let hasNull = false;
  for (const v of vals) {
    if (v.k === "null") hasNull = true;
    else if (v.k !== "num") return unk(REASON.typeMismatch);
  }
  if (hasNull) return [NULL_VALUE, []];
  const first = (vals[0] as Value).num;
  const rest = vals.slice(1).map((v) => v.num);
  const ar = e as Extract<ExprNode, { scale: number }>;
  let res: Decimal;
  switch (ar.kind) {
    case "add":
      res = rest.reduce(addDecimal, first);
      break;
    case "mul":
      res = rest.reduce(mulDecimal, first);
      break;
    case "sub":
      res = subDecimal(first, rest[0] as Decimal);
      break;
    default: {
      // div
      const q = divDecimal(first, rest[0] as Decimal, ar.scale, ar.round);
      if (q === null) return unk(REASON.divisionByZero);
      return [mkValue({ k: "num", ft: "decimal", num: q }), []];
    }
  }
  return [mkValue({ k: "num", ft: "decimal", num: roundDecimal(res, ar.scale, ar.round) }), []];
}

/** Mengevaluasi ekspresi hasil `parseExpr`. Fungsi total. */
export function evalExpr(node: ExprNode, types: FieldTypes, record: RecordData): ExprResult {
  const [v, rs] = evalExprNode(makeCtx(types, record), node, "$");
  if (rs.length > 0) return { type: "null", value: null, reasons: rs };
  switch (v.k) {
    case "null":
      return { type: "null", value: null, reasons: [] };
    case "bool":
      return { type: "boolean", value: v.b, reasons: [] };
    case "num":
      return v.isInt
        ? { type: "integer", value: Number(v.num.coef), reasons: [] }
        : { type: "decimal", value: formatDecimal(v.num), reasons: [] };
  }
  return { type: v.ft, value: v.s, reasons: [] };
}

/**
 * Menafsirkan ekspresi mentah lalu mengevaluasinya. Fungsi total: struktur tidak sah menghasilkan `type: "null"`
 * dengan satu alasan; tidak pernah melempar.
 */
export function evaluateExpr(raw: unknown, types: FieldTypes, record: RecordData): ExprResult {
  try {
    const p = parseExpr(raw);
    if (!p.ok) return { type: "null", value: null, reasons: [p.reason] };
    return evalExpr(p.value, types, record);
  } catch {
    return {
      type: "null",
      value: null,
      reasons: [{ code: REASON.invalidExpression, path: "$" }],
    };
  }
}
