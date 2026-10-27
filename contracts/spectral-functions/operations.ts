// Rule operasi (api-guidelines §1, §3, §7, §13).
import {
  asObj,
  asStr,
  ctx,
  currentDoc,
  follow,
  type Operation,
  operations,
  parametersOf,
  type Result,
} from "./refs";

const escapeRe = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export function operationPermission(root: unknown): Result[] {
  const { slug } = ctx();
  const re = new RegExp(`^${escapeRe(slug)}\\.[a-z][a-z0-9-]*\\.[a-z][a-z0-9_]*$`);
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    if (op.node["x-erp-public"] === true) continue;
    const perm = asStr(op.node["x-erp-permission"]);
    if (perm === undefined || !re.test(perm)) {
      out.push({
        message: `${op.method.toUpperCase()} ${op.path} wajib x-erp-permission berbentuk ${slug}.<resource>.<action> atau x-erp-public: true (api-guidelines §1, §13)`,
        path: [...op.at],
      });
    }
  }
  return out;
}

export function operationId(root: unknown): Result[] {
  const out: Result[] = [];
  const seen = new Map<string, string>();
  for (const op of operations(currentDoc(root))) {
    const id = asStr(op.node.operationId);
    const where = `${op.method.toUpperCase()} ${op.path}`;
    if (id === undefined || !/^[a-z][a-zA-Z0-9]*$/.test(id)) {
      out.push({
        message: `${where} wajib operationId camelCase (api-guidelines §13)`,
        path: [...op.at],
      });
      continue;
    }
    const prev = seen.get(id);
    if (prev) {
      out.push({
        message: `operationId "${id}" pada ${where} sudah dipakai ${prev} (api-guidelines §13)`,
        path: [...op.at, "operationId"],
      });
    } else {
      seen.set(id, where);
    }
  }
  return out;
}

function hasHeader(root: unknown, op: Operation, name: string): boolean {
  return parametersOf(root, op).some(
    (p) => p.in === "header" && String(p.name).toLowerCase() === name.toLowerCase(),
  );
}

export function postIdempotencyKey(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    if (op.method === "post" && !hasHeader(root, op, "Idempotency-Key")) {
      out.push({
        message: `POST ${op.path} wajib parameter header Idempotency-Key (api-guidelines §7.2)`,
        path: [...op.at],
      });
    }
  }
  return out;
}

export function ifMatch(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    const segs = op.path
      .split("/")
      .filter((s) => s !== "")
      .slice(2);
    const isAction =
      op.method === "post" &&
      segs.length >= 2 &&
      !segs[segs.length - 1]?.startsWith("{") &&
      !!segs[segs.length - 2]?.startsWith("{");
    if (
      (op.method === "patch" || op.method === "delete" || isAction) &&
      !hasHeader(root, op, "If-Match")
    ) {
      out.push({
        message: `${op.method.toUpperCase()} ${op.path} wajib parameter header If-Match (api-guidelines §7.1)`,
        path: [...op.at],
      });
    }
  }
  return out;
}

export function noPut(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    if (op.method === "put" && op.node["x-erp-allow-put"] !== true) {
      out.push({
        message: `PUT ${op.path} tidak diizinkan; pakai PATCH atau tandai x-erp-allow-put: true (api-guidelines §3)`,
        path: [...op.at],
      });
    }
  }
  return out;
}

const hasExample = (media: unknown): boolean => {
  const m = asObj(media);
  return !!m && (m.example !== undefined || m.examples !== undefined);
};

export function examples(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    const responses = asObj(op.node.responses) ?? {};
    for (const [code, resp] of Object.entries(responses)) {
      if (!/^2(\d\d|XX)$/.test(code)) continue;
      const r = follow(root, resp);
      const content = r ? asObj(asObj(r.node)?.content) : undefined;
      for (const [mt, media] of Object.entries(content ?? {})) {
        if (mt.includes("json") && !hasExample(media)) {
          out.push({
            message: `Respons ${code} ${op.method.toUpperCase()} ${op.path} wajib punya example atau examples (api-guidelines §13)`,
            path: [...op.at, "responses", code],
          });
        }
      }
    }
    const rb = asObj(op.node.requestBody);
    const r = rb ? follow(root, rb) : undefined;
    const content = r ? asObj(asObj(r.node)?.content) : undefined;
    for (const [mt, media] of Object.entries(content ?? {})) {
      if (mt.includes("json") && !hasExample(media)) {
        out.push({
          message: `requestBody ${op.method.toUpperCase()} ${op.path} wajib punya example atau examples (api-guidelines §13)`,
          path: [...op.at, "requestBody"],
        });
      }
    }
  }
  return out;
}

export function errorCodesDocumented(root: unknown): Result[] {
  const out: Result[] = [];
  for (const op of operations(currentDoc(root))) {
    const codes = Object.keys(asObj(op.node.responses) ?? {});
    const where = `${op.method.toUpperCase()} ${op.path}`;
    const need: string[] = [];
    // Operasi publik tanpa masukan (mis. GET /api/bootstrap) tidak punya jalur error klien; 4xx dan 403 hanya
    // diwajibkan untuk operasi non-publik.
    const isPublic = op.node["x-erp-public"] === true;
    if (!isPublic && !codes.some((c) => c.startsWith("4"))) need.push("minimal satu respons 4xx");
    if (!isPublic && !codes.includes("403")) need.push("403");
    if (op.path.includes("{") && !codes.includes("404")) need.push("404");
    if (op.node.requestBody !== undefined) {
      if (!codes.includes("400")) need.push("400");
      if (!codes.includes("422")) need.push("422");
    }
    if (need.length > 0) {
      out.push({
        message: `${where} belum mendokumentasikan kode error: ${need.join(", ")} (api-guidelines §13)`,
        path: [...op.at, "responses"],
      });
    }
  }
  return out;
}
