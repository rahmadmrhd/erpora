import { type ApiError, type ApiResult, createClient } from "@erpora/platform-contracts";
import { getActiveLocale } from "@erpora/platform-web";
import type { components, paths } from "../api/contract.gen";

export type Session = components["schemas"]["Session"];
export type Enrollment = components["schemas"]["Enrollment"];
export type ConfirmResult = components["schemas"]["ConfirmResult"];

/**
 * Klien API core. Dibuat tanpa `fetch`/`headers` eksplisit: aplikasi memasang transport bawaan (klien token di
 * `@erpora/platform-web`), sehingga route terautentikasi membawa access token dan menyegarkannya otomatis. Hanya
 * `Accept-Language` yang diatur di sini (email reset sandi memakai bahasa permintaan).
 */
export const authClient = createClient<paths>({ getLocale: getActiveLocale });

/** Galat permintaan yang sudah dinormalkan: respons galat server atau kegagalan jaringan. */
export interface Failure {
  status: number;
  error: ApiError;
  /** Detik dari `Retry-After` pada 429. */
  retryAfter?: number;
  /** `true` bila permintaan tidak sampai ke server (jaringan putus, CORS, dsb.). */
  network: boolean;
}

export type Outcome<T> = { ok: true; data: T } | { ok: false; failure: Failure };

/** Menjalankan satu panggilan klien dan tidak pernah melempar: jaringan putus menjadi `network: true`. */
export async function attempt<T>(run: () => Promise<ApiResult<T>>): Promise<Outcome<T>> {
  try {
    const res = await run();
    if (res.ok) return { ok: true, data: res.data };
    return {
      ok: false,
      failure: {
        status: res.status,
        error: res.error,
        network: false,
        ...(res.retryAfter === undefined ? {} : { retryAfter: res.retryAfter }),
      },
    };
  } catch {
    return {
      ok: false,
      failure: { status: 0, error: { code: "network", message: "" }, network: true },
    };
  }
}
