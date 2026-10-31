import { useDesks, useTranslation } from "@erpora/platform-web";
import { Skeleton } from "@erpora/ui";
import type { ReactNode } from "react";
import { EmptyNotice } from "../components/notices";

/**
 * Kerangka slot widget Dashboard (R14.2): wilayah berlabel; tanpa anak (belum ada widget, sampai M3f) menampilkan
 * keadaan kosong. Slot widget kelak mengisi `children`.
 */
export function DashboardSlots({ children }: { children?: ReactNode }) {
  const { t } = useTranslation("platform");
  return (
    <section aria-label={t("dashboard.slots")} className="flex flex-1 flex-col">
      {children ?? (
        <EmptyNotice
          className="min-h-[50vh] justify-center"
          icon="layout-dashboard"
          title={t("dashboard.title")}
          description={t("dashboard.description")}
        />
      )}
    </section>
  );
}

function DashboardSkeleton() {
  return (
    <div data-testid="dashboard-loading" className="flex flex-col gap-3 p-4" aria-hidden="true">
      <Skeleton className="h-7 w-56" />
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-4 w-2/3" />
    </div>
  );
}

/**
 * Dashboard bersama di indeks `/<slug>` dan `/platform` (R14.1). `deskId` = id Desk (slug modul atau `platform`).
 * Desk yang tidak ada di daftar (tidak diizinkan, modul nonaktif, atau tak dikenal) menampilkan pesan netral yang
 * tidak membedakan penyebabnya (design Keamanan: informasi tidak bocor).
 */
export function DashboardPage({ deskId }: { deskId: string }) {
  const { t } = useTranslation("platform");
  const { status, desks } = useDesks();
  if (status === "loading") return <DashboardSkeleton />;
  const desk = desks.find((d) => d.id === deskId);
  if (!desk) {
    return (
      <EmptyNotice
        className="min-h-[60vh] justify-center"
        icon="layout-dashboard"
        title={t("dashboard.unavailable.title")}
        description={t("dashboard.unavailable.description")}
      />
    );
  }
  return (
    <div className="flex min-h-full flex-col gap-4 p-4">
      <DashboardSlots />
    </div>
  );
}
