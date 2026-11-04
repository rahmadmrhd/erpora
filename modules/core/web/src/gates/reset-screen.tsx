import { LoadingButton } from "@erpora/ui";
import { type FormEvent, useEffect, useState } from "react";
import { attempt, authClient } from "../api";
import { type AuthMessage, describeFailure } from "./errors";
import { GateLink } from "./nav";
import { AuthLayout, ErrorAlert, Field, PasswordInput, useCoreT } from "./parts";
import { readResetToken, scrubResetToken } from "./reset-token";

/**
 * Reset sandi dari tautan email. Token dibaca dari fragmen URL (pembacaan murni saat render) lalu fragmen dihapus dari
 * URL oleh efek (idempoten, aman di StrictMode); token hanya hidup di state komponen ini. Tanpa token: pesan tautan
 * tidak lengkap dan tautan minta ulang.
 */
export function ResetScreen() {
  const t = useCoreT();
  const [token] = useState(() => readResetToken(window.location.hash));
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<{ password?: string; confirm?: string }>({});
  const [failure, setFailure] = useState<AuthMessage[]>([]);
  const [tokenRejected, setTokenRejected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  useEffect(() => {
    scrubResetToken();
  }, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy || token === null) return;
    const next: { password?: string; confirm?: string } = {};
    if (password === "") next.password = t("auth.validation.required");
    if (confirm !== password) next.confirm = t("auth.validation.mismatch");
    setErrors(next);
    if (next.password || next.confirm) return;

    setBusy(true);
    setFailure([]);
    const out = await attempt(() =>
      authClient.post("/api/core/auth/password/reset", { body: { token, newPassword: password } }),
    );
    setBusy(false);
    if (out.ok) {
      setPassword("");
      setConfirm("");
      setDone(true);
      return;
    }
    const info = describeFailure(out.failure);
    setTokenRejected(info.detailCodes.includes("invalid_token"));
    setFailure(info.messages);
  }

  const toLogin = <GateLink to="/login">{t("auth.reset.toLogin")}</GateLink>;
  const requestNew = <GateLink to="/forgot-password">{t("auth.reset.requestNew")}</GateLink>;

  if (done) {
    return (
      <AuthLayout title={t("auth.reset.successTitle")} footer={toLogin}>
        <p className="text-sm text-muted-foreground" role="status">
          {t("auth.reset.success")}
        </p>
      </AuthLayout>
    );
  }

  if (token === null) {
    return (
      <AuthLayout title={t("auth.reset.title")} footer={requestNew}>
        <ErrorAlert messages={[{ key: "auth.reset.missingToken" }]} />
      </AuthLayout>
    );
  }

  return (
    <AuthLayout
      title={t("auth.reset.title")}
      description={t("auth.reset.description")}
      footer={tokenRejected ? requestNew : toLogin}
    >
      <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorAlert messages={failure} />
        <Field
          label={t("auth.reset.newPassword")}
          error={errors.password}
          hint={t("auth.common.passwordHint")}
        >
          {(p) => (
            <PasswordInput
              {...p}
              name="new-password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("auth.reset.confirmPassword")} error={errors.confirm}>
          {(p) => (
            <PasswordInput
              {...p}
              name="confirm-password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          )}
        </Field>
        <LoadingButton type="submit" loading={busy} className="w-full">
          {t("auth.reset.submit")}
        </LoadingButton>
      </form>
    </AuthLayout>
  );
}
