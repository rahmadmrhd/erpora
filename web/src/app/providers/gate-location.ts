import { useCallback, useSyncExternalStore } from "react";

const EVENT = "erpora:gate-location";

function subscribe(onChange: () => void): () => void {
  window.addEventListener("popstate", onChange);
  window.addEventListener(EVENT, onChange);
  return () => {
    window.removeEventListener("popstate", onChange);
    window.removeEventListener(EVENT, onChange);
  };
}

const snapshot = (): string => window.location.pathname + window.location.search;

export interface GateLocation {
  path: string;
  search: string;
  /** Pindah ke `to` dengan entri riwayat baru. */
  navigate(to: string, replace?: boolean): void;
  /** Mengganti URL saat ini (tanpa entri riwayat baru). */
  replace(to: string): void;
}

/**
 * Lokasi untuk gerbang pra-bootstrap, yang dirender sebelum router aplikasi ada (PRD-001 Q33 butir 2): membaca
 * `window.location` dan mengikuti `popstate`. Setelah gerbang terbuka, router (TanStack) membaca lokasi yang sama.
 */
export function useGateLocation(): GateLocation {
  const current = useSyncExternalStore(subscribe, snapshot, () => "/");
  const q = current.indexOf("?");
  const navigate = useCallback((to: string, replace = false) => {
    window.history[replace ? "replaceState" : "pushState"](null, "", to);
    window.dispatchEvent(new Event(EVENT));
  }, []);
  const replace = useCallback((to: string) => navigate(to, true), [navigate]);
  return {
    path: q < 0 ? current : current.slice(0, q),
    search: q < 0 ? "" : current.slice(q),
    navigate,
    replace,
  };
}
