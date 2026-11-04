import { LoadingButton } from "@erpora/ui";
import { type FormEvent, useEffect, useRef, useState } from "react";
import { attempt, authClient } from "../api";
import { type AuthMessage, describeFailure } from "./errors";
import { GateLink } from "./nav";
import { AuthLayout, ErrorAlert, Field, PasswordInput, useCoreT } from "./parts";
import { readResetToken, scrubResetToken } from "./reset-token";

/**
 * Reset sandi dari tautan email. Token dibaca dari fragmen URL (pembacaan murni saat render) lalu fragmen dihapus dari
 * URL oleh efek (idempoten, aman di StrictMode); token hanya hidup di state komponen ini. Tanpa token: pesan tautan
 * tidak lengkap dan tautan minta ulang. Membuka tautan reset lain di tab yang sama hanya mengubah fragmen (tanpa
 * memuat ulang halaman): `hashchange` mengambil token baru, menghapusnya dari URL, dan mereset form ke token itu.
 */
export function ResetScreen() {
  const t = useCoreT();
  const [token, setToken] = useState(() => readResetToken(window.location.hash));
  // Token aktif untuk pembanding saat respons tiba: respons submit token lama (sebelum hashchange) harus diabaikan.
  const activeToken = useRef(token);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<{ password?: string; confirm?: string }>({});
  const [failure, setFailure] = useState<AuthMessage[]>([]);
  const [tokenRejected, setTokenRejected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  useEffect(() => {
    scrubResetToken();
    const onHashChange = () => {
      const fresh = readResetToken(window.location.hash);
      if (fresh === null) return; // fragmen lain: token yang sedang dipakai tidak diubah
      scrubResetToken();
      activeToken.current = fresh;
      setToken(fresh);
      setBusy(false); // permintaan token lama (bila ada) tidak boleh menahan form token baru
      // Isian, galat, dan penolakan milik token sebelumnya tidak berlaku lagi untuk token baru.
      setPassword("");
      setConfirm("");
      setErrors({});
      setFailure([]);
      setTokenRejected(false);
      setDone(false);
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
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
    if (activeToken.current !== token) return; // respons untuk token yang sudah diganti: abaikan seluruhnya
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
