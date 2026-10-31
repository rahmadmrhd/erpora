import { Link } from "@erpora/module-sdk";
import {
  type Desk,
  deskDashboardPath,
  Message,
  useCurrentDesk,
  useDesks,
  useTranslation,
} from "@erpora/platform-web";
import { resolveIcon, Skeleton } from "@erpora/ui";
import { EmptyNotice } from "../components/notices";

const GRID =
  "grid w-full grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6";

/**
 * Kartu Desk gaya ERP DS `DeskList`: ubin ikon 64px `rounded-2xl` (token semantik `muted`/`foreground` dengan
 * border; Desk belum punya warna kustom) dan nama 14px medium. Sorotan hover/fokus memakai latar, bukan outline (K2).
 */
function DeskCard({ desk, onChoose }: { desk: Desk; onChoose: (id: string) => void }) {
  const Icon = resolveIcon(desk.icon) ?? resolveIcon("layout-dashboard");
  const to = deskDashboardPath(desk);
  if (to === null) return null;
  return (
    // Tautan sungguhan (klik-tengah dan Ctrl+klik tetap bekerja); memilih Desk mencatatnya sebagai Desk saat ini.
    <Link
      to={to}
      onClick={() => onChoose(desk.id)}
      className="flex flex-col items-center gap-2 rounded-2xl p-3 text-center outline-none transition-colors hover:bg-accent focus-visible:bg-accent"
    >
      <span className="flex size-16 items-center justify-center rounded-2xl border bg-muted text-foreground">
        {Icon ? <Icon aria-hidden="true" className="size-8" /> : null}
      </span>
      <span className="text-sm font-medium break-words">
        <Message message={desk.label} />
      </span>
    </Link>
  );
}

/**
 * Halaman Desks (R10.6, K1): kisi kartu Desk yang diizinkan di tengah halaman, tanpa judul visual (judul hanya
 * untuk pembaca layar), atau keadaan kosong. Tidak punya URL sendiri: dirender oleh `DeskEntry` di `/`.
 * Kustomisasi, folder/grup, dan pengelolaan adalah M3b.
 */
export function DesksPage() {
  const { t } = useTranslation("platform");
  const { status, desks } = useDesks();
  const { setCurrentDesk } = useCurrentDesk();
  const usable = desks.filter((d) => deskDashboardPath(d) !== null);
  return (
    <div className="mx-auto flex min-h-full w-full max-w-5xl flex-col items-center justify-center gap-4 p-6">
      <h1 className="sr-only">{t("desks.title")}</h1>
      {status === "loading" ? (
        <div className={GRID} aria-hidden="true">
          {["a", "b", "c", "d"].map((k) => (
            <Skeleton key={k} className="mx-auto size-16 rounded-2xl" />
          ))}
        </div>
      ) : usable.length === 0 ? (
        <EmptyNotice
          className="min-h-[40vh] justify-center"
          icon="layout-dashboard"
          title={t("desks.empty.title")}
          description={t("desks.empty.description")}
        />
      ) : (
        <ul className={GRID}>
          {usable.map((d) => (
            <li key={d.id} className="flex justify-center">
              <DeskCard desk={d} onChoose={setCurrentDesk} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
