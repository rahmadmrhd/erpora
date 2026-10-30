import { Link } from "@erpora/module-sdk";
import type { ApiError } from "@erpora/platform-contracts";
import {
  type ApiErrorKey,
  describeApiError,
  type ModuleStatus,
  useTranslation,
} from "@erpora/platform-web";
import { Button, EmptyState, ErrorPage } from "@erpora/ui";
import type { ErrorComponentProps } from "@tanstack/react-router";

/** Beranda: dashboard kosong (menunggu M3f). */
export function DashboardPage() {
  const { t } = useTranslation("platform");
  return (
    <EmptyState
      className="min-h-[60vh] justify-center"
      icon="layout-dashboard"
      title={t("dashboard.title")}
      description={t("dashboard.description")}
    />
  );
}

export function NotFoundPage() {
  const { t } = useTranslation("platform");
  return (
    <ErrorPage
      status={404}
      title={t("notFound.title")}
      description={t("notFound.description")}
      actions={
        <Button asChild variant="outline">
          <Link to="/">{t("notFound.backHome")}</Link>
        </Button>
      }
    />
  );
}

/** Dilempar guard route (`beforeLoad`) saat modul bukan `enabled`; chunk halaman modul tidak dimuat. */
export class ModuleInactiveError extends Error {
  override readonly name = "ModuleInactiveError";
  readonly moduleKey: string;
  readonly status: ModuleStatus | undefined;
  constructor(moduleKey: string, status: ModuleStatus | undefined) {
    super(`modul ${moduleKey} tidak aktif (${status ?? "tidak ada"})`);
    this.moduleKey = moduleKey;
    this.status = status;
  }
}

type InactiveKey =
  | "moduleInactive.notEnabled"
  | "moduleInactive.notInstalled"
  | "moduleInactive.unavailable";

/** Teks menurut status: belum diaktifkan, belum dipasang, atau sementara tidak tersedia. */
export function inactiveMessageKey(status: ModuleStatus | undefined): InactiveKey {
  switch (status) {
    case "installed":
    case "disabled":
      return "moduleInactive.notEnabled";
    case "upgrade_pending":
    case "failed":
    case "incompatible":
    case "missing":
      return "moduleInactive.unavailable";
    default:
      return "moduleInactive.notInstalled";
  }
}

/** Halaman "Modul tidak aktif" (UX saja; backend tetap menolak, FR-19). */
export function ModuleInactivePage({ status }: { status: ModuleStatus | undefined }) {
  const { t } = useTranslation("platform");
  return (
    <ErrorPage
      status={503}
      title={t("moduleInactive.title")}
      description={t(inactiveMessageKey(status))}
      actions={
        <Button asChild variant="outline">
          <Link to="/settings/modules">{t("moduleInactive.openSettings")}</Link>
        </Button>
      }
    />
  );
}

function apiErrorOf(error: unknown): ApiError | undefined {
  if (typeof error !== "object" || error === null) return undefined;
  const e = (error as { error?: unknown }).error;
  return typeof e === "object" && e !== null && typeof (e as ApiError).code === "string"
    ? (e as ApiError)
    : undefined;
}

/** Galat render di dalam area konten (sidebar dan navbar tetap). Galat API menampilkan `requestId`/`traceId`. */
export function RouteErrorPage({ error, reset }: Pick<ErrorComponentProps, "error" | "reset">) {
  const { t } = useTranslation("platform");
  const api = apiErrorOf(error);
  const described = api ? describeApiError(api) : undefined;
  const description = described
    ? (t as unknown as (k: ApiErrorKey) => string)(described.key)
    : t("routeError.description");
  return (
    <ErrorPage
      status={500}
      title={t("routeError.title")}
      description={description}
      actions={
        <div className="flex flex-col items-center gap-2">
          {api?.requestId ? (
            <p className="text-xs text-muted-foreground">
              {t("routeError.requestId", { requestId: api.requestId })}
            </p>
          ) : null}
          {api?.traceId ? (
            <p className="text-xs text-muted-foreground">
              {t("routeError.traceId", { traceId: api.traceId })}
            </p>
          ) : null}
          <Button type="button" variant="outline" onClick={reset}>
            {t("shell.retry")}
          </Button>
        </div>
      }
    />
  );
}
