// Client fetch bertipe di atas tipe `paths` hasil `erpora contracts:generate` (PRD-000 FR-06, keputusan rencana 19).
// Berada di paket ini karena frontend modul hanya boleh mengimpor paket @erpora/* tertentu (F2); library pihak ketiga
// seperti openapi-fetch tidak boleh diimpor dari modul.

export interface ApiError {
  code: string;
  message: string;
  details?: unknown;
  requestId?: string;
  traceId?: string;
}

export type ApiResult<T> =
  | { ok: true; status: number; data: T; page?: unknown; etag?: string }
  | { ok: false; status: number; error: ApiError };

export interface ClientOptions {
  baseUrl?: string;
  fetch?: typeof fetch;
  headers?: () => Record<string, string>;
  /** Bahasa aktif untuk header `Accept-Language` (dibaca ulang tiap request); kosong atau undefined = tidak dikirim. */
  getLocale?: () => string | undefined;
}

type Method = "get" | "post" | "patch" | "delete";

/** Path pada `Paths` yang punya method M. */
export type PathsWith<Paths, M extends Method> = {
  [K in keyof Paths]: Paths[K] extends { [m in M]: unknown } ? K : never;
}[keyof Paths] &
  string;

type OperationOf<Paths, P extends keyof Paths, M extends Method> = Paths[P] extends {
  [m in M]: infer O;
}
  ? O
  : never;

type SuccessBody<O> = O extends { responses: infer R }
  ? {
      [S in keyof R]: S extends 200 | 201 | 202
        ? R[S] extends { content: { "application/json": infer B } }
          ? B
          : never
        : never;
    }[keyof R]
  : never;

/** Isi `data` dari body sukses (200/201/202) operasi. */
export type SuccessData<O> =
  SuccessBody<O> extends infer B ? (B extends { data: infer D } ? D : never) : never;

type PathParams<O> = O extends { parameters: { path?: infer P } } ? P : never;
type QueryParams<O> = O extends { parameters: { query?: infer Q } } ? Q : never;
type BodyOf<O> = O extends { requestBody?: { content: { "application/json": infer B } } }
  ? B
  : never;

export interface CallInit<O> {
  params?: { path?: PathParams<O>; query?: QueryParams<O> };
  body?: BodyOf<O>;
  headers?: Record<string, string>;
}

export interface TypedClient<Paths> {
  get<P extends PathsWith<Paths, "get">>(
    path: P,
    init?: CallInit<OperationOf<Paths, P, "get">>,
  ): Promise<ApiResult<SuccessData<OperationOf<Paths, P, "get">>>>;
  post<P extends PathsWith<Paths, "post">>(
    path: P,
    init?: CallInit<OperationOf<Paths, P, "post">>,
  ): Promise<ApiResult<SuccessData<OperationOf<Paths, P, "post">>>>;
  patch<P extends PathsWith<Paths, "patch">>(
    path: P,
    init?: CallInit<OperationOf<Paths, P, "patch">>,
  ): Promise<ApiResult<SuccessData<OperationOf<Paths, P, "patch">>>>;
  delete<P extends PathsWith<Paths, "delete">>(
    path: P,
    init?: CallInit<OperationOf<Paths, P, "delete">>,
  ): Promise<ApiResult<SuccessData<OperationOf<Paths, P, "delete">>>>;
}

interface RawInit {
  params?: { path?: unknown; query?: unknown };
  body?: unknown;
  headers?: Record<string, string>;
}

function fillPath(path: string, params: unknown): string {
  const values = (params ?? {}) as Record<string, unknown>;
  return path.replace(/\{([^}]+)\}/g, (_, name: string) =>
    encodeURIComponent(String(values[name])),
  );
}

function queryString(query: unknown): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries((query ?? {}) as Record<string, unknown>)) {
    if (v === undefined || v === null) continue;
    for (const item of Array.isArray(v) ? v : [v]) q.append(k, String(item));
  }
  const s = q.toString();
  return s === "" ? "" : `?${s}`;
}

/** Membuat client bertipe; credentials "same-origin"; error HTTP menjadi ApiResult { ok: false } (tidak melempar). */
export function createClient<Paths>(opts: ClientOptions = {}): TypedClient<Paths> {
  const call = async (
    method: string,
    path: string,
    init: RawInit = {},
  ): Promise<ApiResult<unknown>> => {
    const doFetch = opts.fetch ?? globalThis.fetch;
    const locale = opts.getLocale?.();
    const headers: Record<string, string> = {
      Accept: "application/json",
      ...(locale ? { "Accept-Language": locale } : {}),
      ...opts.headers?.(),
      ...init.headers,
    };
    let body: string | undefined;
    if (init.body !== undefined) {
      headers["Content-Type"] = "application/json";
      body = JSON.stringify(init.body);
    }
    const url = `${opts.baseUrl ?? ""}${fillPath(path, init.params?.path)}${queryString(init.params?.query)}`;
    const res = await doFetch(url, {
      method: method.toUpperCase(),
      headers,
      body,
      credentials: "same-origin",
    });
    // 304 (If-None-Match/If-Match cocok) bukan galat server: pemanggil memakai `status === 304` dan data terakhirnya.
    if (res.status === 304) {
      return { ok: false, status: 304, error: { code: "not_modified", message: "" } };
    }
    const text = res.status === 204 ? "" : await res.text();
    let parsed: { data?: unknown; page?: unknown; error?: ApiError } | undefined;
    if (text !== "") {
      try {
        parsed = JSON.parse(text) as typeof parsed;
      } catch {
        parsed = undefined;
      }
    }
    if (res.ok) {
      const etag = res.headers.get("ETag") ?? undefined;
      return { ok: true, status: res.status, data: parsed?.data, page: parsed?.page, etag };
    }
    return {
      ok: false,
      status: res.status,
      error: parsed?.error ?? {
        code: "http_error",
        message: res.statusText || `HTTP ${res.status}`,
      },
    };
  };
  return {
    get: (path, init) => call("get", path, init as RawInit),
    post: (path, init) => call("post", path, init as RawInit),
    patch: (path, init) => call("patch", path, init as RawInit),
    delete: (path, init) => call("delete", path, init as RawInit),
  } as TypedClient<Paths>;
}
