import {
  BOOTSTRAP_KEY,
  scopedKey,
  useFeature,
  useMessage,
  useModuleRegistry,
  usePageActions,
  usePermission,
  useTenantId,
  useTranslation,
} from "@erpora/platform-web";
import {
  Alert,
  AlertDescription,
  AlertTitle,
  Button,
  EmptyState,
  ErrorPage,
  Skeleton,
} from "@erpora/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { ActionDialog } from "./action-dialog";
import { ModulesApiError, useModules, useOperationPolling } from "./api";
import { mapModuleError } from "./errors";
import { HistoryDialog } from "./history-dialog";
import { useDescribe, useModulesT } from "./keys";
import { ModulesTable } from "./modules-table";
import type { ModuleOp, ModuleView } from "./types";

/**
 * Halaman `/settings/modules` (lazy; R13): tabel modul dengan status, dampak aksi, eksekusi (200/202 + polling), dan
 * riwayat. Mode hanya-baca bila `features.modulesAdminApi` tidak aktif (atau mutasi membalas 404).
 */
export function ModulesPage() {
  const t = useModulesT();
  const describe = useDescribe();
  const { t: tui } = useTranslation("ui");
  const adminFlag = useFeature("modulesAdminApi");
  // Aksi mutasi tampil bila API mutasi diumumkan aktif DAN pengguna berhak (UX saja; backend tetap otoritas, K-30:
  // permissions bootstrap kosong = optimistis).
  const canManage = usePermission("platform.modules.manage");
  const showActions = adminFlag && canManage;

  const query = useModules();
  const registry = useModuleRegistry();
  const frontend = registry.resolved.map((r) => r.module);

  const [action, setAction] = useState<{ op: ModuleOp; key: string } | null>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const [historyKey, setHistoryKey] = useState<string | null>(null);

  // Pemantauan operasi 202 hidup di halaman: menutup dialog tidak boleh menghentikan invalidasi saat selesai (R13.5),
  // dan polling berhenti otomatis saat halaman unmount.
  const queryClient = useQueryClient();
  const tenantId = useTenantId();
  const [pollIds, setPollIds] = useState<readonly string[]>([]);
  const polling = useOperationPolling(pollIds.length > 0 ? pollIds : null, () => {
    void queryClient.invalidateQueries({ queryKey: BOOTSTRAP_KEY });
    void queryClient.invalidateQueries({ queryKey: scopedKey(tenantId, "platform", "modules") });
  });
  const pollingRef = useRef(polling);
  pollingRef.current = polling;
  const onAccepted = (ids: readonly string[]) => {
    // Operasi lama yang masih berjalan tetap dipantau agar invalidasinya tidak hilang. Dibuang: yang sudah selesai, yang
    // sudah mati (404/403: tidak ada di hasil), dan semuanya bila pemantauan berhenti (stopped/timeout).
    const { phase, operations } = pollingRef.current;
    const running = new Set(operations.filter((o) => o.status === "running").map((o) => o.id));
    const keep = (id: string) =>
      phase === "running" && (operations.length === 0 || running.has(id));
    setPollIds((prev) => [...new Set([...prev.filter(keep), ...ids])]);
  };

  const refreshLabel = useMessage({ ns: "platform", key: "modules.refresh" });
  usePageActions([
    {
      id: "refresh",
      label: refreshLabel,
      icon: "refresh-cw",
      variant: "outline",
      loading: query.isFetching,
      onSelect: () => {
        void query.refetch();
      },
    },
  ]);

  const modules = query.data?.modules ?? [];
  const find = (key: string): ModuleView | undefined => modules.find((m) => m.key === key);
  const actionTarget = action ? find(action.key) : undefined;
  const historyTarget = historyKey ? find(historyKey) : undefined;

  let body: React.ReactNode;
  if (query.isPending) {
    body = (
      <div className="flex flex-col gap-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  } else if (query.isError) {
    const err = query.error;
    if (err instanceof ModulesApiError && err.status === 403) {
      body = (
        <ErrorPage
          status={403}
          title={t("modules.forbidden.title")}
          description={t("modules.forbidden.description")}
        />
      );
    } else {
      const mapped =
        err instanceof ModulesApiError
          ? mapModuleError(err.status, err.apiError, "read")
          : mapModuleError(500, { code: "internal_error", message: "" }, "read");
      body = (
        <Alert variant="destructive">
          <AlertTitle>{t("modules.loadError")}</AlertTitle>
          <AlertDescription>
            {mapped.messages.map((m) => (
              <p key={m.key}>{describe(m)}</p>
            ))}
            <Button
              variant="outline"
              size="sm"
              loading={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {tui("actions.retry")}
            </Button>
          </AlertDescription>
        </Alert>
      );
    }
  } else if (modules.length === 0) {
    body = <EmptyState icon="blocks" title={t("modules.empty")} />;
  } else {
    body = (
      <ModulesTable
        modules={modules}
        showActions={showActions}
        onOperation={(op, m, trigger) => {
          returnFocus.current = trigger;
          setAction({ op, key: m.key });
        }}
        onHistory={(m) => setHistoryKey(m.key)}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4 p-6">
      <header className="flex flex-col gap-1">
        <h1 ref={titleRef} tabIndex={-1} className="text-xl font-semibold">
          {t("modules.title")}
        </h1>
        <p className="text-sm text-muted-foreground">{t("modules.description")}</p>
      </header>

      {!showActions && !query.isError ? (
        <Alert>
          <AlertTitle>{t("modules.readonly.title")}</AlertTitle>
          <AlertDescription>
            {adminFlag ? t("modules.readonly.noPermission") : t("modules.readonly.description")}
          </AlertDescription>
        </Alert>
      ) : null}

      {body}

      {action && actionTarget ? (
        <ActionDialog
          key={`${action.op}:${action.key}`}
          op={action.op}
          target={actionTarget}
          modules={modules}
          frontend={frontend}
          etag={query.data?.etag}
          onClose={() => setAction(null)}
          restoreFocus={() => {
            // Pemicu bisa sudah dilepas (mis. bootstrap menyembunyikan kolom Aksi): fokus jatuh ke judul halaman.
            const trigger = returnFocus.current;
            if (trigger?.isConnected) trigger.focus();
            else titleRef.current?.focus();
          }}
          polling={polling}
          onAccepted={onAccepted}
        />
      ) : null}
      {historyTarget ? (
        <HistoryDialog module={historyTarget} onClose={() => setHistoryKey(null)} />
      ) : null}
    </div>
  );
}
