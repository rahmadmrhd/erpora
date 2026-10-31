import { useSyncExternalStore } from "react";

/** Ambang sama dengan `useIsMobile` shadcn (768 px): di bawahnya layar dianggap sempit (ponsel). */
const NARROW_QUERY = "(max-width: 767px)";

function query(): MediaQueryList | null {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return null;
  return window.matchMedia(NARROW_QUERY);
}

function subscribe(onChange: () => void): () => void {
  const mql = query();
  if (mql === null) return () => {};
  mql.addEventListener("change", onChange);
  return () => mql.removeEventListener("change", onChange);
}

const read = (): boolean => query()?.matches ?? false;

/**
 * `true` bila lebar jendela < 768 px. Berbeda dari `useIsMobile` shadcn (nilai awal `undefined` dibetulkan di efek
 * setelah dilukis, sehingga frame pertama salah), nilai awal dibaca sinkron dari `matchMedia` dan perubahan lebar
 * diikuti lewat `useSyncExternalStore`. Tanpa `window`/`matchMedia` dianggap lebar.
 */
export function useIsNarrow(): boolean {
  return useSyncExternalStore(subscribe, read, () => false);
}
