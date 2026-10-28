// Util bersama fungsi kustom Spectral + rule erp-ref-dependency (PRD-000 FR-23.1).
// Semua fungsi bekerja pada dokumen mentah (rule `resolved: false`) dan menyelesaikan $ref sendiri lewat katalog.

export type Obj = { [k: string]: unknown };
export type PathSeg = string | number;
export interface Result {
  message: string;
  path: PathSeg[];
}

/** Konteks target yang sedang di-lint; diisi runner (web/contracts-lint.ts) sebelum setiap dokumen. */
export interface LintContext {
  target: string; // "common" | "platform" | slug modul
  slug: string; // "" untuk common
  dependencies: string[]; // slug dependensi manifest (tanpa platform.core)
  docs: Record<string, Obj>; // semua dokumen kontrak yang dikenal (nama -> isi)
}

let current: LintContext | undefined;

export function setLintContext(c: LintContext): void {
  current = c;
}

export function ctx(): LintContext {
  if (!current) throw new Error("konteks lint belum diisi (setLintContext)");
  return current;
}

export const asObj = (v: unknown): Obj | undefined =>
  v !== null && typeof v === "object" && !Array.isArray(v) ? (v as Obj) : undefined;
export const asArr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
export const asStr = (v: unknown): string | undefined => (typeof v === "string" ? v : undefined);

export const HTTP_METHODS = ["get", "put", "post", "delete", "options", "head", "patch", "trace"];

/** Dokumen yang sedang di-lint (root `$` dari Spectral). */
export function currentDoc(root: unknown): Obj {
  return asObj(root) ?? {};
}

function unescapeSeg(s: string): string {
  return s.replace(/~1/g, "/").replace(/~0/g, "~");
}

/** Menguraikan $ref menjadi {doc, pointer}; doc "" = dokumen yang sedang di-lint. */
export function parseRef(ref: string): { doc: string; pointer: string[] } | undefined {
  let doc = "";
  let ptr = ref;
  if (ref.startsWith("erp:")) {
    const i = ref.indexOf("#");
    if (i < 0) return undefined;
    doc = ref.slice(4, i);
    ptr = ref.slice(i);
  }
  if (!ptr.startsWith("#/")) return undefined;
  return { doc, pointer: ptr.slice(2).split("/").map(unescapeSeg) };
}

function walk(node: unknown, pointer: string[]): unknown {
  let cur = node;
  for (const seg of pointer) {
    if (Array.isArray(cur)) cur = cur[Number(seg)];
    else if (asObj(cur)) cur = (cur as Obj)[seg];
    else return undefined;
  }
  return cur;
}

export interface Resolved {
  node: unknown;
  doc: string; // nama dokumen pemilik node ("" = dokumen yang sedang di-lint)
}

/** Mengikuti rantai $ref (lokal dan erp:) sampai node tanpa $ref; undefined bila rujukan putus atau siklik. */
export function follow(root: unknown, node: unknown, doc = ""): Resolved | undefined {
  let cur = node;
  let curDoc = doc;
  for (let i = 0; i < 32; i++) {
    const o = asObj(cur);
    const ref = o ? asStr(o.$ref) : undefined;
    if (ref === undefined) return { node: cur, doc: curDoc };
    const p = parseRef(ref);
    if (!p) return undefined;
    const base = p.doc === "" ? curDoc : p.doc;
    const target = base === "" ? root : ctx().docs[base];
    if (target === undefined) return undefined;
    cur = walk(target, p.pointer);
    curDoc = base;
    if (cur === undefined) return undefined;
  }
  return undefined;
}

/** Semua pasangan [path, node] objek di dokumen (rekursif); mengabaikan example(s) dan ekstensi x-*. */
export function* walkDoc(node: unknown, path: PathSeg[] = []): Generator<[PathSeg[], unknown]> {
  yield [path, node];
  if (Array.isArray(node)) {
    for (let i = 0; i < node.length; i++) yield* walkDoc(node[i], [...path, i]);
  } else if (asObj(node)) {
    for (const [k, v] of Object.entries(node as Obj)) {
      if (k === "example" || k === "examples" || k.startsWith("x-")) continue;
      yield* walkDoc(v, [...path, k]);
    }
  }
}

export interface Operation {
  path: string;
  method: string; // huruf kecil
  node: Obj;
  pathItem: Obj;
  at: PathSeg[]; // lokasi operasi di dokumen
}

export function operations(doc: Obj): Operation[] {
  const out: Operation[] = [];
  const paths = asObj(doc.paths) ?? {};
  for (const [p, item] of Object.entries(paths)) {
    const pi = asObj(item);
    if (!pi) continue;
    for (const m of HTTP_METHODS) {
      const node = asObj(pi[m]);
      if (node) out.push({ path: p, method: m, node, pathItem: pi, at: ["paths", p, m] });
    }
  }
  return out;
}

/** Parameter efektif operasi (level path + operasi, $ref diselesaikan). */
export function parametersOf(root: unknown, op: Operation): Obj[] {
  const out: Obj[] = [];
  for (const raw of [...asArr(op.pathItem.parameters), ...asArr(op.node.parameters)]) {
    const r = follow(root, raw);
    const o = r ? asObj(r.node) : undefined;
    if (o) out.push(o);
  }
  return out;
}

/** Rule erp-ref-dependency: $ref erp:<x> hanya common atau slug dependensi manifest (FR-23.1). */
export function refDependency(root: unknown): Result[] {
  const c = ctx();
  const allowed = new Set(["common", ...c.dependencies]);
  const out: Result[] = [];
  for (const [path, node] of walkDoc(root)) {
    const o = asObj(node);
    const ref = o ? asStr(o.$ref) : undefined;
    if (ref === undefined) continue;
    if (!ref.startsWith("erp:")) continue;
    const p = parseRef(ref);
    const name = ref.slice(4, ref.indexOf("#") < 0 ? undefined : ref.indexOf("#"));
    if (!p || !allowed.has(name)) {
      out.push({
        message: `$ref "${ref}" hanya boleh merujuk erp:common atau dokumen dependensi manifest: ${[...allowed].join(", ")} (PRD-000 FR-23.1)`,
        path: [...path, "$ref"],
      });
    }
  }
  return out;
}
