import { Alert, AlertDescription, Button, Checkbox, Label } from "@erpora/ui";
import { useId, useState } from "react";
import { AuthLayout, useCoreT } from "./parts";

/**
 * Menampilkan 10 recovery code SEKALI setelah pendaftaran MFA dikonfirmasi, dengan peringatan menyimpannya. Tombol
 * lanjut baru aktif setelah pengguna menandai sudah menyimpan. Kode hanya ada di props/memori (tidak disimpan).
 */
export function RecoveryCodesScreen({
  codes,
  onContinue,
}: {
  codes: readonly string[];
  onContinue(): void;
}) {
  const t = useCoreT();
  const [saved, setSaved] = useState(false);
  const checkId = useId();
  const listId = useId();
  return (
    <AuthLayout title={t("auth.mfa.recoveryCodes.title")}>
      <Alert>
        <AlertDescription>{t("auth.mfa.recoveryCodes.warning")}</AlertDescription>
      </Alert>
      <ul
        id={listId}
        aria-label={t("auth.mfa.recoveryCodes.listLabel")}
        className="grid grid-cols-2 gap-2 rounded-md border bg-muted p-3 font-mono text-sm select-all"
      >
        {codes.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </ul>
      <div className="flex items-center gap-2">
        <Checkbox id={checkId} checked={saved} onCheckedChange={(v) => setSaved(v === true)} />
        <Label htmlFor={checkId}>{t("auth.mfa.recoveryCodes.acknowledge")}</Label>
      </div>
      <Button type="button" className="w-full" disabled={!saved} onClick={onContinue}>
        {t("auth.mfa.recoveryCodes.continue")}
      </Button>
    </AuthLayout>
  );
}
