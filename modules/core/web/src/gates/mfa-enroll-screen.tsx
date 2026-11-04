import { useAuth } from "@erpora/platform-web";
import { Button, InputOTP, InputOTPGroup, InputOTPSlot, LoadingButton, Spinner } from "@erpora/ui";
import { type FormEvent, lazy, Suspense, useEffect, useRef, useState } from "react";
import { attempt, authClient, type Enrollment, type Session } from "../api";
import { type AuthMessage, describeFailure } from "./errors";
import { AuthLayout, ErrorAlert, Field, useCoreT } from "./parts";
import { RecoveryCodesScreen } from "./recovery-codes";

// Pustaka QR (uqr) hanya dimuat di layar ini: bukan bagian JS awal.
const QrCode = lazy(() => import("./qr-code"));

const OTP_SLOTS = [0, 1, 2, 3, 4, 5] as const;

/** Mengelompokkan secret base32 per 4 karakter agar mudah diketik manual. */
export function groupSecret(secret: string): string {
  return secret
    .replace(/\s+/g, "")
    .replace(/(.{4})/g, "$1 ")
    .trim();
}

interface Confirmed {
  codes: readonly string[];
  session: Session | undefined;
}

/**
 * Pendaftaran MFA (administrator wajib, `enroll_required`): memulai pendaftaran sekali saat layar tampil, menampilkan QR
 * dan secret untuk input manual, lalu mengonfirmasi dengan kode pertama. Setelah itu 10 recovery code ditampilkan
 * sekali; sesi baru baru dipakai setelah pengguna melanjutkan (menerimanya lebih awal akan menutup layar recovery).
 */
export function MfaEnrollScreen() {
  const t = useCoreT();
  const { acceptSession, controller, logout } = useAuth();
  const [enrollment, setEnrollment] = useState<Enrollment | undefined>();
  const [startFailure, setStartFailure] = useState<AuthMessage[]>([]);
  const [signOutFailure, setSignOutFailure] = useState<AuthMessage[]>([]);
  const [code, setCode] = useState("");
  const [fieldError, setFieldError] = useState<string | undefined>();
  const [failure, setFailure] = useState<AuthMessage[]>([]);
  const [busy, setBusy] = useState(false);
  const [confirmed, setConfirmed] = useState<Confirmed | undefined>();
  // Pendaftaran membuat faktor menunggu di server: tepat satu panggilan per tampil (StrictMode menjalankan efek dua kali).
  const started = useRef(false);

  async function start() {
    setStartFailure([]);
    const out = await attempt(() => authClient.post("/api/core/auth/mfa/enroll"));
    if (out.ok) setEnrollment(out.data);
    else setStartFailure(describeFailure(out.failure).messages);
  }

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void start();
  }, []);

  async function submit(value: string) {
    if (busy) return;
    if (!/^[0-9]{6}$/.test(value)) {
      setFieldError(t("auth.validation.code"));
      return;
    }
    setFieldError(undefined);
    setFailure([]);
    setBusy(true);
    const out = await attempt(() =>
      authClient.post("/api/core/auth/mfa/confirm", { body: { code: value } }),
    );
    setBusy(false);
    if (out.ok) {
      setConfirmed({ codes: out.data.codes, session: out.data.session });
      return;
    }
    setCode("");
    setFailure(describeFailure(out.failure).messages);
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    void submit(code);
  }

  async function onContinue() {
    if (confirmed?.session) {
      acceptSession(confirmed.session);
      return;
    }
    // Penaikan sesi gagal setelah faktor aktif: server tahu keadaan sesi (verify_required); ambil lewat refresh.
    await controller.refresh();
  }

  async function onSignOut() {
    // false = server tidak mengakui logout: sesi masih aktif, jadi beri tahu pengguna (bukan diam).
    setSignOutFailure((await logout()) ? [] : [{ key: "auth.common.signOutFailed" }]);
  }

  if (confirmed)
    return <RecoveryCodesScreen codes={confirmed.codes} onContinue={() => void onContinue()} />;

  return (
    <AuthLayout
      title={t("auth.mfa.enroll.title")}
      description={t("auth.mfa.enroll.description")}
      footer={
        <Button type="button" variant="link" onClick={() => void onSignOut()}>
          {t("auth.common.signOut")}
        </Button>
      }
    >
      <ErrorAlert messages={signOutFailure} />
      <ErrorAlert messages={startFailure} />
      {startFailure.length > 0 ? (
        <Button type="button" variant="outline" onClick={() => void start()}>
          {t("auth.mfa.enroll.retry")}
        </Button>
      ) : null}
      {enrollment ? (
        <>
          <Suspense
            fallback={
              <div className="flex h-48 items-center justify-center">
                <Spinner />
              </div>
            }
          >
            <QrCode text={enrollment.otpauthUri} label={t("auth.mfa.enroll.qrLabel")} />
          </Suspense>
          <div className="flex flex-col gap-1 text-center">
            <p className="text-sm text-muted-foreground">{t("auth.mfa.enroll.secretLabel")}</p>
            <code className="font-mono text-sm break-all select-all">
              {groupSecret(enrollment.secret)}
            </code>
          </div>
          <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
            <ErrorAlert messages={failure} />
            <Field label={t("auth.mfa.enroll.codeLabel")} error={fieldError}>
              {(p) => (
                <InputOTP
                  {...p}
                  maxLength={6}
                  value={code}
                  onChange={setCode}
                  onComplete={(v) => void submit(v)}
                  disabled={busy}
                  inputMode="numeric"
                  pattern="^[0-9]*$"
                  autoComplete="one-time-code"
                  containerClassName="justify-center"
                >
                  <InputOTPGroup>
                    {OTP_SLOTS.map((i) => (
                      <InputOTPSlot key={i} index={i} aria-invalid={p["aria-invalid"]} />
                    ))}
                  </InputOTPGroup>
                </InputOTP>
              )}
            </Field>
            <LoadingButton type="submit" loading={busy} className="w-full">
              {t("auth.mfa.enroll.submit")}
            </LoadingButton>
          </form>
        </>
      ) : startFailure.length === 0 ? (
        <div className="flex h-48 items-center justify-center">
          <Spinner />
        </div>
      ) : null}
    </AuthLayout>
  );
}
