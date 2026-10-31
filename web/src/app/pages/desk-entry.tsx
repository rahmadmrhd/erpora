import { useNavigate } from "@erpora/module-sdk";
import {
  deskDashboardPath,
  isShowDesksState,
  useCurrentDesk,
  useDesks,
  useTranslation,
} from "@erpora/platform-web";
import { Button, Skeleton } from "@erpora/ui";
import { useRouter, useRouterState } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
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
 * Mengonsumsi penanda `showDesks` (R10.5): tampilan Desks dipertahankan lewat state React selama komponen terpasang,
 * sedangkan penanda dihapus dari entri riwayat (hanya penanda; state lain tetap) dengan `history.replace`, sehingga muat
 * ulang (peramban mempertahankan `history.state`) dan Back/Forward ke entri itu mengikuti penentuan Desk biasa.
 * Navigasi berikutnya ke `/`: dengan penanda baru (Home) tetap Desks; ke alamat lain di `/` (mis. query berbeda) tanpa
 * penanda kembali ke penentuan biasa.
 */
function useConsumedShowDesks(): boolean {
  const router = useRouter();
  const marker = useRouterState({ select: (s) => isShowDesksState(s.location.state) });
  const href = useRouterState({ select: (s) => s.location.href });
  const [held, setHeld] = useState(marker);
  const selfReplace = useRef(false);
  useEffect(() => {
    if (marker) {
      setHeld(true);
      selfReplace.current = true;
      const { href, state } = router.state.location;
      const { showDesks: _consumed, ...rest } = state as unknown as Record<string, unknown>;
      router.history.replace(href, rest);
    } else if (selfReplace.current) {
      selfReplace.current = false; // perubahan state akibat penghapusan penanda oleh komponen ini
    } else {
      setHeld(false);
    }
  }, [marker, href, router]);
  return held;
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
  const showDesks = useConsumedShowDesks();
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
