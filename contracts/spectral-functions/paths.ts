// Rule path (api-guidelines §2, PRD-000 FR-23.1).
import { asObj, ctx, currentDoc, type Obj, operations, type Result } from "./refs";

const KEBAB = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;
const PARAM = /^\{[a-z][A-Za-z0-9]*\}$/;
const CRUD = "get|list|create|update|delete|remove|add|edit|fetch|save|new";
const CRUD_EXACT = new RegExp(`^(${CRUD})$`);
const CRUD_CAMEL = new RegExp(`^(${CRUD})[A-Z]`);

function pathsOf(root: unknown): [string, Obj][] {
  const paths = asObj(currentDoc(root).paths) ?? {};
  return Object.entries(paths).map(([p, v]) => [p, asObj(v) ?? {}]);
}

const segs = (p: string): string[] => p.split("/").filter((s) => s !== "");

/** Segmen setelah /api/<slug>; /api/bootstrap tidak punya sisa. */
function rest(p: string): string[] {
  return segs(p).slice(2);
}

export function pathModulePrefix(root: unknown): Result[] {
  const { slug, target } = ctx();
  if (target === "common") return [];
  const out: Result[] = [];
  for (const [p] of pathsOf(root)) {
    const ok =
      target === "platform"
        ? p === "/api/bootstrap" || p.startsWith("/api/platform/")
        : p.startsWith(`/api/${slug}/`);
    if (!ok) {
      const want = target === "platform" ? "/api/platform/ atau /api/bootstrap" : `/api/${slug}/`;
      out.push({
        message: `Path ${p} harus diawali ${want} (api-guidelines §2, PRD-000 FR-23.1)`,
        path: ["paths", p],
      });
    }
  }
  return out;
}

export function pathKebabCase(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [p] of pathsOf(root)) {
    for (const s of segs(p)) {
      const ok = s.startsWith("{") ? PARAM.test(s) : KEBAB.test(s);
      if (!ok) {
        out.push({
          message: `Segmen "${s}" pada ${p} harus kebab-case huruf kecil, atau {camelCase} untuk parameter (api-guidelines §2)`,
          path: ["paths", p],
        });
      }
    }
  }
  return out;
}

export function pathNoTrailingSlash(root: unknown): Result[] {
  return pathsOf(root)
    .filter(([p]) => p.length > 1 && p.endsWith("/"))
    .map(([p]) => ({
      message: `Path ${p} tidak boleh diakhiri "/" (api-guidelines §2)`,
      path: ["paths", p],
    }));
}

export function pathNoCrudVerb(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [p] of pathsOf(root)) {
    for (const s of rest(p)) {
      if (s.startsWith("{")) continue;
      if (CRUD_EXACT.test(s) || CRUD_CAMEL.test(s)) {
        out.push({
          message: `Segmen "${s}" pada ${p} memakai kata kerja CRUD; gunakan koleksi jamak dan method HTTP, atau endpoint aksi POST …/{id}/<aksi> (api-guidelines §2)`,
          path: ["paths", p],
        });
      }
    }
  }
  return out;
}

export function pathCollectionPlural(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [p, item] of pathsOf(root)) {
    if (item["x-erp-singleton"] === true) continue;
    const r = rest(p);
    let last = -1;
    for (let i = 0; i < r.length; i++) if (!r[i]?.startsWith("{")) last = i;
    if (last < 0) continue;
    const seg = r[last] as string;
    // Segmen aksi: segmen statis terakhir setelah {id} dan semua operasinya POST.
    const methods = operations({ paths: { [p]: item } }).map((o) => o.method);
    const isAction =
      last === r.length - 1 &&
      last > 0 &&
      r[last - 1]?.startsWith("{") &&
      methods.length > 0 &&
      methods.every((m) => m === "post");
    if (isAction || seg.endsWith("s")) continue;
    out.push({
      message: `Segmen koleksi "${seg}" pada ${p} harus jamak; resource tunggal wajib bertanda x-erp-singleton: true (api-guidelines §2)`,
      path: ["paths", p],
    });
  }
  return out;
}

export function pathMaxNesting(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [p] of pathsOf(root)) {
    const r = rest(p);
    const first = r.findIndex((s) => s.startsWith("{"));
    if (first < 0) continue;
    const staticAfter = r.slice(first + 1).filter((s) => !s.startsWith("{")).length;
    if (staticAfter > 1) {
      out.push({
        message: `Path ${p} bersarang lebih dari satu sub-koleksi setelah {id} (api-guidelines §2)`,
        path: ["paths", p],
      });
    }
  }
  return out;
}
