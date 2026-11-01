import { useNavigate } from "@erpora/module-sdk";
import {
  type Desk,
  deskDashboardPath as platformDeskDashboardPath,
  resolveDeskSwitchTarget,
  SHOW_DESKS_STATE,
  useCurrentDesk,
} from "@erpora/platform-web";
import { useRouter } from "@tanstack/react-router";
import { useCallback } from "react";

const trim = (p: string): string => (p.length > 1 && p.endsWith("/") ? p.slice(0, -1) : p);

/**
 * Navigasi antar-Desk dan halaman Desks.
 * - `openDesk`: menetapkan Desk saat ini lalu **selalu** membuka Dashboard-nya (kartu halaman Desks, crumb Desk).
 * - `switchDesk` (K10): pindah Desk dari popover logo ("Pindah Desk") dan palet (ikon Dock Pinned dan pil Floating memakai popover menu, K8 dan K20); bila rute saat ini juga ada di Desk
 *   tujuan (`resolveDeskSwitchTarget`) tetap di rute itu dan hanya Desk saat ini yang berganti.
 * - `goToDesks`: halaman Desks di `/` tanpa penentuan Desk (penanda state `showDesks`).
 */
export function useDeskNavigation(): {
  openDesk(desk: Desk): void;
  switchDesk(desk: Desk): void;
  goToDesks(): void;
} {
  const navigate = useNavigate();
  const router = useRouter();
  const { setCurrentDesk } = useCurrentDesk();
  const openDesk = useCallback(
    (desk: Desk) => {
      setCurrentDesk(desk.id);
      void navigate({ to: platformDeskDashboardPath(desk) ?? `/${desk.id}` });
    },
    [navigate, setCurrentDesk],
  );
  const switchDesk = useCallback(
    (desk: Desk) => {
      setCurrentDesk(desk.id);
      // Lokasi dibaca saat dipanggil (bukan lewat langganan) agar pemakai hook tidak render ulang tiap navigasi.
      const current = router.state.location.pathname;
      const target = resolveDeskSwitchTarget(current, desk);
      if (trim(target) !== trim(current)) void navigate({ to: target });
    },
    [navigate, router, setCurrentDesk],
  );
  const goToDesks = useCallback(() => {
    void navigate({ to: "/", state: (prev) => ({ ...prev, ...SHOW_DESKS_STATE }) });
  }, [navigate]);
  return { openDesk, switchDesk, goToDesks };
}
