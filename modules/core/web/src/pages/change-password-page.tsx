import { useTranslation } from "@erpora/platform-web";
import { Alert, AlertDescription, LoadingButton } from "@erpora/ui";
import { type FormEvent, useState } from "react";
import { attempt, authClient } from "../api";
import { type AuthMessage, describeFailure } from "../gates/errors";
import { ErrorAlert, Field, PasswordInput } from "../gates/parts";
import "../i18n/keys.gen";

type Errors = { current?: string; next?: string; confirm?: string };

/**
 * Halaman "Ubah sandi" (`/core/change-password`, halaman modul biasa di dalam shell). Mengganti sandi Principal
 * sendiri; sesi lain dicabut server, sesi ini tetap hidup. Teks seluruhnya dari katalog `core`.
 */
export function ChangePasswordPage() {
  const { t } = useTranslation("core");
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<Errors>({});
  const [failure, setFailure] = useState<AuthMessage[]>([]);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    const found: Errors = {};
    if (current === "") found.current = t("auth.validation.required");
    if (next === "") found.next = t("auth.validation.required");
    if (confirm !== next) found.confirm = t("auth.validation.mismatch");
    setErrors(found);
    setDone(false);
    if (found.current || found.next || found.confirm) return;

    setBusy(true);
    setFailure([]);
    const out = await attempt(() =>
      authClient.post("/api/core/auth/password/change", {
        body: { currentPassword: current, newPassword: next },
      }),
    );
    setBusy(false);
    if (out.ok) {
      setCurrent("");
      setNext("");
      setConfirm("");
      setDone(true);
      return;
    }
    setFailure(describeFailure(out.failure).messages);
  }

  return (
    <div className="flex max-w-md flex-col gap-4 p-6">
      <p className="text-sm text-muted-foreground">{t("changePassword.description")}</p>
      {done ? (
        <Alert role="status">
          <AlertDescription>{t("changePassword.success")}</AlertDescription>
        </Alert>
      ) : null}
      <ErrorAlert messages={failure} />
      <form noValidate onSubmit={onSubmit} className="flex flex-col gap-4">
        <Field label={t("changePassword.current")} error={errors.current}>
          {(p) => (
            <PasswordInput
              {...p}
              name="current-password"
              autoComplete="current-password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("changePassword.new")}
          error={errors.next}
          hint={t("auth.common.passwordHint")}
        >
          {(p) => (
            <PasswordInput
              {...p}
              name="new-password"
              autoComplete="new-password"
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          )}
        </Field>
        <Field label={t("changePassword.confirm")} error={errors.confirm}>
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
        <LoadingButton type="submit" loading={busy}>
          {t("changePassword.submit")}
        </LoadingButton>
      </form>
    </div>
  );
}
