// Tipe bersama evaluator metadata. Perilaku dan kode alasan identik dengan sdk/metadata (types.go).

/** Tipe field metadata yang dikenal evaluator. */
export type FieldType =
  | "string"
  | "text"
  | "integer"
  | "decimal"
  | "boolean"
  | "date"
  | "datetime"
  | "enum"
  | "uuid";

/** Tipe hasil ekspresi; "null" untuk nilai null. */
export type ValueType = FieldType | "null";

/** Peta nama field ke tipenya (dari metadata resource). Tipe yang tidak dikenal diperlakukan tak cocok. */
export type FieldTypes = Readonly<Record<string, string>>;

/**
 * Nilai field. Kunci yang tidak ada (atau bernilai undefined) berarti "tidak tersedia" (api-guidelines §4.4),
 * bukan kosong; `null` berarti kosong. decimal selalu string desimal; integer bilangan bulat aman.
 */
export type RecordData = Readonly<Record<string, unknown>>;

export const REASON = {
  invalidCondition: "invalid_condition",
  invalidExpression: "invalid_expression",
  invalidTemplate: "invalid_template",
  tooDeep: "too_deep",
  tooManyNodes: "too_many_nodes",
  listTooLong: "list_too_long",
  unknownField: "unknown_field",
  fieldUnavailable: "field_unavailable",
  typeMismatch: "type_mismatch",
  unsupportedOp: "unsupported_op",
  valueTooLong: "value_too_long",
  divisionByZero: "division_by_zero",
} as const;

export type ReasonCode = (typeof REASON)[keyof typeof REASON];

/** Alasan hasil tak tentu atau struktur tidak sah; `path` menunjuk simpul (mis. "$.all[1].not", "{field}"). */
export interface Reason {
  readonly code: ReasonCode;
  readonly path: string;
}

/** Bentuk "kode@path", dipakai korpus vektor. */
export function formatReason(r: Reason): string {
  return `${r.code}@${r.path}`;
}

/** Hasil penguraian: berhasil membawa nilai, gagal membawa tepat satu alasan. */
export type Parsed<T> =
  | { readonly ok: true; readonly value: T }
  | { readonly ok: false; readonly reason: Reason };
