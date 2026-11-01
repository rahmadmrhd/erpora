import { useEffect, useMemo } from "react";
import { deskContains } from "./match-menu";
import { useDeskContext } from "./provider";
import { resolveDesk } from "./resolver";
import { readRememberedDesk, readTabDesk } from "./storage";

/**
 * Validasi Desk pada setiap perubahan lokasi (M3a R9.3), tanpa memuat ulang halaman: bila Desk saat ini tidak memuat
 * `pathname`, jalankan `resolveDesk` dengan `page` dan beralih (`setCurrentDesk`, tanpa navigasi). `/` bukan halaman
 * (pintu masuk) sehingga tidak divalidasi.
 *
 * Mengembalikan `deskId` Desk yang berlaku untuk halaman ini (dihitung sinkron, tidak menunggu efek), atau `null`
 * bila halaman tidak masuk Desk mana pun (R9.6: breadcrumb `Home › halaman` tanpa segmen Desk; Desk saat ini tetap).
 * Pasang sekali di bawah router dengan `useLocation().pathname`.
 */
export function useDeskValidation(pathname: string): { deskId: string | null } {
  const { status, desks, currentDeskId, setCurrentDesk, defaultDesk } = useDeskContext();
  const page = pathname === "/" ? null : pathname;

  const target = useMemo<string | null>(() => {
    if (status !== "ready" || page === null) return null;
    const current = desks.find((d) => d.id === currentDeskId);
    if (current && deskContains(current, page)) return current.id;
    return resolveDesk({
      desks,
      tabDesk: readTabDesk(),
      rememberedDesk: readRememberedDesk(),
      defaultDesk,
      page,
    }).deskId;
  }, [status, desks, currentDeskId, defaultDesk, page]);

  useEffect(() => {
    if (target !== null && target !== currentDeskId) setCurrentDesk(target);
  }, [target, currentDeskId, setCurrentDesk]);

  return { deskId: target };
}
