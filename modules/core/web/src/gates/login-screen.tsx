import { useAuth } from "@erpora/platform-web";
import { Input, LoadingButton } from "@erpora/ui";
import { type FormEvent, useState } from "react";
import { attempt, authClient } from "../api";
import { type AuthMessage, describeFailure } from "./errors";
import { GateLink } from "./nav";
import { AuthLayout, ErrorAlert, Field, looksLikeEmail, PasswordInput, useCoreT } from "./parts";

/**
 * Layar login (email + sandi). Kegagalan memakai pesan seragam `invalid_credentials` (tidak membedakan email atau
 * sandi). Sukses menyerahkan sesi ke klien token; gerbang lalu menampilkan layar MFA atau aplikasi.
 */
export function LoginScreen() {
  const t = useCoreT();
  const { acceptSession } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errors, setErrors] = useState<{ email?: string; password?: string }>({});
  const [failure, setFailure] = useState<AuthMessage[]>([]);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    const next: { email?: string; password?: string } = {};
    if (email.trim() === "") next.email = t("auth.validation.required");
    else if (!looksLikeEmail(email)) next.email = t("auth.validation.email");
    if (password === "") next.password = t("auth.validation.required");
    setErrors(next);
    if (next.email || next.password) return;

    setBusy(true);
    setFailure([]);
    const out = await attempt(() =>
      authClient.post("/api/core/auth/login", { body: { email: email.trim(), password } }),
    );
    if (out.ok) {
      acceptSession(out.data);
      return;
    }
    setBusy(false);
    setPassword("");
    setFailure(describeFailure(out.failure).messages);
  }

  return (
    <AuthLayout title={t("auth.login.title")} description={t("auth.login.description")}>
      <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorAlert messages={failure} />
        <Field label={t("auth.common.email")} error={errors.email}>
          {(p) => (
            <Input
              {...p}
              type="email"
              name="email"
              autoComplete="username"
              autoCapitalize="none"
              spellCheck={false}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("auth.common.password")} error={errors.password}>
          {(p) => (
            <PasswordInput
              {...p}
              name="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          )}
        </Field>
        <div className="flex justify-end">
          <GateLink to="/forgot-password">{t("auth.login.forgot")}</GateLink>
        </div>
        <LoadingButton type="submit" loading={busy} className="w-full">
          {t("auth.login.submit")}
        </LoadingButton>
      </form>
    </AuthLayout>
  );
}
