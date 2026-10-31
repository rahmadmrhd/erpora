import { Link } from "@erpora/module-sdk";
import {
  type Desk,
  deskDashboardPath,
  Message,
  useCurrentDesk,
  useDesks,
  useTranslation,
} from "@erpora/platform-web";
import { Card, CardHeader, CardTitle, resolveIcon, Skeleton } from "@erpora/ui";
import { EmptyNotice } from "../components/notices";

function DeskCard({ desk, onChoose }: { desk: Desk; onChoose: (id: string) => void }) {
  const Icon = resolveIcon(desk.icon);
  const to = deskDashboardPath(desk);
  if (to === null) return null;
  return (
    <Card className="transition-colors hover:bg-accent">
      <CardHeader>
        <CardTitle className="text-base">
          {/* Tautan sungguhan (klik-tengah dan Ctrl+klik tetap bekerja); memilih Desk mencatatnya sebagai Desk saat ini. */}
          <Link to={to} className="flex items-center gap-2" onClick={() => onChoose(desk.id)}>
            {Icon ? <Icon aria-hidden="true" className="size-5 shrink-0" /> : null}
            <span>
              <Message message={desk.label} />
            </span>
          </Link>
        </CardTitle>
      </CardHeader>
    </Card>
  );
}

/**
 * Halaman Desks (R10.6): kerangka daftar kartu Desk yang diizinkan, atau keadaan kosong. Tidak punya URL sendiri:
 * dirender oleh `DeskEntry` di `/`. Kustomisasi, folder/grup, dan pengelolaan adalah M3b.
 */
export function DesksPage() {
  const { t } = useTranslation("platform");
  const { status, desks } = useDesks();
  const { setCurrentDesk } = useCurrentDesk();
  const usable = desks.filter((d) => deskDashboardPath(d) !== null);
  return (
    <div className="flex min-h-full flex-col gap-4 p-4">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">{t("desks.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("desks.description")}</p>
      </header>
      {status === "loading" ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3" aria-hidden="true">
          <Skeleton className="h-16" />
          <Skeleton className="h-16" />
        </div>
      ) : usable.length === 0 ? (
        <EmptyNotice
          className="min-h-[40vh] justify-center"
          icon="layout-dashboard"
          title={t("desks.empty.title")}
          description={t("desks.empty.description")}
        />
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {usable.map((d) => (
            <li key={d.id}>
              <DeskCard desk={d} onChoose={setCurrentDesk} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
