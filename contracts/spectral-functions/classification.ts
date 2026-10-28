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

/**
 * Memecah nama menjadi token huruf kecil: batas camelCase/PascalCase/akronim dan pemisah non-alfanumerik
 * (`newPassword` -> [new, password]; `APIKey` -> [api, key]; `reset_token` -> [reset, token]).
 */
export function nameTokens(name: string): string[] {
  return name
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2")
    .split(/[^A-Za-z0-9]+/)
    .filter((t) => t !== "")
    .map((t) => t.toLowerCase());
}

// Cocok per token (bukan seluruh nama), sehingga newPassword, passwordHash, clientSecret, resetToken, apiToken, dan
// bankAccountNumber tertangkap. Pasangan token berurutan untuk istilah dua kata. Tingkat klasifikasi per kelompok
// (SENSITIVE >= confidential, HIGHLY = highly_confidential) tidak berubah.
const SENSITIVE_TOKENS = new Set([
  "password",
  "passwd",
  "secret",
  "token",
  "nik",
  "npwp",
  "salary",
]);
const SENSITIVE_PAIRS = new Set([
  "api key",
  "private key",
  "bank account",
  "access token",
  "refresh token",
]);
// K-4 (Q32): data pribadi/keuangan sensitif (nik, npwp, bankAccount*, salary) wajib highly_confidential; di respons hanya
// diizinkan dengan pengecualian eksplisit x-erp-allow-highly-confidential (alasan string tidak kosong).
const HIGHLY_TOKENS = new Set(["password", "passwd", "secret", "token", "nik", "npwp", "salary"]);
const HIGHLY_PAIRS = new Set([
  "api key",
  "private key",
  "bank account",
  "access token",
  "refresh token",
]);

function matchesTerms(name: string, singles: Set<string>, pairs: Set<string>): boolean {
  const t = nameTokens(name);
  // Istilah multi-kata yang ditulis menyambung atau huruf besar semua (apikey, APIKEY, bankaccount, accesstoken) menjadi
  // satu token; cocokkan juga bentuk gabungannya sebagai substring token tunggal (R-2).
  const joined = [...pairs].map((p) => p.replace(" ", ""));
  for (let i = 0; i < t.length; i++) {
    const tok = t[i] ?? "";
    if (singles.has(tok)) return true;
    if (i + 1 < t.length && pairs.has(`${tok} ${t[i + 1]}`)) return true;
    if (joined.some((j) => tok.includes(j))) return true;
  }
  return false;
}

const SENSITIVE = {
  test: (n: string): boolean => matchesTerms(n, SENSITIVE_TOKENS, SENSITIVE_PAIRS),
};
const HIGHLY = { test: (n: string): boolean => matchesTerms(n, HIGHLY_TOKENS, HIGHLY_PAIRS) };

/** Keyword JSON Schema 2020-12 yang berisi satu subskema, daftar subskema, atau peta subskema. */
const SCHEMA_SINGLE = [
  "items",
  "additionalProperties",
  "unevaluatedProperties",
  "unevaluatedItems",
  "contains",
  "not",
  "if",
  "then",
  "else",
  "propertyNames",
];
const SCHEMA_LISTS = ["allOf", "oneOf", "anyOf", "prefixItems"];
const SCHEMA_MAPS = ["properties", "patternProperties", "dependentSchemas"];

/** Pengecualian eksplisit: x-erp-allow-highly-confidential berisi alasan string tidak kosong. */
export const ALLOW_HIGHLY = "x-erp-allow-highly-confidential";
function hasAllowReason(o: Obj): boolean {
  const v = o[ALLOW_HIGHLY];
  return typeof v === "string" && v.trim() !== "";
}

// R-3 (Q32 butir 14, direvisi 28 Okt 2026): pengecualian x-erp-allow-highly-confidential dibatasi ke data HR, yaitu
// properti bernama nik, npwp, bankAccount*, atau salary (per token, termasuk bentuk gabungan). Nama kredensial
// (password, secret, token, apiKey, privateKey, ...) tidak pernah boleh dikecualikan.
const ALLOW_SINGLES = new Set(["nik", "npwp", "salary"]);
const ALLOW_PAIRS = new Set(["bank account"]);
const CRED_SINGLES = new Set(["password", "passwd", "secret", "token"]);
const CRED_PAIRS = new Set(["api key", "private key", "access token", "refresh token"]);

/** Mengumpulkan properti (nilai di peta `properties`) beserta namanya dalam subpohon skema; mengikuti $ref, aman siklus. */
function collectProps(
  root: unknown,
  node: unknown,
  doc: string,
  seen: Set<unknown>,
  out: Map<Obj, Set<string>>,
): void {
  const o = asObj(node);
  if (!o || seen.has(o)) return;
  seen.add(o);
  for (const kw of SCHEMA_MAPS) {
    for (const [k, v] of Object.entries(asObj(o[kw]) ?? {})) {
      const vo = asObj(v);
      if (kw === "properties" && vo) {
        const names = out.get(vo) ?? new Set<string>();
        names.add(k);
        out.set(vo, names);
      }
      collectProps(root, v, doc, seen, out);
    }
  }
  for (const kw of SCHEMA_SINGLE) collectProps(root, o[kw], doc, seen, out);
  for (const kw of SCHEMA_LISTS)
    for (const v of asArr(o[kw])) collectProps(root, v, doc, seen, out);
  if (asStr(o.$ref) !== undefined) {
    const r = follow(root, o, doc);
    if (r && asObj(r.node) !== o) collectProps(root, r.node, r.doc, seen, out);
  }
}

function collectMediaProps(
  root: unknown,
  content: unknown,
  doc: string,
  out: Map<Obj, Set<string>>,
): void {
  for (const media of Object.values(asObj(content) ?? {})) {
    collectProps(root, asObj(media)?.schema, doc, new Set(), out);
  }
}

/** Properti yang dapat dicapai dari respons 2xx (sah untuk pengecualian) dan dari konteks lain (terlarang). */
function allowContexts(root: unknown): { allowed: Map<Obj, Set<string>>; forbidden: Set<Obj> } {
  const allowed = new Map<Obj, Set<string>>();
  const other = new Map<Obj, Set<string>>();
  for (const op of operations(currentDoc(root))) {
    for (const [code, resp] of Object.entries(asObj(op.node.responses) ?? {})) {
      const r = follow(root, resp);
      const content = r ? asObj(r.node)?.content : undefined;
      collectMediaProps(root, content, r?.doc ?? "", /^2/.test(code) ? allowed : other);
    }
    const rb = follow(root, op.node.requestBody);
    if (rb) collectMediaProps(root, asObj(rb.node)?.content, rb.doc, other);
    for (const p of parametersOf(root, op)) {
      collectProps(root, p.schema, "", new Set(), other);
      collectMediaProps(root, p.content, "", other);
    }
  }
  return { allowed, forbidden: new Set(other.keys()) };
}

/** erp-allow-highly-confidential-valid: pengecualian harus berisi alasan string tidak kosong, menyertai highly_confidential, dan sah penempatan/namanya. */
export function allowHighlyValid(root: unknown): Result[] {
  const out: Result[] = [];
  const ctxs = allowContexts(root);
  for (const [path, node] of walkDoc(root)) {
    const o = asObj(node);
    if (!o || !(ALLOW_HIGHLY in o)) continue;
    const at = [...path, ALLOW_HIGHLY];
    if (!hasAllowReason(o)) {
      out.push({
        message: `${ALLOW_HIGHLY} wajib berisi alasan berupa string tidak kosong (FR-24.2, security-guidelines §2)`,
        path: at,
      });
    } else if (o["x-erp-classification"] !== "highly_confidential") {
      out.push({
        message: `${ALLOW_HIGHLY} hanya bermakna bersama x-erp-classification: highly_confidential pada elemen yang sama (FR-24.2, security-guidelines §2)`,
        path: at,
      });
    }
    const names = ctxs.allowed.get(o);
    if (!names || ctxs.forbidden.has(o)) {
      out.push({
        message: `${ALLOW_HIGHLY} hanya sah pada properti skema respons 2xx (bukan di requestBody, parameter, respons non-2xx, atau skema akar) (FR-24.2, security-guidelines §2)`,
        path: at,
      });
      continue;
    }
    for (const name of names) {
      if (matchesTerms(name, CRED_SINGLES, CRED_PAIRS)) {
        out.push({
          message: `${ALLOW_HIGHLY} tidak boleh dipakai pada properti kredensial "${name}" (FR-24.2, security-guidelines §2)`,
          path: at,
        });
      } else if (!matchesTerms(name, ALLOW_SINGLES, ALLOW_PAIRS)) {
        out.push({
          message: `${ALLOW_HIGHLY} hanya sah untuk properti data HR (nik, npwp, bankAccount*, salary); "${name}" di luar kelompok itu (FR-24.2, security-guidelines §2)`,
          path: at,
        });
      }
    }
  }
  return out;
}

/** erp-highly-confidential-exception (INFO): daftar pengecualian untuk ditinjau reviewer. */
export function highlyExceptions(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [path, node] of walkDoc(root)) {
    const o = asObj(node);
    if (!o || !hasAllowReason(o) || o["x-erp-classification"] !== "highly_confidential") continue;
    const owner = path.filter((s) => s !== "properties").slice(-1)[0];
    out.push({
      message: `Pengecualian highly_confidential pada "${String(owner ?? "(skema)")}": ${String(o[ALLOW_HIGHLY]).trim()} (tinjau di code review; security-guidelines §2)`,
      path: [...path, ALLOW_HIGHLY],
    });
  }
  return out;
}

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
  const o = asObj(node);
  if (!o || seen.has(o)) return undefined;
  seen.add(o);
  // Anotasi dan keyword yang bersanding dengan $ref (OpenAPI 3.1) dihitung pada node itu sendiri; follow() hanya
  // mengembalikan node tujuan dan membuang sibling-nya.
  if (o["x-erp-classification"] === "highly_confidential" && !hasAllowReason(o))
    return trail.join(".") || "(skema)";
  for (const kw of SCHEMA_MAPS) {
    for (const [k, v] of Object.entries(asObj(o[kw]) ?? {})) {
      const f = findHighly(root, v, doc, seen, [...trail, k]);
      if (f) return f;
    }
  }
  for (const kw of SCHEMA_SINGLE) {
    const child = o[kw];
    if (asObj(child) === undefined) continue;
    const f = findHighly(root, child, doc, seen, [...trail, kw === "items" ? "[]" : kw]);
    if (f) return f;
  }
  for (const kw of SCHEMA_LISTS) {
    for (const v of asArr(o[kw])) {
      const f = findHighly(root, v, doc, seen, trail);
      if (f) return f;
    }
  }
  if (asStr(o.$ref) !== undefined) {
    const r = follow(root, o, doc);
    const target = r ? asObj(r.node) : undefined;
    if (r && target && target !== o) return findHighly(root, target, r.doc, seen, trail);
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

/** Klasifikasi pada parameter itu sendiri dan pada skemanya (termasuk anotasi yang bersanding dengan $ref). */
function paramClasses(root: unknown, p: Obj): unknown[] {
  const out: unknown[] = [p["x-erp-classification"]];
  const s = asObj(p.schema);
  if (s) {
    out.push(s["x-erp-classification"]);
    const r = follow(root, s);
    const t = r ? asObj(r.node) : undefined;
    if (t) out.push(t["x-erp-classification"]);
  }
  return out;
}

export function noSensitiveInUrl(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    for (const p of parametersOf(root, op)) {
      if (p.in !== "path" && p.in !== "query") continue;
      const name = asStr(p.name) ?? "";
      if (SENSITIVE.test(name) || paramClasses(root, p).includes("highly_confidential")) {
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
  extra?: unknown[],
): void {
  if (!SENSITIVE.test(name)) return;
  const need = HIGHLY.test(name) ? "highly_confidential" : "confidential";
  const best = Math.max(rank(node["x-erp-classification"]), ...(extra ?? []).map(rank));
  if (best < rank(need)) {
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
      checkSensitive(o.name, o, path, "Parameter", out, paramClasses(root, o));
    }
  }
  return out;
}

export { CLASSES, rank };
