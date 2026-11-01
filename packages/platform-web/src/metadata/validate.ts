// Validator draf record di klien (Requirement 18.1): tipe, wajib, nullable, panjang, rentang, pola, enum, dan
// `rules`. Hanya kenyamanan: server tetap otoritatif (Requirement 4.7, 9) dan batasan di sini tidak pernah lebih
// ketat daripada server. Hasilnya berupa kode isu dan parameter (bukan teks): teks UI dirender pemanggil lewat i18n.

import type { MessageRef } from "@erpora/module-sdk";
import { evaluateCondition } from "./condition";
import { compareDecimal, type Decimal, decimalFromInt, parseDecimal } from "./decimal";
import {
  type Condition,
  type FieldMeta,
  fieldTypes,
  isFieldAvailable,
  labelRef,
  type ResourceMeta,
} from "./meta";
import { testPattern } from "./pattern";
import { coerce } from "./value";

export type IssueCode =
  | "required"
  | "invalid_type"
  | "too_short"
  | "too_long"
  | "pattern"
  | "format"
  | "below_minimum"
  | "above_maximum"
  | "not_in_values"
  | "decimal_precision"
  | "decimal_scale"
  | "rule";

/**
 * Satu isu validasi. `code` dan `params` dipetakan pemanggil ke pesan i18n (namespace `platform`/`ui`); isu `rule`
 * membawa `message` (MessageRef ke katalog modul pemilik resource) dan `ruleId`. Tidak pernah memuat nilai field.
 */
export interface ValidationIssue {
  readonly field: string;
  readonly code: IssueCode;
  readonly params?: Readonly<Record<string, string | number>>;
  readonly ruleId?: string;
  readonly message?: MessageRef;
}

export interface ValidateOptions {
  /**
   * `create` (default): field yang tidak ada dianggap kosong (wajib bila non-nullable tanpa bawaan).
   * `update`: field yang tidak ada (atau tidak tersedia) dilewati; hanya yang dikirim divalidasi.
   */
  readonly mode?: "create" | "update";
}

const ENUM_VALUE_RE = /^[a-z][a-z0-9_]{0,62}$/;
// Sengaja longgar (klien tidak boleh lebih ketat dari server): satu "@", tanpa spasi.
const EMAIL_RE = /^[^\s@]+@[^\s@]+$/;
// format url hanya memeriksa skema http/https (design: SSRF; tidak pernah di-fetch).
const URL_RE = /^https?:\/\/\S+$/i;

/** true/false bila pasti; null bila tak tentu (field tidak tersedia, tipe tak cocok, dst.). */
function holds(
  cond: Condition | undefined,
  meta: ResourceMeta,
  record: Readonly<Record<string, unknown>>,
): boolean | null {
  if (cond === undefined) return null;
  const r = evaluateCondition(cond, fieldTypes(meta), record);
  return r.reasons.length > 0 ? null : r.value;
}

function codePoints(s: string): number {
  let n = 0;
  for (const _ of s) n++;
  return n;
}

function bound(v: number | string | undefined): Decimal | null {
  if (v === undefined) return null;
  if (typeof v === "number") return Number.isSafeInteger(v) ? decimalFromInt(v) : null;
  return parseDecimal(v);
}

/** Menghapus nol di ekor pecahan: 1.500 dan 1.5 setara. */
function trimmed(d: Decimal): { digits: number; scale: number } {
  let { coef, scale } = d;
  if (coef < 0n) coef = -coef;
  while (scale > 0 && coef % 10n === 0n) {
    coef /= 10n;
    scale--;
  }
  return { digits: coef === 0n ? 0 : coef.toString().length, scale };
}

function checkValue(f: FieldMeta, v: unknown, out: ValidationIssue[]): void {
  const push = (code: IssueCode, params?: Record<string, string | number>) =>
    out.push(params ? { field: f.name, code, params } : { field: f.name, code });
  const typed = coerce(f.type, v);
  if (typeof typed === "string") {
    // tipe salah, atau teks lebih panjang dari batas evaluator (65.536 satuan UTF-16)
    push(
      typed === "value_too_long" ? "too_long" : "invalid_type",
      typed === "value_too_long" ? { max: f.maxLength ?? 65536 } : undefined,
    );
    return;
  }
  switch (f.type) {
    case "string":
    case "text": {
      const s = typed.s;
      const n = codePoints(s);
      if (f.minLength !== undefined && n < f.minLength) push("too_short", { min: f.minLength });
      if (f.maxLength !== undefined && n > f.maxLength) push("too_long", { max: f.maxLength });
      if (f.pattern !== undefined && testPattern(f.pattern, s).match === false) push("pattern");
      if (f.format === "email" && !EMAIL_RE.test(s)) push("format");
      if (f.format === "url" && !URL_RE.test(s)) push("format");
      return;
    }
    case "enum": {
      if (f.values === undefined) return;
      const ok = f.extensible ? ENUM_VALUE_RE.test(typed.s) : f.values.includes(typed.s);
      if (!ok) push("not_in_values");
      return;
    }
    case "integer":
    case "decimal": {
      if (f.type === "decimal" && (f.precision !== undefined || f.scale !== undefined)) {
        const scale = f.scale ?? 0;
        const t = trimmed(typed.num);
        if (t.scale > scale) push("decimal_scale", { scale });
        if (f.precision !== undefined && t.digits - t.scale > f.precision - scale) {
          push("decimal_precision", { precision: f.precision, scale });
        }
      }
      const min = bound(f.minimum);
      const max = bound(f.maximum);
      if (min && compareDecimal(typed.num, min) < 0)
        push("below_minimum", { min: String(f.minimum) });
      if (max && compareDecimal(typed.num, max) > 0)
        push("above_maximum", { max: String(f.maximum) });
      return;
    }
  }
}

/**
 * Memvalidasi draf record terhadap metadata. Field readOnly dan accessor dilewati; field yang pasti tersembunyi
 * (`visibleWhen` salah) atau sedang terkunci (`readonlyWhen` benar) dilewati; `requiredWhen` menambah kewajiban.
 * Aturan `rules` yang hasilnya tak tentu (field tidak tersedia, null, tipe tak cocok) dilewati: server memutuskan.
 * Fungsi total: tidak melempar untuk nilai apa pun.
 */
export function validateRecord(
  meta: ResourceMeta,
  record: Readonly<Record<string, unknown>>,
  options: ValidateOptions = {},
): ValidationIssue[] {
  const mode = options.mode ?? "create";
  const out: ValidationIssue[] = [];
  for (const f of meta.fields) {
    if (f.readOnly || f.accessor) continue;
    const available = isFieldAvailable(record, f.name);
    if (!available && mode === "update") continue;
    if (holds(f.visibleWhen, meta, record) === false) continue;
    if (holds(f.readonlyWhen, meta, record) === true) continue;
    const v = available ? record[f.name] : undefined;
    const empty = v === null || (v === undefined && f.default === undefined);
    if (empty) {
      const required = !f.nullable || holds(f.requiredWhen, meta, record) === true;
      if (required) out.push({ field: f.name, code: "required" });
      continue;
    }
    if (v === undefined) continue; // tidak dikirim, server memakai bawaan
    checkValue(f, v, out);
  }
  for (const r of meta.rules) {
    const res = evaluateCondition(r.assert, fieldTypes(meta), record);
    if (res.value || res.reasons.length > 0) continue;
    out.push({
      field: r.field ?? "",
      code: "rule",
      ruleId: r.id,
      message: labelRef(meta, r.message),
    });
  }
  return out;
}
