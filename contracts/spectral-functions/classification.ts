// Rule klasifikasi data (security-guidelines §2, §3.4; PRD-000 FR-24.2).
import { schemaProperties } from "./naming";
import {
  asArr,
  asObj,
  asStr,
  currentDoc,
  follow,
  type Obj,
  operations,
  type PathSeg,
  parametersOf,
  type Result,
  walkDoc,
} from "./refs";

const CLASSES = ["public", "internal", "confidential", "highly_confidential"];
const rank = (c: unknown): number => (typeof c === "string" ? CLASSES.indexOf(c) : -1);

const SENSITIVE =
  /^(password|passwd|secret|token|accessToken|refreshToken|apiKey|privateKey|nik|npwp|bankAccount|salary)$/i;
const HIGHLY = /^(password|secret|token|apiKey|privateKey|accessToken|refreshToken)$/i;

export function classificationValid(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [path, node] of walkDoc(root)) {
    const o = asObj(node);
    if (o && "x-erp-classification" in o && rank(o["x-erp-classification"]) < 0) {
      out.push({
        message: `x-erp-classification "${String(o["x-erp-classification"])}" tidak valid; pilih ${CLASSES.join(", ")} (security-guidelines §2)`,
        path: [...path, "x-erp-classification"],
      });
    }
  }
  return out;
}

/** Klasifikasi efektif tertinggi dalam subpohon skema (mengikuti $ref, aman siklus); null bila tidak ada. */
function findHighly(
  root: unknown,
  node: unknown,
  doc: string,
  seen: Set<unknown>,
  trail: string[],
): string | undefined {
  const r = follow(root, node, doc);
  if (!r) return undefined;
  const o = asObj(r.node);
  if (!o || seen.has(o)) return undefined;
  seen.add(o);
  if (o["x-erp-classification"] === "highly_confidential") return trail.join(".") || "(skema)";
  for (const kw of ["properties", "patternProperties"]) {
    for (const [k, v] of Object.entries(asObj(o[kw]) ?? {})) {
      const f = findHighly(root, v, r.doc, seen, [...trail, k]);
      if (f) return f;
    }
  }
  for (const kw of ["items", "additionalProperties"]) {
    const f = asObj(o[kw])
      ? findHighly(root, o[kw], r.doc, seen, [...trail, kw === "items" ? "[]" : "*"])
      : undefined;
    if (f) return f;
  }
  for (const kw of ["allOf", "oneOf", "anyOf", "prefixItems"]) {
    for (const v of asArr(o[kw])) {
      const f = findHighly(root, v, r.doc, seen, trail);
      if (f) return f;
    }
  }
  return undefined;
}

export function noHighlyConfidentialInResponse(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    for (const [code, resp] of Object.entries(asObj(op.node.responses) ?? {})) {
      if (!/^2/.test(code)) continue;
      const r = follow(root, resp);
      const content = r ? asObj(asObj(r.node)?.content) : undefined;
      for (const media of Object.values(content ?? {})) {
        const schema = asObj(media)?.schema;
        if (schema === undefined) continue;
        const hit = findHighly(root, schema, r?.doc ?? "", new Set(), []);
        if (hit) {
          out.push({
            message: `Respons ${code} ${op.method.toUpperCase()} ${op.path} memuat properti highly_confidential (${hit}); data itu tidak boleh keluar lewat API (FR-24.2, security-guidelines §2, §3.4)`,
            path: [...op.at, "responses", code],
          });
        }
      }
    }
  }
  return out;
}

export function noSensitiveInUrl(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    for (const p of parametersOf(root, op)) {
      if (p.in !== "path" && p.in !== "query") continue;
      const name = asStr(p.name) ?? "";
      if (SENSITIVE.test(name) || p["x-erp-classification"] === "highly_confidential") {
        out.push({
          message: `Parameter ${String(p.in)} "${name}" pada ${op.method.toUpperCase()} ${op.path} sensitif; jangan kirim data sensitif lewat URL (FR-24.2, security-guidelines §2)`,
          path: [...op.at, "parameters"],
        });
      }
    }
  }
  return out;
}

function checkSensitive(
  name: string,
  node: Obj,
  path: PathSeg[],
  kind: string,
  out: Result[],
): void {
  if (!SENSITIVE.test(name)) return;
  const need = HIGHLY.test(name) ? "highly_confidential" : "confidential";
  if (rank(node["x-erp-classification"]) < rank(need)) {
    out.push({
      message: `${kind} "${name}" bernama sensitif dan wajib x-erp-classification minimal ${need} (FR-24.2, security-guidelines §2)`,
      path,
    });
  }
}

export function sensitiveNeedsClassification(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [name, schema, path] of schemaProperties(root)) {
    checkSensitive(name, asObj(schema) ?? {}, path, "Properti", out);
  }
  for (const [path, node] of walkDoc(root)) {
    const o = asObj(node);
    if (o && typeof o.name === "string" && typeof o.in === "string" && !("properties" in o)) {
      checkSensitive(o.name, o, path, "Parameter", out);
    }
  }
  return out;
}

export { CLASSES, rank };
