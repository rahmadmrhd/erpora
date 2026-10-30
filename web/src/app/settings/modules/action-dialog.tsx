import type { FrontendModule } from "@erpora/module-sdk";
import { BOOTSTRAP_KEY, scopedKey, useTenantId } from "@erpora/platform-web";
import {
  Alert,
  AlertDescription,
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  BadgeStatus,
  Button,
  Checkbox,
  Label,
  LoadingIcon,
} from "@erpora/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { ModulesApiError, type OperationPolling, runModuleOperation } from "./api";
import { useModulesClient } from "./client";
import { type MappedError, mapFailureCode, mapModuleError } from "./errors";
import { computeImpact, type Effect, type InstallOptions } from "./impact";
import { type MessageDescriptor, opKey, useDescribe, useModulesT } from "./keys";
import type { ModuleOp, ModuleView, OperationView } from "./types";
import { MODULE_OPS } from "./types";

type Phase =
  | { kind: "confirm" }
  | { kind: "running" }
  | { kind: "done"; operations: readonly OperationView[] }
  | { kind: "polling"; ids: readonly string[] }
  | { kind: "error"; mapped: MappedError };

/**
 * Satu baris dampak. Label menu memakai `name` dan `key` dari API modul, BUKAN katalog namespace modul: backend hanya
 * menyajikan katalog modul yang `enabled` (404 untuk lainnya), jadi memuatnya untuk modul nonaktif meruntuhkan halaman.
 */
function EffectLine({ effect, target }: { effect: Effect; target: ModuleView }) {
  const t = useModulesT();
  switch (effect.kind) {
    case "menu_hidden":
    case "menu_shown":
      return (
        <li>{t(`modules.impact.${effect.kind}`, { labels: `${target.name} (${target.key})` })}</li>
      );
    case "routes_inactive":
    case "routes_active":
      return <li>{t(`modules.impact.${effect.kind}`, { paths: effect.paths.join(", ") })}</li>;
    case "api_closed":
      return (
        <li>{t("modules.impact.api_closed", { prefix: effect.prefix, code: effect.code })}</li>
      );
    case "data_kept":
      return <li>{t("modules.impact.data_kept")}</li>;
    case "migrations_run":
      return <li>{t("modules.impact.migrations_run")}</li>;
    case "version_change":
      return (
        <li>{t("modules.impact.version_change", { from: effect.from ?? "—", to: effect.to })}</li>
      );
    case "dependents_uncertain":
      return (
        <li>{t("modules.impact.dependents_uncertain", { modules: effect.modules.join(", ") })}</li>
      );
    case "needs_dependencies":
      return (
        <li>{t("modules.impact.needs_dependencies", { modules: effect.modules.join(", ") })}</li>
      );
  }
}

const OP_TONE = { succeeded: "success", failed: "error", running: "primary" } as const;

function OperationResults({ operations }: { operations: readonly OperationView[] }) {
  const t = useModulesT();
  const describe = useDescribe();
  if (operations.length === 0) return <p className="text-sm">{t("modules.result.noop")}</p>;
  return (
    <ul className="flex flex-col gap-3">
      {operations.map((o) => (
        <li key={o.id} className="flex flex-col gap-1">
          <span className="flex items-center gap-2 text-sm font-medium">
            {(MODULE_OPS as readonly string[]).includes(o.operation)
              ? t(opKey(o.operation as ModuleOp))
              : o.operation}
            <BadgeStatus tone={OP_TONE[o.status]}>{t(`modules.result.${o.status}`)}</BadgeStatus>
          </span>
          {o.failure ? (
            <p className="text-sm text-muted-foreground">
              {describe(mapFailureCode(o.failure.code))}
            </p>
          ) : null}
          {o.steps.length > 0 ? (
            <ul className="text-xs text-muted-foreground">
              {o.steps.map((s) => (
                <li key={s.name}>
                  {t("modules.history.step", {
                    name: s.name,
                    status: s.status,
                    durationMs: s.durationMs,
                  })}
                </li>
              ))}
            </ul>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

/**
 * Tampilan pemantauan operasi 202. Polling-nya sendiri hidup di halaman (bukan di dialog) agar menutup dialog tidak
 * menghentikan invalidasi bootstrap saat operasi selesai (R13.5); dialog hanya membaca statusnya.
 */
function PollingResult({ ids, polling }: { ids: readonly string[]; polling: OperationPolling }) {
  const t = useModulesT();
  const mine = polling.operations.filter((o) => ids.includes(o.id));
  const done = mine.length > 0 && mine.every((o) => o.status !== "running");
  if (done) return <OperationResults operations={mine} />;
  return (
    <div className="flex flex-col gap-2 text-sm">
      {polling.phase === "running" || polling.phase === "idle" ? (
        <p className="flex items-center gap-2">
          <LoadingIcon aria-label={t("modules.result.running")} />
          {t("modules.result.running")}
        </p>
      ) : null}
      {polling.pollError ? (
        <p className="text-muted-foreground">{t("modules.result.pollError")}</p>
      ) : null}
      {polling.phase === "timeout" ? <p>{t("modules.result.timeout")}</p> : null}
      {polling.phase === "stopped" ? <p>{t("modules.result.pollStopped")}</p> : null}
      {mine.length > 0 ? <OperationResults operations={mine} /> : null}
    </div>
  );
}

export interface ActionDialogProps {
  op: ModuleOp;
  target: ModuleView;
  modules: readonly ModuleView[];
  frontend: readonly FrontendModule[];
  /** ETag daftar yang dipakai menghitung dampak (dikirim sebagai `If-Match`). */
  etag: string | undefined;
  onClose(): void;
  /** Mengembalikan fokus saat dialog ditutup (ke tombol menu aksi baris, atau cadangan bila sudah hilang; R14.1). */
  restoreFocus?(): void;
  /** Status pemantauan operasi 202 yang dimiliki halaman. */
  polling: OperationPolling;
  /** Operasi diterima (202): halaman mulai/meneruskan pemantauan. */
  onAccepted(ids: readonly string[]): void;
}

/**
 * Dialog konfirmasi aksi modul (`AlertDialog`): dampak, blocker (tombol konfirmasi nonaktif), opsi install, lalu hasil
 * (`200`) atau pemantauan (`202`). Esc dan klik di luar tidak menutup; hanya Batal/Tutup.
 */
export function ActionDialog({
  op,
  target,
  modules,
  frontend,
  etag,
  onClose,
  restoreFocus,
  polling,
  onAccepted,
}: ActionDialogProps) {
  const t = useModulesT();
  const describe = useDescribe();
  const client = useModulesClient();
  const queryClient = useQueryClient();
  const tenantId = useTenantId();
  const depsId = useId();
  const enableId = useId();

  const initialOptions = useMemo(
    () => computeImpact({ op, target, modules, frontend }).installOptions,
    // Nilai awal opsi install ditetapkan sekali saat dialog dibuka.
    [],
  );
  const [options, setOptions] = useState<InstallOptions | undefined>(initialOptions);
  const [phase, setPhase] = useState<Phase>({ kind: "confirm" });
  const started = phase.kind !== "confirm";
  const contentRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  // Tombol yang fokus (Lanjutkan, Batal) hilang saat fase berganti sehingga fokus jatuh ke body; pindahkan ke dialog
  // selama berjalan dan ke tombol Tutup saat hasil/galat tampil (R14.1).
  useEffect(() => {
    if (phase.kind === "running") contentRef.current?.focus();
    else if (phase.kind !== "confirm") closeRef.current?.focus();
  }, [phase.kind]);

  const impact = useMemo(
    () => computeImpact({ op, target, modules, frontend, options }),
    [op, target, modules, frontend, options],
  );

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: BOOTSTRAP_KEY });
    void queryClient.invalidateQueries({ queryKey: scopedKey(tenantId, "platform", "modules") });
  };

  const confirm = async () => {
    setPhase({ kind: "running" });
    try {
      const result = await runModuleOperation(client, {
        key: target.key,
        op,
        withDeps: options?.withDeps,
        enable: options?.enable,
        ifMatch: etag,
      });
      refresh();
      if (result.kind === "accepted") onAccepted(result.operationIds);
      setPhase(
        result.kind === "done"
          ? { kind: "done", operations: result.operations }
          : { kind: "polling", ids: result.operationIds },
      );
    } catch (error) {
      const mapped =
        error instanceof ModulesApiError
          ? mapModuleError(error.status, error.apiError, "mutation")
          : mapModuleError(500, { code: "internal_error", message: "" }, "mutation");
      if (mapped.reloadList)
        void queryClient.invalidateQueries({
          queryKey: scopedKey(tenantId, "platform", "modules"),
        });
      if (mapped.refetchBootstrap) void queryClient.invalidateQueries({ queryKey: BOOTSTRAP_KEY });
      setPhase({ kind: "error", mapped });
    }
  };

  const blocked = impact.blockers.length > 0;
  const title = t("modules.dialog.title", { operation: t(opKey(op)), name: target.name });

  return (
    <AlertDialog open>
      <AlertDialogContent
        ref={contentRef}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          restoreFocus?.();
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>
            {phase.kind === "confirm" || phase.kind === "running"
              ? t("modules.impact.effects")
              : t("modules.result.title")}
          </AlertDialogDescription>
        </AlertDialogHeader>

        {phase.kind === "confirm" || phase.kind === "running" ? (
          <div className="flex flex-col gap-3 text-sm">
            {impact.blockers.length > 0 ? (
              <Alert variant="destructive">
                <AlertDescription>
                  <p className="font-medium text-foreground">{t("modules.impact.blockers")}</p>
                  <ul className="list-disc pl-4">
                    {impact.blockers.map((b) => (
                      <li key={`${b.code}:${b.modules.join(",")}`}>
                        {t(`modules.blocker.${b.code}`, { modules: b.modules.join(", ") })}
                      </li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            ) : null}
            {impact.effects.length > 0 ? (
              <ul className="list-disc pl-4">
                {impact.effects.map((e, i) => (
                  <EffectLine key={`${e.kind}:${i}`} effect={e} target={target} />
                ))}
              </ul>
            ) : (
              <p className="text-muted-foreground">{t("modules.dialog.noImpact")}</p>
            )}
            {options ? (
              <div className="flex flex-col gap-2">
                <div className="flex items-center gap-2">
                  <Checkbox
                    id={depsId}
                    checked={options.withDeps}
                    onCheckedChange={(v) => setOptions({ ...options, withDeps: v === true })}
                  />
                  <Label htmlFor={depsId}>{t("modules.install.withDeps")}</Label>
                </div>
                <div className="flex items-center gap-2">
                  <Checkbox
                    id={enableId}
                    checked={options.enable}
                    onCheckedChange={(v) => setOptions({ ...options, enable: v === true })}
                  />
                  <Label htmlFor={enableId}>{t("modules.install.enable")}</Label>
                </div>
              </div>
            ) : null}
          </div>
        ) : null}

        {phase.kind === "done" ? <OperationResults operations={phase.operations} /> : null}
        {phase.kind === "polling" ? <PollingResult ids={phase.ids} polling={polling} /> : null}
        {phase.kind === "error" ? (
          <Alert variant="destructive">
            <AlertDescription>
              {phase.mapped.messages.map((m: MessageDescriptor) => (
                <p key={m.key}>{describe(m)}</p>
              ))}
            </AlertDescription>
          </Alert>
        ) : null}

        <AlertDialogFooter>
          {started && phase.kind !== "running" ? (
            <Button ref={closeRef} onClick={onClose}>
              {t("modules.dialog.close")}
            </Button>
          ) : (
            <>
              <AlertDialogCancel disabled={phase.kind === "running"} onClick={onClose}>
                {t("modules.dialog.cancel")}
              </AlertDialogCancel>
              <Button
                variant={op === "disable" || op === "uninstall" ? "destructive" : "primary"}
                disabled={blocked}
                loading={phase.kind === "running"}
                onClick={() => void confirm()}
              >
                {t("modules.dialog.confirm")}
              </Button>
            </>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
