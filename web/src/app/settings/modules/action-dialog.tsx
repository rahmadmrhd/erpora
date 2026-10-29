import type { FrontendModule, MessageRef } from "@erpora/module-sdk";
import { BOOTSTRAP_KEY, Message, useTenantId } from "@erpora/platform-web";
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
import { Fragment, type ReactNode, useId, useMemo, useState } from "react";
import { ModulesApiError, modulesKey, runModuleOperation, useOperationPolling } from "./api";
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

const SENTINEL = "";

/** Menyisipkan simpul React di tempat argumen `{labels}`: pesan diterjemahkan dengan penanda lalu dipecah. */
function withNode(text: string, node: ReactNode): ReactNode {
  const [before, after] = text.split(SENTINEL);
  return (
    <>
      {before}
      {node}
      {after}
    </>
  );
}

/** Daftar label menu (MessageRef lintas namespace) sebagai teks bersambung koma. */
function LabelList({ refs }: { refs: readonly MessageRef[] }) {
  return (
    <>
      {refs.map((ref, i) => (
        <Fragment key={`${ref.ns}:${ref.key}`}>
          {i > 0 ? ", " : null}
          <Message message={ref} />
        </Fragment>
      ))}
    </>
  );
}

function EffectLine({ effect }: { effect: Effect }) {
  const t = useModulesT();
  switch (effect.kind) {
    case "menu_hidden":
    case "menu_shown":
      return (
        <li>
          {withNode(
            t(`modules.impact.${effect.kind}`, { labels: SENTINEL }),
            <LabelList refs={effect.labels} />,
          )}
        </li>
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

function PollingResult({ ids, onFinished }: { ids: readonly string[]; onFinished: () => void }) {
  const t = useModulesT();
  const polling = useOperationPolling(ids, onFinished);
  if (polling.phase === "finished") return <OperationResults operations={polling.operations} />;
  return (
    <div className="flex flex-col gap-2 text-sm">
      <p className="flex items-center gap-2">
        <LoadingIcon aria-label={t("modules.result.running")} />
        {t("modules.result.running")}
      </p>
      {polling.pollError ? (
        <p className="text-muted-foreground">{t("modules.result.pollError")}</p>
      ) : null}
      {polling.phase === "timeout" ? <p>{t("modules.result.timeout")}</p> : null}
      {polling.operations.length > 0 ? <OperationResults operations={polling.operations} /> : null}
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
  /** API mutasi tidak aktif (404): halaman beralih ke mode hanya-baca. */
  onReadOnly(): void;
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
  onReadOnly,
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

  const impact = useMemo(
    () => computeImpact({ op, target, modules, frontend, options }),
    [op, target, modules, frontend, options],
  );

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: BOOTSTRAP_KEY });
    void queryClient.invalidateQueries({ queryKey: modulesKey(tenantId) });
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
      if (mapped.reloadList) void queryClient.invalidateQueries({ queryKey: modulesKey(tenantId) });
      if (mapped.switchToReadOnly) onReadOnly();
      setPhase({ kind: "error", mapped });
    }
  };

  const blocked = impact.blockers.length > 0;
  const title = t("modules.dialog.title", { operation: t(opKey(op)), name: target.name });

  return (
    <AlertDialog open>
      <AlertDialogContent>
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
                  <EffectLine key={`${e.kind}:${i}`} effect={e} />
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
        {phase.kind === "polling" ? <PollingResult ids={phase.ids} onFinished={refresh} /> : null}
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
            <Button onClick={onClose}>{t("modules.dialog.close")}</Button>
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
