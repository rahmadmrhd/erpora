import { StaticI18nProvider, setActiveLocale, useAuth } from "@erpora/platform-web";
import { type UiLabelFn, UiLabelsProvider } from "@erpora/ui";
import { type ReactNode, useCallback, useEffect, useState } from "react";
import "../i18n/keys.gen";
import { GATE_CATALOGS, pickGateLocale } from "./catalogs";
import { ForgotScreen } from "./forgot-screen";
import { LoginScreen } from "./login-screen";
import { MfaEnrollScreen } from "./mfa-enroll-screen";
import { MfaVerifyScreen } from "./mfa-verify-screen";
import { type GateNavigate, GateNavProvider } from "./nav";
import { useCoreT } from "./parts";
import { ResetScreen } from "./reset-screen";

export interface AuthScreensProps {
  /** Path saat ini (gerbang tidak punya router): `/forgot-password`, `/reset-password`, selain itu layar login. */
  path: string;
  /** Pindah ke path gerbang lain (history API milik shell). */
  navigate: GateNavigate;
}

/** `@erpora/ui` membaca teks bawaannya lewat `UiLabels`; gerbang hanya butuh label pemuat (Spinner). */
function UiLabels({ children }: { children: ReactNode }) {
  const t = useCoreT();
  const label = useCallback<UiLabelFn>(
    (key) => (key === "status.loading" ? t("auth.common.loading") : key),
    [t],
  );
  return <UiLabelsProvider t={label}>{children}</UiLabelsProvider>;
}

function Screen({ path }: { path: string }) {
  const { state } = useAuth();
  // Sesi yang menunggu MFA menimpa path: layar MFA tampil di URL mana pun sampai verifikasi selesai.
  if (state.status === "mfa") {
    return state.mfaState === "enroll_required" ? <MfaEnrollScreen /> : <MfaVerifyScreen />;
  }
  if (path === "/forgot-password") return <ForgotScreen />;
  if (path === "/reset-password") return <ResetScreen />;
  return <LoginScreen />;
}

/**
 * Gerbang autentikasi pra-bootstrap (PRD-001 Q33): login, MFA (verifikasi dan pendaftaran), lupa sandi, dan reset
 * sandi, tanpa kerangka shell. Dirender di dalam `AuthProvider`; membawa katalog `core` sendiri (tanpa jaringan)
 * karena runtime i18n biasa membutuhkan bootstrap yang baru tersedia setelah login.
 */
export function AuthScreens({ path, navigate }: AuthScreensProps) {
  const [locale] = useState(pickGateLocale);
  // Idempoten: klien API memakai bahasa ini untuk Accept-Language (bahasa email reset sandi).
  setActiveLocale(locale);
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  return (
    <StaticI18nProvider locale={locale} catalogsByLocale={GATE_CATALOGS}>
      <UiLabels>
        <GateNavProvider navigate={navigate}>
          <Screen path={path} />
        </GateNavProvider>
      </UiLabels>
    </StaticI18nProvider>
  );
}
