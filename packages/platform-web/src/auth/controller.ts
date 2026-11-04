// Klien token (PRD-001 Q33 butir 3, FR-107, ADR-0013 §1): access token HANYA di memori (variabel closure ini);
// refresh token hanya cookie HttpOnly yang tidak terbaca JS. Tidak ada localStorage/sessionStorage/document.cookie/
// IndexedDB di berkas ini (diuji dengan spy penyimpanan).

export type MfaState = "none" | "enroll_required" | "verify_required";

/** Hasil login, refresh, atau verifikasi MFA (`Session` di kontrak core). */
export interface AuthSessionPayload {
  accessToken: string;
  expiresIn: number;
  mfaState: MfaState;
}

export type AuthState =
  | { readonly status: "booting" }
  | { readonly status: "anonymous" }
  | { readonly status: "mfa"; readonly mfaState: "enroll_required" | "verify_required" }
  | { readonly status: "authenticated" };

/** Bentuk `fetch` tanpa anggota statis (`preconnect`) agar fungsi biasa dapat memenuhinya. */
export type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export type RefreshOutcome = "ok" | "rejected" | "error";

export interface AuthControllerOptions {
  /** Bawaan: `globalThis.fetch` pada saat dipanggil (agar uji dapat menggantinya). */
  fetch?: FetchLike;
  baseUrl?: string;
  /** Keadaan awal: `booting` (bawaan; menunggu `boot()`) atau `anonymous` (tanpa refresh senyap). */
  initial?: "booting" | "anonymous";
}

export interface AuthController {
  getState(): AuthState;
  subscribe(listener: () => void): () => void;
  getAccessToken(): string | undefined;
  /** Header `Authorization` bila ada token; objek kosong bila tidak. */
  headers(): Record<string, string>;
  /** `fetch` berautentikasi: Bearer, refresh single-flight pada 401, ulang sekali, deteksi `mfa_required`. */
  fetch: FetchLike;
  /** Menerima sesi dari login/verifikasi/konfirmasi MFA/refresh; mengatur keadaan menurut `mfaState`. */
  acceptSession(session: AuthSessionPayload): void;
  /** Refresh senyap saat boot (cookie); gagal apa pun -> `anonymous`. Aman dipanggil dua kali. */
  boot(): Promise<void>;
  /** Boot tanpa jaringan (mis. halaman lupa/reset sandi). */
  skipBoot(): void;
  /** Tukar cookie refresh dengan access token baru (single-flight). */
  refresh(): Promise<RefreshOutcome>;
  /** POST logout (bila ada token), lalu mengakhiri sesi lokal apa pun hasilnya. */
  logout(): Promise<void>;
  /** Membuang token dan beralih ke `anonymous`. */
  endSession(): void;
}

const REFRESH_PATH = "/api/core/auth/refresh";
const LOGOUT_PATH = "/api/core/auth/logout";
const LOCK_NAME = "erpora-auth-refresh";

const ANONYMOUS: AuthState = { status: "anonymous" };
const BOOTING: AuthState = { status: "booting" };
const AUTHENTICATED: AuthState = { status: "authenticated" };
const MFA_VERIFY: AuthState = { status: "mfa", mfaState: "verify_required" };
const MFA_ENROLL: AuthState = { status: "mfa", mfaState: "enroll_required" };

function stateFor(mfaState: MfaState): AuthState {
  if (mfaState === "enroll_required") return MFA_ENROLL;
  if (mfaState === "verify_required") return MFA_VERIFY;
  return AUTHENTICATED;
}

const MFA_STATES: readonly unknown[] = ["none", "enroll_required", "verify_required"];

function isSessionPayload(v: unknown): v is AuthSessionPayload {
  const s = v as Partial<AuthSessionPayload> | null;
  return (
    typeof s?.accessToken === "string" &&
    s.accessToken !== "" &&
    typeof s.expiresIn === "number" &&
    MFA_STATES.includes(s.mfaState)
  );
}

/** Kode galat dari salinan respons (tanpa mengonsumsi body asli); `undefined` bila bukan amplop galat. */
async function peekErrorCode(res: Response): Promise<string | undefined> {
  try {
    const body = (await res.clone().json()) as { error?: { code?: unknown } };
    return typeof body?.error?.code === "string" ? body.error.code : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Kunci lintas tab (Web Locks) bila tersedia: dua tab yang me-refresh bersamaan dengan cookie yang sama akan
 * memicu deteksi pemakaian ulang token (ADR-0013) dan mencabut seluruh keluarga sesi. Tanpa Web Locks tetap berjalan.
 */
function withRefreshLock<T>(fn: () => Promise<T>): Promise<T> {
  const locks = (globalThis.navigator as { locks?: LockManager } | undefined)?.locks;
  return locks?.request ? (locks.request(LOCK_NAME, fn) as Promise<T>) : fn();
}

export function createAuthController(options: AuthControllerOptions = {}): AuthController {
  const baseFetch: FetchLike = (input, init) => (options.fetch ?? globalThis.fetch)(input, init);
  const url = (path: string) => `${options.baseUrl ?? ""}${path}`;

  let token: string | undefined;
  let state: AuthState = options.initial === "anonymous" ? ANONYMOUS : BOOTING;
  const listeners = new Set<() => void>();
  let inflight: Promise<RefreshOutcome> | undefined;
  let booting: Promise<void> | undefined;

  const setState = (next: AuthState): void => {
    if (next === state) return;
    state = next;
    for (const l of [...listeners]) l();
  };

  const endSession = (): void => {
    token = undefined;
    setState(ANONYMOUS);
  };

  const acceptSession = (s: AuthSessionPayload): void => {
    token = s.accessToken;
    setState(stateFor(s.mfaState));
  };

  async function doRefresh(): Promise<RefreshOutcome> {
    try {
      const res = await baseFetch(url(REFRESH_PATH), {
        method: "POST",
        headers: { Accept: "application/json" },
        // Satu-satunya permintaan yang membawa cookie refresh secara eksplisit (Q33 butir 3).
        credentials: "include",
      });
      if (res.status === 401 || res.status === 403) {
        endSession();
        return "rejected";
      }
      if (!res.ok) return "error";
      const body = (await res.json()) as { data?: unknown } | null;
      if (!isSessionPayload(body?.data)) return "error";
      acceptSession(body.data);
      return "ok";
    } catch {
      return "error";
    }
  }

  const refresh = (): Promise<RefreshOutcome> => {
    inflight ??= withRefreshLock(doRefresh).finally(() => {
      inflight = undefined;
    });
    return inflight;
  };

  const withAuth = (init: RequestInit | undefined, bearer: string | undefined): RequestInit => {
    const headers = new Headers(init?.headers);
    if (bearer !== undefined) headers.set("Authorization", `Bearer ${bearer}`);
    return { ...init, headers };
  };

  const authedFetch: FetchLike = async (input, init) => {
    const sent = token;
    const res = await baseFetch(input, withAuth(init, sent));
    if (sent === undefined || (res.status !== 401 && res.status !== 403)) return res;
    const code = await peekErrorCode(res);
    if (res.status === 401) {
      // 401 token_expired/unauthenticated: refresh single-flight lalu ulang SEKALI (gagal: respons asli).
      if ((code === "token_expired" || code === "unauthenticated") && (await refresh()) === "ok") {
        return baseFetch(input, withAuth(init, token));
      }
    } else if (code === "mfa_required" && state.status === "authenticated") {
      // Keadaan sebenarnya diambil dari refresh (server tahu enroll/verify); bawaan verify_required.
      await refresh();
      if (state.status === "authenticated") setState(MFA_VERIFY);
    }
    return res;
  };

  return {
    getState: () => state,
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    getAccessToken: () => token,
    headers: (): Record<string, string> =>
      token === undefined ? {} : { Authorization: `Bearer ${token}` },
    fetch: authedFetch,
    acceptSession,
    boot() {
      if (state.status !== "booting") return Promise.resolve();
      booting ??= refresh().then((outcome) => {
        if (outcome === "error" && state.status === "booting") setState(ANONYMOUS);
      });
      return booting;
    },
    skipBoot() {
      if (state.status === "booting") setState(ANONYMOUS);
    },
    refresh,
    async logout() {
      // Sesi lokal tetap berakhir apa pun hasilnya; cookie refresh yang tersisa dicabut server saat kedaluwarsa.
      if (token !== undefined)
        await authedFetch(url(LOGOUT_PATH), { method: "POST" }).catch(() => {});
      endSession();
    },
    endSession,
  };
}
