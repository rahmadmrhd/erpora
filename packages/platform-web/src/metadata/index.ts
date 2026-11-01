// Evaluator metadata (spec resource-metadata, T1 grup 3): kondisi, ekspresi, templat label, desimal berskala,
// dan pencocokan pattern. Diekspor lewat subpath `@erpora/platform-web/metadata` (bukan barrel utama) dan dimuat
// malas agar entry awal tetap di bawah anggaran 200 KB gzip. Sisa paket metadata (validator, registri tipe field)
// menyusul di grup 16.

export {
  type ConditionNode,
  type ConditionOp,
  type EvalResult,
  evalCondition,
  evaluateCondition,
  parseCondition,
} from "./condition";
export {
  addDecimal,
  compareDecimal,
  type Decimal,
  decimalFromInt,
  divDecimal,
  formatDecimal,
  isRoundMode,
  isZeroDecimal,
  mulDecimal,
  parseDecimal,
  ROUND_MODES,
  type RoundMode,
  roundDecimal,
  subDecimal,
} from "./decimal";
export { type ExprNode, type ExprResult, evalExpr, evaluateExpr, parseExpr } from "./expr";
export {
  type LabelResult,
  type LabelTemplate,
  parseLabelTemplate,
  renderLabel,
} from "./label";
export * from "./limits";
export { type PatternResult, testPattern } from "./pattern";
export {
  type FieldType,
  type FieldTypes,
  formatReason,
  type Parsed,
  REASON,
  type Reason,
  type ReasonCode,
  type RecordData,
  type ValueType,
} from "./types";
