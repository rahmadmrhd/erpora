// Evaluator kondisi (Requirement 4): pohon all/any/not dengan daun {field, op, value|ref}. Fungsi total: input apa
// pun menghasilkan hasil (tidak pernah melempar), berbiaya linear terhadap jumlah simpul. Semantik dan kode alasan
// identik dengan sdk/metadata (condition.go); paritas dibuktikan korpus vektor bersama.

import {
  MAX_CONDITION_DEPTH,
  MAX_CONDITION_NODES,
  MAX_IDENTIFIER_LEN,
  MAX_IN_VALUES,
} from "./limits";
import {
  type FieldTypes,
  type Parsed,
  REASON,
  type Reason,
  type ReasonCode,
  type RecordData,
} from "./types";
import { asciiFold, cmpValues, coerce, familyOf, type Kind, NULL_VALUE, type Value } from "./value";

export type ConditionOp = "eq" | "ne" | "gt" | "gte" | "lt" | "lte" | "in" | "contains" | "isnull";

const OPS: readonly string[] = ["eq", "ne", "gt", "gte", "lt", "lte", "in", "contains", "isnull"];

/** Simpul kondisi hasil penguraian (terlepas dari objek masukan; aman dari rantai prototipe). */
export type ConditionNode =
  | { readonly kind: "all" | "any"; readonly children: readonly ConditionNode[] }
  | { readonly kind: "not"; readonly children: readonly [ConditionNode] }
  | {
      readonly kind: "leaf";
      readonly field: string;
      readonly op: ConditionOp;
      /** Literal: null, boolean, string, number, atau larik (untuk `in`); untuk `isnull`: boolean/null (null = true). */
      readonly value: unknown;
      /** Tidak kosong: bandingkan dengan field lain (value harus null). */
      readonly ref: string;
    };

/** Hasil evaluasi. `value` false disertai `reasons` berarti tak tentu atau struktur tidak sah. */
export interface EvalResult {
  readonly value: boolean;
  readonly reasons: readonly Reason[];
}

export interface Budget {
  nodes: number;
  readonly max: number;
}

const utf8 = new TextEncoder();

/** Nama field: tidak kosong dan paling banyak 63 byte UTF-8 (sama dengan Go). */
export function isIdentLen(s: unknown): s is string {
  return typeof s === "string" && s !== "" && utf8.encode(s).length <= MAX_IDENTIFIER_LEN;
}

export function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isScalarLiteral(v: unknown): boolean {
  return (
    v === null ||
    typeof v === "boolean" ||
    typeof v === "string" ||
    (typeof v === "number" && Number.isFinite(v))
  );
}

function leafCode(field: unknown, op: unknown, value: unknown, ref: string): ReasonCode | "" {
  const bad = REASON.invalidCondition;
  if (!isIdentLen(field) || typeof op !== "string" || !OPS.includes(op)) return bad;
  if (utf8.encode(ref).length > MAX_IDENTIFIER_LEN) return bad;
  switch (op) {
    case "isnull":
      if (ref !== "" || (value !== null && typeof value !== "boolean")) return bad;
      break;
    case "in": {
      if (ref !== "" || !Array.isArray(value) || value.length === 0) return bad;
      if (value.length > MAX_IN_VALUES) return REASON.listTooLong;
      for (const e of value) if (e === null || !isScalarLiteral(e)) return bad;
      break;
    }
    case "eq":
    case "ne":
      if ((ref !== "" && value !== null) || !isScalarLiteral(value)) return bad;
      break;
    default: // gt, gte, lt, lte, contains
      if (ref !== "") {
        if (value !== null) return bad;
      } else if (value === null || !isScalarLiteral(value)) return bad;
  }
  return "";
}

type PR = { readonly node: ConditionNode } | { readonly reason: Reason };

/** Menguraikan kondisi mentah secara pre-order; berhenti pada pelanggaran pertama (batas, lalu bentuk). */
export function parseCondNode(raw: unknown, path: string, depth: number, b: Budget): PR {
  if (depth > MAX_CONDITION_DEPTH) return { reason: { code: REASON.tooDeep, path } };
  b.nodes++;
  if (b.nodes > b.max) return { reason: { code: REASON.tooManyNodes, path } };
  const invalid: PR = { reason: { code: REASON.invalidCondition, path } };
  if (!isPlainObject(raw)) return invalid;
  const m = raw;
  const has = (k: string) => Object.hasOwn(m, k);
  const keys = Object.keys(m);
  if (has("all") || has("any") || has("not")) {
    if (keys.length !== 1) return invalid;
    if (has("not")) {
      const r = parseCondNode(m.not, `${path}.not`, depth + 1, b);
      return "reason" in r ? r : { node: { kind: "not", children: [r.node] } };
    }
    const key = has("any") ? "any" : "all";
    const list = m[key];
    if (!Array.isArray(list) || list.length === 0) return invalid;
    const children: ConditionNode[] = [];
    for (let i = 0; i < list.length; i++) {
      const r = parseCondNode(list[i], `${path}.${key}[${i}]`, depth + 1, b);
      if ("reason" in r) return r;
      children.push(r.node);
    }
    return { node: { kind: key, children } };
  }
  for (const k of keys)
    if (k !== "field" && k !== "op" && k !== "value" && k !== "ref") return invalid;
  const field = m.field;
  const op = m.op;
  if (typeof field !== "string" || typeof op !== "string") return invalid;
  const hasValue = has("value");
  const hasRef = has("ref");
  let ref = "";
  if (hasRef) {
    if (typeof m.ref !== "string" || m.ref === "") return invalid;
    ref = m.ref;
  }
  if (hasRef && hasValue) return invalid;
  if (!hasRef && !hasValue && op !== "isnull") return invalid;
  const value = hasValue ? m.value : null;
  const code = leafCode(field, op, value, ref);
  if (code !== "") return { reason: { code, path } };
  return { node: { kind: "leaf", field, op: op as ConditionOp, value, ref } };
}

/** Menguraikan kondisi mentah (JSON hasil dekode). Fungsi total. */
export function parseCondition(raw: unknown): Parsed<ConditionNode> {
  const r = parseCondNode(raw, "$", 1, { nodes: 0, max: MAX_CONDITION_NODES });
  return "reason" in r ? { ok: false, reason: r.reason } : { ok: true, value: r.node };
}

/** Konteks satu evaluasi; steps menghitung simpul yang dikunjungi (untuk uji linear). */
export interface Ctx {
  readonly types: FieldTypes;
  readonly record: RecordData;
  steps: number;
}

export function makeCtx(types: FieldTypes, record: RecordData): Ctx {
  return { types: types ?? {}, record: record ?? {}, steps: 0 };
}

/** Mengambil dan mengoersi nilai field; mengembalikan nilai atau kode alasan. */
export function fieldValue(ctx: Ctx, name: string): Value | ReasonCode {
  if (!Object.hasOwn(ctx.types, name)) return REASON.unknownField;
  const raw = Object.hasOwn(ctx.record, name) ? ctx.record[name] : undefined;
  if (raw === undefined) return REASON.fieldUnavailable;
  return coerce(ctx.types[name] as string, raw);
}

const F = 0;
const T = 1;
const U = 2;
type Tri = 0 | 1 | 2;
type TR = readonly [Tri, readonly Reason[]];
const TRUE: TR = [T, []];
const FALSE: TR = [F, []];
const tri = (b: boolean): TR => (b ? TRUE : FALSE);

function orderable(ft: string): boolean {
  return (
    ft === "string" ||
    ft === "text" ||
    ft === "integer" ||
    ft === "decimal" ||
    ft === "date" ||
    ft === "datetime"
  );
}

function textual(ft: string): boolean {
  return ft === "string" || ft === "text";
}

function opSupported(op: ConditionOp, ft: string): boolean {
  switch (op) {
    case "gt":
    case "gte":
    case "lt":
    case "lte":
      return orderable(ft);
    case "contains":
      return textual(ft);
  }
  return true;
}

export function evalCondNode(ctx: Ctx, c: ConditionNode, path: string): TR {
  ctx.steps++;
  switch (c.kind) {
    case "all":
    case "any": {
      let sawTrue = false;
      let sawFalse = false;
      const unk: Reason[] = [];
      for (let i = 0; i < c.children.length; i++) {
        const [t, rs] = evalCondNode(
          ctx,
          c.children[i] as ConditionNode,
          `${path}.${c.kind}[${i}]`,
        );
        if (t === T) sawTrue = true;
        else if (t === F) sawFalse = true;
        else unk.push(...rs);
      }
      if (c.kind === "all") {
        if (sawFalse) return FALSE;
        return unk.length > 0 ? [U, unk] : TRUE;
      }
      if (sawTrue) return TRUE;
      return unk.length > 0 ? [U, unk] : FALSE;
    }
    case "not": {
      const [t, rs] = evalCondNode(ctx, c.children[0], `${path}.not`);
      if (t === T) return FALSE;
      if (t === F) return TRUE;
      return [U, rs];
    }
  }
  return evalLeaf(ctx, c, path);
}

function evalLeaf(ctx: Ctx, c: Extract<ConditionNode, { kind: "leaf" }>, path: string): TR {
  const unk = (code: ReasonCode): TR => [U, [{ code, path }]];
  if (!Object.hasOwn(ctx.types, c.field)) return unk(REASON.unknownField);
  const ft = ctx.types[c.field] as string;
  const a = fieldValue(ctx, c.field);
  if (typeof a === "string") return unk(a);
  if (c.op === "isnull") {
    const want = typeof c.value === "boolean" ? c.value : true;
    return tri((a.k === "null") === want);
  }
  if (!opSupported(c.op, ft)) return unk(REASON.unsupportedOp);

  if (c.op === "in") {
    const vals: Value[] = [];
    for (const e of c.value as unknown[]) {
      const v = coerce(ft, e);
      if (typeof v === "string") return unk(v);
      vals.push(v);
    }
    if (a.k === "null") return FALSE;
    return tri(vals.some((v) => cmpValues(a, v) === 0));
  }

  let b: Value;
  if (c.ref !== "") {
    if (!Object.hasOwn(ctx.types, c.ref)) return unk(REASON.unknownField);
    const rt = ctx.types[c.ref] as string;
    const rv = fieldValue(ctx, c.ref);
    if (typeof rv === "string") return unk(rv);
    b = rv;
    if (!opSupported(c.op, rt)) return unk(REASON.unsupportedOp);
    const fa: Kind | null = familyOf(ft);
    if (fa !== familyOf(rt)) return unk(REASON.typeMismatch);
  } else if (c.value === null) {
    b = NULL_VALUE; // literal null (hanya eq/ne yang lolos validasi)
  } else {
    const lv = coerce(ft, c.value);
    if (typeof lv === "string") return unk(lv);
    b = lv;
  }

  if (c.op === "eq" || c.op === "ne") {
    const eq =
      (a.k === "null" && b.k === "null") ||
      (a.k !== "null" && b.k !== "null" && cmpValues(a, b) === 0);
    return tri(eq === (c.op === "eq"));
  }
  if (c.op === "contains") {
    if (a.k === "null" || b.k === "null") return FALSE;
    return tri(asciiFold(a.s).includes(asciiFold(b.s)));
  }
  if (a.k === "null" || b.k === "null") return FALSE;
  const n = cmpValues(a, b);
  switch (c.op) {
    case "gt":
      return tri(n > 0);
    case "gte":
      return tri(n >= 0);
    case "lt":
      return tri(n < 0);
  }
  return tri(n <= 0); // lte
}

/** Mengevaluasi kondisi hasil `parseCondition`. Fungsi total. */
export function evalCondition(
  node: ConditionNode,
  types: FieldTypes,
  record: RecordData,
): EvalResult {
  const [t, rs] = evalCondNode(makeCtx(types, record), node, "$");
  return t === U ? { value: false, reasons: rs } : { value: t === T, reasons: [] };
}

/**
 * Menafsirkan kondisi mentah (JSON hasil dekode atau literal bangkitan) lalu mengevaluasinya. Fungsi total:
 * struktur tidak sah menghasilkan `value: false` dengan satu alasan; tidak pernah melempar.
 */
export function evaluateCondition(raw: unknown, types: FieldTypes, record: RecordData): EvalResult {
  try {
    const p = parseCondition(raw);
    if (!p.ok) return { value: false, reasons: [p.reason] };
    return evalCondition(p.value, types, record);
  } catch {
    // Pertahanan lapis kedua: evaluator total; tidak ada jalur yang diketahui mencapai ini.
    return { value: false, reasons: [{ code: REASON.invalidCondition, path: "$" }] };
  }
}
