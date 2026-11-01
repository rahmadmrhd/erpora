// Paket metadata frontend (spec resource-metadata, T1 grup 3 dan T3 grup 16): tipe metadata, evaluator kondisi dan
// ekspresi, templat label, desimal berskala, pencocokan pattern, validator record, registri tipe field, dan
// pemuat malas per resource. Diekspor lewat subpath `@erpora/platform-web/metadata` (bukan barrel utama) dan dimuat
// malas agar entry awal tetap di bawah anggaran 200 KB gzip.

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
export { type FormatDecimalOptions, formatDecimalString } from "./format";
export {
  type LabelResult,
  type LabelTemplate,
  parseLabelTemplate,
  renderLabel,
} from "./label";
export * from "./limits";
export {
  checkClientMeta,
  createResourceMetaLoader,
  MetadataLoadError,
  type MetadataLoadErrorCode,
  type ResourceMetaImporter,
  type ResourceMetaLoader,
  resourceMetaLoader,
} from "./loader";
export type {
  AccessorMeta,
  Availability,
  Classification,
  Condition,
  ConditionValue,
  DeprecatedMeta,
  FieldMeta,
  RelationMeta,
  RelationType,
  ResourceMeta,
  ResourceMetaModule,
  RuleMeta,
  SectionMeta,
  SnapshotMapping,
} from "./meta";
export {
  emptyRecord,
  fieldAvailability,
  fieldOf,
  fieldTypes,
  isFieldAvailable,
  labelRef,
  moduleNamespace,
  writablePayload,
} from "./meta";
export { type PatternResult, testPattern } from "./pattern";
export {
  createFieldTypeRegistry,
  type EditorKind,
  FIELD_TYPES,
  type FieldEditorComponent,
  type FieldEditorProps,
  type FieldTypeRegistry,
  type FilterEditorComponent,
  type FilterEditorProps,
  type FilterEditorSpec,
  fieldTypeRegistry,
  OPERATORS_BY_TYPE,
  type ResolvedFieldType,
  type WidgetRegistration,
} from "./registry";
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
export {
  type IssueCode,
  type ValidateOptions,
  type ValidationIssue,
  validateRecord,
} from "./validate";
