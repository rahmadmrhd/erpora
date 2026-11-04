import { Input, LoadingButton } from "@erpora/ui";
import { type FormEvent, useState } from "react";
import { attempt, authClient } from "../api";
import { type AuthMessage, describeFailure } from "./errors";
import { GateLink } from "./nav";
import { AuthLayout, ErrorAlert, Field, looksLikeEmail, useCoreT } from "./parts";

/**
 * Lupa sandi: selalu menampilkan pesan netral "bila terdaftar" setelah permintaan diterima (server menjawab 204 tanpa
 * membedakan akun, FR-107), jadi layar tidak membuka enumerasi email.
 */
export function ForgotScreen() {
  const t = useCoreT();
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | undefined>();
  const [failure, setFailure] = useState<AuthMessage[]>([]);
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    if (email.trim() === "") return setError(t("auth.validation.required"));
    if (!looksLikeEmail(email)) return setError(t("auth.validation.email"));
    setError(undefined);
    setFailure([]);
    setBusy(true);
    const out = await attempt(() =>
      authClient.post("/api/core/auth/password/forgot", { body: { email: email.trim() } }),
    );
    setBusy(false);
    if (out.ok) setSent(true);
    else setFailure(describeFailure(out.failure).messages);
  }

  const back = <GateLink to="/login">{t("auth.common.backToLogin")}</GateLink>;

  if (sent) {
    return (
      <AuthLayout title={t("auth.forgot.sentTitle")} footer={back}>
        <p className="text-sm text-muted-foreground" role="status">
          {t("auth.forgot.sent")}
        </p>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout
      title={t("auth.forgot.title")}
      description={t("auth.forgot.description")}
      footer={back}
    >
      <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
        <ErrorAlert messages={failure} />
        <Field label={t("auth.common.email")} error={error}>
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
        <LoadingButton type="submit" loading={busy} className="w-full">
          {t("auth.forgot.submit")}
        </LoadingButton>
      </form>
    </AuthLayout>
  );
}
