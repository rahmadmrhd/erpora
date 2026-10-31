import { useNavigate } from "@erpora/module-sdk";
import { type Desk, SHOW_DESKS_STATE, useCurrentDesk } from "@erpora/platform-web";
import { useCallback } from "react";

/**
 * Path Dashboard sebuah Desk: item terkunci (Dashboard `/<slug>`) bila ada, selain itu item pertama, selain itu
 * `/<id>` (id Desk bawaan = slug modul, D-3).
 */
export function deskDashboardPath(desk: Desk): string {
  const leaves = desk.menus.flatMap((m) => (m.type === "item" ? [m] : m.children));
  return (leaves.find((i) => i.locked === true) ?? leaves[0])?.to ?? `/${desk.id}`;
}

/** Berpindah Desk (menetapkan Desk saat ini lalu membuka Dashboard-nya) dan membuka halaman Desks tanpa resolve. */
export function useDeskNavigation(): {
  openDesk(desk: Desk): void;
  goToDesks(): void;
} {
  const navigate = useNavigate();
  const { setCurrentDesk } = useCurrentDesk();
  const openDesk = useCallback(
    (desk: Desk) => {
      setCurrentDesk(desk.id);
      void navigate({ to: deskDashboardPath(desk) });
    },
    [navigate, setCurrentDesk],
  );
  const goToDesks = useCallback(() => {
    void navigate({ to: "/", state: (prev) => ({ ...prev, ...SHOW_DESKS_STATE }) });
  }, [navigate]);
  return { openDesk, goToDesks };
}
