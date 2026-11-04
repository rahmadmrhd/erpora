import { setDefaultTransport } from "@erpora/platform-contracts";
import { useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useSyncExternalStore,
} from "react";
import type { AuthController, AuthSessionPayload, AuthState } from "./controller";

const AuthContext = createContext<AuthController | null>(null);

export interface AuthProviderProps {
  controller: AuthController;
  /** `refresh` (bawaan): refresh senyap lewat cookie saat boot; `skip`: langsung anonymous (mis. halaman lupa/reset). */
  boot?: "refresh" | "skip";
  children: ReactNode;
}

/**
 * Menyediakan klien token ke pohon React dan memasangnya sebagai transport bawaan klien kontrak
 * (`setDefaultTransport`), sehingga setiap `createClient` modul otomatis membawa access token dan menyegarkannya.
 * Saat sesi berakhir (token dibuang), seluruh cache query dibersihkan agar data pengguna sebelumnya tidak bocor ke
 * pengguna berikutnya. Wajib di dalam `QueryClientProvider`.
 */
export function AuthProvider({ controller, boot = "refresh", children }: AuthProviderProps) {
  const queryClient = useQueryClient();
  const state = useSyncExternalStore(
    controller.subscribe,
    controller.getState,
    controller.getState,
  );

  // Layout effect: berjalan sebelum efek pasif anak mana pun (query pertama), jadi request pertama sudah berautentikasi.
  useLayoutEffect(() => {
    const transport = { fetch: controller.fetch as typeof fetch, headers: controller.headers };
    setDefaultTransport(transport);
    return () => setDefaultTransport(undefined);
  }, [controller]);

  useEffect(() => {
    if (boot === "skip") controller.skipBoot();
    else void controller.boot();
  }, [controller, boot]);

  const previous = useRef<AuthState["status"]>(state.status);
  useEffect(() => {
    const was = previous.current;
    previous.current = state.status;
    if (state.status === "anonymous" && (was === "authenticated" || was === "mfa")) {
      queryClient.clear();
    }
  }, [state.status, queryClient]);

  return <AuthContext.Provider value={controller}>{children}</AuthContext.Provider>;
}

export function useAuthController(): AuthController {
  const controller = useContext(AuthContext);
  if (controller === null) throw new Error("useAuth harus dipakai di dalam AuthProvider");
  return controller;
}

export interface UseAuth {
  state: AuthState;
  /** Dipanggil layar auth setelah login, verifikasi MFA, atau konfirmasi MFA berhasil. */
  acceptSession(session: AuthSessionPayload): void;
  logout(): Promise<void>;
  controller: AuthController;
}

/** Keadaan sesi dan aksi untuk layar auth dan menu pengguna. */
export function useAuth(): UseAuth {
  const controller = useAuthController();
  const state = useSyncExternalStore(
    controller.subscribe,
    controller.getState,
    controller.getState,
  );
  return {
    state,
    acceptSession: controller.acceptSession,
    logout: controller.logout,
    controller,
  };
}
