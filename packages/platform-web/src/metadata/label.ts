// Templat label (Requirement 5.5): teks tetap dan placeholder {field}; hasil render selalu teks biasa (bukan
// HTML). Perilaku identik dengan sdk/metadata (label.go).

import { fieldValue, makeCtx } from "./condition";
import { MAX_IDENTIFIER_LEN, MAX_TEMPLATE_LEN, MAX_TEMPLATE_PLACEHOLDERS } from "./limits";
import { type FieldTypes, type Parsed, REASON, type Reason, type RecordData } from "./types";
import { normText, valueText } from "./value";

type Seg =
  | { readonly lit: string; readonly field: "" }
  | { readonly lit: ""; readonly field: string };

/** Templat label yang sudah diurai. */
export interface LabelTemplate {
  readonly segs: readonly Seg[];
}

/**
 * Hasil render: placeholder bernilai null, tidak tersedia, atau tidak sah dirender sebagai teks kosong; sebab yang
 * bukan null dicatat di `reasons` (path berbentuk "{namaField}").
 */
export interface LabelResult {
  readonly text: string;
  readonly reasons: readonly Reason[];
}

// Keabsahan nama sebagai identifier metadata (camelCase ketat) diperiksa statis oleh metadata:check, bukan di sini.
const NAME_RE = /^[a-z][A-Za-z0-9]*$/;

/** Menguraikan templat; gagal dengan satu alasan `invalid_template`. Fungsi total. */
export function parseLabelTemplate(tpl: string): Parsed<LabelTemplate> {
  const bad: Parsed<LabelTemplate> = {
    ok: false,
    reason: { code: REASON.invalidTemplate, path: "$" },
  };
  if (typeof tpl !== "string" || tpl.length > MAX_TEMPLATE_LEN) return bad;
  const segs: Seg[] = [];
  let lit = "";
  let placeholders = 0;
  for (let i = 0; i < tpl.length; i++) {
    const c = tpl[i] as string;
    if (c === "{") {
      const end = tpl.indexOf("}", i + 1);
      if (end < 0) return bad;
      const name = tpl.slice(i + 1, end);
      if (name.length > MAX_IDENTIFIER_LEN || !NAME_RE.test(name)) return bad;
      placeholders++;
      if (placeholders > MAX_TEMPLATE_PLACEHOLDERS) return bad;
      if (lit !== "") {
        segs.push({ lit, field: "" });
        lit = "";
      }
      segs.push({ lit: "", field: name });
      i = end;
    } else if (c === "}") {
      return bad;
    } else {
      lit += c;
    }
  }
  if (lit !== "") segs.push({ lit, field: "" });
  return { ok: true, value: { segs } };
}

/** Menyusun label dari record. Fungsi total. */
export function renderLabel(t: LabelTemplate, types: FieldTypes, record: RecordData): LabelResult {
  const ctx = makeCtx(types, record);
  let text = "";
  const reasons: Reason[] = [];
  for (const s of t.segs) {
    if (s.field === "") {
      text += normText(s.lit);
      continue;
    }
    const v = fieldValue(ctx, s.field);
    if (typeof v === "string") {
      reasons.push({ code: v, path: `{${s.field}}` });
      continue;
    }
    text += valueText(v);
  }
  return { text, reasons };
}
