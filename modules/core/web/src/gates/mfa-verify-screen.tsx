import { useAuth } from "@erpora/platform-web";
import { Button, Input, InputOTP, InputOTPGroup, InputOTPSlot, LoadingButton } from "@erpora/ui";
import { type FormEvent, useState } from "react";
import { attempt, authClient } from "../api";
import { type AuthMessage, describeFailure } from "./errors";
import { AuthLayout, ErrorAlert, Field, useCoreT } from "./parts";

const OTP_SLOTS = [0, 1, 2, 3, 4, 5] as const;

/**
 * Verifikasi MFA pada sesi yang menunggu verifikasi: kode TOTP 6 digit (komponen `input-otp`) atau recovery code
 * sekali pakai. Tersedia "Keluar" (logout boleh untuk sesi menunggu MFA, ADR-0013 §4).
 */
export function MfaVerifyScreen() {
  const t = useCoreT();
  const { acceptSession, logout } = useAuth();
  const [mode, setMode] = useState<"totp" | "recovery">("totp");
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState("");
  const [fieldError, setFieldError] = useState<string | undefined>();
  const [failure, setFailure] = useState<AuthMessage[]>([]);
  const [busy, setBusy] = useState(false);

  async function submit(value: string) {
    if (busy) return;
    const trimmed = value.trim();
    const valid = mode === "totp" ? /^[0-9]{6}$/.test(trimmed) : trimmed !== "";
    if (!valid) {
      setFieldError(t(mode === "totp" ? "auth.validation.code" : "auth.validation.recovery"));
      return;
    }
    setFieldError(undefined);
    setFailure([]);
    setBusy(true);
    const path = mode === "totp" ? "/api/core/auth/mfa/verify" : "/api/core/auth/mfa/recovery";
    const out = await attempt(() => authClient.post(path, { body: { code: trimmed } }));
    if (out.ok) {
      acceptSession(out.data);
      return;
    }
    setBusy(false);
    if (mode === "totp") setCode("");
    else setRecovery("");
    setFailure(describeFailure(out.failure).messages);
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    void submit(mode === "totp" ? code : recovery);
  }

  function switchMode() {
    setMode((m) => (m === "totp" ? "recovery" : "totp"));
    setFieldError(undefined);
    setFailure([]);
  }

  return (
    <AuthLayout
      title={t("auth.mfa.verify.title")}
      description={mode === "totp" ? t("auth.mfa.verify.description") : undefined}
      footer={
        <Button type="button" variant="link" onClick={() => void logout()}>
          {t("auth.common.signOut")}
        </Button>
      }
    >
      <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorAlert messages={failure} />
        {mode === "totp" ? (
          <Field label={t("auth.mfa.verify.codeLabel")} error={fieldError}>
            {(p) => (
              <InputOTP
                {...p}
                maxLength={6}
                value={code}
                onChange={setCode}
                onComplete={(v) => void submit(v)}
                disabled={busy}
                autoFocus
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
        ) : (
          <Field
            label={t("auth.mfa.verify.recoveryLabel")}
            error={fieldError}
            hint={t("auth.mfa.verify.recoveryHint")}
          >
            {(p) => (
              <Input
                {...p}
                name="recovery-code"
                autoComplete="off"
                autoCapitalize="none"
                spellCheck={false}
                autoFocus
                value={recovery}
                onChange={(e) => setRecovery(e.target.value)}
              />
            )}
          </Field>
        )}
        <LoadingButton type="submit" loading={busy} className="w-full">
          {t("auth.mfa.verify.submit")}
        </LoadingButton>
        <Button type="button" variant="link" onClick={switchMode} disabled={busy}>
          {t(mode === "totp" ? "auth.mfa.verify.useRecovery" : "auth.mfa.verify.useTotp")}
        </Button>
      </form>
    </AuthLayout>
  );
}
