// Pemuat malas metadata per resource (Requirement 18.4) dan pagar keamanan chunk publik (Requirement 18.3, 18.8;
// design D-8, D-21). Tiap resource punya importer sendiri (`import()` ke `*.meta.gen.ts` = satu chunk per
// resource); tidak ada metadata yang masuk bundel awal. Chunk bersifat publik (SPA statis tanpa autentikasi),
// sehingga pemuat menolak chunk yang memuat apa yang seharusnya tetap di server.

import { type ConditionNode, isPlainObject, parseCondition } from "./condition";
import type { Condition, ResourceMeta, ResourceMetaModule } from "./meta";

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

function names(node: ConditionNode, out: { field: boolean; name: string }[]): void {
  if (node.kind === "leaf") {
    out.push({ field: true, name: node.field });
    if (node.ref !== "") out.push({ field: false, name: node.ref });
    return;
  }
  for (const c of node.children) names(c, out);
}

const FORBIDDEN_KEYS = new Set(["clientVisible", "clientVisibleReason"]);
const SCAN_LIMIT = 200_000;

function forbiddenKeys(root: unknown): string[] {
  const found = new Set<string>();
  const stack: unknown[] = [root];
  const seen = new Set<unknown>();
  let steps = 0;
  while (stack.length > 0 && steps++ < SCAN_LIMIT) {
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

/**
 * Memeriksa bahwa metadata aman untuk chunk publik: tidak ada `clientVisible`/`scope: server`, dan setiap kondisi
 * hanya merujuk field yang ada di metadata klien, bukan field berklasifikasi `confidential` ke atas atau di luar
 * grup default (aturan seperti itu harus `scope: server`, D-8). Mengembalikan daftar pelanggaran (kosong = aman).
 */
export function checkClientMeta(meta: ResourceMeta): readonly string[] {
  const out = forbiddenKeys(meta);
  const byName = new Map(meta.fields.map((f) => [f.name, f]));
  const check = (where: string, cond: Condition | undefined, relationFilter = false) => {
    if (cond === undefined) return;
    const p = parseCondition(cond);
    if (!p.ok) {
      out.push(`${where}: kondisi tidak sah (${p.reason.code})`);
      return;
    }
    const refs: { field: boolean; name: string }[] = [];
    names(p.value, refs);
    for (const r of refs) {
      // pada filter relasi, `field` menunjuk field resource seberang; hanya `ref` menunjuk form ini
      if (relationFilter && r.field) continue;
      const f = byName.get(r.name);
      if (!f) {
        out.push(`${where}: merujuk field "${r.name}" yang tidak ada di metadata klien`);
      } else if (f.classification !== "public" && f.classification !== "internal") {
        out.push(
          `${where}: merujuk field "${r.name}" berklasifikasi ${f.classification} (harus scope server)`,
        );
      } else if (f.fieldGroup !== meta.defaultGroup) {
        out.push(
          `${where}: merujuk field "${r.name}" di grup ${f.fieldGroup} di luar grup default (harus scope server)`,
        );
      }
    }
  };
  for (const f of meta.fields) {
    check(`field ${f.name}.visibleWhen`, f.visibleWhen);
    check(`field ${f.name}.requiredWhen`, f.requiredWhen);
    check(`field ${f.name}.readonlyWhen`, f.readonlyWhen);
  }
  for (const r of meta.rules) check(`rule ${r.id}`, r.assert);
  for (const rel of meta.relations) check(`relation ${rel.name}.filter`, rel.filter, true);
  check("lockWhen", meta.lockWhen);
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
    if (!isPlainObject(meta) || !Array.isArray((meta as { fields?: unknown }).fields)) {
      throw new MetadataLoadError(
        "invalid_module",
        key,
        "ekspor resourceMeta tidak ada atau bentuknya salah",
      );
    }
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
