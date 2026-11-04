import { AuthScreens } from "@erpora/module-core";
import { loginHref, safeReturnTo, useAuth } from "@erpora/platform-web";
import {
  createContext,
  type ReactNode,
  Suspense,
  useCallback,
  useContext,
  useLayoutEffect,
  useRef,
} from "react";
import { BootLoading } from "./boot-screens";
import { useGateLocation } from "./gate-location";

/** Jalur gerbang autentikasi tingkat atas (PRD-001 Q33 butir 2; slug tercadang di registry). */
export const GATE_PATHS: ReadonlySet<string> = new Set([
  "/login",
  "/forgot-password",
  "/reset-password",
]);

/** Jalur yang tidak membutuhkan sesi dan tidak boleh memicu refresh senyap saat boot. */
export const PUBLIC_GATE_PATHS: ReadonlySet<string> = new Set([
  "/forgot-password",
  "/reset-password",
]);

const SignOutContext = createContext<() => Promise<void>>(() => Promise.resolve());

/**
 * Keluar dari menu pengguna: mengakhiri sesi lalu membuka `/login` bersih (tanpa `returnTo`). Menolak (reject) bila server
 * tidak mengakui logout; sesi lokal tetap aktif.
 */
export function useSignOut(): () => Promise<void> {
  return useContext(SignOutContext);
}

/**
 * Gerbang sebelum bootstrap dan router (PRD-001 Q33 butir 2): selama sesi belum siap, hanya layar auth modul core
 * (dimuat malas) yang dirender, tanpa kerangka shell dan tanpa memanggil `/api/bootstrap`. Anak (bootstrap, router,
 * shell) baru dibangun setelah sesi penuh (tanpa MFA yang tertunda) dan URL bukan jalur gerbang.
 */
export function AuthGate({ children }: { children: ReactNode }) {
  const { state, logout } = useAuth();
  const location = useGateLocation();
  const explicitSignOut = useRef(false);
  const onGatePath = GATE_PATHS.has(location.path);
  const { replace } = location;

  const signOut = useCallback(async () => {
    // Dibaca dan dihapus oleh efek di bawah saat keadaan berubah ke anonymous (urutan dengan lanjutan await tak pasti).
    explicitSignOut.current = true;
    const signedOut = await logout();
    if (!signedOut) {
      // Server tidak mengakui logout: sesi tetap aktif (komputer bersama tidak boleh tampak sudah keluar). Menu pengguna
      // menampilkan galat dengan opsi coba lagi.
      explicitSignOut.current = false;
      throw new Error("signout_failed");
    }
  }, [logout]);

  const returnTo = safeReturnTo(new URLSearchParams(location.search).get("returnTo"));

  useLayoutEffect(() => {
    if (state.status === "anonymous") {
      const explicit = explicitSignOut.current;
      explicitSignOut.current = false;
      if (!onGatePath) {
        replace(explicit ? "/login" : loginHref(`${location.path}${location.search}`));
      }
    } else if (state.status === "authenticated" && onGatePath) {
      replace(returnTo);
    }
  }, [state.status, onGatePath, replace, location.path, location.search, returnTo]);

  if (state.status === "booting") return <BootLoading />;
  if (state.status === "authenticated") {
    return onGatePath ? (
      <BootLoading />
    ) : (
      <SignOutContext.Provider value={signOut}>{children}</SignOutContext.Provider>
    );
  }
  return (
    <Suspense fallback={<BootLoading />}>
      <AuthScreens path={location.path} navigate={location.navigate} />
    </Suspense>
  );
}
