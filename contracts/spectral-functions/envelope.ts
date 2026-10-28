// Rule amplop respons, error standar, dan pagination (api-guidelines §4.2, §6, §8).
import {
  asArr,
  asObj,
  asStr,
  ctx,
  currentDoc,
  follow,
  type Obj,
  type Operation,
  operations,
  type PathSeg,
  parametersOf,
  type Result,
} from "./refs";

const isSuccess = (code: string): boolean => /^2(\d\d|XX)$/.test(code);

/** Skema JSON (setelah $ref diikuti) suatu respons; undefined bila respons tanpa body JSON. */
export function jsonSchemaOf(
  root: unknown,
  resp: unknown,
): { schema: Obj; at: string } | undefined {
  const r = follow(root, resp);
  const content = r ? asObj(asObj(r.node)?.content) : undefined;
  const media = content ? asObj(content["application/json"]) : undefined;
  if (!media || media.schema === undefined) return undefined;
  const s = follow(root, media.schema, r?.doc);
  const schema = s ? asObj(s.node) : undefined;
  return schema ? { schema, at: "application/json" } : undefined;
}

/** Properti dan required efektif suatu skema objek (menggabungkan allOf). */
export function flatten(root: unknown, schema: Obj, depth = 0): { props: Obj; required: string[] } {
  const props: Obj = { ...(asObj(schema.properties) ?? {}) };
  const required = asArr(schema.required).filter((x): x is string => typeof x === "string");
  if (depth < 8) {
    for (const part of asArr(schema.allOf)) {
      const r = follow(root, part);
      const o = r ? asObj(r.node) : undefined;
      if (!o) continue;
      const sub = flatten(root, o, depth + 1);
      Object.assign(props, sub.props);
      required.push(...sub.required);
    }
  }
  return { props, required };
}

export function successEnvelope(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    const responses = asObj(op.node.responses) ?? {};
    for (const [code, resp] of Object.entries(responses)) {
      if (!isSuccess(code) || code === "204") continue;
      const js = jsonSchemaOf(root, resp);
      if (!js) continue;
      const { props, required } = flatten(root, js.schema);
      if (props.data === undefined || !required.includes("data")) {
        out.push({
          message: `Respons ${code} ${op.method.toUpperCase()} ${op.path} wajib berbentuk {"data": …} dengan data di required (api-guidelines §4.2)`,
          path: [...op.at, "responses", code],
        });
      }
    }
  }
  return out;
}

const COMMON_RESPONSE = /^erp:common#\/components\/responses\//;
const COMMON_ERROR = "erp:common#/components/schemas/Error";

export function errorStandard(root: unknown): Result[] {
  const out: Result[] = [];
  const own = ctx().target === "common";
  for (const op of operations(currentDoc(root))) {
    const responses = asObj(op.node.responses) ?? {};
    for (const [code, resp] of Object.entries(responses)) {
      if (!/^[45]/.test(code)) continue;
      const o = asObj(resp) ?? {};
      const ref = asStr(o.$ref);
      if (
        ref !== undefined &&
        (COMMON_RESPONSE.test(ref) || (own && ref.startsWith("#/components/responses/")))
      )
        continue;
      const media = asObj(asObj(o.content)?.["application/json"]);
      const sref = asStr(asObj(media?.schema)?.$ref);
      if (sref === COMMON_ERROR || (own && sref === "#/components/schemas/Error")) continue;
      out.push({
        message: `Respons ${code} ${op.method.toUpperCase()} ${op.path} harus memakai erp:common#/components/responses/* atau skema erp:common#/components/schemas/Error (api-guidelines §8)`,
        path: [...op.at, "responses", code],
      });
    }
  }
  return out;
}

function paramNames(root: unknown, op: Operation): Set<string> {
  return new Set(parametersOf(root, op).map((p) => asStr(p.name) ?? ""));
}

export function collectionPagination(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    if (op.method !== "get") continue;
    if (op.path.endsWith("}") || op.pathItem["x-erp-singleton"] === true) continue;
    const resp = asObj(op.node.responses)?.["200"];
    const js = resp ? jsonSchemaOf(root, resp) : undefined;
    if (!js) continue;
    const data = follow(root, flatten(root, js.schema).props.data);
    const ds = data ? asObj(data.node) : undefined;
    if (ds?.type !== "array") continue;
    const names = paramNames(root, op);
    const page = names.has("page") && names.has("pageSize");
    const cursor = names.has("cursor") && names.has("limit");
    if (!page && !cursor) {
      out.push({
        message: `GET koleksi ${op.path} wajib memakai parameter page+pageSize atau cursor+limit (api-guidelines §6)`,
        path: [...op.at],
      });
    }
  }
  return out;
}

export type { PathSeg };
