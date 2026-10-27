// Fungsi kustom ruleset contracts/spectral.yaml. Setiap rule `erp-*` memakai `given: $` dan `resolved: false`;
// fungsi menelusuri dokumen sendiri dan menyelesaikan $ref (termasuk erp:) lewat konteks yang diisi runner.
import {
  allowHighlyValid,
  classificationValid,
  highlyExceptions,
  noHighlyConfidentialInResponse,
  noSensitiveInUrl,
  sensitiveNeedsClassification,
} from "./classification";
import { collectionPagination, errorStandard, successEnvelope } from "./envelope";
import { decimalNotNumber, enumSnakeCase, propertyCamelCase } from "./naming";
import {
  errorCodesDocumented,
  examples,
  ifMatch,
  noPut,
  operationId,
  operationPermission,
  postIdempotencyKey,
} from "./operations";
import {
  pathCollectionPlural,
  pathKebabCase,
  pathMaxNesting,
  pathModulePrefix,
  pathNoCrudVerb,
  pathNoTrailingSlash,
} from "./paths";
import { type Result, refDependency } from "./refs";

export { type LintContext, setLintContext } from "./refs";

type RuleFn = (root: unknown) => Result[];

// Nama fungsi pada spectral.yaml -> implementasi.
const rules: Record<string, RuleFn> = {
  erpPathModulePrefix: pathModulePrefix,
  erpPathKebabCase: pathKebabCase,
  erpPathNoTrailingSlash: pathNoTrailingSlash,
  erpPathNoCrudVerb: pathNoCrudVerb,
  erpPathCollectionPlural: pathCollectionPlural,
  erpPathMaxNesting: pathMaxNesting,
  erpOperationPermission: operationPermission,
  erpOperationId: operationId,
  erpPropertyCamelCase: propertyCamelCase,
  erpEnumSnakeCase: enumSnakeCase,
  erpDecimalNotNumber: decimalNotNumber,
  erpSuccessEnvelope: successEnvelope,
  erpErrorStandard: errorStandard,
  erpCollectionPagination: collectionPagination,
  erpPostIdempotencyKey: postIdempotencyKey,
  erpIfMatch: ifMatch,
  erpNoPut: noPut,
  erpRefDependency: refDependency,
  erpExamples: examples,
  erpErrorCodesDocumented: errorCodesDocumented,
  erpClassificationValid: classificationValid,
  erpNoHighlyConfidentialInResponse: noHighlyConfidentialInResponse,
  erpNoSensitiveInUrl: noSensitiveInUrl,
  erpSensitiveNeedsClassification: sensitiveNeedsClassification,
  erpAllowHighlyValid: allowHighlyValid,
  erpHighlyExceptions: highlyExceptions,
};

/** Fungsi Spectral kustom: nama -> (targetVal) => hasil. */
export const functions: Record<string, (targetVal: unknown) => Result[]> = Object.fromEntries(
  Object.entries(rules).map(([name, fn]) => [name, (targetVal: unknown) => fn(targetVal)]),
);
