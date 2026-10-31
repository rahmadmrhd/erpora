import { useNavigate } from "@erpora/module-sdk";
import {
  deskDashboardPath,
  isShowDesksState,
  useCurrentDesk,
  useDesks,
  useTranslation,
} from "@erpora/platform-web";
import { Button, Skeleton } from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { useEffect } from "react";
import { ErrorNotice } from "../components/notices";
import { DesksPage } from "./desks-page";

function EntrySkeleton() {
  return (
    <div data-testid="desk-entry-loading" className="flex flex-col gap-3 p-4" aria-hidden="true">
      <Skeleton className="h-7 w-40" />
      <Skeleton className="h-16 w-full" />
    </div>
  );
}

/**
 * Pintu masuk `/` (R10.3): tanpa konten sendiri. Menjalankan penentuan Desk tahap 1-3 (lewat `DeskProvider`) dan
 * mengarahkan ke Dashboard Desk itu dengan `replace`; tanpa hasil, atau bila state navigasi `showDesks` ada (ikon Home
 * di breadcrumb, R10.5), menampilkan halaman Desks. Muat ulang menghapus state itu sehingga penentuan biasa berlaku.
 */
export function DeskEntry() {
  const { t } = useTranslation("platform");
  const { status, refetch } = useDesks();
  const { desk } = useCurrentDesk();
  const navigate = useNavigate();
  const showDesks = useRouterState({ select: (s) => isShowDesksState(s.location.state) });
  const target = desk ? deskDashboardPath(desk) : null;
  const redirect = status === "ready" && !showDesks && target !== null;

  useEffect(() => {
    if (redirect) void navigate({ to: target, replace: true });
  }, [redirect, target, navigate]);

  if (status === "loading") return <EntrySkeleton />;
  if (status === "error") {
    return (
      <ErrorNotice
        status={500}
        title={t("desks.loadError.title")}
        description={t("desks.loadError.description")}
        actions={
          <Button type="button" variant="outline" onClick={() => refetch()}>
            {t("shell.retry")}
          </Button>
        }
      />
    );
  }
  if (redirect) return <EntrySkeleton />;
  return <DesksPage />;
}
