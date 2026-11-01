// Pemuat malas metadata per resource (Requirement 18.4) dan pagar keamanan chunk publik (Requirement 18.3, 18.8;
// design D-8, D-21). Tiap resource punya importer sendiri (`import()` ke `*.meta.gen.ts` = satu chunk per
// resource); tidak ada metadata yang masuk bundel awal. Chunk bersifat publik (SPA statis tanpa autentikasi),
// sehingga pemuat menolak chunk yang memuat apa yang seharusnya tetap di server.

import { type ConditionNode, isPlainObject, parseCondition } from "./condition";
import { type ExprNode, parseExpr } from "./expr";
import type { AccessorMeta, Condition, FieldMeta, ResourceMeta, ResourceMetaModule } from "./meta";

export type MetadataLoadErrorCode =
  | "not_registered"
  | "import_failed"
  | "invalid_module"
  | "key_mismatch"
  | "unsafe_for_client";

export class MetadataLoadError extends Error {
  readonly code: MetadataLoadErrorCode;
  readonly resourceKey: string;
  constructor(
    code: MetadataLoadErrorCode,
    resourceKey: string,
    detail?: string,
    options?: { cause?: unknown },
  ) {
    super(`metadata ${resourceKey}: ${code}${detail ? ` (${detail})` : ""}`, options);
    this.name = "MetadataLoadError";
    this.code = code;
    this.resourceKey = resourceKey;
  }
}

interface Ref {
  /** true: nama pada posisi `field` daun; false: nama pada `ref`. */
  readonly field: boolean;
  readonly name: string;
}

function condNames(node: ConditionNode, out: Ref[]): void {
  if (node.kind === "leaf") {
    out.push({ field: true, name: node.field });
    if (node.ref !== "") out.push({ field: false, name: node.ref });
    return;
  }
  for (const c of node.children) condNames(c, out);
}

/** Mengumpulkan rujukan field dari pohon ekspresi, termasuk kondisi `if` (kondisi, lalu cabang). */
function exprNames(node: ExprNode, out: Ref[]): void {
  switch (node.kind) {
    case "field":
      out.push({ field: true, name: node.field });
      return;
    case "if":
      condNames(node.cond, out);
      exprNames(node.whenTrue, out);
      exprNames(node.whenFalse, out);
      return;
    case "concat":
    case "coalesce":
    case "add":
    case "sub":
    case "mul":
    case "div":
      for (const a of node.args) exprNames(a, out);
  }
}

const FORBIDDEN_KEYS = new Set(["clientVisible", "clientVisibleReason"]);
const SCAN_LIMIT = 200_000;
const SCAN_EXCEEDED =
  "metadata terlalu besar untuk dipindai (melampaui batas): ditolak (gagal tertutup)";

function forbiddenKeys(root: unknown): string[] {
  const found = new Set<string>();
  const stack: unknown[] = [root];
  const seen = new Set<unknown>();
  let steps = 0;
  while (stack.length > 0) {
    if (steps++ >= SCAN_LIMIT) {
      found.add(SCAN_EXCEEDED);
      break;
    }
    const v = stack.pop();
    if (typeof v !== "object" || v === null || seen.has(v)) continue;
    seen.add(v);
    if (Array.isArray(v)) {
      for (const e of v) stack.push(e);
      continue;
    }
    for (const [k, x] of Object.entries(v)) {
      if (FORBIDDEN_KEYS.has(k))
        found.add(
          `kunci terlarang "${k}" (hanya untuk pembangkit; tidak boleh ada di chunk klien)`,
        );
      if (k === "scope" && x === "server")
        found.add('aturan "scope: server" tidak boleh ada di chunk klien');
      stack.push(x);
    }
  }
  return [...found];
}

const objs = (v: unknown): v is readonly Record<string, unknown>[] =>
  Array.isArray(v) && v.every(isPlainObject);

/**
 * Memeriksa kelengkapan bentuk metadata (semua koleksi ada dan berupa larik objek) sebelum dipindai; mengembalikan
 * deskripsi masalah atau null. Dipakai pemuat (`invalid_module`) dan `checkClientMeta` agar chunk tak lengkap tidak
 * pernah menimbulkan TypeError.
 */
export function metaShapeProblem(meta: unknown): string | null {
  if (!isPlainObject(meta)) return "bukan objek";
  if (typeof meta.key !== "string") return "key hilang";
  if (typeof meta.defaultGroup !== "string" || meta.defaultGroup === "")
    return "defaultGroup hilang";
  for (const k of ["fields", "relations", "rules", "sections"] as const) {
    if (!objs(meta[k])) return `${k} hilang atau bukan larik objek`;
  }
  for (const f of meta.fields as readonly Record<string, unknown>[]) {
    if (typeof f.name !== "string") return "field tanpa name";
    if (f.accessor !== undefined && !isPlainObject(f.accessor))
      return `accessor ${f.name} bukan objek`;
  }
  for (const r of meta.relations as readonly Record<string, unknown>[]) {
    if (r.pivotFields !== undefined) {
      if (!objs(r.pivotFields)) return "pivotFields bukan larik objek";
      for (const p of r.pivotFields) {
        if (typeof p.name !== "string") return "pivotField tanpa name";
        if (p.accessor !== undefined && !isPlainObject(p.accessor))
          return `accessor ${p.name} bukan objek`;
      }
    }
  }
  return null;
}

/**
 * Memeriksa bahwa metadata aman untuk chunk publik: tidak ada `clientVisible`/`scope: server`, dan setiap kondisi
 * (field, aturan, filter relasi, `lockWhen`, serta field pivot) dan setiap `accessor.expr` (termasuk kondisi `if`
 * di dalamnya) hanya merujuk field yang ada di metadata klien, bukan field berklasifikasi `confidential` ke atas
 * atau di luar grup default (aturan seperti itu harus `scope: server`, D-8, Requirement 4.6 dan 5.8). Pemindaian
 * yang melampaui batas dianggap pelanggaran (gagal tertutup). Fungsi total. Mengembalikan daftar pelanggaran
 * (kosong = aman).
 */
export function checkClientMeta(meta: ResourceMeta): readonly string[] {
  const problem = metaShapeProblem(meta);
  if (problem !== null) return [`struktur metadata tidak lengkap: ${problem}`];
  const out = forbiddenKeys(meta);
  const byName = new Map(meta.fields.map((f) => [f.name, f]));

  /** Memeriksa satu rujukan terhadap cakupan nama (pivot: field pivot lebih dulu, lalu field resource). */
  const checkRef = (where: string, name: string, scopes: readonly Map<string, FieldMeta>[]) => {
    const f = scopes.map((s) => s.get(name)).find((x) => x !== undefined);
    if (!f) {
      out.push(`${where}: merujuk field "${name}" yang tidak ada di metadata klien`);
    } else if (f.classification !== "public" && f.classification !== "internal") {
      out.push(
        `${where}: merujuk field "${name}" berklasifikasi ${f.classification} (harus scope server)`,
      );
    } else if (f.fieldGroup !== meta.defaultGroup) {
      out.push(
        `${where}: merujuk field "${name}" di grup ${f.fieldGroup} di luar grup default (harus scope server)`,
      );
    }
  };
  const check = (
    where: string,
    cond: Condition | undefined,
    scopes: readonly Map<string, FieldMeta>[],
    relationFilter = false,
  ) => {
    if (cond === undefined) return;
    const p = parseCondition(cond);
    if (!p.ok) {
      out.push(`${where}: kondisi tidak sah (${p.reason.code})`);
      return;
    }
    const refs: Ref[] = [];
    condNames(p.value, refs);
    for (const r of refs) {
      // pada filter relasi, `field` menunjuk field resource seberang; hanya `ref` menunjuk form ini
      if (relationFilter && r.field) continue;
      checkRef(where, r.name, scopes);
    }
  };
  const checkAccessor = (
    where: string,
    accessor: AccessorMeta | undefined,
    scopes: readonly Map<string, FieldMeta>[],
  ) => {
    if (accessor === undefined || accessor.kind === "func") return;
    const expr = accessor.expr;
    if (expr === undefined) {
      if (accessor.kind === "expr") out.push(`${where}: accessor expr tanpa ekspresi`);
      return;
    }
    const p = parseExpr(expr);
    if (!p.ok) {
      out.push(`${where}: ekspresi accessor tidak sah (${p.reason.code})`);
      return;
    }
    const refs: Ref[] = [];
    exprNames(p.value, refs);
    for (const r of refs) checkRef(`${where}.accessor`, r.name, scopes);
  };
  const checkField = (f: FieldMeta, scopes: readonly Map<string, FieldMeta>[], prefix: string) => {
    check(`${prefix}${f.name}.visibleWhen`, f.visibleWhen, scopes);
    check(`${prefix}${f.name}.requiredWhen`, f.requiredWhen, scopes);
    check(`${prefix}${f.name}.readonlyWhen`, f.readonlyWhen, scopes);
    checkAccessor(`${prefix}${f.name}`, f.accessor, scopes);
  };

  const top = [byName];
  for (const f of meta.fields) checkField(f, top, "field ");
  for (const r of meta.rules) check(`rule ${r.id}`, r.assert, top);
  for (const rel of meta.relations) {
    check(`relation ${rel.name}.filter`, rel.filter, top, true);
    if (rel.pivotFields) {
      const pivotByName = new Map(rel.pivotFields.map((p) => [p.name, p]));
      for (const p of rel.pivotFields)
        checkField(p, [pivotByName, byName], `relation ${rel.name}.pivot `);
    }
  }
  check("lockWhen", meta.lockWhen, top);
  return out;
}

export type ResourceMetaImporter = () => Promise<ResourceMetaModule>;

export interface ResourceMetaLoader {
  /** Mendaftarkan importer chunk satu resource (mis. `() => import("./metadata/invoice.meta.gen")`). */
  register(key: string, importer: ResourceMetaImporter): void;
  /** Memuat (sekali; permintaan bersamaan dipadukan) dan memvalidasi chunk; gagal impor tidak di-cache. */
  load(key: string): Promise<ResourceMeta>;
  /** Metadata yang sudah termuat, undefined bila belum. */
  peek(key: string): ResourceMeta | undefined;
  keys(): string[];
}

const RESOURCE_KEY_RE = /^[a-z][a-z0-9]*(-[a-z0-9]+)*\.[A-Za-z][A-Za-z0-9]*$/;

/** Membuat pemuat baru (terisolasi); aplikasi memakai singleton `resourceMetaLoader`. */
export function createResourceMetaLoader(): ResourceMetaLoader {
  const importers = new Map<string, ResourceMetaImporter>();
  const loaded = new Map<string, ResourceMeta>();
  const inflight = new Map<string, Promise<ResourceMeta>>();

  async function run(key: string, importer: ResourceMetaImporter): Promise<ResourceMeta> {
    let mod: ResourceMetaModule;
    try {
      mod = await importer();
    } catch (err) {
      throw new MetadataLoadError("import_failed", key, undefined, { cause: err });
    }
    const meta = isPlainObject(mod) ? (mod as { resourceMeta?: unknown }).resourceMeta : undefined;
    const problem = metaShapeProblem(meta);
    if (problem !== null) throw new MetadataLoadError("invalid_module", key, problem);
    const m = meta as unknown as ResourceMeta;
    if (m.key !== key)
      throw new MetadataLoadError("key_mismatch", key, `chunk berisi ${String(m.key)}`);
    const violations = checkClientMeta(m);
    if (violations.length > 0)
      throw new MetadataLoadError("unsafe_for_client", key, violations.join("; "));
    loaded.set(key, m);
    return m;
  }

  return {
    register(key, importer) {
      if (!RESOURCE_KEY_RE.test(key)) throw new Error(`kunci resource tidak sah: ${key}`);
      if (importers.has(key)) throw new Error(`metadata sudah terdaftar: ${key}`);
      importers.set(key, importer);
    },
    load(key) {
      const hit = loaded.get(key);
      if (hit) return Promise.resolve(hit);
      const pending = inflight.get(key);
      if (pending) return pending;
      const importer = importers.get(key);
      if (!importer) return Promise.reject(new MetadataLoadError("not_registered", key));
      const p = run(key, importer).finally(() => inflight.delete(key));
      inflight.set(key, p);
      return p;
    },
    peek: (key) => loaded.get(key),
    keys: () => [...importers.keys()],
  };
}

/** Pemuat aplikasi. Modul mendaftarkan importer chunk metadata resource-nya di sini saat registrasi modul. */
export const resourceMetaLoader: ResourceMetaLoader = createResourceMetaLoader();
